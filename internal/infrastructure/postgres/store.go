package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) PingDB(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) ListReportDefinitions(ctx context.Context, filter domain.ReportListFilter) ([]domain.ReportDefinition, int, error) {
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	sortField := "name"
	switch filter.Sort {
	case "slug", "name", "updated_at", "cache_ttl_seconds":
		sortField = filter.Sort
	}

	order := "ASC"
	if strings.EqualFold(filter.Order, "desc") {
		order = "DESC"
	}

	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM report_definitions rd
		WHERE rd.is_public = true
		AND ($1 = '' OR rd.slug ILIKE '%' || $1 || '%' OR rd.name ILIKE '%' || $1 || '%')
	`, filter.Search).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf(`
		SELECT
			rd.slug,
			rd.name,
			rd.description,
			rd.cache_ttl_seconds,
			rd.default_window,
			rd.supported_filters,
			COALESCE(
				(
					SELECT ARRAY_AGG(aw.window_name ORDER BY CASE aw.window_name WHEN 'day' THEN 1 ELSE 2 END)
					FROM aggregate_windows aw
					WHERE aw.report_slug = rd.slug
				),
				ARRAY[]::text[]
			) AS allowed_windows
		FROM report_definitions rd
		WHERE rd.is_public = true
		AND ($1 = '' OR rd.slug ILIKE '%%' || $1 || '%%' OR rd.name ILIKE '%%' || $1 || '%%')
		ORDER BY %s %s
		LIMIT $2 OFFSET $3
	`, sortField, order)

	rows, err := s.pool.Query(ctx, query, filter.Search, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	reports := make([]domain.ReportDefinition, 0, filter.Limit)
	for rows.Next() {
		report, err := scanReportDefinition(rows)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, report)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

func (s *Store) GetReportDefinition(ctx context.Context, slug string) (*domain.ReportDefinition, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT
			rd.slug,
			rd.name,
			rd.description,
			rd.cache_ttl_seconds,
			rd.default_window,
			rd.supported_filters,
			COALESCE(
				(
					SELECT ARRAY_AGG(aw.window_name ORDER BY CASE aw.window_name WHEN 'day' THEN 1 ELSE 2 END)
					FROM aggregate_windows aw
					WHERE aw.report_slug = rd.slug
				),
				ARRAY[]::text[]
			) AS allowed_windows
		FROM report_definitions rd
		WHERE rd.slug = $1
		AND rd.is_public = true
	`, slug)

	report, err := scanReportDefinition(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.NewAppError(domain.ErrCodeNotFound, "report was not found")
		}
		return nil, err
	}

	return &report, nil
}

func (s *Store) RunVolumeReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	if strings.EqualFold(params.Breakdown, "source") {
		rows, err := s.pool.Query(ctx, `
			SELECT
				bucket_start::date AS bucket_date,
				dimension_value AS source,
				SUM(metric_value)::bigint AS event_count
			FROM metric_snapshots
			WHERE report_slug = 'volume-by-period'
			AND window_name = $1
			AND metric_name = 'event_count'
			AND dimension_key = 'source'
			AND bucket_start >= $2::date
			AND bucket_start < ($3::date + INTERVAL '1 day')
			AND ($4 = '' OR dimension_value = $4)
			GROUP BY bucket_date, source
			ORDER BY bucket_date ASC, source ASC
			LIMIT $5 OFFSET $6
		`, params.Window, params.DateFrom, params.DateTo, params.Source, params.Limit, params.Offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return collectRows(rows, []string{"bucket_date", "source", "event_count"})
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			bucket_start::date AS bucket_date,
			SUM(metric_value)::bigint AS event_count
		FROM metric_snapshots
		WHERE report_slug = 'volume-by-period'
		AND window_name = $1
		AND metric_name = 'event_count'
		AND dimension_key = 'source'
		AND bucket_start >= $2::date
		AND bucket_start < ($3::date + INTERVAL '1 day')
		AND ($4 = '' OR dimension_value = $4)
		GROUP BY bucket_date
		ORDER BY bucket_date ASC
		LIMIT $5 OFFSET $6
	`, params.Window, params.DateFrom, params.DateTo, params.Source, params.Limit, params.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, []string{"bucket_date", "event_count"})
}

func (s *Store) RunStatusCountsReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	if strings.EqualFold(params.Breakdown, "period") {
		rows, err := s.pool.Query(ctx, `
			SELECT
				bucket_start::date AS bucket_date,
				dimension_value AS status,
				SUM(metric_value)::bigint AS event_count
			FROM metric_snapshots
			WHERE report_slug = 'status-counts'
			AND window_name = $1
			AND metric_name = 'event_count'
			AND dimension_key = 'status'
			AND bucket_start >= $2::date
			AND bucket_start < ($3::date + INTERVAL '1 day')
			AND ($4 = '' OR dimension_value = $4)
			GROUP BY bucket_date, status
			ORDER BY bucket_date ASC, status ASC
			LIMIT $5 OFFSET $6
		`, params.Window, params.DateFrom, params.DateTo, params.Status, params.Limit, params.Offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return collectRows(rows, []string{"bucket_date", "status", "event_count"})
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			dimension_value AS status,
			SUM(metric_value)::bigint AS event_count
		FROM metric_snapshots
		WHERE report_slug = 'status-counts'
		AND window_name = $1
		AND metric_name = 'event_count'
		AND dimension_key = 'status'
		AND bucket_start >= $2::date
		AND bucket_start < ($3::date + INTERVAL '1 day')
		AND ($4 = '' OR dimension_value = $4)
		GROUP BY status
		ORDER BY event_count DESC, status ASC
		LIMIT $5 OFFSET $6
	`, params.Window, params.DateFrom, params.DateTo, params.Status, params.Limit, params.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, []string{"status", "event_count"})
}

