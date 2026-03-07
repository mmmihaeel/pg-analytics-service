package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"

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

	store := postgres.NewStore(pool)
	seeder := postgres.NewSeeder(store, logger)
	force := parseBool(os.Getenv("SEED_FORCE_RESET"))
	if err := seeder.Seed(ctx, force); err != nil {
		logger.Error("seeding failed", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("seeding completed", "force_reset", force)
}

func parseBool(raw string) bool {
	if raw == "" {
		return false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return value
}
