package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/services"
	"ai-incident-platform/backend/internal/store"
)

// DatadogWebhookPayload is the structure sent by Datadog monitor webhooks.
type DatadogWebhookPayload struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Message   string `json:"text"`
	Priority  string `json:"priority"`   // P1-P5
	AlertType string `json:"alert_type"` // error, warning, success, info
	AlertID   int64  `json:"alert_id"`
	Date      int64  `json:"date"` // unix timestamp
	Host      string `json:"host"`
	Tags      string `json:"tags"` // comma-separated key:value
	URL       string `json:"url"`
	Metric    string `json:"metric"`
	Org       struct {
		Name string `json:"name"`
		ID   int64  `json:"id"`
	} `json:"org"`
}

// DatadogWebhookHandler processes Datadog monitor webhook events.
type DatadogWebhookHandler struct {
	eventStore            *store.EventStore
	correlationService    *services.CorrelationService
	sourceRegistryService *services.SourceRegistryService
	webhookSecret         string // shared secret validated via constant-time comparison
}

func NewDatadogWebhookHandler(
	es *store.EventStore,
	cs *services.CorrelationService,
	srs *services.SourceRegistryService,
	secret string,
) *DatadogWebhookHandler {
	return &DatadogWebhookHandler{
		eventStore:            es,
		correlationService:    cs,
		sourceRegistryService: srs,
		webhookSecret:         secret,
	}
}

func (h *DatadogWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Enforce body size limit before reading.
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large (max 1 MiB)")
		return
	}
	defer r.Body.Close()

	// Verify shared secret via constant-time comparison to prevent timing attacks.
	// Datadog's native custom webhooks send a fixed token header; HMAC is not
	// part of the Datadog webhook spec, so constant-time equality is the correct hardening.
	if h.webhookSecret != "" {
		token := r.Header.Get("X-Datadog-Webhook-Token")
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.webhookSecret)) != 1 {
			api.WriteError(w, http.StatusUnauthorized, "invalid datadog webhook token")
			return
		}
	}

	// Tenant resolution via optional X-Source-Token.
	// Enterprise users configure a source in the registry and include the token in
	// their Datadog webhook URL or custom headers for multi-tenant routing.
	tenantID, sourceID := h.resolveTenant(r)

	var payload DatadogWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Skip recovery alerts.
	if payload.AlertType == "success" {
		api.WriteJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	// Schema validation — require at minimum a title.
	if strings.TrimSpace(payload.Title) == "" {
		api.WriteError(w, http.StatusBadRequest, "payload missing required field: title")
		return
	}

	severity := mapDatadogSeverity(payload.Priority, payload.AlertType)
	service := extractDatadogService(payload.Tags, payload.Host)

	ts := time.Now()
	if payload.Date > 0 {
		ts = time.Unix(payload.Date, 0)
	}

	eventID := fmt.Sprintf("dd-%s-%d", payload.ID, ts.UnixNano())
	if payload.ID == "" {
		eventID = fmt.Sprintf("dd-%d-%d", payload.AlertID, ts.UnixNano())
	}

	event := models.Event{
		ID:        eventID,
		TenantID:  tenantID, // stamped at boundary — never from body
		Source:    "datadog",
		Type:      "monitor",
		Service:   service,
		Severity:  severity,
		Title:     payload.Title,
		Message:   payload.Message,
		Timestamp: ts,
	}

	if err := h.eventStore.SaveEvent(event); err != nil {
		slog.ErrorContext(r.Context(), "datadog_webhook: failed to save event", "event_id", event.ID, "error", err)
		api.WriteError(w, http.StatusInternalServerError, "failed to store event")
		return
	}
	if h.correlationService.ProcessEvent(event) {
		slog.InfoContext(r.Context(), "datadog_webhook: processed monitor", "title", payload.Title, "tenant_id", tenantID, "service", service, "severity", severity)
	}

	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, 1)
	}

	api.WriteJSON(w, http.StatusOK, map[string]string{"status": "processed"})
}

// resolveTenant derives tenantID and sourceID from an optional X-Source-Token header.
// Falls back to "default" so existing single-tenant Datadog integrations keep working.
func (h *DatadogWebhookHandler) resolveTenant(r *http.Request) (tenantID, sourceID string) {
	if h.sourceRegistryService == nil {
		return "default", ""
	}
	token := r.Header.Get("X-Source-Token")
	if token == "" {
		return "default", ""
	}
	src, tenant, ok := h.sourceRegistryService.FindByToken(token)
	if !ok {
		slog.Warn("datadog_webhook: unknown X-Source-Token — falling back to default tenant")
		return "default", ""
	}
	return tenant, src.ID
}

// mapDatadogSeverity converts Datadog priority/alert_type to internal severity.
func mapDatadogSeverity(priority, alertType string) string {
	if priority == "P1" || alertType == "error" {
		return "critical"
	}
	if priority == "P2" {
		return "high"
	}
	if priority == "P3" {
		return "medium"
	}
	if priority == "P4" || priority == "P5" || alertType == "warning" {
		return "low"
	}
	return "medium"
}

// extractDatadogService extracts the service name from comma-separated tags.
// Falls back to host, then "datadog".
func extractDatadogService(tags, host string) string {
	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if strings.HasPrefix(tag, "service:") {
			svc := strings.TrimPrefix(tag, "service:")
			if svc != "" {
				return svc
			}
		}
	}
	if host != "" {
		return host
	}
	return "datadog"
}
