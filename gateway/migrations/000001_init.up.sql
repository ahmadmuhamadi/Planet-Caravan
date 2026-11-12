-- Phase 1 tables (SPEC-gateway.md). Nothing else until Phase 3 is accepted (B1).

-- SPHEREx data releases seen in coverage results. B11 (Phase 2) uses this to spot new ones.
CREATE TABLE releases (
    collection    text PRIMARY KEY,
    name          text        NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now()
);

-- Rendered views. One row per cache key (B5); frame images live in object storage.
-- stretch and warnings are not in the spec's column list, but GET /views/{id}
-- must return them (CONTRACT) and they come from compute, not from the request.
CREATE TABLE views (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cache_key  text        NOT NULL UNIQUE,
    target     jsonb       NOT NULL,
    params     jsonb       NOT NULL,
    epochs     jsonb       NOT NULL,
    frames     jsonb       NOT NULL,
    stretch    jsonb       NOT NULL,
    warnings   jsonb       NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- View jobs. cache_key is unique so identical requests share one job (B4).
CREATE TABLE jobs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cache_key  text        NOT NULL UNIQUE,
    status     text        NOT NULL CHECK (status IN ('queued', 'running', 'done', 'failed')),
    progress   smallint    NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    params     jsonb       NOT NULL,
    result     jsonb,
    error      jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
