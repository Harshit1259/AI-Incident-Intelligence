package handlers

// Zabbix webhook ingest — POST /api/v1/ingest/zabbix
//
// Zabbix has no built-in NeuroOps format. Customers import our webhook media
// type (deploy/zabbix/neuroops-zabbix.yaml); its script sends one JSON object
// per problem, recovery or update, built from Zabbix macros. See plan.md.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/models"
)

// zabbixPayload is what our media-type script sends. Every value comes from
// a Zabbix macro, so every field is a string.
type zabbixPayload struct {
	EventID       string      `json:"event_id"`       // {EVENT.ID} — shared by a problem and its recovery
	EventValue    string      `json:"event_value"`    // {EVENT.VALUE}: 1 problem, 0 recovered
	EventStatus   string      `json:"event_status"`   // {EVENT.STATUS}: PROBLEM | RESOLVED
	UpdateStatus  string      `json:"update_status"`  // {EVENT.UPDATE.STATUS}: 1 = acknowledge/comment/…
	UpdateAction  string      `json:"update_action"`  // {EVENT.UPDATE.ACTION}
	UpdateMessage string      `json:"update_message"` // {EVENT.UPDATE.MESSAGE}
	UpdateUser    string      `json:"update_user"`    // {USER.FULLNAME}
	TriggerID     string      `json:"trigger_id"`     // {TRIGGER.ID}
	TriggerName   string      `json:"trigger_name"`   // {TRIGGER.NAME}
	EventName     string      `json:"event_name"`     // {EVENT.NAME}
	Severity      string      `json:"severity"`       // {EVENT.NSEVERITY}: 0–5
	Host          string      `json:"host"`           // {HOST.HOST}
	HostName      string      `json:"host_name"`      // {HOST.NAME}
	HostIP        string      `json:"host_ip"`        // {HOST.IP}
	HostGroup     string      `json:"host_group"`     // {TRIGGER.HOSTGROUP.NAME}
	Tags          []zabbixTag `json:"tags"`           // {EVENT.TAGSJSON}
	ItemValue     string      `json:"item_value"`     // {ITEM.LASTVALUE1}
	OpData        string      `json:"opdata"`         // {EVENT.OPDATA}
	EventTime     string      `json:"event_time"`     // {EVENT.DATE} {EVENT.TIME}
	ZabbixURL     string      `json:"zabbix_url"`     // link back to the event
}

