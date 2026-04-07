package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
)

type SourceHandler struct {
	sourceRegistryService *services.SourceRegistryService
	ingestHandler         *IngestHandler
}

func NewSourceHandler(
	sourceRegistryService *services.SourceRegistryService,
	ingestHandler *IngestHandler,
) *SourceHandler {
	return &SourceHandler{
		sourceRegistryService: sourceRegistryService,
		ingestHandler:         ingestHandler,
	}
}

func (sourceHandler *SourceHandler) ListSources(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items": sourceHandler.sourceRegistryService.ListSources(),
	})
}

func (sourceHandler *SourceHandler) CreateSource(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if requestBody.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if requestBody.Type == "" {
		requestBody.Type = "generic"
	}

	source := sourceHandler.sourceRegistryService.CreateSource(requestBody.Name, requestBody.Type)
	_ = json.NewEncoder(w).Encode(source)
}

func (sourceHandler *SourceHandler) ListSourceHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items": sourceHandler.sourceRegistryService.ListSources(),
	})
}

func (sourceHandler *SourceHandler) SendTestEvent(w http.ResponseWriter, r *http.Request) {
	var requestBody struct {
		SourceID string `json:"source_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var selectedSource models.SourceConnection
	found := false

	for _, source := range sourceHandler.sourceRegistryService.ListSources() {
		if source.ID == requestBody.SourceID {
			selectedSource = source
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "source not found", http.StatusNotFound)
		return
	}

	if selectedSource.Type == "prometheus" {
		testPayload := map[string]interface{}{
			"alerts": []map[string]interface{}{
				{
					"status": "firing",
					"labels": map[string]string{
						"service":     "payments-api",
						"severity":    "critical",
						"instance":    "pod-1",
						"environment": "prod",
					},
					"annotations": map[string]string{
						"summary":     "DB Down",
						"description": "Database unreachable from payments-api",
					},
					"startsAt":    time.Now().UTC().Format(time.RFC3339),
					"fingerprint": fmt.Sprintf("test-prometheus-%d", time.Now().UnixNano()),
				},
			},
		}

		requestBytes, _ := json.Marshal(testPayload)
		request, _ := http.NewRequest(http.MethodPost, "/api/v1/ingest/prometheus", bytes.NewReader(requestBytes))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Source-Token", selectedSource.Token)

		recorder := newResponseRecorder()
		sourceHandler.ingestHandler.PrometheusWebhook(recorder, request)

		if recorder.statusCode >= 400 {
			http.Error(w, recorder.body.String(), recorder.statusCode)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		return
	}

	testPayload := map[string]interface{}{
		"source":      "generic",
		"service":     "payments-api",
		"severity":    "critical",
		"title":       "Synthetic onboarding test",
		"message":     "database connectivity failure detected",
		"environment": "prod",
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
	}

	requestBytes, _ := json.Marshal(testPayload)
	request, _ := http.NewRequest(http.MethodPost, "/api/v1/ingest/webhook", bytes.NewReader(requestBytes))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Source-Token", selectedSource.Token)

	recorder := newResponseRecorder()
	sourceHandler.ingestHandler.GenericWebhook(recorder, request)

	if recorder.statusCode >= 400 {
		http.Error(w, recorder.body.String(), recorder.statusCode)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

type responseRecorder struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (responseRecorder *responseRecorder) Header() http.Header {
	return responseRecorder.header
}

func (responseRecorder *responseRecorder) Write(data []byte) (int, error) {
	return responseRecorder.body.Write(data)
}

func (responseRecorder *responseRecorder) WriteHeader(statusCode int) {
	responseRecorder.statusCode = statusCode
}