func (s *Store) RunTopEntitiesReport(ctx context.Context, params domain.ReportRunParams) ([]map[string]any, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			dimension_value AS entity_id,
			SUM(metric_value)::bigint AS event_count
		FROM metric_snapshots
		WHERE report_slug = 'top-entities'
		AND window_name = $1
		AND metric_name = 'event_count'
		AND dimension_key = 'entity_id'
		AND bucket_start >= $2::date
		AND bucket_start < ($3::date + INTERVAL '1 day')
		GROUP BY entity_id
		ORDER BY event_count DESC, entity_id ASC
		LIMIT $4 OFFSET $5
	`, params.Window, params.DateFrom, params.DateTo, params.Limit, params.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRows(rows, []string{"entity_id", "event_count"})
}

func (s *Store) RecomputeReport(ctx context.Context, request domain.RecomputeRequest, runID string) (domain.RecomputeSummary, error) {
	fromTS := time.Date(request.DateFrom.Year(), request.DateFrom.Month(), request.DateFrom.Day(), 0, 0, 0, 0, time.UTC)
	toExclusive := time.Date(request.DateTo.Year(), request.DateTo.Month(), request.DateTo.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.RecomputeSummary{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	deleteCmd, err := tx.Exec(ctx, `
		DELETE FROM metric_snapshots
		WHERE report_slug = $1
		AND window_name = $2
		AND bucket_start >= date_trunc($2, $3::timestamptz)
		AND bucket_start < date_trunc($2, $4::timestamptz) + CASE WHEN $2 = 'week' THEN INTERVAL '1 week' ELSE INTERVAL '1 day' END
	`, request.ReportSlug, request.Window, fromTS, toExclusive)
	if err != nil {
		return domain.RecomputeSummary{}, err
	}

	var insertCmd pgconnCommandTag
	switch request.ReportSlug {
	case "volume-by-period":
		insertCmd, err = execInsert(ctx, tx, `
			INSERT INTO metric_snapshots (
				report_slug,
				window_name,
				bucket_start,
				dimension_key,
				dimension_value,
				metric_name,
				metric_value,
				computed_at,
				run_id
			)
			SELECT
				'volume-by-period',
				$1,
				date_trunc($1, e.occurred_at),
				'source',
				s.slug,
				'event_count',
				COUNT(*)::bigint,
				now(),
				$4::uuid
			FROM analytics_events e
			JOIN event_sources s ON s.id = e.source_id
			WHERE e.occurred_at >= $2
			AND e.occurred_at < $3
			GROUP BY date_trunc($1, e.occurred_at), s.slug
		`, request.Window, fromTS, toExclusive, runID)
	case "status-counts":
		insertCmd, err = execInsert(ctx, tx, `
			INSERT INTO metric_snapshots (
				report_slug,
				window_name,
				bucket_start,
				dimension_key,
				dimension_value,
				metric_name,
				metric_value,
				computed_at,
				run_id
			)
			SELECT
				'status-counts',
				$1,
				date_trunc($1, e.occurred_at),
				'status',
				e.status,
				'event_count',
				COUNT(*)::bigint,
				now(),
				$4::uuid
			FROM analytics_events e
			WHERE e.occurred_at >= $2
			AND e.occurred_at < $3
			GROUP BY date_trunc($1, e.occurred_at), e.status
		`, request.Window, fromTS, toExclusive, runID)
	case "top-entities":
		insertCmd, err = execInsert(ctx, tx, `
			INSERT INTO metric_snapshots (
				report_slug,
				window_name,
				bucket_start,
				dimension_key,
				dimension_value,
				metric_name,
				metric_value,
				computed_at,
				run_id
			)
			SELECT
				'top-entities',
				$1,
				date_trunc($1, e.occurred_at),
				'entity_id',
				e.entity_id,
				'event_count',
				COUNT(*)::bigint,
				now(),
				$4::uuid
			FROM analytics_events e
			WHERE e.occurred_at >= $2
			AND e.occurred_at < $3
			GROUP BY date_trunc($1, e.occurred_at), e.entity_id
		`, request.Window, fromTS, toExclusive, runID)
	default:
		return domain.RecomputeSummary{}, domain.NewAppError(domain.ErrCodeInvalid, "unsupported report slug for recompute")
	}
	if err != nil {
		return domain.RecomputeSummary{}, err
	}

	var buckets int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(DISTINCT bucket_start)
		FROM metric_snapshots
		WHERE report_slug = $1
		AND window_name = $2
		AND bucket_start >= date_trunc($2, $3::timestamptz)
		AND bucket_start < date_trunc($2, $4::timestamptz) + CASE WHEN $2 = 'week' THEN INTERVAL '1 week' ELSE INTERVAL '1 day' END
	`, request.ReportSlug, request.Window, fromTS, toExclusive).Scan(&buckets); err != nil {
		return domain.RecomputeSummary{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.RecomputeSummary{}, err
	}

	return domain.RecomputeSummary{
		RowsDeleted:  deleteCmd.RowsAffected(),
		RowsInserted: insertCmd.RowsAffected(),
		BucketCount:  buckets,
	}, nil
}

