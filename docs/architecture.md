# Architecture

## System Overview

`pg-analytics-service` is a containerized Go backend that exposes analytics reports over PostgreSQL event data. The service maintains read-optimized snapshot tables, supports manual recomputation, and uses Redis for report caching and lock coordination.

## Runtime Components

1. API process (`cmd/api`)
2. PostgreSQL (`reporting + operational state`)
3. Redis (`cache + lock state`)

## Why PostgreSQL Is Central

PostgreSQL is the primary system in this design, not just a backing store.

- Raw event data lives in PostgreSQL (`analytics_events`).
- Aggregated read models are materialized into PostgreSQL (`metric_snapshots`).
- Operational history and auditability are persisted in PostgreSQL (`recompute_runs`, `audit_entries`).

Reasons this project is PostgreSQL-first:

- strong grouping/filtering/query-plan capabilities for analytics workloads
- transactional consistency for recompute state transitions
- clear and reviewable SQL for portfolio-level backend evaluation

## Layered Code Structure

- `internal/domain`
  - Domain entities and API-facing model structures.
  - Shared application error type (`AppError`) with stable error codes.
- `internal/application`
  - Report execution service.
  - Recompute orchestration service (queue + worker interaction).
  - Audit and health services.
  - Ports/interfaces for repository, cache, and lock adapters.
- `internal/infrastructure`
  - PostgreSQL repository implementation (queries, recompute SQL, migrations, seed).
  - Redis implementation (cache, lock, versioning).
  - Environment configuration.
- `internal/http`
  - chi routes, handlers, management auth middleware.
  - Consistent JSON response envelope and error mapping.

## Request Lifecycle

### Report Query

1. Client calls `GET /api/v1/reports/{slug}/run` with filters.
2. Handler validates and normalizes query params.
3. `ReportService` resolves report definition and checks Redis cache.
4. On miss, PostgreSQL snapshot query executes.
5. Response is cached (if report cache TTL > 0) and returned.

### Recompute Trigger

1. Management client calls `POST /api/v1/recomputations`.
2. Handler validates payload and enforces management key.
3. `RecomputeService` acquires Redis lock for `(report, window, date range)`.
4. `recompute_runs` row is created with `pending` status.
5. Job is pushed to an in-process queue.
6. Worker marks run `running`, executes report-specific recompute SQL, marks run `completed`/`failed`.
7. Worker bumps report cache version and writes audit events.

## Data Design and Query Strategy

- Raw events: `analytics_events`
- Snapshot table: `metric_snapshots`
- Report metadata: `report_definitions`, `aggregate_windows`
- Operations tables: `recompute_runs`, `audit_entries`

The service uses explicit SQL with targeted indexes:

- time-based event indexes for range filtering
- snapshot indexes for report/window/bucket scans
- audit and run indexes for operational queries

## Redis Responsibilities

Redis has two focused responsibilities:

1. Report response caching with versioned keys.
2. Recompute scope locks to prevent duplicate concurrent triggers.

Redis is intentionally not used as a system of record. If Redis is unavailable, durable state remains in PostgreSQL.

## Operational Trade-offs

- Recompute worker is in-process by design.
  - Pros: low operational overhead, simple local setup.
  - Cons: queue is not durable across API restarts.
- Snapshot recompute is full-range for selected scope.
  - Pros: deterministic, easy to reason about.
  - Cons: heavier than incremental recompute at larger scale.
- Cache invalidation uses version bumping rather than targeted deletion.
  - Pros: simple invalidation semantics, no key scan/delete patterns.
  - Cons: stale keys expire naturally by TTL instead of immediate physical removal.

## Extension Points

- Replace in-memory queue with a durable queue/broker.
- Add per-report recompute scheduling.
- Add tenant scoping and authorization layers.
- Add observability (metrics, tracing, structured request logs).
