package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type ReportService struct {
	repo         ReportRepository
	cache        ReportCache
	logger       *slog.Logger
	maxRangeDays int
}

func NewReportService(repo ReportRepository, cache ReportCache, logger *slog.Logger, maxRangeDays int) *ReportService {
	return &ReportService{
		repo:         repo,
		cache:        cache,
		logger:       logger,
		maxRangeDays: maxRangeDays,
	}
}

func (s *ReportService) ListReports(ctx context.Context, filter domain.ReportListFilter) ([]domain.ReportDefinition, domain.Pagination, error) {
	reports, total, err := s.repo.ListReportDefinitions(ctx, filter)
	if err != nil {
		return nil, domain.Pagination{}, domain.WrapAppError(domain.ErrCodeInternal, "failed to list reports", err)
	}

	return reports, domain.Pagination{Limit: filter.Limit, Offset: filter.Offset, Total: total}, nil
}

func (s *ReportService) GetReport(ctx context.Context, slug string) (*domain.ReportDefinition, error) {
	report, err := s.repo.GetReportDefinition(ctx, slug)
	if err != nil {
		return nil, err
	}

	return report, nil
}

func (s *ReportService) RunReport(ctx context.Context, slug string, params domain.ReportRunParams) (*domain.ReportResult, error) {
	report, err := s.repo.GetReportDefinition(ctx, slug)
	if err != nil {
		return nil, err
	}

	if err := s.validateRunParams(params); err != nil {
		return nil, err
	}

	cacheTTL := time.Duration(report.CacheTTLSeconds) * time.Second
	if cacheTTL > 0 {
		result, cacheHit, cacheErr := s.getCachedResult(ctx, slug, params)
		if cacheErr != nil {
			s.logger.Warn("report cache read failed", "slug", slug, "error", cacheErr.Error())
		} else if cacheHit {
			result.CacheHit = true
			result.SourceSystem = "redis"
			return result, nil
		}
	}

	started := time.Now()
	rows, err := s.executeReportQuery(ctx, slug, params)
	if err != nil {
		return nil, err
	}

	result := &domain.ReportResult{
		ReportSlug:   slug,
		Window:       params.Window,
		DateFrom:     params.DateFrom.Format("2006-01-02"),
		DateTo:       params.DateTo.Format("2006-01-02"),
		GeneratedAt:  time.Now().UTC(),
		Rows:         rows,
		CacheHit:     false,
		RowCount:     len(rows),
		ExecutionMS:  time.Since(started).Milliseconds(),
		SourceSystem: "postgres",
	}

	if cacheTTL > 0 {
		if cacheErr := s.storeCachedResult(ctx, slug, params, result, cacheTTL); cacheErr != nil {
			s.logger.Warn("report cache write failed", "slug", slug, "error", cacheErr.Error())
		}
	}

	return result, nil
}

func (s *ReportService) executeReportQuery(ctx context.Context, slug string, params domain.ReportRunParams) ([]map[string]any, error) {
	switch slug {
	case "volume-by-period":
		rows, err := s.repo.RunVolumeReport(ctx, params)
		if err != nil {
			return nil, domain.WrapAppError(domain.ErrCodeInternal, "failed to run volume report", err)
		}
		return rows, nil
	case "status-counts":
		rows, err := s.repo.RunStatusCountsReport(ctx, params)
		if err != nil {
			return nil, domain.WrapAppError(domain.ErrCodeInternal, "failed to run status report", err)
		}
		return rows, nil
	case "top-entities":
		rows, err := s.repo.RunTopEntitiesReport(ctx, params)
		if err != nil {
			return nil, domain.WrapAppError(domain.ErrCodeInternal, "failed to run top entities report", err)
		}
		return rows, nil
	default:
		return nil, domain.NewAppError(domain.ErrCodeNotFound, "report was not found")
	}
}

func (s *ReportService) validateRunParams(params domain.ReportRunParams) error {
	if params.Window != domain.WindowDay && params.Window != domain.WindowWeek {
		return domain.NewAppError(domain.ErrCodeInvalid, "window must be either day or week")
	}

	if params.DateFrom.After(params.DateTo) {
		return domain.NewAppError(domain.ErrCodeInvalid, "date_from must be on or before date_to")
	}

	rangeDays := int(params.DateTo.Sub(params.DateFrom).Hours()/24) + 1
	if rangeDays > s.maxRangeDays {
		return domain.NewAppError(domain.ErrCodeInvalid, fmt.Sprintf("date range exceeds %d days", s.maxRangeDays))
	}

	if params.Breakdown != "" && params.Breakdown != "source" && params.Breakdown != "status" && params.Breakdown != "period" {
		return domain.NewAppError(domain.ErrCodeInvalid, "breakdown must be one of source, status, period")
	}

	if params.Limit < 1 || params.Limit > 100 {
		return domain.NewAppError(domain.ErrCodeInvalid, "limit must be between 1 and 100")
	}

	if params.Offset < 0 {
		return domain.NewAppError(domain.ErrCodeInvalid, "offset must be non-negative")
	}

	return nil
}

func (s *ReportService) getCachedResult(ctx context.Context, slug string, params domain.ReportRunParams) (*domain.ReportResult, bool, error) {
	if s.cache == nil {
		return nil, false, nil
	}

	key, err := s.cacheKey(ctx, slug, params)
	if err != nil {
		return nil, false, err
	}

	payload, found, err := s.cache.Get(ctx, key)
	if err != nil || !found {
		return nil, found, err
	}

	var result domain.ReportResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, false, err
	}

	return &result, true, nil
}

func (s *ReportService) storeCachedResult(ctx context.Context, slug string, params domain.ReportRunParams, result *domain.ReportResult, ttl time.Duration) error {
	if s.cache == nil {
		return nil
	}

	key, err := s.cacheKey(ctx, slug, params)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return s.cache.Set(ctx, key, payload, ttl)
}

func (s *ReportService) cacheKey(ctx context.Context, slug string, params domain.ReportRunParams) (string, error) {
	if s.cache == nil {
		return "", nil
	}

	version, err := s.cache.GetVersion(ctx, slug)
	if err != nil {
		return "", err
	}

	normalized := map[string]any{
		"window":    params.Window,
		"date_from": params.DateFrom.Format("2006-01-02"),
		"date_to":   params.DateTo.Format("2006-01-02"),
		"breakdown": params.Breakdown,
		"limit":     params.Limit,
		"offset":    params.Offset,
		"source":    params.Source,
		"status":    params.Status,
	}

	blob, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(blob)
	return fmt.Sprintf("report:%s:v%d:%s", slug, version, hex.EncodeToString(sum[:])), nil
}
