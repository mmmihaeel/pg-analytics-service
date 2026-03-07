package application

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

func TestRunReportRejectsInvalidWindow(t *testing.T) {
	svc := NewReportService(&fakeReportRepo{}, &fakeCacheStore{}, slog.Default(), 30)
	_, err := svc.RunReport(context.Background(), "volume-by-period", domain.ReportRunParams{
		Window:   "month",
		DateFrom: time.Now().AddDate(0, 0, -1),
		DateTo:   time.Now(),
		Limit:    10,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !domain.IsAppErrorCode(err, domain.ErrCodeInvalid) {
		t.Fatalf("expected invalid request error, got %v", err)
	}
}

func TestRunReportUsesCacheOnSecondCall(t *testing.T) {
	repo := &fakeReportRepo{}
	cache := &fakeCacheStore{versions: map[string]int64{}, data: map[string][]byte{}}
	svc := NewReportService(repo, cache, slog.Default(), 366)

	params := domain.ReportRunParams{
		Window:   domain.WindowDay,
		DateFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DateTo:   time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		Limit:    10,
	}

	first, err := svc.RunReport(context.Background(), "status-counts", params)
	if err != nil {
		t.Fatalf("unexpected error on first run: %v", err)
	}
	if first.CacheHit {
		t.Fatal("expected first run to be uncached")
	}

	second, err := svc.RunReport(context.Background(), "status-counts", params)
	if err != nil {
		t.Fatalf("unexpected error on second run: %v", err)
	}
	if !second.CacheHit {
		t.Fatal("expected second run to be cache hit")
	}
	if repo.statusCalls != 1 {
		t.Fatalf("expected repository query to run once, ran %d times", repo.statusCalls)
	}
}

type fakeReportRepo struct {
	statusCalls int
}

func (f *fakeReportRepo) ListReportDefinitions(ctx context.Context, filter domain.ReportListFilter) ([]domain.ReportDefinition, int, error) {
	return nil, 0, errors.New("not implemented")
}

func (f *fakeReportRepo) GetReportDefinition(ctx context.Context, slug string) (*domain.ReportDefinition, error) {
	if slug == "status-counts" {
		return &domain.ReportDefinition{Slug: slug, CacheTTLSeconds: 60}, nil
	}
	if slug == "volume-by-period" {
		return &domain.ReportDefinition{Slug: slug, CacheTTLSeconds: 60}, nil
	}
	return nil, domain.NewAppError(domain.ErrCodeNotFound, "not found")
}

func (f *fakeReportRepo) RunVolumeReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	return []map[string]any{{"bucket_date": "2026-01-01", "event_count": int64(4)}}, nil
}

func (f *fakeReportRepo) RunStatusCountsReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	f.statusCalls++
	return []map[string]any{{"status": "success", "event_count": int64(20)}}, nil
}

func (f *fakeReportRepo) RunTopEntitiesReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	return []map[string]any{{"entity_id": "entity-1", "event_count": int64(12)}}, nil
}

func (f *fakeReportRepo) RecomputeReport(ctx context.Context, request domain.RecomputeRequest, runID string) (domain.RecomputeSummary, error) {
	return domain.RecomputeSummary{}, errors.New("not implemented")
}

type fakeCacheStore struct {
	versions map[string]int64
	data     map[string][]byte
}

func (f *fakeCacheStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, ok := f.data[key]
	return value, ok, nil
}

func (f *fakeCacheStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	blob := make([]byte, len(value))
	copy(blob, value)
	f.data[key] = blob
	return nil
}

func (f *fakeCacheStore) GetVersion(ctx context.Context, slug string) (int64, error) {
	if version, ok := f.versions[slug]; ok {
		return version, nil
	}
	f.versions[slug] = 1
	return 1, nil
}

func (f *fakeCacheStore) BumpVersion(ctx context.Context, slug string) (int64, error) {
	version := f.versions[slug] + 1
	f.versions[slug] = version
	return version, nil
}
