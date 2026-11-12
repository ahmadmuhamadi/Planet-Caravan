// Package store keeps jobs, views and releases in Postgres.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5" // registers pgx5://
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	computeclient "github.com/ahmadmuhamadi/Planet-Caravan/gateway/internal/compute_client"
	"github.com/ahmadmuhamadi/Planet-Caravan/gateway/migrations"
)

// ErrNotFound means no row matched.
var ErrNotFound = errors.New("not found")

// Job statuses.
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Job is a view job.
type Job struct {
	ID        string
	CacheKey  string
	Status    string
	Progress  int
	Params    computeclient.ViewParams
	ViewID    string    // set when done
	Error     *JobError // set when failed
	UpdatedAt time.Time
}

// JobError is the code and plain message of a failed job (B3).
type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// View is a rendered view.
type View struct {
	ID        string
	CacheKey  string
	Target    computeclient.Target
	Params    computeclient.ViewParams
	Epochs    []computeclient.Epoch
	Frames    []computeclient.Frame
	Stretch   computeclient.Stretch
	Warnings  []string
	CreatedAt time.Time
}

// Store wraps a Postgres pool.
type Store struct {
	DB *pgxpool.Pool
}

// Open connects to Postgres and applies pending migrations.
func Open(ctx context.Context, url string) (*Store, error) {
	if err := migrateUp(url); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	return &Store{DB: pool}, nil
}

func migrateUp(url string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	// golang-migrate picks its pgx v5 driver by URL scheme.
	_, rest, ok := strings.Cut(url, "://")
	if !ok {
		return errors.New("DATABASE_URL must be a postgres:// URL")
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, "pgx5://"+rest)
	if err != nil {
		return err
	}
	defer func() { _, _ = m.Close() }()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

// Ping checks the connection.
func (s *Store) Ping(ctx context.Context) error { return s.DB.Ping(ctx) }

// Close closes the pool.
func (s *Store) Close() { s.DB.Close() }

const viewColumns = `id, cache_key, target, params, epochs, frames, stretch, warnings, created_at`

func scanView(row pgx.Row) (*View, error) {
	var v View
	err := row.Scan(&v.ID, &v.CacheKey, &v.Target, &v.Params, &v.Epochs, &v.Frames, &v.Stretch, &v.Warnings, &v.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

// ViewByKey returns the view for a cache key.
func (s *Store) ViewByKey(ctx context.Context, key string) (*View, error) {
	return scanView(s.DB.QueryRow(ctx, `SELECT `+viewColumns+` FROM views WHERE cache_key = $1`, key))
}

// ViewByID returns a view. Malformed IDs are ErrNotFound.
func (s *Store) ViewByID(ctx context.Context, id string) (*View, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanView(s.DB.QueryRow(ctx, `SELECT `+viewColumns+` FROM views WHERE id = $1`, id))
}

const jobColumns = `id, cache_key, status, progress, params, result->>'view_id', error, updated_at`

func scanJob(row pgx.Row) (*Job, error) {
	var j Job
	var viewID *string
	err := row.Scan(&j.ID, &j.CacheKey, &j.Status, &j.Progress, &j.Params, &viewID, &j.Error, &j.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if viewID != nil {
		j.ViewID = *viewID
	}
	return &j, err
}

// JobByID returns a job. Malformed IDs are ErrNotFound.
func (s *Store) JobByID(ctx context.Context, id string) (*Job, error) {
	if !isUUID(id) {
		return nil, ErrNotFound
	}
	return scanJob(s.DB.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = $1`, id))
}

// ActiveJob returns the queued or running job for a cache key, ignoring jobs
// whose runner went silent for longer than stale (a crashed gateway).
func (s *Store) ActiveJob(ctx context.Context, key string, stale time.Duration) (*Job, error) {
	return scanJob(s.DB.QueryRow(ctx, `
		SELECT `+jobColumns+` FROM jobs
		WHERE cache_key = $1 AND status IN ('queued', 'running') AND updated_at > now() - $2::interval`,
		key, stale))
}

// ClaimJob creates the job for a cache key, or takes over a finished, failed or
// stale one. It returns nil if a live job already holds the key; the unique
// cache_key makes this safe across concurrent requests and gateway instances (B4).
func (s *Store) ClaimJob(ctx context.Context, key string, params computeclient.ViewParams, stale time.Duration) (*Job, error) {
	j, err := scanJob(s.DB.QueryRow(ctx, `
		INSERT INTO jobs (cache_key, status, params) VALUES ($1, 'queued', $2)
		ON CONFLICT (cache_key) DO UPDATE
			SET status = 'queued', progress = 0, params = EXCLUDED.params,
			    result = NULL, error = NULL, created_at = now(), updated_at = now()
			WHERE jobs.status IN ('done', 'failed') OR jobs.updated_at <= now() - $3::interval
		RETURNING `+jobColumns, key, params, stale))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return j, err
}

// StartJob marks a queued job as running.
func (s *Store) StartJob(ctx context.Context, id string) error {
	_, err := s.DB.Exec(ctx, `UPDATE jobs SET status = 'running', updated_at = now() WHERE id = $1`, id)
	return err
}

// SetProgress records progress of a running job. Progress never goes back and
// stays below 100 until the job is done.
func (s *Store) SetProgress(ctx context.Context, id string, progress int) (bool, error) {
	if !isUUID(id) {
		return false, nil
	}
	tag, err := s.DB.Exec(ctx, `
		UPDATE jobs SET progress = GREATEST(progress, LEAST($2, 99)), updated_at = now()
		WHERE id = $1 AND status = 'running'`, id, max(progress, 0))
	return tag.RowsAffected() == 1, err
}

// FinishJob saves the view and marks the job done, in one transaction.
func (s *Store) FinishJob(ctx context.Context, jobID string, v *View) (string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var viewID string
	err = tx.QueryRow(ctx, `
		INSERT INTO views (cache_key, target, params, epochs, frames, stretch, warnings)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (cache_key) DO UPDATE
			SET target = EXCLUDED.target, params = EXCLUDED.params, epochs = EXCLUDED.epochs,
			    frames = EXCLUDED.frames, stretch = EXCLUDED.stretch, warnings = EXCLUDED.warnings
		RETURNING id`,
		v.CacheKey, v.Target, v.Params, nonNil(v.Epochs), nonNil(v.Frames), v.Stretch, nonNil(v.Warnings),
	).Scan(&viewID)
	if err != nil {
		return "", err
	}
	result := map[string]string{"view_id": viewID}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs SET status = 'done', progress = 100, result = $2, error = NULL, updated_at = now()
		WHERE id = $1`, jobID, result); err != nil {
		return "", err
	}
	return viewID, tx.Commit(ctx)
}

// FailJob marks a job failed with an error code and message.
func (s *Store) FailJob(ctx context.Context, id string, e JobError) error {
	_, err := s.DB.Exec(ctx, `
		UPDATE jobs SET status = 'failed', error = $2, updated_at = now() WHERE id = $1`, id, e)
	return err
}

// AddReleases records releases not seen before.
func (s *Store) AddReleases(ctx context.Context, rs []computeclient.Release) error {
	for _, r := range rs {
		if _, err := s.DB.Exec(ctx, `
			INSERT INTO releases (collection, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			r.Collection, r.Name); err != nil {
			return err
		}
	}
	return nil
}

// isUUID reports whether s is a canonical UUID, so bad IDs never reach Postgres.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return false
			}
		case !strings.ContainsRune("0123456789abcdefABCDEF", c):
			return false
		}
	}
	return true
}

// nonNil stores empty lists as [] rather than JSON null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
