package application

import (
	"context"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type ReportRepository interface {
	ListReportDefinitions(ctx context.Context, filter domain.ReportListFilter) ([]domain.ReportDefinition, int, error)
	GetReportDefinition(ctx context.Context, slug string) (*domain.ReportDefinition, error)
	RunVolumeReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error)
	RunStatusCountsReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error)
	RunTopEntitiesReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error)
	RecomputeReport(ctx context.Context, request domain.RecomputeRequest, runID string) (domain.RecomputeSummary, error)
}

type RecomputeRunRepository interface {
	CreateRun(ctx context.Context, request domain.RecomputeRequest, status string) (*domain.RecomputeRun, error)
	GetRun(ctx context.Context, runID string) (*domain.RecomputeRun, error)
	MarkRunRunning(ctx context.Context, runID string) error
	MarkRunCompleted(ctx context.Context, runID string, summary domain.RecomputeSummary) error
	MarkRunFailed(ctx context.Context, runID, message string) error
}

type AuditRepository interface {
	CreateEntry(ctx context.Context, entry domain.AuditEntry) error
	ListEntries(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, int, error)
}

type ReportCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	GetVersion(ctx context.Context, slug string) (int64, error)
	BumpVersion(ctx context.Context, slug string) (int64, error)
}

type RecomputeLock interface {
	TryAcquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key, token string) error
}

type HealthStore interface {
	PingDB(ctx context.Context) error
	PingCache(ctx context.Context) error
}
