package handlers

import (
	"encoding/json"
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

type WhatsAppHandler struct {
	svc           *services.WhatsAppService
	incidentStore *store.IncidentStore
}

func NewWhatsAppHandler(svc *services.WhatsAppService, is *store.IncidentStore) *WhatsAppHandler {
	return &WhatsAppHandler{svc: svc, incidentStore: is}
}

// HandleGetConfig handles GET /api/v1/whatsapp/config
func (h *WhatsAppHandler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	config, err := h.svc.GetConfig(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if config == nil {
		api.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
			"message": "no WhatsApp config found",
		})
		return
	}

	// Mask sensitive fields
	config.AccessToken = ""
	config.VerifyToken = ""
	api.WriteJSON(w, http.StatusOK, config)
}

// HandleSaveConfig handles PUT /api/v1/whatsapp/config
func (h *WhatsAppHandler) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var config models.WhatsAppConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	config.TenantID = middleware.TenantFromRequest(r)

	if err := h.svc.SaveConfig(config); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// HandleTest handles POST /api/v1/whatsapp/test
func (h *WhatsAppHandler) HandleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	// Create a test incident
	testIncident := models.Incident{
		ID:               "test-incident",
		Service:          "test-service",
		Severity:         "critical",
		Status:           "open",
		Title:            "Test WhatsApp Alert",
		RootCauseSummary: "This is a test alert to verify WhatsApp integration",
	}

	if err := h.svc.SendIncidentAlert(tenantID, testIncident); err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "test sent"})
}
