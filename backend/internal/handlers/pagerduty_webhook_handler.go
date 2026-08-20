package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

// PagerDutyPayload is the top-level webhook structure from PagerDuty.
type PagerDutyPayload struct {
	Messages []struct {
		Event    string `json:"event"`
		Incident struct {
			ID        string `json:"id"`
			Title     string `json:"title"`
			Urgency   string `json:"urgency"` // "high" / "low"
			Status    string `json:"status"`
			CreatedAt string `json:"created_at"`
			Service   struct {
				Summary string `json:"summary"`
				HTMLUrl string `json:"html_url"`
			} `json:"service"`
		} `json:"incident"`
	} `json:"messages"`
}

// PagerDutyWebhookHandler processes PagerDuty webhook events.
type PagerDutyWebhookHandler struct {
	eventStore            *store.EventStore
	correlationService    *services.CorrelationService
	sourceRegistryService *services.SourceRegistryService
	webhookSecret         string // HMAC-SHA256 secret for X-PagerDuty-Signature v3
}

func NewPagerDutyWebhookHandler(
	es *store.EventStore,
	cs *services.CorrelationService,
	srs *services.SourceRegistryService,
	secret string,
) *PagerDutyWebhookHandler {
	return &PagerDutyWebhookHandler{
		eventStore:            es,
		correlationService:    cs,
		sourceRegistryService: srs,
		webhookSecret:         secret,
	}
}

func (h *PagerDutyWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
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

	// Verify PagerDuty v3 HMAC signature if secret is configured.
	// Header format: X-PagerDuty-Signature: v1=<hex-digest>[,v1=<hex-digest>]
	if h.webhookSecret != "" {
		sig := r.Header.Get("X-PagerDuty-Signature")
		if !verifyPagerDutySignature(h.webhookSecret, sig, body) {
			api.WriteError(w, http.StatusUnauthorized, "invalid pagerduty signature")
			return
		}
	}

	// Tenant resolution via optional X-Source-Token.
	// PagerDuty webhook URLs can include a source token as a query parameter or
	// Datadog-style custom header for multi-tenant routing.
	tenantID, sourceID := h.resolveTenant(r)

	var pd PagerDutyPayload
	if err := json.Unmarshal(body, &pd); err != nil {
		api.WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	processed := 0
	for _, msg := range pd.Messages {
		if msg.Event != "incident.trigger" && msg.Event != "incident.alert" {
			continue
		}

		inc := msg.Incident
		severity := mapPDUrgencyToSeverity(inc.Urgency)
		service := inc.Service.Summary
		if service == "" {
			service = "pagerduty"
		}

		event := models.Event{
			ID:        fmt.Sprintf("pd-%s-%d", inc.ID, time.Now().UnixNano()),
			TenantID:  tenantID, // stamped at boundary
			Source:    "pagerduty",
			Type:      "alert",
			Service:   service,
			Severity:  severity,
			Title:     inc.Title,
			Message:   inc.Title,
			Timestamp: time.Now(),
		}

		if err := h.eventStore.SaveEvent(event); err != nil {
			slog.Error("pagerduty_webhook: failed to save event for incident", "incident_id", inc.ID, "error", err)
			continue
		}
		if h.correlationService.ProcessEvent(event) {
			slog.Info("pagerduty_webhook: processed incident", "incident_id", inc.ID, "tenant_id", tenantID, "service", service)
			processed++
		}
	}

	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, processed)
	}

	api.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":    "processed",
		"processed": processed,
	})
}

// resolveTenant derives tenantID and sourceID from an optional X-Source-Token header.
func (h *PagerDutyWebhookHandler) resolveTenant(r *http.Request) (tenantID, sourceID string) {
	if h.sourceRegistryService == nil {
		return "default", ""
	}
	token := r.Header.Get("X-Source-Token")
	if token == "" {
		return "default", ""
	}
	src, tenant, ok := h.sourceRegistryService.FindByToken(token)
	if !ok {
		slog.Warn("pagerduty_webhook: unknown X-Source-Token — falling back to default tenant")
		return "default", ""
	}
	return tenant, src.ID
}

// verifyPagerDutySignature validates the X-PagerDuty-Signature header.
// PagerDuty v3 sends one or more signatures as "v1=<hex>[,v1=<hex>]".
func verifyPagerDutySignature(secret, header string, body []byte) bool {
	if header == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "v1=") {
			continue
		}
		sig := strings.TrimPrefix(part, "v1=")
		if hmac.Equal([]byte(expected), []byte(sig)) {
			return true
		}
	}
	return false
}

// mapPDUrgencyToSeverity maps PagerDuty urgency to our internal severity.
func mapPDUrgencyToSeverity(urgency string) string {
	switch urgency {
	case "high":
		return "critical"
	case "low":
		return "medium"
	default:
		return "medium"
	}
}
