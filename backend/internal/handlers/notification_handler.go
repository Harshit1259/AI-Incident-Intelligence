package handlers

import (
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

// NotificationHandler serves the operator notification feed.
//
// One endpoint powers both the bell indicator and the slide-over panel. Keeping
// it to a single call means the badge count and the list can never disagree,
// which a separate /count endpoint would eventually allow.
type NotificationHandler struct {
	service *services.NotificationService
}

func NewNotificationHandler(s *services.NotificationService) *NotificationHandler {
	return &NotificationHandler{service: s}
}

// Handle serves GET /api/v1/notifications.
//
// The tenant is taken from the caller's JWT, never from the request, so a
// caller can only ever see their own notifications.
func (h *NotificationHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Defensive: if the service was never wired, return an empty feed rather
	// than a 500. A missing notification feed must not look like an outage.
	if h.service == nil {
		api.WriteJSON(w, http.StatusOK, models.NotificationFeed{
			Items: []models.Notification{},
		})
		return
	}

	tenantID := middleware.TenantFromRequest(r)
	api.WriteJSON(w, http.StatusOK, h.service.Feed(tenantID))
}
