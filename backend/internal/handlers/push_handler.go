package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// PushHandler manages device registration for push notifications.
//
// Routes (all require JWT auth):
//
//	POST   /api/v1/push/register              — register a device token
//	DELETE /api/v1/push/register/{sub_id}     — unregister a device
//	GET    /api/v1/push/register              — list current user's devices
type PushHandler struct {
	store *store.PushStore
}

func NewPushHandler(ps *store.PushStore) *PushHandler {
	return &PushHandler{store: ps}
}

func (h *PushHandler) Handle(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sub := strings.TrimPrefix(r.URL.Path, "/api/v1/push/register")
	sub = strings.TrimSuffix(sub, "/")

	if sub == "" {
		switch r.Method {
		case http.MethodPost:
			h.register(w, r, claims)
		case http.MethodGet:
			h.list(w, claims)
		default:
			api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}

	// /api/v1/push/register/{id}
	subID := strings.TrimPrefix(sub, "/")
	if subID == "" {
		api.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodDelete {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.unregister(w, claims, subID)
}

func (h *PushHandler) register(w http.ResponseWriter, r *http.Request, claims models.TokenClaims) {
	var req models.RegisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Platform != "expo" && req.Platform != "web" {
		api.WriteError(w, http.StatusBadRequest, "platform must be 'expo' or 'web'")
		return
	}
	if req.DeviceToken == "" {
		api.WriteError(w, http.StatusBadRequest, "device_token is required")
		return
	}

	ver := req.AppVersion
	if ver == "" {
		ver = "1.0.0"
	}

	sub := models.PushSubscription{
		UserID:      claims.UserID,
		TenantID:    claims.TenantID,
		Platform:    req.Platform,
		DeviceToken: req.DeviceToken,
		P256DH:      req.P256DH,
		AuthKey:     req.AuthKey,
		AppVersion:  ver,
	}
	if err := h.store.Upsert(sub); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{
		"status":   "registered",
		"platform": req.Platform,
		"message":  "Push notifications enabled for this device.",
	})
}

func (h *PushHandler) unregister(w http.ResponseWriter, claims models.TokenClaims, subID string) {
	if err := h.store.Delete(claims.UserID, subID); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "unregister failed")
		return
	}
	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "unregistered"})
}

func (h *PushHandler) list(w http.ResponseWriter, claims models.TokenClaims) {
	subs, err := h.store.GetByUser(claims.UserID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if subs == nil {
		subs = []models.PushSubscription{}
	}
	// Strip sensitive keys from the list response.
	type safeEntry struct {
		ID         string `json:"id"`
		Platform   string `json:"platform"`
		AppVersion string `json:"app_version"`
	}
	safe := make([]safeEntry, len(subs))
	for i, s := range subs {
		safe[i] = safeEntry{ID: s.ID, Platform: s.Platform, AppVersion: s.AppVersion}
	}
	api.WriteJSON(w, http.StatusOK, map[string]any{"devices": safe, "total": len(safe)})
}
