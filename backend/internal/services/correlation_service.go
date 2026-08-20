package services

// CorrelationService — Phase 1 rebuild
//
// Design:
//   1. Compute a deterministic fingerprint for each incoming event.
//   2. Check the dedup window (5 min): if the same fingerprint was already
//      processed, suppress the event (identical alert storm → 0 noise).
//   3. Check the correlation window (5 min): if an open/acknowledged incident
//      exists for the same service whose last_event_time is within the window,
//      merge the event in.  This collapses 100 alerts → 1 incident.
//   4. Otherwise create a fresh incident.
//
// All state is in PostgreSQL — no in-memory maps, safe for multiple replicas.

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const (
	correlationWindowMinutes = 5 // minutes: alerts within this window → 1 incident
	dedupWindowMinutes       = 5 // minutes: identical fingerprint within window → suppressed
)

// CorrelationService wires together event-store, incident-store, and change-store.
type CorrelationService struct {
	incidentStore            *store.IncidentStore
	eventStore               *store.EventStore
	changeStore              *store.ChangeStore
	slackService             *SlackService
	engineeringHealthService *EngineeringHealthService
	onboardingService        *OnboardingService
	alertFeedbackService     *AlertFeedbackService
	autoResolveService       *AutoResolveService
	whatsappService          *WhatsAppService
	knowledgeBase            *KnowledgeBase
	remediationOrchestrator  *RemediationOrchestrator
	pushService              *PushService
}

// SetPushService wires in the push notification service.
func (s *CorrelationService) SetPushService(ps *PushService) {
	s.pushService = ps
}

// SetRemediationOrchestrator wires the auto-remediation orchestrator.
// When set, every new high/critical incident triggers closed-loop remediation.
func (s *CorrelationService) SetRemediationOrchestrator(o *RemediationOrchestrator) {
	s.remediationOrchestrator = o
}

// SetKnowledgeBase wires in the knowledge base for enriched incident creation.
func (s *CorrelationService) SetKnowledgeBase(kb *KnowledgeBase) {
	s.knowledgeBase = kb
}

func NewCorrelationService(
	incidentStore *store.IncidentStore,
	eventStore *store.EventStore,
	changeStore *store.ChangeStore,
) *CorrelationService {
	return &CorrelationService{
		incidentStore: incidentStore,
		eventStore:    eventStore,
		changeStore:   changeStore,
	}
}

// SetSlackService wires in an optional Slack integration.
func (s *CorrelationService) SetSlackService(ss *SlackService) {
	s.slackService = ss
}

// SetEngineeringHealthService wires in the engineering health service for metrics recording.
func (s *CorrelationService) SetEngineeringHealthService(ehs *EngineeringHealthService) {
	s.engineeringHealthService = ehs
}

// SetOnboardingService wires in the onboarding service for milestone tracking.
func (s *CorrelationService) SetOnboardingService(os *OnboardingService) {
	s.onboardingService = os
}

// SetAlertFeedbackService wires in the alert feedback service for suppression.
func (s *CorrelationService) SetAlertFeedbackService(afs *AlertFeedbackService) {
	s.alertFeedbackService = afs
}

// SetAutoResolveService wires in the auto-resolve service.
func (s *CorrelationService) SetAutoResolveService(ars *AutoResolveService) {
	s.autoResolveService = ars
}

// SetWhatsAppService wires in the WhatsApp notification service.
func (s *CorrelationService) SetWhatsAppService(ws *WhatsAppService) {
	s.whatsappService = ws
}

