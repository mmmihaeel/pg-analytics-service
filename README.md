# pg-analytics-service

`pg-analytics-service` is a production-style Go backend for query-heavy analytics over PostgreSQL. It exposes report endpoints backed by precomputed metric snapshots, supports manual recomputation runs, records an audit trail for operational actions, and uses Redis for cache acceleration and recompute locking.

## Why This Project

Many internal analytics APIs fail in one of two ways: either they stay as raw SQL wrappers with no operational controls, or they overcomplicate analytics with unnecessary infrastructure. This project targets a maintainable middle ground:

- PostgreSQL remains the system of record.
- Report queries are optimized and reviewable.
- Recompute workflows are explicit, auditable, and lock-protected.
- Redis is used where it adds concrete value (cache + lock), not as a generic dependency.

## Feature Highlights

- Versioned `/api/v1` HTTP API with consistent JSON envelopes.
- Public reporting endpoints for:
  - volume by period
  - status counts
  - top entities by event volume
- Configurable date ranges, windows (`day` / `week`), breakdowns, filters, and pagination.
- Manual recompute trigger endpoint with management authentication.
- Asynchronous recompute worker with run tracking (`pending`, `running`, `completed`, `failed`).
- Audit trail endpoint for management actions.
- Redis-backed report response cache with per-report version invalidation.
- Redis lock to prevent duplicate recompute triggers for the same scope.
- Docker Compose local stack (app + postgres + redis) with automatic migrations and seed.
- CI pipeline with formatting check, lint, tests, build, and compose config validation.

## Technology Stack

- Go 1.23
- PostgreSQL 16
- Redis 7
- Docker / Docker Compose
- chi router
- pgx (PostgreSQL driver / pool)
- go-redis
- GitHub Actions

## Architecture Summary

The codebase follows a layered structure:

- `cmd/` executable entrypoints (`api`, `migrate`, `seed`)
- `internal/domain` core entities and application error model
- `internal/application` business services and ports
- `internal/infrastructure` adapters (PostgreSQL, Redis, config)
- `internal/http` transport handlers, middleware, response contracts
- `tests/` integration test harness and API integration tests

Key architectural decisions:

- Handlers stay thin and translate HTTP concerns to application service calls.
- SQL is explicit in repository methods so report logic and recomputation can be reviewed directly.
- Snapshot recomputation is asynchronous but intentionally in-process for local-operational simplicity.
- Cache invalidation uses report version bumping, avoiding wildcard key scans.

## Report Design

Implemented reports:

1. `volume-by-period`
2. `status-counts`
3. `top-entities`

Each report reads from `metric_snapshots`, which are refreshed by recompute runs. Snapshot rows include:

- report slug
- window (`day` or `week`)
- bucket timestamp
- dimension key/value
- metric name/value
- optional recompute run id

## Report Types and Use Cases

### `volume-by-period`

Use this report when you need traffic/throughput trends over time.

- Typical questions:
  - "Did daily volume drop after a deployment?"
  - "Which source is driving this week-over-week spike?"
- Common params:
  - `window=day|week`
  - `breakdown=source`
  - `source=<slug>` (optional filter)

### `status-counts`

Use this report for quality/reliability distribution (`success`, `failed`, `pending`).

- Typical questions:
  - "What is failure-rate drift over the last 14 days?"
  - "Is pending backlog growing?"
- Common params:
  - `window=day|week`
  - `breakdown=period` (trend view)
  - `status=<status>` (optional filter)

### `top-entities`

Use this report for high-volume actor analysis (hot entities/customers/integrations).

- Typical questions:
  - "Which entities dominate event load this month?"
  - "Who should we profile first for performance tuning?"
- Common params:
  - `window=day|week`
  - `limit`, `offset`

## Recompute Flow

1. Management client calls `POST /api/v1/recomputations`.
2. API validates input and acquires a Redis lock keyed by report/window/date scope.
3. API creates a `recompute_runs` record (`pending`) and writes an audit entry.
4. In-process worker consumes the queued job.
5. Worker marks run `running`, rebuilds snapshot data in PostgreSQL, marks run `completed`/`failed`.
6. Worker bumps Redis cache version for the report and writes completion/failure audit entry.
7. Client checks status via `GET /api/v1/recomputations/:id`.

Current trade-offs:

- Recompute queue is intentionally in-process for operational simplicity.
- Queue jobs are not durable across API restarts.
- Recompute currently rewrites selected range snapshots (deterministic and simple, not incremental).

## Caching Strategy

- Cache key format: `report:{slug}:v{version}:{params_hash}`
- Version key format: `report:version:{slug}`
- Report reads:
  - resolve current report version
  - read/write cache by hashed normalized query params
- Recompute completion:
  - `INCR report:version:{slug}`
  - existing cached entries become stale automatically

What is cached:

- Report responses for cache-enabled report definitions.

What is not cached:

- Health endpoint
- recompute run status
- audit entry listings

Why this approach:

- no wildcard key deletion
- predictable invalidation during recompute completion
- bounded stale-window behavior controlled by TTL + version bump

## SQL and Indexing Notes

