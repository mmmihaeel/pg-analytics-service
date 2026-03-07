.PHONY: up down logs migrate seed test lint fmt build

up:
	docker compose up --build -d

down:
	docker compose down -v

logs:
	docker compose logs -f app

migrate:
	docker compose run --rm app go run ./cmd/migrate

seed:
	docker compose run --rm -e SEED_FORCE_RESET=true app go run ./cmd/seed

test:
	docker compose run --rm app sh -c "go test ./... -count=1"

lint:
	docker compose run --rm app sh -c "go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.6 && /go/bin/golangci-lint run ./..."

fmt:
	docker compose run --rm app sh -c "gofmt -w $$(find . -name '*.go' -not -path './vendor/*')"

build:
	docker compose run --rm app go build ./...