// ProcessEvent is the single entry point called by ingest handlers and demo service.
// Returns true when the event was processed, false when it was suppressed by dedup.
func (s *CorrelationService) ProcessEvent(event models.Event) bool {
	// 1. Compute fingerprint if not already set (Prometheus sends its own)
	if event.Fingerprint == "" {
		event.Fingerprint = computeEventFingerprint(event)
	}

	// Upsert the event with the computed fingerprint. Handlers already persisted the
	// raw event; this call only refreshes the fingerprint column in-place.
	if err := s.eventStore.SaveEvent(event); err != nil {
		slog.Error("correlation: failed to upsert fingerprint for event", "event_id", event.ID, "error", err)
	}

	// 1b. Check if fingerprint is suppressed via alert feedback
	if s.alertFeedbackService != nil && s.alertFeedbackService.IsSuppressed(event.Fingerprint) {
		slog.Info("correlation: suppressed event (fingerprint marked as noise)", "event_id", event.ID, "fingerprint", event.Fingerprint)
		return false
	}

	// 2. Dedup check — suppress if we've seen this fingerprint recently
	dedupWindowStart := event.Timestamp.Add(-time.Duration(dedupWindowMinutes) * time.Minute)
	if s.eventStore.FingerprintSeenInWindow(event.Fingerprint, dedupWindowStart, event.ID) {
		// Identical alert fired again within the dedup window → suppress silently
		return false
	}

	// 3a. Fingerprint merge — if ANY open incident has the same fingerprint, merge into it
	//     regardless of time window. This prevents duplicate incidents for recurring anomalies.
	existing := s.incidentStore.FindOpenIncidentByFingerprint(event.Fingerprint)

	// 3b. Time-window correlation — find open incident for same/related service within 5 min
	if existing == nil {
		corrWindowStart := event.Timestamp.Add(-time.Duration(correlationWindowMinutes) * time.Minute)
		existing = s.findRelatedIncident(event, corrWindowStart)
	}

	if existing != nil {
		// Merge into existing incident
		s.mergeEvent(existing, event)
		return true
	}

	// 4. No existing incident → create one
	s.createIncident(event)
	return true
}

// ─────────────────────────────────────────────────────
// Fingerprinting
// ─────────────────────────────────────────────────────

// computeEventFingerprint produces a stable 16-char hex hash that identifies
// "same alert, same service" regardless of instance-specific words (pod names,
// UUIDs, port numbers, etc.).
func computeEventFingerprint(event models.Event) string {
	src := strings.ToLower(strings.TrimSpace(event.Source))
	svc := strings.ToLower(strings.TrimSpace(event.Service))
	sev := strings.ToLower(strings.TrimSpace(event.Severity))
	title := normalizeAlertTitle(strings.ToLower(strings.TrimSpace(event.Title)))
	if title == "" {
		title = normalizeAlertTitle(strings.ToLower(strings.TrimSpace(event.Message)))
	}

	raw := fmt.Sprintf("%s|%s|%s|%s", src, svc, sev, title)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:8]) // 16 hex chars
}

// normalizeAlertTitle strips instance-specific tokens (UUIDs, pod names, IP
// addresses, numeric IDs) so that semantically equivalent alerts share the
// same fingerprint.
func normalizeAlertTitle(title string) string {
	words := strings.Fields(title)
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if !isInstanceToken(w) {
			kept = append(kept, w)
		}
	}
	return strings.Join(kept, " ")
}

// isInstanceToken returns true for words that carry no semantic meaning for
// classification: numeric IDs, UUIDs, pod/hash suffixes, IP addresses.
func isInstanceToken(word string) bool {
	if len(word) == 0 {
		return false
	}

	digits, letters, dots, dashes := 0, 0, 0, 0
	for _, c := range word {
		switch {
		case c >= '0' && c <= '9':
			digits++
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letters++
		case c == '.':
			dots++
		case c == '-':
			dashes++
		}
	}

	total := len(word)

	// All digits (port, count, etc.)
	if digits == total && total > 3 {
		return true
	}

	// Looks like an IP address (e.g. 10.0.0.1)
	if dots == 3 && digits > dots {
		return true
	}

	// UUID-like: many dashes + hex chars, length >= 32
	if total >= 32 && dashes >= 3 && letters > 0 && digits > 0 {
		return true
	}

	// Mixed alphanum with dashes, long (pod/container name suffix)
	if total >= 10 && dashes >= 1 && digits > 0 && letters > 0 {
		return true
	}

	return false
}

// ─────────────────────────────────────────────────────
// Merge path (existing incident)
// ─────────────────────────────────────────────────────