func (s *Store) CreateRun(ctx context.Context, request domain.RecomputeRequest, status string) (*domain.RecomputeRun, error) {
	run := &domain.RecomputeRun{}
	var dateFrom time.Time
	var dateTo time.Time
	var summaryRaw []byte
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO recompute_runs (
			report_slug,
			window_name,
			date_from,
			date_to,
			requested_by,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, report_slug, window_name, date_from, date_to, requested_by, status, requested_at, summary
	`, request.ReportSlug, request.Window, request.DateFrom, request.DateTo, request.RequestedBy, status).Scan(
		&run.ID,
		&run.ReportSlug,
		&run.Window,
		&dateFrom,
		&dateTo,
		&run.RequestedBy,
		&run.Status,
		&run.RequestedAt,
		&summaryRaw,
	); err != nil {
		return nil, err
	}

	run.DateFrom = dateFrom.Format("2006-01-02")
	run.DateTo = dateTo.Format("2006-01-02")
	if len(summaryRaw) > 0 {
		if err := json.Unmarshal(summaryRaw, &run.Summary); err != nil {
			return nil, err
		}
	}
	if run.Summary == nil {
		run.Summary = map[string]any{}
	}

	return run, nil
}

func (s *Store) GetRun(ctx context.Context, runID string) (*domain.RecomputeRun, error) {
	run := &domain.RecomputeRun{}
	var dateFrom time.Time
	var dateTo time.Time
	var summaryRaw []byte
	var errorMessage sql.NullString
	if err := s.pool.QueryRow(ctx, `
		SELECT
			id,
			report_slug,
			window_name,
			date_from,
			date_to,
			requested_by,
			status,
			requested_at,
			started_at,
			finished_at,
			error_message,
			summary
		FROM recompute_runs
		WHERE id = $1
	`, runID).Scan(
		&run.ID,
		&run.ReportSlug,
		&run.Window,
		&dateFrom,
		&dateTo,
		&run.RequestedBy,
		&run.Status,
		&run.RequestedAt,
		&run.StartedAt,
		&run.FinishedAt,
		&errorMessage,
		&summaryRaw,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, domain.NewAppError(domain.ErrCodeNotFound, "recompute run was not found")
		}
		return nil, err
	}

	run.DateFrom = dateFrom.Format("2006-01-02")
	run.DateTo = dateTo.Format("2006-01-02")
	if len(summaryRaw) > 0 {
		if err := json.Unmarshal(summaryRaw, &run.Summary); err != nil {
			return nil, err
		}
	}
	if run.Summary == nil {
		run.Summary = map[string]any{}
	}
	if errorMessage.Valid {
		run.ErrorMessage = errorMessage.String
	}

	return run, nil
}

func (s *Store) MarkRunRunning(ctx context.Context, runID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE recompute_runs
		SET status = 'running', started_at = now()
		WHERE id = $1
	`, runID)
	return err
}

