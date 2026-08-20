package services

// PushService dispatches push notifications to mobile devices registered through
// the AIOps mobile app.
//
// Transport: Expo Push API (https://exp.host/--/api/v2/push/send)
// Expo handles the FCM (Android) and APNs (iOS) routing transparently.
// No Firebase project or Apple Developer account is needed at the backend level —
// only the Expo SDK in the mobile app.
//
// Delivery policy: best-effort, fire-and-forget. Push failure never blocks
// the incident creation path.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const expoPushURL = "https://exp.host/--/api/v2/push/send"

// PushService sends push notifications to registered devices.
type PushService struct {
	store      *store.PushStore
	httpClient *http.Client
}

func NewPushService(ps *store.PushStore) *PushService {
	return &PushService{
		store: ps,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// NotifyNewIncident broadcasts a push to all registered devices for a tenant
// when a new incident is created. Only fires for critical and high incidents
// to avoid alert fatigue.
func (s *PushService) NotifyNewIncident(tenantID string, inc models.Incident) error {
	if inc.Severity != "critical" && inc.Severity != "high" {
		return nil
	}

	subs, err := s.store.GetByTenant(tenantID)
	if err != nil {
		slog.Warn("push: failed to load subscriptions", "tenant_id", tenantID, "error", err)
		return err
	}
	if len(subs) == 0 {
		return nil
	}

	sevIcon := "🔴"
	if inc.Severity == "high" {
		sevIcon = "🟠"
	}

	notif := models.PushNotification{
		Title: fmt.Sprintf("%s %s — %s", sevIcon, strings.ToUpper(inc.Severity), inc.Service),
		Body:  inc.Title,
		Sound: "default",
		Badge: 1,
		Data: map[string]string{
			"incident_id": inc.ID,
			"severity":    inc.Severity,
			"service":     inc.Service,
			"screen":      "IncidentDetail",
		},
	}

	return s.dispatch(subs, notif)
}

// NotifyIncidentUpdate sends a push when an incident status changes (e.g. resolved).
func (s *PushService) NotifyIncidentUpdate(tenantID string, inc models.Incident, updateMsg string) error {
	subs, err := s.store.GetByTenant(tenantID)
	if err != nil || len(subs) == 0 {
		return err
	}
	notif := models.PushNotification{
		Title: fmt.Sprintf("✅ %s — %s", inc.Service, strings.ToUpper(inc.Status)),
		Body:  updateMsg,
		Sound: "default",
		Data: map[string]string{
			"incident_id": inc.ID,
			"severity":    inc.Severity,
			"screen":      "IncidentDetail",
		},
	}
	return s.dispatch(subs, notif)
}

// NotifyActionPending alerts on-call engineers that an automation action
// requires human approval before execution.
func (s *PushService) NotifyActionPending(tenantID, incidentID, actionName string) error {
	subs, err := s.store.GetByTenant(tenantID)
	if err != nil || len(subs) == 0 {
		return err
	}
	notif := models.PushNotification{
		Title: "⚙️ Action Approval Required",
		Body:  fmt.Sprintf("'%s' is waiting for your approval.", actionName),
		Sound: "default",
		Data: map[string]string{
			"incident_id": incidentID,
			"screen":      "ActionApproval",
		},
	}
	return s.dispatch(subs, notif)
}

// ── Transport layer ───────────────────────────────────────────────────────────

// expoPushMessage is one Expo push message in the batch send format.
type expoPushMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Sound string            `json:"sound,omitempty"`
	Badge *int              `json:"badge,omitempty"`
	Data  map[string]string `json:"data,omitempty"`
}

func (s *PushService) dispatch(subs []models.PushSubscription, notif models.PushNotification) error {
	// Build Expo push messages (batch up to 100 per request as per Expo docs).
	var expoMessages []expoPushMessage
	for _, sub := range subs {
		if sub.Platform != "expo" {
			continue // web push not yet implemented at transport level
		}
		msg := expoPushMessage{
			To:    sub.DeviceToken,
			Title: notif.Title,
			Body:  notif.Body,
			Sound: notif.Sound,
			Data:  notif.Data,
		}
		if notif.Badge > 0 {
			b := notif.Badge
			msg.Badge = &b
		}
		expoMessages = append(expoMessages, msg)
	}

	if len(expoMessages) == 0 {
		return nil
	}

	// Send in batches of 100.
	const batchSize = 100
	for i := 0; i < len(expoMessages); i += batchSize {
		end := i + batchSize
		if end > len(expoMessages) {
			end = len(expoMessages)
		}
		if err := s.sendExpoBatch(expoMessages[i:end]); err != nil {
			slog.Warn("push: expo batch send failed", "error", err)
		}
	}
	return nil
}

func (s *PushService) sendExpoBatch(msgs []expoPushMessage) error {
	body, err := json.Marshal(msgs)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, expoPushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("expo push returned %d", resp.StatusCode)
	}
	return nil
}
