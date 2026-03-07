package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pg-analytics-service/pg-analytics-service/internal/application"
	httpapi "github.com/pg-analytics-service/pg-analytics-service/internal/http"
	"github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/config"
	"github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/postgres"
	redisstore "github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/redis"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{AddSource: false}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("failed to connect to postgres", "error", err.Error())
		os.Exit(1)
	}
	defer pool.Close()

	redis, err := redisstore.NewStore(cfg.RedisURL)
	if err != nil {
		logger.Error("failed to connect to redis", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		if closeErr := redis.Close(); closeErr != nil {
			logger.Warn("failed to close redis client", "error", closeErr.Error())
		}
	}()

	store := postgres.NewStore(pool)

	migrator := postgres.NewMigrator(pool, "internal/infrastructure/postgres/migrations", logger)
	if cfg.AutoMigrate {
		if err := migrator.Up(ctx); err != nil {
			logger.Error("failed to apply migrations", "error", err.Error())
			os.Exit(1)
		}
	}

	if cfg.AutoSeed {
		seeder := postgres.NewSeeder(store, logger)
		if err := seeder.Seed(ctx, false); err != nil {
			logger.Error("failed to seed demo data", "error", err.Error())
			os.Exit(1)
		}
	}

	reportService := application.NewReportService(store, redis, logger, cfg.MaxReportRangeDays)
	recomputeService := application.NewRecomputeService(
		store,
		store,
		store,
		redis,
		redis,
		logger,
		cfg.MaxReportRangeDays,
		cfg.RecomputeLockTTL,
		cfg.RecomputeQueueSize,
	)
	recomputeService.Start(ctx, cfg.RecomputeWorkers)

	auditService := application.NewAuditService(store)
	healthService := application.NewHealthService(healthDependencies{db: store, cache: redis}, cfg.Version)
	handler := httpapi.NewHandler(reportService, recomputeService, auditService, healthService, 30)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler.Routes(cfg.ManagementAPIKey),
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
	}

	serverErrCh := make(chan error, 1)
	go func() {
		logger.Info("starting api server", "port", cfg.Port, "env", cfg.Env)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErrCh:
		logger.Error("server failed", "error", err.Error())
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTPShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("failed to shutdown server gracefully", "error", err.Error())
		os.Exit(1)
	}

	logger.Info("server shutdown complete")
}

type healthDependencies struct {
	db    *postgres.Store
	cache *redisstore.Store
}

func (h healthDependencies) PingDB(ctx context.Context) error {
	return h.db.PingDB(ctx)
}

func (h healthDependencies) PingCache(ctx context.Context) error {
	return h.cache.PingCache(ctx)
}
