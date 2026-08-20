package handlers

import (
	"log/slog"
	"net/http"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/services"
)

type DigestHandler struct {
	digestService *services.DigestService
}

func NewDigestHandler(ds *services.DigestService) *DigestHandler {
	return &DigestHandler{digestService: ds}
}

// HandleWeeklyDigest handles GET /api/v1/digest/weekly
func (h *DigestHandler) HandleWeeklyDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	digest, err := h.digestService.GenerateWeeklyDigest(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to generate weekly digest")
		return
	}

	api.WriteJSON(w, http.StatusOK, digest)
}

// HandleSendDigest handles POST /api/v1/digest/send
func (h *DigestHandler) HandleSendDigest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	digest, err := h.digestService.GenerateWeeklyDigest(tenantID)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, "failed to generate digest for sending")
		return
	}

	// Log the digest (email sending placeholder)
	slog.InfoContext(r.Context(), "digest: would send weekly digest", "tenant_id", digest.TenantID, "total_incidents", digest.TotalIncidents, "reliability_score", digest.ReliabilityScore)

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status": "sent",
		"digest": digest,
	})
}
