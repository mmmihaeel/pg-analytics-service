# Deployment Notes

The repository is local-first, but the runtime shape is production-minded enough to document clearly: build a static Go binary, run it with PostgreSQL and Redis, control startup behavior through env vars, and be explicit about where durability starts and stops.

## Related Docs

- [README](../README.md)
- [Architecture](architecture.md)
- [Security](security.md)

## Image Strategy

`docker/go/Dockerfile` defines three stages:

| Stage | Purpose |
| --- | --- |
| `dev` | Compose-oriented local development image with Go tooling available |
| `build` | Compiles the Linux AMD64 binary with `CGO_ENABLED=0` |
| `runtime` | Minimal distroless image that runs only the compiled service binary |

Compose currently uses the `dev` target because the local workflow is source-mounted and build-tool-driven. A deployment pipeline would typically build and run the `runtime` stage instead.

## Runtime Configuration

| Variable | Required | Default | Purpose |
| --- | --- | --- | --- |
| `DATABASE_URL` | Yes | none | PostgreSQL connection string |
| `REDIS_URL` | Yes | none | Redis connection string |
| `MANAGEMENT_API_KEY` | Yes in practice | `change-me` | Shared key for protected endpoints |
| `APP_ENV` | No | `development` | Environment label used in logs and startup context |
| `APP_PORT` | No | `3004` | HTTP listen port |
| `APP_VERSION` | No | `dev` | Version surfaced by `/api/v1/health` |
| `APP_AUTO_MIGRATE` | No | `true` | Apply migrations during API startup |
| `APP_AUTO_SEED` | No | `false` | Seed demo data during API startup |
| `MAX_REPORT_RANGE_DAYS` | No | `366` | Upper bound for report execution and recompute range |
| `RECOMPUTE_LOCK_TTL` | No | `15m` | Expiry for Redis recompute locks |
| `RECOMPUTE_WORKERS` | No | `1` | Number of worker goroutines consuming the queue |
| `RECOMPUTE_QUEUE_SIZE` | No | `64` | Buffered in-process queue depth |
| `HTTP_READ_TIMEOUT` | No | `10s` | Server read timeout |
| `HTTP_WRITE_TIMEOUT` | No | `20s` | Server write timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | No | `15s` | Graceful shutdown timeout |

## Deployment Guidance

For production-like environments:

- run migrations explicitly with `go run ./cmd/migrate` or an equivalent migration step in the release pipeline
- keep `APP_AUTO_SEED=false`
- treat the fallback `MANAGEMENT_API_KEY=change-me` as invalid and always provide a real secret
- wire health checks to `GET /api/v1/health`
- persist PostgreSQL storage and treat Redis data as disposable

## Scaling and Durability

| Area | Current behavior | Implication |
| --- | --- | --- |
| Public report reads | Stateless HTTP handlers backed by PostgreSQL and Redis | Horizontal read scaling is straightforward if PostgreSQL can absorb the query volume |
| Recomputation queue | Buffered channel inside the API process | Queue state is not durable across restarts and is local to each replica |
| Duplicate-trigger protection | Redis scope lock | Multiple replicas can still reject duplicate recompute triggers safely if they share Redis |
| Cache invalidation | Per-report Redis version keys | Invalidation semantics stay simple even with multiple app replicas |

The main scaling caveat is recomputation durability. The current design is strong for local development and moderate load, but a durable external queue would be the next step for higher operational demands.

## Data Durability and Recovery

PostgreSQL is the durable store for:

- raw analytics events
- report definitions and aggregate window metadata
- snapshot aggregates
- recompute history
- audit entries

Redis is safe to treat as disposable because it only holds caches and locks. Recovery planning should therefore focus on PostgreSQL backups, migration discipline, and point-in-time recovery if the service is deployed outside local development.

## Production Hardening Checklist

- Provide managed PostgreSQL backups.
- Run the distroless runtime image or an equivalent minimal artifact.
- Terminate TLS upstream of the service.
- Restrict management endpoints at the network edge.
- Monitor health status, recompute failures, and queue-pressure symptoms.

More future-facing work is tracked in [Roadmap](roadmap.md).
