# Domain Model

## Core Entities

### ReportDefinition

Describes a report that can be queried through the API.

- `slug`
- `name`
- `description`
- `cache_ttl_seconds`
- `default_window`
- `supported_filters`

### AggregateWindow

Defines supported aggregation windows and retention expectations per report.

- `report_slug`
- `window` (`day`, `week`)
- `retention_days`
- `refresh_interval_minutes`
- `is_default`

### MetricSnapshot

Stores precomputed analytics values used by report endpoints.

- `report_slug`
- `window`
- `bucket_start`
- `dimension_key`
- `dimension_value`
- `metric_name`
- `metric_value`
- `run_id` (nullable)

### RecomputeRun

Tracks manual recompute execution lifecycle.

- `id`
- `report_slug`
- `window`
- `date_from`, `date_to`
- `requested_by`
- `status` (`pending`, `running`, `completed`, `failed`)
- `summary`
- `error_message`

### AuditEntry

Records management and operational events.

- `actor`
- `action`
- `resource_type`
- `resource_id`
- `metadata`
- `created_at`

### EventSource

Represents an analytics data source/provider.

- `slug`
- `provider_name`
- `description`
- `is_active`

### AnalyticsEvent

Raw source event used for recompute and analytics derivation.

- `source_id`
- `external_ref`
- `entity_id`
- `status`
- `processing_ms`
- `amount_cents`
- `occurred_at`
- `payload`

## Report Semantics

Implemented report slugs:

1. `volume-by-period`
2. `status-counts`
3. `top-entities`

All three read from `metric_snapshots`, populated by recompute runs.

## Lifecycle Notes

- Reports are read-mostly.
- Recompute updates snapshot rows for a bounded scope.
- Recompute creates auditable state transitions and cache invalidation side effects.