func (s *CorrelationService) mergeEvent(incident *models.Incident, event models.Event) {
	// Extend the time window
	if event.Timestamp.After(incident.LastEventTime) {
		incident.LastEventTime = event.Timestamp
	}

	// Accumulate
	incident.EventCount++
	incident.EventIDs = append(incident.EventIDs, event.ID)

	// Escalate severity (never downgrade)
	if severityWeight(event.Severity) > severityWeight(incident.Severity) {
		incident.Severity = event.Severity
	}

	// Update correlation metadata
	incident.CorrelationPattern = "time_window_service_grouping"
	if incident.CorrelationScore < 100 {
		incident.CorrelationScore = clampInt(incident.CorrelationScore+10, 0, 100)
	}
	incident.CorrelationReason = fmt.Sprintf(
		"%d alerts grouped for service '%s' within %d-min window",
		incident.EventCount, incident.Service, correlationWindowMinutes,
	)

	// Confidence grows as more correlated signals arrive
	incident.Confidence = clampInt(incident.Confidence+5, 0, 100)

	incident.PriorityScore = computePriorityScore(*incident)

	_ = s.incidentStore.UpdateIncident(*incident)
	// Also ensure this event is linked (AddEventToIncident handles ON CONFLICT)
	_ = s.incidentStore.AddEventToIncident(incident.ID, event.ID)
}

// ─────────────────────────────────────────────────────
// Create path (new incident)
// ─────────────────────────────────────────────────────

func (s *CorrelationService) createIncident(event models.Event) {
	ts := event.Timestamp
	rootCause := buildDefaultRootCause(event)
	reasoning := buildReasoning(event, rootCause)
	confidence := initialConfidence(event.Severity)
	riskScore := initialRiskScore(event.Severity)
	correlationPattern := "new_incident"

	// Enrich with knowledge base if available
	if s.knowledgeBase != nil {
		metricName := ""
		if event.Labels != nil {
			metricName = event.Labels["metric.name"]
		}
		if entry := s.knowledgeBase.Match(event.Title, event.Message, metricName); entry != nil {
			rootCause = entry.RootCause
			reasoning = entry.Reasoning
			confidence = entry.BaseConfidence
			riskScore = entry.BaseRiskScore
			correlationPattern = "kb_match:" + entry.ID
		}
	}

	incident := models.Incident{
		ID:             generateIncidentID(),
		Title:          buildTitle(event),
		Service:        event.Service,
		Severity:       event.Severity,
		Status:         "open",
		FirstEventTime: ts,
		LastEventTime:  ts,
		EventCount:     1,
		EventIDs:       []string{event.ID},
		Fingerprint:    event.Fingerprint,
		TenantID:       event.TenantID,

		CorrelationPattern: correlationPattern,
		CorrelationScore:   50,
		CorrelationReason:  fmt.Sprintf("First alert for service '%s'", event.Service),
		Confidence:         confidence,
		RiskScore:          riskScore,
		RootCauseSummary:   rootCause,
		RootCauseType:      classifyRootCauseType(event),
		Reasoning:          reasoning,
	}

	incident.PriorityScore = computePriorityScore(incident)

	_ = s.incidentStore.AddIncident(incident)

	// Phase 4: mark onboarding milestone
	if s.onboardingService != nil {
		go func(tenantID string) {
			_ = s.onboardingService.MarkMilestone(tenantID, "incident_created")
		}(incident.TenantID)
	}

	// Phase 3: record detected_at for engineering health metrics
	if s.engineeringHealthService != nil {
		_ = s.engineeringHealthService.RecordIncidentMetrics(incident, "detected", "")
	}

	// Gap: check auto-resolve rules
	if s.autoResolveService != nil {
		resolved, ruleName := s.autoResolveService.CheckAndResolve(incident)
		if resolved {
			slog.Info("correlation: auto-resolved incident", "incident_id", incident.ID, "rule_name", ruleName)
		}
	}

	// Gap: send WhatsApp alert for critical incidents
	if incident.Severity == "critical" && s.whatsappService != nil {
		go func(inc models.Incident) {
			_ = s.whatsappService.SendIncidentAlert(inc.TenantID, inc)
		}(incident)
	}

	// SaaS Feature 7: push notification to mobile app for critical/high incidents
	if s.pushService != nil {
		go func(inc models.Incident) {
			_ = s.pushService.NotifyNewIncident(inc.TenantID, inc)
		}(incident)
	}

	// Phase 2: for critical incidents, create a Slack channel asynchronously.
	if incident.Severity == "critical" && s.slackService != nil && s.slackService.IsConfigured() {
		go func(inc models.Incident) {
			channelID, err := s.slackService.CreateIncidentChannel(inc)
			if err != nil {
				return
			}
			if channelID == "" {
				return
			}
			_ = s.slackService.PostIncidentAlert(channelID, inc, inc.RootCauseSummary)
			_ = s.incidentStore.UpdateSlackChannelID(inc.ID, channelID)
		}(incident)
	}

	// Feature 3: trigger closed-loop auto-remediation for high/critical incidents.
	if s.remediationOrchestrator != nil {
		go s.remediationOrchestrator.TriggerRemediation(incident)
	}
}

