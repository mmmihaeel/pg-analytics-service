package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type Seeder struct {
	store  *Store
	logger *slog.Logger
}

func NewSeeder(store *Store, logger *slog.Logger) *Seeder {
	return &Seeder{store: store, logger: logger}
}

func (s *Seeder) Seed(ctx context.Context, force bool) error {
	if force {
		if _, err := s.store.pool.Exec(ctx, `
			TRUNCATE TABLE
				metric_snapshots,
				recompute_runs,
				audit_entries,
				analytics_events
			RESTART IDENTITY CASCADE
		`); err != nil {
			return fmt.Errorf("failed to reset demo tables: %w", err)
		}
	}

	var eventsCount int64
	if err := s.store.pool.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_events`).Scan(&eventsCount); err != nil {
		return fmt.Errorf("failed to check analytics_events count: %w", err)
	}
	if eventsCount > 0 {
		s.logger.Info("seed skipped because analytics_events already contains data", "count", eventsCount)
		return nil
	}

	if _, err := s.store.pool.Exec(ctx, `
		WITH source_map AS (
			SELECT
				MAX(id) FILTER (WHERE slug = 'stripe') AS stripe_id,
				MAX(id) FILTER (WHERE slug = 'adyen') AS adyen_id,
				MAX(id) FILTER (WHERE slug = 'paypal') AS paypal_id
			FROM event_sources
		)
		INSERT INTO analytics_events (
			source_id,
			external_ref,
			entity_id,
			status,
			processing_ms,
			amount_cents,
			occurred_at,
			payload
		)
		SELECT
			CASE gs % 3
				WHEN 0 THEN source_map.stripe_id
				WHEN 1 THEN source_map.adyen_id
				ELSE source_map.paypal_id
			END,
			'evt-' || gs::text,
			'entity-' || ((gs % 240) + 1)::text,
			CASE
				WHEN gs % 17 = 0 THEN 'failed'
				WHEN gs % 11 = 0 THEN 'pending'
				ELSE 'success'
			END,
			80 + (gs % 700),
			100 + ((gs * 37) % 100000),
			date_trunc('hour', now() - ((gs % 90) || ' days')::interval - ((gs % 24) || ' hours')::interval),
			jsonb_build_object(
				'region', (ARRAY['us-east', 'eu-west', 'ap-south'])[1 + (gs % 3)],
				'batch', gs / 100,
				'provider_rank', gs % 5
			)
		FROM generate_series(1, 25000) AS gs
		CROSS JOIN source_map
	`); err != nil {
		return fmt.Errorf("failed to insert demo events: %w", err)
	}

	rangeFrom := time.Now().UTC().AddDate(0, 0, -90)
	rangeTo := time.Now().UTC()
	for _, slug := range []string{"volume-by-period", "status-counts", "top-entities"} {
		for _, window := range []string{domain.WindowDay, domain.WindowWeek} {
			if err := s.seedSnapshots(ctx, slug, window, rangeFrom, rangeTo); err != nil {
				return err
			}
		}
	}

	s.logger.Info("seed completed", "events_inserted", 25000)
	return nil
}

func (s *Seeder) seedSnapshots(ctx context.Context, slug, window string, from, to time.Time) error {
	request := domain.RecomputeRequest{
		ReportSlug:  slug,
		Window:      window,
		DateFrom:    from,
		DateTo:      to,
		RequestedBy: "seed",
	}

	run, err := s.store.CreateRun(ctx, request, domain.RunStatusRunning)
	if err != nil {
		return fmt.Errorf("failed to create seed run for %s/%s: %w", slug, window, err)
	}

	summary, err := s.store.RecomputeReport(ctx, request, run.ID)
	if err != nil {
		_ = s.store.MarkRunFailed(ctx, run.ID, err.Error())
		return fmt.Errorf("failed to recompute seed snapshots for %s/%s: %w", slug, window, err)
	}

	if err := s.store.MarkRunCompleted(ctx, run.ID, summary); err != nil {
		return fmt.Errorf("failed to finalize seed run for %s/%s: %w", slug, window, err)
	}

	return nil
}
