package handlers

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type ActivityHandler struct {
	incidentService  *services.IncidentService
	actionAuditStore *store.ActionAuditStore
}

func NewActivityHandler(
	incidentService *services.IncidentService,
	actionAuditStore *store.ActionAuditStore,
) *ActivityHandler {
	return &ActivityHandler{
		incidentService:  incidentService,
		actionAuditStore: actionAuditStore,
	}
}

func formatActivityTimestamp(value interface{}) string {
	switch timestamp := value.(type) {
	case time.Time:
		if timestamp.IsZero() {
			return ""
		}
		return timestamp.UTC().Format(time.RFC3339)
	case string:
		trimmed := strings.TrimSpace(timestamp)
		if trimmed == "" {
			return ""
		}

		parsedFormats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02 15:04:05",
			time.DateTime,
		}

		for _, layout := range parsedFormats {
			if parsedTime, err := time.Parse(layout, trimmed); err == nil {
				return parsedTime.UTC().Format(time.RFC3339)
			}
		}

		return trimmed
	default:
		return ""
	}
}

func (handler *ActivityHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	const prefix = "/api/v1/incidents/activity/"
	incidentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, prefix))

	if incidentID == "" || incidentID == r.URL.Path || strings.Contains(incidentID, "/") {
		api.WriteError(w, http.StatusBadRequest, "invalid incident id")
		return
	}

	detail, found := handler.incidentService.GetIncidentDetail(incidentID)
	if !found {
		api.WriteError(w, http.StatusNotFound, "incident not found")
		return
	}

	items := make([]models.ActivityItem, 0)

	for _, timelineEvent := range detail.Events {
		items = append(items, models.ActivityItem{
			Type:        "event",
			Title:       timelineEvent.StoryLabel,
			Description: timelineEvent.Event.Message,
			Timestamp:   formatActivityTimestamp(timelineEvent.Event.Timestamp),
			Severity:    timelineEvent.Event.Severity,
			Status:      "",
		})
	}

	for _, statusAudit := range detail.StatusAudit {
		items = append(items, models.ActivityItem{
			Type:        "status_change",
			Title:       statusAudit.PreviousStatus + " → " + statusAudit.NewStatus,
			Description: statusAudit.Note,
			Timestamp:   formatActivityTimestamp(statusAudit.ChangedAt),
			Severity:    "",
			Status:      statusAudit.NewStatus,
		})
	}

	if detail.WhatChanged.Type != "" {
		items = append(items, models.ActivityItem{
			Type:        "change",
			Title:       "Recent " + detail.WhatChanged.Type,
			Description: detail.WhatChanged.Description,
			Timestamp:   formatActivityTimestamp(detail.WhatChanged.Timestamp),
			Severity:    "",
			Status:      "",
		})
	}

	if handler.actionAuditStore != nil {
		actionAudits := handler.actionAuditStore.GetAuditsByIncident(incidentID)

		for _, audit := range actionAudits {
			items = append(items, models.ActivityItem{
				Type:        "action",
				Title:       audit.ActionID,
				Description: audit.Message,
				Timestamp:   formatActivityTimestamp(audit.ExecutedAt),
				Severity:    "",
				Status:      audit.Status,
			})
		}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Timestamp > items[j].Timestamp
	})

	api.WriteJSON(w, http.StatusOK, items)
}