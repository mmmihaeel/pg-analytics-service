INSERT INTO report_definitions (slug, name, description, cache_ttl_seconds, default_window, supported_filters)
VALUES
    (
        'volume-by-period',
        'Volume by Period',
        'Event volume grouped by day or week with optional source breakdown.',
        120,
        'day',
        '{"window":"day|week","breakdown":"period|source","source":"source slug"}'::jsonb
    ),
    (
        'status-counts',
        'Status Counts',
        'Distribution of processing outcomes with optional trend view by period.',
        90,
        'day',
        '{"window":"day|week","breakdown":"status|period","status":"success|failed|pending"}'::jsonb
    ),
    (
        'top-entities',
        'Top Entities',
        'Highest-volume entities over a date range.',
        45,
        'day',
        '{"window":"day|week","limit":"1-100","offset":">=0"}'::jsonb
    )
ON CONFLICT (slug) DO UPDATE
SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    cache_ttl_seconds = EXCLUDED.cache_ttl_seconds,
    default_window = EXCLUDED.default_window,
    supported_filters = EXCLUDED.supported_filters,
    updated_at = now();

INSERT INTO aggregate_windows (report_slug, window_name, retention_days, refresh_interval_minutes, is_default)
VALUES
    ('volume-by-period', 'day', 365, 60, true),
    ('volume-by-period', 'week', 730, 240, false),
    ('status-counts', 'day', 365, 60, true),
    ('status-counts', 'week', 730, 240, false),
    ('top-entities', 'day', 180, 120, true),
    ('top-entities', 'week', 365, 360, false)
ON CONFLICT (report_slug, window_name) DO UPDATE
SET
    retention_days = EXCLUDED.retention_days,
    refresh_interval_minutes = EXCLUDED.refresh_interval_minutes,
    is_default = EXCLUDED.is_default;

INSERT INTO event_sources (slug, provider_name, description)
VALUES
    ('stripe', 'Stripe', 'Primary card processor events.'),
    ('adyen', 'Adyen', 'Secondary processor and regional failover.'),
    ('paypal', 'PayPal', 'Wallet checkout event stream.')
ON CONFLICT (slug) DO UPDATE
SET
    provider_name = EXCLUDED.provider_name,
    description = EXCLUDED.description,
    is_active = true;
