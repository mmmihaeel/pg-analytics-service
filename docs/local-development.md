# Local Development

## Prerequisites

- Docker
- Docker Compose

## Environment

Create local env file:

```bash
cp .env.example .env
```

Default `.env` values are already aligned with the Docker Compose network.

## Start

```bash
docker compose up --build
```

On startup the API container can automatically:

- apply migrations (`APP_AUTO_MIGRATE=true`)
- seed demo data (`APP_AUTO_SEED=true`)

## Service Endpoints

- API: `http://localhost:3004`
- PostgreSQL: `localhost:5436`
- Redis: `localhost:6383`

## Design Intent in Local Runs

- PostgreSQL is the authoritative data engine for both analytics and operational state.
- Redis supports cache reads and recompute lock coordination only.
- You can inspect recompute and audit state directly in PostgreSQL to verify workflow behavior.

## Common Commands

```bash
# stop and remove stack
docker compose down -v

# run migrations manually
docker compose run --rm app go run ./cmd/migrate

# seed manually (force reset)
docker compose run --rm -e SEED_FORCE_RESET=true app go run ./cmd/seed

# run test suite
docker compose run --rm app go test ./... -count=1

# run linter
docker compose run --rm app sh -c "go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.6 && /go/bin/golangci-lint run ./..."
```

## Troubleshooting

- If the API fails during startup, inspect logs with `docker compose logs app`.
- If you need a clean dataset, run:
  - `docker compose down -v`
  - `docker compose up --build`
- If report responses seem stale, trigger a recomputation via management endpoint.
