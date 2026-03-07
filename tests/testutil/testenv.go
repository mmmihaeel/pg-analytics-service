package testutil

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pg-analytics-service/pg-analytics-service/internal/application"
	httpapi "github.com/pg-analytics-service/pg-analytics-service/internal/http"
	"github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/postgres"
	redisstore "github.com/pg-analytics-service/pg-analytics-service/internal/infrastructure/redis"
)

var (
	setupOnce sync.Once
	sharedEnv *SharedEnv
	setupErr  error
)

type SharedEnv struct {
	Pool          *pgxpool.Pool
	Redis         *redisstore.Store
	Store         *postgres.Store
	Logger        *slog.Logger
	ManagementKey string
}

type ServerHarness struct {
	Env    *SharedEnv
	Server *httptest.Server
	cancel context.CancelFunc
}

func NewServerHarness(t *testing.T, startWorkers bool) *ServerHarness {
	t.Helper()
	env := MustInitSharedEnv(t)

	ctx := context.Background()
	if err := resetRuntimeState(ctx, env); err != nil {
		t.Fatalf("failed to reset runtime state: %v", err)
	}

	reportService := application.NewReportService(env.Store, env.Redis, env.Logger, 366)
	recomputeService := application.NewRecomputeService(
		env.Store,
		env.Store,
		env.Store,
		env.Redis,
		env.Redis,
		env.Logger,
		366,
		15*time.Minute,
		16,
	)

	workerCtx, cancel := context.WithCancel(context.Background())
	if startWorkers {
		recomputeService.Start(workerCtx, 1)
	}

	auditService := application.NewAuditService(env.Store)
	healthService := application.NewHealthService(healthDependencies{db: env.Store, cache: env.Redis}, "test")
	handler := httpapi.NewHandler(reportService, recomputeService, auditService, healthService, 30)

	server := httptest.NewServer(handler.Routes(env.ManagementKey))
	return &ServerHarness{Env: env, Server: server, cancel: cancel}
}

func (h *ServerHarness) Close() {
	h.cancel()
	h.Server.Close()
}

func MustInitSharedEnv(t *testing.T) *SharedEnv {
	t.Helper()

	setupOnce.Do(func() {
		logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
		dbURL := getEnv("DATABASE_URL", "postgres://app:app@localhost:5436/pg_analytics?sslmode=disable")
		redisURL := getEnv("REDIS_URL", "redis://localhost:6383/0")
		managementKey := getEnv("MANAGEMENT_API_KEY", "test-management-key")

		ctx := context.Background()
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			setupErr = err
			return
		}

		redis, err := redisstore.NewStore(redisURL)
		if err != nil {
			pool.Close()
			setupErr = err
			return
		}

		store := postgres.NewStore(pool)
		migrator := postgres.NewMigrator(pool, migrationDir(), logger)
		if err := migrator.Up(ctx); err != nil {
			_ = redis.Close()
			pool.Close()
			setupErr = err
			return
		}

		seeder := postgres.NewSeeder(store, logger)
		if err := seeder.Seed(ctx, false); err != nil {
			_ = redis.Close()
			pool.Close()
			setupErr = err
			return
		}

		sharedEnv = &SharedEnv{
			Pool:          pool,
			Redis:         redis,
			Store:         store,
			Logger:        logger,
			ManagementKey: managementKey,
		}
	})

	if setupErr != nil {
		t.Fatalf("failed to initialize shared test env: %v", setupErr)
	}

	return sharedEnv
}

func resetRuntimeState(ctx context.Context, env *SharedEnv) error {
	if err := env.Redis.PingCache(ctx); err != nil {
		return err
	}
	if err := env.Redis.FlushAll(ctx); err != nil {
		return err
	}

	_, err := env.Pool.Exec(ctx, `
		DELETE FROM audit_entries;
		DELETE FROM recompute_runs WHERE requested_by <> 'seed';
	`)
	if err != nil {
		return err
	}

	return nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func migrationDir() string {
	candidates := []string{
		"internal/infrastructure/postgres/migrations",
		"../internal/infrastructure/postgres/migrations",
		"../../internal/infrastructure/postgres/migrations",
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}

	return filepath.Clean(candidates[0])
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
