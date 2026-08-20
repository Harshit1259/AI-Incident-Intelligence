package services

import (
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type RunbookService struct {
	store *store.RunbookStore
}

func NewRunbookService(s *store.RunbookStore) *RunbookService {
	return &RunbookService{store: s}
}

// FindRelevant returns runbooks that match the given service, severity, and pattern.
func (s *RunbookService) FindRelevant(service, severity, pattern string) ([]models.Runbook, error) {
	runbooks, err := s.store.GetByService(service)
	if err != nil {
		return nil, err
	}

	var results []models.Runbook
	lowerPattern := strings.ToLower(pattern)

	for _, rb := range runbooks {
		// Filter by severity match (empty means all)
		if rb.SeverityMatch != "" && !strings.EqualFold(rb.SeverityMatch, severity) {
			continue
		}
		// Filter by pattern match (empty means all)
		if rb.PatternMatch != "" && !strings.Contains(lowerPattern, strings.ToLower(rb.PatternMatch)) {
			continue
		}
		results = append(results, rb)
	}

	return results, nil
}

func (s *RunbookService) Create(req models.RunbookCreateRequest, tenantID, createdBy string) (*models.Runbook, error) {
	if req.Title == "" {
		return nil, fmt.Errorf("runbook title is required")
	}

	now := time.Now()
	rb := models.Runbook{
		ID:            fmt.Sprintf("rb-%d", now.UnixNano()),
		TenantID:      tenantID,
		Service:       req.Service,
		Title:         req.Title,
		Content:       req.Content,
		Tags:          req.Tags,
		SeverityMatch: req.SeverityMatch,
		PatternMatch:  req.PatternMatch,
		CreatedBy:     createdBy,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.store.Create(rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

func (s *RunbookService) Search(tenantID, query string) ([]models.Runbook, error) {
	return s.store.Search(tenantID, query)
}

func (s *RunbookService) Update(id string, req models.RunbookCreateRequest) (*models.Runbook, error) {
	existing, err := s.store.GetByID(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("runbook not found: %s", id)
	}

	existing.Service = req.Service
	existing.Title = req.Title
	existing.Content = req.Content
	existing.Tags = req.Tags
	existing.SeverityMatch = req.SeverityMatch
	existing.PatternMatch = req.PatternMatch
	existing.UpdatedAt = time.Now()

	if err := s.store.Update(*existing); err != nil {
		return nil, err
	}
	return existing, nil
}

func (s *RunbookService) Delete(id string) error {
	return s.store.Delete(id)
}

func (s *RunbookService) GetByID(id string) (*models.Runbook, error) {
	return s.store.GetByID(id)
}
