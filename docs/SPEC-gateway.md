# SPEC — gateway repo (Go, Gin)

Read CONTRACT.md first. Work one phase at a time.

## Role
Public face. Validates requests, runs jobs, caches, rate-limits, serves results. Calls compute for all astronomy. Stays thin until Phase 3 is accepted (B1).

## Stack
Go, Gin, Postgres (golang-migrate), Redis, S3-compatible object storage (local disk or MinIO for dev).
Layout: cmd/, internal/{api,jobs,cache,store,compute_client}, migrations/

## Phase 1 — core
API (shapes in CONTRACT.md)
- /resolve, /autocomplete, /coverage, POST /views, GET /views (query params), /jobs/{id}, /views/{id}, frames .png, /healthz, /readyz, /metrics
- B13: every error uses the CONTRACT error format
- B9: no coverage → 200 with covered:false, not an error
Jobs
- B2: POST /views returns cached result (200) or job (202) immediately; status has progress %
- B3: global job timeout 5 min (config); failed job stores error code + message
- B4: identical requests while a job runs share that job (singleflight on cache key)
Cache
- B5: cache key per CONTRACT (coords rounded to 0.1 arcsec, fov, mode, options, release set)
- Redis for JSON/metadata, object storage for images
- TTLs (config): resolved names 30 days; coverage 6 h; views until a new release covers the position
Compute client
- B6: types generated from pinned compute OpenAPI schema, or a contract test fails the build on mismatch
- Send X-Internal-Secret and X-Request-ID on every call; per-call timeout + retries with backoff and jitter
Limits
- R1: fov min 1, default 3, max 30 arcmin; reject above max
- R2: token-bucket rate limit per IP; stricter on job-creating endpoints; cap on concurrent jobs per IP
- R3: validate RA 0-360, Dec -90 to 90, fov bounds, name length; request size limit
- CORS: allow only the client's deployed domains (H6)
Tables (Phase 1 only): jobs, views, releases
- jobs(id, cache_key unique, status, progress, params jsonb, result jsonb, error, created_at, updated_at)
- views(id, cache_key unique, target jsonb, params jsonb, epochs jsonb, frames jsonb, created_at)
- releases(name, collection, first_seen_at)

## Phase 2 — links and featured targets
- B8: GET /objects/{slug} from a fixed featured list (from compute Phase 0, G4); other names resolve via SIMBAD
- B11: precompute featured targets; recompute when a new release appears
- R5: featured targets are served fully from cache + storage when IRSA/compute is unreachable
- B10: view result includes release_set and newer_data_available
- B12 (Should): GET /views/{id}/blink.gif
- R6 (Should): non-featured rendered files expire (config); total storage capped

## Phase 3 — analysis pass-through
- /views/{id}/sources (filters, sort, limit/offset), /sources/{source_id}, /lightcurve, /track
- Table: sources(id, view_id, ra, dec, mag, type, catalog_ids jsonb, motion jsonb, variability, flags, is_unknown)
- B7: pass through pixel_positions and Frame geometry unchanged
- Info links per source from IDs/coords: SIMBAD, NED, JPL Small-Body Database, Gaia Archive. No match → links: [] and no_known_match: true
- Pass compute warnings through unchanged

## Tests
- Unit: cache key normalisation, job dedup, rate limits, validation, error format
- Contract test against the pinned compute schema (H3)
- Integration with a stubbed compute; suite passes offline
- Featured target loads with compute unreachable

## Deploy
- H5: this repo holds the shared docs (requirements, decisions.md) and one docker-compose file for local dev that pulls the compute and client images
- H9 (Should): managed Postgres, Redis, object storage
- H10: secrets from environment only
- H11: pipeline = test, build image, deploy on tag
- R7 (Should): structured JSON logs with request/job ID; metrics for job duration, cache hit rate, compute latency/errors
- Dockerfile, make targets, README, 12-factor config

## Do not (Later, design only)
- Watch list, scheduler, change summaries, candidates, votes, SSE events, spectrum endpoint
- Any astronomy logic in Go
- Expose compute publicly