func (s *Store) MarkRunCompleted(ctx context.Context, runID string, summary domain.RecomputeSummary) error {
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		UPDATE recompute_runs
		SET status = 'completed', finished_at = now(), error_message = NULL, summary = $2::jsonb
		WHERE id = $1
	`, runID, summaryJSON)
	return err
}

func (s *Store) MarkRunFailed(ctx context.Context, runID, message string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE recompute_runs
		SET status = 'failed', finished_at = now(), error_message = $2
		WHERE id = $1
	`, runID, message)
	return err
}

func (s *Store) CreateEntry(ctx context.Context, entry domain.AuditEntry) error {
	metadataJSON, err := json.Marshal(entry.Metadata)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_entries (actor, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, $3, $4, $5::jsonb)
	`, entry.Actor, entry.Action, entry.ResourceType, entry.ResourceID, metadataJSON)
	return err
}

func (s *Store) ListEntries(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, int, error) {
	var total int
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM audit_entries
		WHERE ($1 = '' OR action = $1)
		AND ($2 = '' OR actor = $2)
	`, filter.Action, filter.Actor).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, actor, action, resource_type, resource_id, metadata, created_at
		FROM audit_entries
		WHERE ($1 = '' OR action = $1)
		AND ($2 = '' OR actor = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, filter.Action, filter.Actor, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries := make([]domain.AuditEntry, 0, filter.Limit)
	for rows.Next() {
		var entry domain.AuditEntry
		var metadataRaw []byte
		if err := rows.Scan(&entry.ID, &entry.Actor, &entry.Action, &entry.ResourceType, &entry.ResourceID, &metadataRaw, &entry.CreatedAt); err != nil {
			return nil, 0, err
		}
		if len(metadataRaw) > 0 {
			if err := json.Unmarshal(metadataRaw, &entry.Metadata); err != nil {
				return nil, 0, err
			}
		}
		if entry.Metadata == nil {
			entry.Metadata = map[string]any{}
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

func scanReportDefinition(row pgx.Row) (domain.ReportDefinition, error) {
	report := domain.ReportDefinition{}
	var filtersRaw []byte
	if err := row.Scan(
		&report.Slug,
		&report.Name,
		&report.Description,
		&report.CacheTTLSeconds,
		&report.DefaultWindow,
		&filtersRaw,
		&report.AllowedWindows,
	); err != nil {
		return domain.ReportDefinition{}, err
	}

	if len(filtersRaw) > 0 {
		if err := json.Unmarshal(filtersRaw, &report.SupportedFilters); err != nil {
			return domain.ReportDefinition{}, err
		}
	} else {
		report.SupportedFilters = map[string]string{}
	}

	if report.SupportedFilters == nil {
		report.SupportedFilters = map[string]string{}
	}

	return report, nil
}

func collectRows(rows pgx.Rows, columns []string) ([]map[string]any, error) {
	result := make([]map[string]any, 0)
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeValue(values[i])
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func normalizeValue(value any) any {
	switch v := value.(type) {
	case time.Time:
		return v.Format("2006-01-02")
	default:
		return v
	}
}

type pgconnCommandTag interface {
	RowsAffected() int64
}

func execInsert(ctx context.Context, tx pgx.Tx, query string, args ...any) (pgconnCommandTag, error) {
	return tx.Exec(ctx, query, args...)
}
