# Deployment Notes

## Container Images

The Dockerfile provides:

- `dev` stage for local workflow (Go toolchain available)
- `build` stage for static binary compilation
- `runtime` stage (distroless) for minimal production image

## Runtime Configuration

Required environment variables:

- `DATABASE_URL`
- `REDIS_URL`
- `MANAGEMENT_API_KEY`

Recommended runtime variables:

- `MAX_REPORT_RANGE_DAYS`
- `RECOMPUTE_LOCK_TTL`
- `RECOMPUTE_WORKERS`
- `RECOMPUTE_QUEUE_SIZE`

## Data-Store Roles

- PostgreSQL:
  - source events
  - snapshot aggregates
  - recompute run lifecycle
  - audit history
- Redis:
  - report response cache
  - recompute lock coordination

## Startup Behavior

For production-like deployments:

- Keep `APP_AUTO_MIGRATE=false` and run migrations explicitly in deployment pipeline.
- Keep `APP_AUTO_SEED=false`.
- Enable health checks against `/api/v1/health`.

## Horizontal Scaling Notes

- Public report reads can scale horizontally.
- Recompute worker is in-process; multiple replicas can still run safely due Redis locks, but queue state is local to each replica.
- For larger scale or strict durability, replace in-process queue with an external queue.

## Current Recompute Trade-offs

- In-process queue means no durable backlog across restarts.
- Full-range recompute favors correctness/readability over minimal compute cost.
- Locking is scope-based and short-lived; operational retry logic remains API-driven.

## Data Backups and Recovery

- PostgreSQL stores source events, snapshots, recompute history, and audit entries.
- Backup strategy should include point-in-time recovery if deployed in production contexts.
- Redis data can be treated as disposable cache/lock state.
