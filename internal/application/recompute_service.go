package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type recomputeJob struct {
	runID     string
	request   domain.RecomputeRequest
	lockKey   string
	lockToken string
}

type RecomputeService struct {
	reports      ReportRepository
	runs         RecomputeRunRepository
	audits       AuditRepository
	locks        RecomputeLock
	cache        ReportCache
	logger       *slog.Logger
	maxRangeDays int
	lockTTL      time.Duration
	queue        chan recomputeJob
}

func NewRecomputeService(
	reports ReportRepository,
	runs RecomputeRunRepository,
	audits AuditRepository,
	locks RecomputeLock,
	cache ReportCache,
	logger *slog.Logger,
	maxRangeDays int,
	lockTTL time.Duration,
	queueSize int,
) *RecomputeService {
	return &RecomputeService{
		reports:      reports,
		runs:         runs,
		audits:       audits,
		locks:        locks,
		cache:        cache,
		logger:       logger,
		maxRangeDays: maxRangeDays,
		lockTTL:      lockTTL,
		queue:        make(chan recomputeJob, queueSize),
	}
}

func (s *RecomputeService) Start(ctx context.Context, workers int) {
	if workers < 1 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		go s.worker(ctx, i+1)
	}
}

func (s *RecomputeService) Trigger(ctx context.Context, request domain.RecomputeRequest) (*domain.RecomputeRun, error) {
	if request.Window != domain.WindowDay && request.Window != domain.WindowWeek {
		return nil, domain.NewAppError(domain.ErrCodeInvalid, "window must be day or week")
	}

	if request.DateFrom.After(request.DateTo) {
		return nil, domain.NewAppError(domain.ErrCodeInvalid, "date_from must be on or before date_to")
	}

	rangeDays := int(request.DateTo.Sub(request.DateFrom).Hours()/24) + 1
	if rangeDays > s.maxRangeDays {
		return nil, domain.NewAppError(domain.ErrCodeInvalid, fmt.Sprintf("date range exceeds %d days", s.maxRangeDays))
	}

	if _, err := s.reports.GetReportDefinition(ctx, request.ReportSlug); err != nil {
		return nil, err
	}

	if request.RequestedBy == "" {
		request.RequestedBy = "management-api"
	}

	if request.RequestedVia == "" {
		request.RequestedVia = "http"
	}

	lockKey := s.lockKey(request)
	lockToken := uuid.NewString()

	acquired, err := s.locks.TryAcquire(ctx, lockKey, lockToken, s.lockTTL)
	if err != nil {
		return nil, domain.WrapAppError(domain.ErrCodeInternal, "failed to acquire recompute lock", err)
	}

	if !acquired {
		return nil, domain.NewAppError(domain.ErrCodeConflict, "a recompute run for this scope is already in progress")
	}

	run, err := s.runs.CreateRun(ctx, request, domain.RunStatusPending)
	if err != nil {
		_ = s.locks.Release(ctx, lockKey, lockToken)
		return nil, domain.WrapAppError(domain.ErrCodeInternal, "failed to create recompute run", err)
	}

	entry := domain.AuditEntry{
		Actor:        request.RequestedBy,
		Action:       "recompute.triggered",
		ResourceType: "recompute_run",
		ResourceID:   run.ID,
		Metadata: map[string]any{
			"report_slug":   request.ReportSlug,
			"window":        request.Window,
			"date_from":     request.DateFrom.Format("2006-01-02"),
			"date_to":       request.DateTo.Format("2006-01-02"),
			"requested_via": request.RequestedVia,
		},
	}
	if err := s.audits.CreateEntry(ctx, entry); err != nil {
		s.logger.Warn("failed to write audit entry", "error", err.Error(), "run_id", run.ID)
	}

	job := recomputeJob{runID: run.ID, request: request, lockKey: lockKey, lockToken: lockToken}
	select {
	case s.queue <- job:
		return run, nil
	default:
		_ = s.runs.MarkRunFailed(ctx, run.ID, "worker queue is full")
		_ = s.locks.Release(ctx, lockKey, lockToken)
		return nil, domain.NewAppError(domain.ErrCodeUnavailable, "recompute queue is full")
	}
}

func (s *RecomputeService) GetRun(ctx context.Context, runID string) (*domain.RecomputeRun, error) {
	run, err := s.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}

	return run, nil
}

func (s *RecomputeService) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.queue:
			s.processJob(ctx, workerID, job)
		}
	}
}

func (s *RecomputeService) processJob(ctx context.Context, workerID int, job recomputeJob) {
	workerCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	if err := s.runs.MarkRunRunning(workerCtx, job.runID); err != nil {
		s.logger.Error("failed to mark recompute run as running", "run_id", job.runID, "error", err.Error(), "worker", workerID)
		_ = s.locks.Release(workerCtx, job.lockKey, job.lockToken)
		return
	}

	summary, err := s.reports.RecomputeReport(workerCtx, job.request, job.runID)
	if err != nil {
		_ = s.runs.MarkRunFailed(workerCtx, job.runID, err.Error())
		_ = s.audits.CreateEntry(workerCtx, domain.AuditEntry{
			Actor:        "recompute-worker",
			Action:       "recompute.failed",
			ResourceType: "recompute_run",
			ResourceID:   job.runID,
			Metadata: map[string]any{
				"report_slug": job.request.ReportSlug,
				"window":      job.request.Window,
				"error":       err.Error(),
			},
		})
		_ = s.locks.Release(workerCtx, job.lockKey, job.lockToken)
		s.logger.Error("recompute run failed", "run_id", job.runID, "error", err.Error(), "worker", workerID)
		return
	}

	if err := s.runs.MarkRunCompleted(workerCtx, job.runID, summary); err != nil {
		s.logger.Error("failed to mark recompute run as completed", "run_id", job.runID, "error", err.Error(), "worker", workerID)
	}

	if s.cache != nil {
		if _, err := s.cache.BumpVersion(workerCtx, job.request.ReportSlug); err != nil {
			s.logger.Warn("failed to bump report cache version", "slug", job.request.ReportSlug, "error", err.Error())
		}
	}

	_ = s.audits.CreateEntry(workerCtx, domain.AuditEntry{
		Actor:        "recompute-worker",
		Action:       "recompute.completed",
		ResourceType: "recompute_run",
		ResourceID:   job.runID,
		Metadata: map[string]any{
			"report_slug":   job.request.ReportSlug,
			"window":        job.request.Window,
			"rows_deleted":  summary.RowsDeleted,
			"rows_inserted": summary.RowsInserted,
			"bucket_count":  summary.BucketCount,
		},
	})

	if err := s.locks.Release(workerCtx, job.lockKey, job.lockToken); err != nil {
		s.logger.Warn("failed to release recompute lock", "run_id", job.runID, "error", err.Error())
	}
}

func (s *RecomputeService) lockKey(request domain.RecomputeRequest) string {
	return fmt.Sprintf("recompute:lock:%s:%s:%s:%s", request.ReportSlug, request.Window, request.DateFrom.Format("20060102"), request.DateTo.Format("20060102"))
}
