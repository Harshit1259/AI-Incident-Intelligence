package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
)

type ComplianceHandler struct {
	svc *services.ComplianceService
}

func NewComplianceHandler(svc *services.ComplianceService) *ComplianceHandler {
	return &ComplianceHandler{svc: svc}
}

// HandleReport handles GET /api/v1/compliance/report
func (h *ComplianceHandler) HandleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	report, err := h.svc.GenerateReport(tenantID, days)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	api.WriteJSON(w, http.StatusOK, report)
}

// HandleExport handles GET /api/v1/compliance/export
func (h *ComplianceHandler) HandleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID := middleware.TenantFromRequest(r)

	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	csvData, err := h.svc.ExportCSV(tenantID, days)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=compliance_report_%dd.csv", days))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(csvData)); err != nil {
		slog.ErrorContext(r.Context(), "compliance: CSV write error", "tenant_id", tenantID, "error", err)
	}
}