// findRelatedIncident searches for an open incident that this event should merge into.
// It checks: (a) same service, (b) dependent services, (c) same fingerprint across services.
func (s *CorrelationService) findRelatedIncident(event models.Event, windowStart time.Time) *models.Incident {
	// a) Same service (existing behavior)
	existing := s.incidentStore.FindOpenIncidentForService(event.Service, windowStart)
	if existing != nil {
		return existing
	}

	// b) Dependent services — find services that depend on or are depended upon by event.Service
	relatedServices := getRelatedServices(event.Service)
	if len(relatedServices) > 0 {
		existing = s.incidentStore.FindOpenIncidentForServices(relatedServices, windowStart)
		if existing != nil {
			return existing
		}
	}

	// c) Same fingerprint pattern across services — not found by same-service check
	// This is a lighter heuristic; we don't search all services, just return nil
	return nil
}

// getRelatedServices returns all services that are related to the given service
// via the service dependency map (both upstream and downstream).
func getRelatedServices(service string) []string {
	svcLower := strings.ToLower(service)
	related := make(map[string]bool)

	// Direct dependencies (services that this service depends on)
	if deps, ok := config.ServiceDependencies[svcLower]; ok {
		for _, dep := range deps {
			related[dep] = true
		}
	}

	// Reverse dependencies (services that depend on this service)
	for svc, deps := range config.ServiceDependencies {
		for _, dep := range deps {
			if strings.EqualFold(dep, service) {
				related[svc] = true
			}
		}
	}

	result := make([]string, 0, len(related))
	for svc := range related {
		result = append(result, svc)
	}
	return result
}

// MergeIncidents merges a child incident into a parent incident.
// All events from the child are moved to the parent, and the child is marked as merged.
func (s *CorrelationService) MergeIncidents(parentID, childID string) error {
	parent, found := s.incidentStore.GetIncidentByID(parentID)
	if !found {
		return fmt.Errorf("parent incident %q not found", parentID)
	}

	child, found := s.incidentStore.GetIncidentByID(childID)
	if !found {
		return fmt.Errorf("child incident %q not found", childID)
	}

	// Move all events from child to parent
	if err := s.incidentStore.MoveEventsToIncident(childID, parentID); err != nil {
		return fmt.Errorf("move events: %w", err)
	}

	// Mark child as merged
	child.IsMerged = true
	child.ParentIncidentID = parentID
	child.Status = "resolved"
	if err := s.incidentStore.UpdateIncident(child); err != nil {
		return fmt.Errorf("update child: %w", err)
	}

	// Update parent: append child ID to merged list, update event count
	if parent.MergedIncidentIDs == nil {
		parent.MergedIncidentIDs = []string{}
	}
	parent.MergedIncidentIDs = append(parent.MergedIncidentIDs, childID)
	parent.EventCount += child.EventCount

	// Extend time window
	if child.LastEventTime.After(parent.LastEventTime) {
		parent.LastEventTime = child.LastEventTime
	}

	// Escalate severity
	if severityWeight(child.Severity) > severityWeight(parent.Severity) {
		parent.Severity = child.Severity
	}

	parent.CorrelationPattern = "cross_service_merge"
	parent.CorrelationReason = fmt.Sprintf(
		"Merged %d incidents across services", len(parent.MergedIncidentIDs)+1,
	)

	// Reload event IDs for parent
	eventIDs, err := s.incidentStore.GetEventIDsByIncidentID(parentID)
	if err == nil {
		parent.EventIDs = eventIDs
	}

	if err := s.incidentStore.UpdateIncident(parent); err != nil {
		return fmt.Errorf("update parent: %w", err)
	}

	slog.Info("correlation: merged incident", "child_id", childID, "parent_id", parentID)
	return nil
}

