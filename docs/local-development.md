# Local Development

The local workflow is Docker-first and intentionally close to the real runtime shape: API, PostgreSQL, and Redis all run together, migrations can be applied explicitly, and seeded data gives the report surface something realistic to query immediately.

## Related Docs

- [README](../README.md)
- [Architecture](architecture.md)
- [Deployment Notes](deployment-notes.md)

## Prerequisites

- Docker
- Docker Compose

## Quick Start

1. Create the local environment file.

```sh
cp .env.example .env
```

2. Start the stack.

```sh
docker compose up --build
```

3. Verify health.

```sh
curl http://localhost:3004/api/v1/health
```

4. Run a sample report.

```sh
curl "http://localhost:3004/api/v1/reports/volume-by-period/run?window=day&date_from=2026-01-01&date_to=2026-02-01&breakdown=source"
```

5. Trigger a recompute.

```sh
curl -X POST http://localhost:3004/api/v1/recomputations \
  -H "Content-Type: application/json" \
  -H "X-Management-Key: local-management-key" \
  -d '{
    "report_slug": "status-counts",
    "window": "day",
    "date_from": "2026-01-01",
    "date_to": "2026-02-01",
    "requested_by": "local-operator"
  }'
```

## Local Services

| Service | Local address | Role |
| --- | --- | --- |
| API | `http://localhost:3004` | HTTP surface for reports, recomputations, audit, and health |
| PostgreSQL | `localhost:5436` | Durable store for raw events, snapshots, run history, and audit trail |
| Redis | `localhost:6383` | Cache and lock coordination |

## Startup Behavior

With the default compose setup:

- the app container runs `go run ./cmd/api`
- `APP_AUTO_MIGRATE=true` applies SQL migrations at startup
- `APP_AUTO_SEED=true` seeds the database if `analytics_events` is empty

The seeded dataset currently:

- inserts 25,000 demo events
- uses three event sources: `stripe`, `adyen`, and `paypal`
- precomputes both `day` and `week` snapshots for all three reports over the last 90 days

## Common Commands

| Task | Command |
| --- | --- |
| Start stack in background | `make up` |
| Stop and remove local volumes | `make down` |
| Tail app logs | `make logs` |
| Apply migrations manually | `make migrate` |
| Reset and reseed demo data | `make seed` |
| Run tests | `make test` |
| Run lint checks | `make lint` |
| Build the service | `make build` |

Equivalent compose commands remain available if you want to bypass `make`.

## Validation Notes

- `make test` runs inside the Docker workflow, which is the easiest path when you want environment parity.
- `go test ./... -count=1` also works locally if PostgreSQL and Redis are already available on the ports used by the test harness.
- `docker compose config` is a fast check for broken environment or compose edits and is also part of CI.

## Troubleshooting

- If the API container fails on boot, inspect logs with `docker compose logs app`.
- If you want a fully clean database and cache, run `make down` and then `docker compose up --build`.
- If report responses appear stale after a snapshot rebuild, verify the recompute run completed successfully and check Redis connectivity through `/api/v1/health`.
- If management requests return `401`, confirm that the local `MANAGEMENT_API_KEY` in `.env` matches the key you are sending in the request.
