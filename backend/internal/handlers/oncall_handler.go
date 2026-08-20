package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type OnCallHandler struct {
	oncallService *services.OnCallService
	oncallStore   *store.OnCallStore
}

func NewOnCallHandler(os *services.OnCallService, st *store.OnCallStore) *OnCallHandler {
	return &OnCallHandler{oncallService: os, oncallStore: st}
}

// HandleOnCall dispatches GET/POST on /api/v1/oncall
func (h *OnCallHandler) HandleOnCall(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listSchedules(w, r)
	case http.MethodPost:
		h.createSchedule(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleOnCallByID dispatches GET/DELETE on /api/v1/oncall/{id} and POST on /api/v1/oncall/{id}/override
func (h *OnCallHandler) HandleOnCallByID(w http.ResponseWriter, r *http.Request) {
	id := extractOnCallID(r.URL.Path)
	if id == "" {
		api.WriteError(w, http.StatusBadRequest, "schedule id is required")
		return
	}

	// Check for override route
	if strings.HasSuffix(r.URL.Path, "/override") {
		if r.Method == http.MethodPost {
			h.addOverride(w, r, id)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getSchedule(w, r, id)
	case http.MethodDelete:
		h.deleteSchedule(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *OnCallHandler) listSchedules(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.TenantFromRequest(r)

	schedules, err := h.oncallStore.GetSchedules(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to fetch schedules")
		return
	}

	if schedules == nil {
		schedules = []models.OnCallSchedule{}
	}

	api.WriteJSON(w, http.StatusOK, schedules)
}

func (h *OnCallHandler) createSchedule(w http.ResponseWriter, r *http.Request) {
	var req models.OnCallCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.TeamName == "" {
		api.WriteError(w, http.StatusBadRequest, "team_name is required")
		return
	}

	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	if req.RotationType == "" {
		req.RotationType = "weekly"
	}

	now := time.Now()
	sched := models.OnCallSchedule{
		ID:           fmt.Sprintf("oncall-%d", now.UnixNano()),
		TenantID:     middleware.TenantFromRequest(r),
		TeamName:     req.TeamName,
		Timezone:     req.Timezone,
		RotationType: req.RotationType,
		CreatedAt:    now,
	}

	for i, m := range req.Members {
		sched.Members = append(sched.Members, models.OnCallMember{
			ScheduleID: sched.ID,
			UserName:   m.UserName,
			UserEmail:  m.UserEmail,
			Position:   i,
		})
	}

	if err := h.oncallStore.CreateSchedule(sched); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}

	api.WriteJSON(w, http.StatusCreated, sched)
}

func (h *OnCallHandler) getSchedule(w http.ResponseWriter, r *http.Request, id string) {
	current, err := h.oncallService.GetCurrentOnCall(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.WriteError(w, http.StatusNotFound, "schedule not found")
			return
		}
		api.WriteError(w, http.StatusInternalServerError, "failed to get on-call info")
		return
	}

	api.WriteJSON(w, http.StatusOK, current)
}

func (h *OnCallHandler) deleteSchedule(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.oncallStore.DeleteSchedule(id); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *OnCallHandler) addOverride(w http.ResponseWriter, r *http.Request, scheduleID string) {
	var req models.OnCallOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.OverrideUser == "" {
		api.WriteError(w, http.StatusBadRequest, "override_user is required")
		return
	}

	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid start_time format (use RFC3339)")
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid end_time format (use RFC3339)")
		return
	}

	override := models.OnCallOverride{
		ScheduleID:   scheduleID,
		OverrideUser: req.OverrideUser,
		StartTime:    startTime,
		EndTime:      endTime,
		Reason:       req.Reason,
	}

	if err := h.oncallStore.AddOverride(override); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to add override")
		return
	}

	api.WriteJSON(w, http.StatusCreated, override)
}

func extractOnCallID(path string) string {
	trimmed := strings.Trim(path, "/")
	parts := strings.Split(trimmed, "/")
	// api/v1/oncall/{id} or api/v1/oncall/{id}/override
	if len(parts) >= 4 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "oncall" {
		return parts[3]
	}
	return ""
}
