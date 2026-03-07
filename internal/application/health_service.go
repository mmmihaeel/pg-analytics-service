package application

import (
	"context"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type HealthService struct {
	store   HealthStore
	version string
}

func NewHealthService(store HealthStore, version string) *HealthService {
	return &HealthService{store: store, version: version}
}

func (s *HealthService) Check(ctx context.Context) domain.HealthStatus {
	deps := map[string]string{
		"postgres": "ok",
		"redis":    "ok",
	}
	status := "ok"

	dbCtx, dbCancel := context.WithTimeout(ctx, time.Second)
	defer dbCancel()
	if err := s.store.PingDB(dbCtx); err != nil {
		deps["postgres"] = "unavailable"
		status = "degraded"
	}

	cacheCtx, cacheCancel := context.WithTimeout(ctx, time.Second)
	defer cacheCancel()
	if err := s.store.PingCache(cacheCtx); err != nil {
		deps["redis"] = "unavailable"
		status = "degraded"
	}

	return domain.HealthStatus{
		Status:       status,
		Service:      "pg-analytics-service",
		Version:      s.version,
		Timestamp:    time.Now().UTC(),
		Dependencies: deps,
	}
}
