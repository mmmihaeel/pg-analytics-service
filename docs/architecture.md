# Architecture

`pg-analytics-service` is a deliberately compact analytics backend: one Go API, one PostgreSQL database, and one Redis instance. The design is centered on keeping report logic easy to review, recomputation explicit, and operational behavior visible in durable state.

## Related Docs

- [README](../README.md)
- [Domain Model](domain-model.md)
- [API Overview](api-overview.md)
- [Deployment Notes](deployment-notes.md)

## System Overview

The main runtime paths are:

- public report discovery and execution over precomputed snapshots
- management-triggered recomputation of snapshot ranges
- audit and health visibility for operational inspection

```mermaid
flowchart LR
    Client["Public client"] --> HTTP["HTTP handlers (`internal/http`)"]
    Operator["Management operator"] --> HTTP
    HTTP --> Reports["ReportService"]
    HTTP --> Recompute["RecomputeService"]
    HTTP --> Audit["AuditService"]
    HTTP --> Health["HealthService"]
    Reports --> PG["PostgreSQL"]
    Reports --> Redis["Redis"]
    Recompute --> Lock["Redis scope lock"]
    Recompute --> RunRow["`recompute_runs` row"]
    Recompute --> Queue["In-process queue"]
    Queue --> Worker["Worker goroutine"]
    Worker --> PG
    Worker --> Redis
    Audit --> PG
    Health --> PG
    Health --> Redis
```

## Runtime Components

| Component | Primary code location | Responsibility |
| --- | --- | --- |
| API entrypoint | `cmd/api` | Loads config, opens PostgreSQL and Redis clients, optionally migrates and seeds, starts HTTP server and worker goroutines. |
| HTTP transport | `internal/http` | Routes `/api/v1`, parses query and JSON inputs, applies timeouts, enforces management auth, and maps application errors into JSON responses. |
| Report service | `internal/application/report_service.go` | Validates query scope, reads report definitions, serves cached results when possible, and executes snapshot queries on cache miss. |
| Recompute service | `internal/application/recompute_service.go` | Validates recompute scope, acquires Redis locks, creates run records, enqueues work, updates run status, bumps cache version, and writes audit events. |
| PostgreSQL store | `internal/infrastructure/postgres` | Owns schema, seed data, report catalog queries, report execution SQL, recompute SQL, run persistence, and audit persistence. |
| Redis store | `internal/infrastructure/redis` | Owns report cache reads and writes, per-report cache version keys, and expiring lock acquisition and release. |

## Request Paths

### Report execution

1. A client calls `GET /api/v1/reports/{slug}/run`.
2. The handler parses `window`, `date_from`, `date_to`, `limit`, `offset`, and optional breakdown or filter parameters.
3. `ReportService` validates the window and range. If dates are omitted, the HTTP layer defaults to the last 30 days.
4. The service loads the report definition from PostgreSQL and resolves the cache TTL.
5. If caching is enabled, Redis is checked using a key derived from the report slug, current cache version, and normalized request parameters.
6. On cache miss, PostgreSQL queries `metric_snapshots` and returns report rows.
7. The result is serialized back into Redis and returned with execution metadata.

### Recomputation orchestration

1. A management client calls `POST /api/v1/recomputations`.
2. The handler requires a management key and validates the JSON body.
3. `RecomputeService` validates the requested report, window, and bounded date range.
4. Redis attempts to acquire a scope lock keyed by report slug, window, and date range.
5. A `recompute_runs` row is inserted with `pending` status, and an audit entry records the trigger event.
6. The run is pushed onto an in-process buffered channel.
7. A worker goroutine marks the run `running`, deletes the affected snapshot range, rebuilds it from `analytics_events`, and then marks the run `completed` or `failed`.
8. On success, Redis increments the report cache version; on failure, the error is stored on the run and written to the audit trail.

### Health and audit

- `GET /api/v1/health` checks PostgreSQL and Redis independently. If either dependency is unavailable, the endpoint returns a degraded status and the HTTP status becomes `503 Service Unavailable`.
- `GET /api/v1/audit-entries` is management-only and reads the persisted operational history from PostgreSQL.

## Data Stores and Ownership

| Store | Durable | What it owns |
| --- | --- | --- |
| PostgreSQL | Yes | `analytics_events`, `event_sources`, `report_definitions`, `aggregate_windows`, `metric_snapshots`, `recompute_runs`, and `audit_entries` |
| Redis | No | report-response cache payloads, per-report cache versions, and recompute locks |

This split is intentional. Analytical truth, report metadata, and operational history remain in PostgreSQL. Redis accelerates or protects workflows, but never becomes the system of record.

## PostgreSQL-First Analytics Design

The project is intentionally PostgreSQL-centered for both data and architecture reasons:

- Recompute SQL stays explicit and readable, which is important in a portfolio repository where reviewers will inspect the actual aggregation logic.
- `date_trunc('day'|'week', occurred_at)` gives a direct and predictable bucket model for snapshot generation.
- Snapshot reads avoid repeatedly grouping raw events on every API request.
- Run history and audit trail stay transactionally close to the data model they describe.
- Indexes on time, source, status, entity, run status, and audit timestamps match the dominant query paths.

## Trade-offs

| Decision | Benefit | Trade-off |
| --- | --- | --- |
| In-process recompute queue | Very small operational footprint and straightforward local setup | No durable backlog across API restarts |
| Full-range snapshot rebuild per requested scope | Deterministic and easy to reason about | More work than incremental recompute at larger scale |
| Redis version-bump invalidation | No wildcard delete or cache-key scanning | Old cache generations expire naturally rather than being actively removed |
| Single management API key | Simple, explicit control surface | Not a substitute for tenant-aware or role-based authorization |

Roadmap context: [Roadmap](roadmap.md)
