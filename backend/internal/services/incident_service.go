package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type IncidentService struct {
	incidentStore         *store.IncidentStore
	incidentDetailService *IncidentDetailService
	historyStore          *store.IncidentStatusHistoryStore
}

func NewIncidentService(
	incidentStore *store.IncidentStore,
	incidentDetailService *IncidentDetailService,
	historyStore *store.IncidentStatusHistoryStore,
) *IncidentService {
	return &IncidentService{
		incidentStore:         incidentStore,
		incidentDetailService: incidentDetailService,
		historyStore:          historyStore,
	}
}

func (incidentService *IncidentService) ListIncidents(filter models.IncidentListFilter) (models.IncidentListResponse, error) {
	normalizedFilter, err := normalizeIncidentListFilter(filter)
	if err != nil {
		return models.IncidentListResponse{}, err
	}

	return incidentService.incidentStore.ListIncidents(normalizedFilter)
}

func (incidentService *IncidentService) GetIncidentDetail(incidentID string) (models.IncidentDetail, bool) {
	return incidentService.incidentDetailService.GetIncidentDetail(strings.TrimSpace(incidentID))
}

func (incidentService *IncidentService) UpdateIncidentStatus(incidentID string, action string) (models.Incident, error) {
	normalizedIncidentID := strings.TrimSpace(incidentID)
	if normalizedIncidentID == "" {
		return models.Incident{}, fmt.Errorf("incident id is required")
	}

	nextStatus, err := mapIncidentActionToStatus(action)
	if err != nil {
		return models.Incident{}, err
	}

	currentIncident, found := incidentService.incidentStore.GetIncidentByID(normalizedIncidentID)
	if !found {
		return models.Incident{}, sql.ErrNoRows
	}

	updatedIncident, err := incidentService.incidentStore.UpdateIncidentStatus(normalizedIncidentID, nextStatus)
	if err != nil {
		return models.Incident{}, err
	}

	note := buildStatusChangeNote(action, currentIncident.Status, nextStatus)
	if incidentService.historyStore != nil {
		_ = incidentService.historyStore.AddRecord(
			normalizedIncidentID,
			currentIncident.Status,
			nextStatus,
			note,
			"operator",
		)
	}

	return updatedIncident, nil
}

func normalizeIncidentListFilter(filter models.IncidentListFilter) (models.IncidentListFilter, error) {
	filter.Status = strings.ToLower(strings.TrimSpace(filter.Status))
	filter.Severity = strings.ToLower(strings.TrimSpace(filter.Severity))
	filter.Service = strings.TrimSpace(filter.Service)
	filter.Search = strings.TrimSpace(filter.Search)
	filter.SortBy = strings.TrimSpace(filter.SortBy)
	filter.SortOrder = strings.TrimSpace(filter.SortOrder)

	if filter.Status != "" {
		switch filter.Status {
		case "open", "acknowledged", "resolved":
		default:
			return models.IncidentListFilter{}, fmt.Errorf("invalid status value")
		}
	}
	if filter.Severity != "" {
		switch filter.Severity {
		case "critical", "high", "medium", "low":
		default:
			return models.IncidentListFilter{}, fmt.Errorf("invalid severity value")
		}
	}

	validSort := map[string]bool{
		"last_event_time":  true,
		"first_event_time": true,
		"severity":         true,
		"status":           true,
		"service":          true,
		"title":            true,
		"risk_score":       true,
		"confidence":       true,
	}

	if filter.SortBy != "" && !validSort[filter.SortBy] {
		return models.IncidentListFilter{}, fmt.Errorf("invalid sort_by value")
	}

	if filter.Page <= 0 {
		filter.Page = 1
	}

	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}

	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	if filter.SortBy == "" {
		filter.SortBy = "last_event_time"
	}

	if filter.SortOrder == "" {
		filter.SortOrder = "desc"
	} else {
		normalizedSortOrder := strings.ToLower(filter.SortOrder)
		if normalizedSortOrder != "asc" && normalizedSortOrder != "desc" {
			return models.IncidentListFilter{}, fmt.Errorf("invalid sort_order value")
		}
		filter.SortOrder = normalizedSortOrder
	}

	if len(filter.Search) > 200 {
		return models.IncidentListFilter{}, fmt.Errorf("search value too long")
	}

	if len(filter.Service) > 120 {
		return models.IncidentListFilter{}, fmt.Errorf("service value too long")
	}

	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return models.IncidentListFilter{}, fmt.Errorf("from must be earlier than to")
	}

	return filter, nil
}

func ParseOptionalTime(value string) (*time.Time, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return nil, nil
	}

	layouts := []string{time.RFC3339, "2006-01-02"}
	for _, layout := range layouts {
		parsedTime, err := time.Parse(layout, trimmedValue)
		if err == nil {
			return &parsedTime, nil
		}
	}

	return nil, fmt.Errorf("invalid time value: %s", value)
}

func mapIncidentActionToStatus(action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "ack":
		return "acknowledged", nil
	case "resolve":
		return "resolved", nil
	case "reopen":
		return "open", nil
	default:
		return "", fmt.Errorf("invalid action")
	}
}

func buildStatusChangeNote(action string, previousStatus string, nextStatus string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "ack":
		return fmt.Sprintf("Incident moved from %s to %s for active triage.", previousStatus, nextStatus)
	case "resolve":
		return fmt.Sprintf("Incident moved from %s to %s after remediation.", previousStatus, nextStatus)
	case "reopen":
		return fmt.Sprintf("Incident moved from %s to %s because the issue reappeared or requires further validation.", previousStatus, nextStatus)
	default:
		return fmt.Sprintf("Incident moved from %s to %s.", previousStatus, nextStatus)
	}
}
