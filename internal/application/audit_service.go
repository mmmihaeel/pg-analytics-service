package application

import (
	"context"

	"github.com/pg-analytics-service/pg-analytics-service/internal/domain"
)

type AuditService struct {
	repo AuditRepository
}

func NewAuditService(repo AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) ListEntries(ctx context.Context, filter domain.AuditFilter) ([]domain.AuditEntry, domain.Pagination, error) {
	if filter.Limit < 1 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	entries, total, err := s.repo.ListEntries(ctx, filter)
	if err != nil {
		return nil, domain.Pagination{}, domain.WrapAppError(domain.ErrCodeInternal, "failed to list audit entries", err)
	}

	return entries, domain.Pagination{Limit: filter.Limit, Offset: filter.Offset, Total: total}, nil
}
