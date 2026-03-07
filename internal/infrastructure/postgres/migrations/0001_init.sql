CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS event_sources (
    id BIGSERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    provider_name TEXT NOT NULL,
    description TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS analytics_events (
    id BIGSERIAL PRIMARY KEY,
    source_id BIGINT NOT NULL REFERENCES event_sources(id),
    external_ref TEXT NOT NULL UNIQUE,
    entity_id TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('success', 'failed', 'pending')),
    processing_ms INTEGER NOT NULL CHECK (processing_ms >= 0),
    amount_cents INTEGER NOT NULL CHECK (amount_cents >= 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS report_definitions (
    slug TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    cache_ttl_seconds INTEGER NOT NULL CHECK (cache_ttl_seconds >= 0),
    default_window TEXT NOT NULL CHECK (default_window IN ('day', 'week')),
    is_public BOOLEAN NOT NULL DEFAULT TRUE,
    supported_filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS aggregate_windows (
    id BIGSERIAL PRIMARY KEY,
    report_slug TEXT NOT NULL REFERENCES report_definitions(slug) ON DELETE CASCADE,
    window_name TEXT NOT NULL CHECK (window_name IN ('day', 'week')),
    retention_days INTEGER NOT NULL CHECK (retention_days > 0),
    refresh_interval_minutes INTEGER NOT NULL CHECK (refresh_interval_minutes > 0),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE(report_slug, window_name)
);

CREATE TABLE IF NOT EXISTS recompute_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_slug TEXT NOT NULL REFERENCES report_definitions(slug),
    window_name TEXT NOT NULL CHECK (window_name IN ('day', 'week')),
    date_from DATE NOT NULL,
    date_to DATE NOT NULL,
    requested_by TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    error_message TEXT,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS metric_snapshots (
    id BIGSERIAL PRIMARY KEY,
    report_slug TEXT NOT NULL REFERENCES report_definitions(slug) ON DELETE CASCADE,
    window_name TEXT NOT NULL CHECK (window_name IN ('day', 'week')),
    bucket_start TIMESTAMPTZ NOT NULL,
    dimension_key TEXT NOT NULL,
    dimension_value TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_value BIGINT NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    run_id UUID REFERENCES recompute_runs(id) ON DELETE SET NULL,
    UNIQUE (report_slug, window_name, bucket_start, dimension_key, dimension_value, metric_name)
);

CREATE TABLE IF NOT EXISTS audit_entries (
    id BIGSERIAL PRIMARY KEY,
    actor TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_analytics_events_occurred_at ON analytics_events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_source_time ON analytics_events(source_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_status_time ON analytics_events(status, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_analytics_events_entity_time ON analytics_events(entity_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_metric_snapshots_lookup ON metric_snapshots(report_slug, window_name, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_metric_snapshots_dimension ON metric_snapshots(report_slug, dimension_key, dimension_value, bucket_start DESC);

CREATE INDEX IF NOT EXISTS idx_recompute_runs_status_requested_at ON recompute_runs(status, requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_recompute_runs_report_requested_at ON recompute_runs(report_slug, requested_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_entries_created_at ON audit_entries(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_entries_action_created_at ON audit_entries(action, created_at DESC);
