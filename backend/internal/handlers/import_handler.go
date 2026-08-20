package handlers

// import_handler.go
//
// CSV bulk-import endpoints. Accepts multipart form upload or raw text/csv body.
//
// Routes (all require JWT):
//   POST /api/v1/admin/import/services          – upload services.csv
//   POST /api/v1/admin/import/profiles          – upload business_profiles.csv
//   POST /api/v1/admin/import/baselines         – upload baselines.csv
//
// Multipart field name: "file"
// Raw CSV: send body with Content-Type: text/csv

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/services"
)

// ImportHandler handles CSV bulk-import endpoints.
type ImportHandler struct {
	importSvc *services.ImportService
}

func NewImportHandler(importSvc *services.ImportService) *ImportHandler {
	return &ImportHandler{importSvc: importSvc}
}

// HandleImportServices handles POST /api/v1/admin/import/services
func (h *ImportHandler) HandleImportServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	reader, err := csvReaderFromRequest(r)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.importSvc.ImportServices(tenantID, reader)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, result)
}

// HandleImportProfiles handles POST /api/v1/admin/import/profiles
func (h *ImportHandler) HandleImportProfiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	reader, err := csvReaderFromRequest(r)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.importSvc.ImportBusinessProfiles(tenantID, reader)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, result)
}

// HandleImportBaselines handles POST /api/v1/admin/import/baselines
func (h *ImportHandler) HandleImportBaselines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	tenantID := middleware.TenantFromRequest(r)
	reader, err := csvReaderFromRequest(r)
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.importSvc.ImportBaselines(tenantID, reader)
	if err != nil {
		api.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	api.WriteJSON(w, http.StatusOK, result)
}

// csvReaderFromRequest extracts CSV data from either multipart form or raw body.
func csvReaderFromRequest(r *http.Request) (io.Reader, error) {
	ct := r.Header.Get("Content-Type")

	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, err
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			return nil, err
		}
		buf := new(bytes.Buffer)
		if _, err := io.Copy(buf, file); err != nil {
			file.Close()
			return nil, err
		}
		file.Close()
		return buf, nil
	}

	// Raw body (text/csv or application/octet-stream)
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, r.Body); err != nil {
		return nil, err
	}
	return buf, nil
}
