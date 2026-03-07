package application

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

func TestTriggerReturnsConflictWhenLockExists(t *testing.T) {
	svc := NewRecomputeService(
		&recomputeFakeReportRepo{},
		&recomputeFakeRunRepo{},
		&recomputeFakeAuditRepo{},
		&recomputeFakeLock{acquireResult: false},
		&recomputeFakeCache{},
		slog.Default(),
		366,
		10*time.Minute,
		1,
	)

	_, err := svc.Trigger(context.Background(), domain.RecomputeRequest{
		ReportSlug: "status-counts",
		Window:     domain.WindowDay,
		DateFrom:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DateTo:     time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected lock conflict error")
	}
	if !domain.IsAppErrorCode(err, domain.ErrCodeConflict) {
		t.Fatalf("expected conflict error code, got %v", err)
	}
}

func TestTriggerValidatesDateRange(t *testing.T) {
	svc := NewRecomputeService(
		&recomputeFakeReportRepo{},
		&recomputeFakeRunRepo{},
		&recomputeFakeAuditRepo{},
		&recomputeFakeLock{acquireResult: true},
		&recomputeFakeCache{},
		slog.Default(),
		30,
		10*time.Minute,
		1,
	)

	_, err := svc.Trigger(context.Background(), domain.RecomputeRequest{
		ReportSlug: "status-counts",
		Window:     domain.WindowDay,
		DateFrom:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DateTo:     time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected range validation error")
	}
	if !domain.IsAppErrorCode(err, domain.ErrCodeInvalid) {
		t.Fatalf("expected invalid error code, got %v", err)
	}
}

type recomputeFakeReportRepo struct{}

func (f *recomputeFakeReportRepo) ListReportDefinitions(ctx context.Context, filter domain.ReportListFilter) ([]domain.ReportDefinition, int, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *recomputeFakeReportRepo) GetReportDefinition(ctx context.Context, slug string) (*domain.ReportDefinition, error) {
	return &domain.ReportDefinition{Slug: slug, CacheTTLSeconds: 0}, nil
}

func (f *recomputeFakeReportRepo) RunVolumeReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	return nil, nil
}

func (f *recomputeFakeReportRepo) RunStatusCountsReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	return nil, nil
}

func (f *recomputeFakeReportRepo) RunTopEntitiesReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	return nil, nil
}

func (f *recomputeFakeReportRepo) RecomputeReport(ctx context.Context, request domain.RecomputeRequest, runID string) (domain.RecomputeSummary, error) {
	return domain.RecomputeSummary{}, nil
}

type recomputeFakeRunRepo struct{}

func (f *recomputeFakeRunRepo) CreateRun(ctx context.Context, request domain.RecomputeRequest, status string) (*domain.RecomputeRun, error) {
	return &domain.RecomputeRun{ID: "run-1", Status: status, Summary: map[string]any{}}, nil
}

func (f *recomputeFakeRunRepo) GetRun(ctx context.Context, runID string) (*domain.RecomputeRun, error) {
	return &domain.RecomputeRun{ID: runID, Summary: map[string]any{}}, nil
}

func (f *recomputeFakeRunRepo) MarkRunRunning(ctx context.Context, runID string) error {
	return nil
}

func (f *recomputeFakeRunRepo) MarkRunCompleted(ctx context.Context, runID string, summary domain.RecomputeSummary) error {
	return nil
}

func (f *recomputeFakeRunRepo) MarkRunFailed(ctx context.Context, runID, message string) error {
	return nil
}

type recomputeFakeAuditRepo struct{}

func (f *recomputeFakeAuditRepo) CreateEntry(ctx context.Context, entry domain.AuditEntry) error {
	return nil
}

func (f *recomputeFakeAuditRepo) ListEntries(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, int, error) {
	return nil, 0, nil
}

type recomputeFakeLock struct {
	acquireResult bool
}

func (f *recomputeFakeLock) TryAcquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	return f.acquireResult, nil
}

func (f *recomputeFakeLock) Release(ctx context.Context, key, token string) error {
	return nil
}

type recomputeFakeCache struct{}

func (f *recomputeFakeCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return nil, false, nil
}

func (f *recomputeFakeCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return nil
}

func (f *recomputeFakeCache) GetVersion(ctx context.Context, slug string) (int64, error) {
	return 1, nil
}

func (f *recomputeFakeCache) BumpVersion(ctx context.Context, slug string) (int64, error) {
	return 2, nil
}