PostgreSQL query design is intentionally explicit in repository code to keep analytics behavior reviewable.

Primary query patterns:

- Time-range scans over `analytics_events.occurred_at` during recomputation.
- Snapshot reads over `(report_slug, window_name, bucket_start)` for reporting endpoints.
- Dimension filtering over `(report_slug, dimension_key, dimension_value, bucket_start)`.

Key indexes:

- `idx_analytics_events_occurred_at`
- `idx_analytics_events_source_time`
- `idx_analytics_events_status_time`
- `idx_metric_snapshots_lookup`
- `idx_metric_snapshots_dimension`
- operational indexes for run and audit retrieval

Design rationale:

- keep heavy aggregation in PostgreSQL where grouping and filtering are strongest
- precompute to `metric_snapshots` so read APIs stay fast and predictable
- preserve auditable operational state (`recompute_runs`, `audit_entries`) in the same durable datastore

## Local Development (Docker First)

### Prerequisites

- Docker
- Docker Compose

### 1) Configure environment

```bash
cp .env.example .env
```

### 2) Start the stack

```bash
docker compose up --build
```

Services:

- API: `http://localhost:3004`
- PostgreSQL: `localhost:5436`
- Redis: `localhost:6383`

On first startup the app:

- applies SQL migrations
- seeds realistic event data if the event table is empty
- precomputes day/week snapshots for all reports

### 3) Trigger recompute manually

```bash
curl -X POST http://localhost:3004/api/v1/recomputations \
  -H 'Content-Type: application/json' \
  -H 'X-Management-Key: local-management-key' \
  -d '{
    "report_slug":"status-counts",
    "window":"day",
    "date_from":"2026-01-01",
    "date_to":"2026-02-01",
    "requested_by":"local-operator"
  }'
```

### 4) Query a report

```bash
curl 'http://localhost:3004/api/v1/reports/volume-by-period/run?window=day&date_from=2026-01-01&date_to=2026-02-01&breakdown=source'
```

## Local Demo Walkthrough

### 1) Boot the stack

```bash
docker compose up --build
```

### 2) Verify health

```bash
curl http://localhost:3004/api/v1/health
```

### 3) List report catalog

```bash
curl 'http://localhost:3004/api/v1/reports?limit=10'
```

### 4) Run the same report twice to observe cache behavior

```bash
curl 'http://localhost:3004/api/v1/reports/status-counts/run?window=day&date_from=2026-02-01&date_to=2026-02-07'
curl 'http://localhost:3004/api/v1/reports/status-counts/run?window=day&date_from=2026-02-01&date_to=2026-02-07'
```

Expected:

- first response: `"cache_hit": false`
- second response: `"cache_hit": true`

### 5) Trigger recompute

```bash
curl -X POST http://localhost:3004/api/v1/recomputations \
  -H 'Content-Type: application/json' \
  -H 'X-Management-Key: local-management-key' \
  -d '{
    "report_slug":"status-counts",
    "window":"day",
    "date_from":"2026-02-01",
    "date_to":"2026-02-07",
    "requested_by":"local-demo"
  }'
```

### 6) Poll recompute status and inspect audit trail

```bash
curl -H 'X-Management-Key: local-management-key' \
  'http://localhost:3004/api/v1/recomputations/<run_id>'

curl -H 'X-Management-Key: local-management-key' \
  'http://localhost:3004/api/v1/audit-entries?limit=5'
```

## Useful Commands

```bash
# stack lifecycle
make up
make down

# migrations / seed
make migrate
make seed

# quality gates
make test
make lint
make build
```

## Testing and Quality Checks

Automated checks cover:

- report endpoints
- validation behavior
- management auth enforcement
- recompute trigger and status flow
- duplicate recompute lock behavior
- cache-hit behavior
- audit entries endpoint
- runtime state reset between tests

Run all tests:

```bash
go test ./... -count=1
```

## API Overview

Base path: `/api/v1`

Public routes:

- `GET /health`
- `GET /reports`
- `GET /reports/{slug}`
- `GET /reports/{slug}/run`

Management routes (require `X-Management-Key` or `Authorization: Bearer <key>`):

- `POST /recomputations`
- `GET /recomputations/{id}`
- `GET /audit-entries`

Detailed request/response notes are in [`docs/api-overview.md`](docs/api-overview.md).

## Repository Structure

```text
cmd/
  api/
  migrate/
  seed/
internal/
  application/
  domain/
  http/
  infrastructure/
    config/
    postgres/
      migrations/
    redis/
docs/
tests/
  integration/
  testutil/
.github/workflows/
```

## Security Notes

- Management endpoints are protected by a required management API key.
- Input validation bounds expensive query windows and pagination.
- Query fields with dynamic sorting are allowlisted.
- Recompute actions are persisted to audit entries.
- Error payloads avoid leaking internal stack traces.

## Future Improvements

- Persistent external job queue for recompute workers.
- Per-tenant partitioning and row-level authorization.
- Incremental snapshot recompute strategies.
- OpenAPI schema generation and contract tests.
- Optional Prometheus metrics and distributed tracing.
