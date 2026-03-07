package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/config"
	"github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/postgres"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err.Error())
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	migrator := postgres.NewMigrator(pool, "internal/infrastructure/postgres/migrations", logger)
	if err := migrator.Up(ctx); err != nil {
		logger.Error("migration failed", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("migrations applied successfully")
}