func buildDefaultRootCause(event models.Event) string {
	// Use the actual event title/message as the root cause — this is real data
	// from the agent, not a generic template.
	msg := strings.TrimSpace(event.Title)
	if msg == "" {
		msg = strings.TrimSpace(event.Message)
	}
	// Strip the [SEVERITY] prefix if present (we added it in buildLogTitle)
	if idx := strings.Index(msg, "] "); idx >= 0 && idx < 12 {
		msg = strings.TrimSpace(msg[idx+2:])
	}
	// Strip service prefix "service: " if present
	if idx := strings.Index(msg, ": "); idx >= 0 && idx < 30 {
		msg = strings.TrimSpace(msg[idx+2:])
	}

	if len(msg) > 200 {
		msg = msg[:200]
	}
	if msg == "" {
		return fmt.Sprintf("%s alert on %s", titleCaseSeverity(event.Severity), event.Service)
	}
	return msg
}

func buildReasoning(event models.Event, rootCause string) []string {
	r := []string{}
	r = append(r, fmt.Sprintf("Detected %s-severity event from %s on service %s", event.Severity, event.Source, event.Service))
	if rootCause != "" {
		r = append(r, fmt.Sprintf("Root cause analysis: %s", rootCause))
	}
	if event.Source == "neuroops-agent" {
		r = append(r, "Event originated from NeuroOps agent monitoring — this is a real system signal, not a synthetic test")
	}
	msg := strings.ToLower(event.Message)
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset") {
		r = append(r, "Network connectivity failure detected — the target service or port is not accepting connections")
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
		r = append(r, "Request timeout indicates either the downstream service is overloaded or network latency is elevated")
	}
	if strings.Contains(msg, "permission denied") || strings.Contains(msg, "access denied") {
		r = append(r, "Access control failure — check credentials, certificates, or IAM policies")
	}
	if strings.Contains(msg, "out of memory") || strings.Contains(msg, "oom") {
		r = append(r, "Memory exhaustion — the process was killed by the OS OOM killer")
	}
	if strings.Contains(msg, "disk") || strings.Contains(msg, "no space left") {
		r = append(r, "Disk space exhaustion — writes are failing and services may crash")
	}
	return r
}

func classifyRootCauseType(event models.Event) string {
	text := strings.ToLower(event.Title + " " + event.Message)
	switch {
	case strings.Contains(text, "database") || strings.Contains(text, "db") || strings.Contains(text, "sql") || strings.Contains(text, "redis"):
		return "database"
	case strings.Contains(text, "timeout"):
		return "timeout"
	case strings.Contains(text, "memory") || strings.Contains(text, "oom"):
		return "memory"
	case strings.Contains(text, "cpu"):
		return "cpu"
	case strings.Contains(text, "network") || strings.Contains(text, "connect"):
		return "network"
	default:
		return "service_alert"
	}
}

// ─────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────

func buildTitle(event models.Event) string {
	if event.Title != "" {
		return event.Title
	}
	if event.Message != "" {
		if len(event.Message) > 120 {
			return event.Message[:120]
		}
		return event.Message
	}
	return fmt.Sprintf("%s incident on %s", titleCaseSeverity(event.Severity), event.Service)
}

func titleCaseSeverity(sev string) string {
	s := strings.ToLower(strings.TrimSpace(sev))
	if s == "" {
		return "Unknown"
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func severityWeight(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func initialConfidence(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 75
	case "high":
		return 65
	case "medium":
		return 55
	default:
		return 45
	}
}

func initialRiskScore(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 90
	case "high":
		return 70
	case "medium":
		return 50
	default:
		return 30
	}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func generateIncidentID() string {
	return fmt.Sprintf("inc-%d", time.Now().UnixNano())
}

func computePriorityScore(incident models.Incident) int {
	score := 0

	// Severity weight (0-40)
	switch strings.ToLower(incident.Severity) {
	case "critical":
		score += 40
	case "high":
		score += 30
	case "medium":
		score += 15
	case "low":
		score += 5
	}

	// Event count weight (0-20)
	eventScore := incident.EventCount * 4
	if eventScore > 20 {
		eventScore = 20
	}
	score += eventScore

	// Risk score contribution (0-20)
	score += incident.RiskScore / 5

	// Recurrence bonus (0-10)
	if incident.SeenBefore {
		score += 10
	}

	// Impact count (0-10)
	impactScore := incident.ImpactCount * 3
	if impactScore > 10 {
		impactScore = 10
	}
	score += impactScore

	if score > 100 {
		score = 100
	}
	return score
}