type zabbixTag struct {
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

// Labels that Zabbix-specific UI and mutes read.
const (
	ZabbixValueLabel = "zabbix.item_value"
	ZabbixURLLabel   = "zabbix.url"
)

// zabbixSeverity maps {EVENT.NSEVERITY} to NeuroOps severities.
var zabbixSeverity = map[string]string{
	"5": "critical", // Disaster
	"4": "high",     // High
	"3": "medium",   // Average
	"2": "low",      // Warning
	"1": "info",     // Information
	"0": "info",     // Not classified
}

var errZabbixTest = errors.New("zabbix test message")

// unexpandedMacro reports whether Zabbix left a macro as literal text, which
// is what its media-type "Test" button sends unless values are typed in.
func unexpandedMacro(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || (strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}"))
}

// zabbixKind classifies a message.
type zabbixKind int

const (
	zabbixProblem zabbixKind = iota
	zabbixRecovery
	zabbixUpdate
)

// zabbixToEvent turns a payload into our event. It returns errZabbixTest for
// the media-type Test button, and an error when required fields are missing.
func zabbixToEvent(p zabbixPayload, tenantID string) (models.Event, zabbixKind, error) {
	if unexpandedMacro(p.EventID) {
		return models.Event{}, 0, errZabbixTest
	}
	if unexpandedMacro(p.TriggerID) || unexpandedMacro(p.Host) {
		return models.Event{}, 0, errors.New("trigger_id and host are required")
	}

	kind := zabbixProblem
	switch {
	case strings.TrimSpace(p.UpdateStatus) == "1":
		kind = zabbixUpdate
	case strings.TrimSpace(p.EventValue) == "0" || strings.EqualFold(strings.TrimSpace(p.EventStatus), "RESOLVED"):
		kind = zabbixRecovery
	}

	labels := map[string]string{
		"alertname":  strings.TrimSpace(p.TriggerName),
		"trigger_id": strings.TrimSpace(p.TriggerID),
		"host":       strings.TrimSpace(p.Host),
	}
	setIf := func(k, v string) {
		if v = strings.TrimSpace(v); v != "" && !unexpandedMacro(v) {
			labels[k] = v
		}
	}
	setIf("host_name", p.HostName)
	setIf("host_ip", p.HostIP)
	setIf("host_group", p.HostGroup)
	setIf("event_time", p.EventTime)
	setIf("opdata", p.OpData)
	setIf(ZabbixValueLabel, p.ItemValue)
	if strings.HasPrefix(strings.TrimSpace(p.ZabbixURL), "http") {
		labels[ZabbixURLLabel] = strings.TrimSpace(p.ZabbixURL)
	}

	serviceTag, environment := "", ""
	for _, t := range p.Tags {
		name := strings.ToLower(strings.TrimSpace(t.Tag))
		if name == "" {
			continue
		}
		labels["tag."+name] = strings.TrimSpace(t.Value)
		switch name {
		case "service":
			serviceTag = strings.TrimSpace(t.Value)
		case "env", "environment":
			environment = strings.TrimSpace(t.Value)
		}
	}

	// Service: the "service" tag, else the host group, else the host.
	service := serviceTag
	if service == "" && !unexpandedMacro(p.HostGroup) {
		service = strings.TrimSpace(p.HostGroup)
	}
	if service == "" {
		service = strings.TrimSpace(p.Host)
	}

	title := strings.TrimSpace(p.EventName)
	if unexpandedMacro(title) {
		title = strings.TrimSpace(p.TriggerName)
	}
	severity, ok := zabbixSeverity[strings.TrimSpace(p.Severity)]
	if !ok {
		severity = "medium"
	}
	status := "firing"
	if kind == zabbixRecovery {
		status = "resolved"
	}

	return models.Event{
		// Problem, re-send and recovery share {EVENT.ID}, so they update one
		// row and auto-close can pair them. Tenant-scoped like Prometheus.
		ID:          "zbx-" + tenantID + "-" + strings.TrimSpace(p.EventID),
		TenantID:    tenantID,
		Source:      "zabbix",
		Service:     service,
		Resource:    strings.TrimSpace(p.Host),
		Environment: environment,
		Severity:    severity,
		Type:        "alert",
		Title:       title,
		Message:     strings.TrimSpace(p.OpData),
		Labels:      labels,
		Timestamp:   time.Now().UTC(),
		// The same trigger on the same host is the same alert every time.
		Fingerprint:  "zbx:" + strings.TrimSpace(p.TriggerID) + ":" + strings.TrimSpace(p.Host),
		IngestSchema: "zabbix",
		AlertStatus:  status,
	}, kind, nil
}

// zabbixUpdateNote is the timeline line for an acknowledgement or comment made in Zabbix.
func zabbixUpdateNote(p zabbixPayload) string {
	who := strings.TrimSpace(p.UpdateUser)
	// Zabbix writes "Inaccessible user" when the notified user may not see
	// the person who made the update (typical for a restricted NeuroOps user).
	if unexpandedMacro(who) || strings.EqualFold(who, "Inaccessible user") {
		who = "a Zabbix user"
	}
	action := strings.TrimSpace(p.UpdateAction)
	if unexpandedMacro(action) {
		action = "updated"
	}
	note := fmt.Sprintf("Zabbix: %s %s the problem", who, action)
	if msg := strings.TrimSpace(p.UpdateMessage); msg != "" && !unexpandedMacro(msg) {
		note += fmt.Sprintf(": %q", msg)
	}
	return note
}

// incidentNoter adds a note to the open incidents that contain an alert.
type incidentNoter interface {
	NoteForEvent(tenantID, eventID, actor, note string) int
}

// SetIncidentNoter wires the service that records Zabbix updates on incidents.
func (h *IngestHandler) SetIncidentNoter(n incidentNoter) {
	h.noter = n
}

// ZabbixWebhook handles POST /api/v1/ingest/zabbix.
func (h *IngestHandler) ZabbixWebhook(w http.ResponseWriter, r *http.Request) {
	tenantID, src, ok := h.resolveSource(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "missing or invalid source token (send X-Source-Token or Authorization: Bearer <token>)")
		return
	}
	sourceID := ""
	if src != nil {
		if src.Type != "" && src.Type != "zabbix" {
			api.WriteError(w, http.StatusForbidden, "source token not authorized for /ingest/zabbix")
			return
		}
		sourceID = src.ID
	}

	cached, seen, err := h.checkIdempotency(r, tenantID)
	if err != nil {
		api.WriteError(w, http.StatusServiceUnavailable, "service temporarily unavailable")
		return
	}
	if seen {
		w.Header().Set("X-Idempotent-Replay", "true")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cached)) //nolint:errcheck
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		api.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large (max 1 MiB)")
		return
	}

	result, err := h.ingestZabbixPayload(body, tenantID, sourceID)
	if errors.Is(err, errZabbixTest) {
		api.WriteJSON(w, http.StatusOK, map[string]string{
			"status":  "test_received",
			"message": "NeuroOps received the Zabbix test message. Real problems will create incidents.",
		})
		return
	}
	if err != nil {
		api.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp := fmt.Sprintf(`{"status":"accepted","result":%q}`, result)
	h.recordIdempotency(r, tenantID, sourceID, resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(resp)) //nolint:errcheck
}

// ingestZabbixPayload stores and routes one Zabbix message for an established
// tenant. The result says what happened ("problem", "recovery", "update").
func (h *IngestHandler) ingestZabbixPayload(body []byte, tenantID, sourceID string) (string, error) {
	var p zabbixPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	event, kind, err := zabbixToEvent(p, tenantID)
	if err != nil {
		return "", err
	}

	if kind == zabbixUpdate {
		// An acknowledgement or comment in Zabbix is not a new occurrence:
		// record it on the incident's timeline and change nothing else.
		if h.noter != nil {
			h.noter.NoteForEvent(tenantID, event.ID, "zabbix", zabbixUpdateNote(p))
		}
		if sourceID != "" {
			h.sourceRegistryService.RecordSuccess(sourceID, 0)
		}
		return "update", nil
	}

	if err := h.eventStore.SaveEvent(event); err != nil {
		slog.Error("ingest: failed to save zabbix event", "event_id", event.ID, "error", err)
		return "", errors.New("could not store the event")
	}
	result := "problem"
	if kind == zabbixRecovery {
		result = "recovery"
		h.correlationService.ProcessResolved(event)
	} else {
		h.correlationService.ProcessEvent(event)
	}
	if sourceID != "" {
		h.sourceRegistryService.RecordSuccess(sourceID, 1)
	}
	return result, nil
}
