package services

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/config"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type IncidentDetailService struct {
	incidentStore        *store.IncidentStore
	eventStore           *store.EventStore
	historyStore         *store.IncidentStatusHistoryStore
	knowledgeBase        *KnowledgeBase
	logStore             *store.LogStore
	agentStore           *store.AgentStore
	actionExecutionStore *store.ActionExecutionStore
	businessImpactSvc    *BusinessImpactService
}

// SetBusinessImpactService wires the live dollar-impact calculator into incident detail.
func (s *IncidentDetailService) SetBusinessImpactService(svc *BusinessImpactService) {
	s.businessImpactSvc = svc
}

func NewIncidentDetailService(
	incidentStore *store.IncidentStore,
	eventStore *store.EventStore,
	historyStore *store.IncidentStatusHistoryStore,
	logStore *store.LogStore,
	agentStore *store.AgentStore,
) *IncidentDetailService {
	return &IncidentDetailService{
		incidentStore: incidentStore,
		eventStore:    eventStore,
		historyStore:  historyStore,
		logStore:      logStore,
		agentStore:    agentStore,
	}
}

// SetKnowledgeBase wires in the knowledge base for enriched incident details.
func (s *IncidentDetailService) SetKnowledgeBase(kb *KnowledgeBase) {
	s.knowledgeBase = kb
}

func formatNullableTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func containsKeyword(text string, keywords []string) bool {
	lowerText := strings.ToLower(text)

	for _, keyword := range keywords {
		if strings.Contains(lowerText, keyword) {
			return true
		}
	}

	return false
}

func classifyIncidentPattern(events []models.Event) string {
	hasFailure := false
	hasTimeout := false
	hasLatency := false
	hasDatabase := false

	for _, event := range events {
		message := strings.ToLower(event.Message)

		if containsKeyword(message, []string{"database", "db", "sql", "connection refused", "primary node"}) {
			hasDatabase = true
		}

		if containsKeyword(message, []string{"fail", "failure", "error", "exception"}) {
			hasFailure = true
		}

		if containsKeyword(message, []string{"timeout", "timed out"}) {
			hasTimeout = true
		}

		if containsKeyword(message, []string{"latency", "slow", "delay"}) {
			hasLatency = true
		}
	}

	switch {
	case hasDatabase:
		return "database_issue"
	case hasFailure && hasTimeout:
		return "failure_then_timeout"
	case hasTimeout:
		return "timeout_only"
	case hasLatency:
		return "latency_only"
	case hasFailure:
		return "failure_only"
	default:
		return "unknown"
	}
}

func buildInsight(pattern string, incident models.Incident, events []models.Event) models.IncidentInsight {
	confidenceLabel := "low"
	if incident.Confidence >= 80 {
		confidenceLabel = "high"
	} else if incident.Confidence >= 60 {
		confidenceLabel = "medium"
	}

	baseWhy := preferReasoning(incident.Reasoning, []string{
		"Correlated signals point to an active service issue.",
	})

	if incident.SeenBefore {
		baseWhy = append(baseWhy, "A similar incident was seen previously, which increases pattern confidence.")
	}

	switch pattern {
	case "database_issue":
		return models.IncidentInsight{
			IncidentType:      "database_failure",
			LikelyRootCause:   incident.RootCauseSummary,
			WhyThisIsLikely:   append(baseWhy, "Database-oriented keywords were found in the incident event stream."),
			RecommendedChecks: []string{"Check database primary-node health and listener availability.", "Validate network connectivity between the service and the database tier.", "Inspect connection pool exhaustion and authentication failures.", "Review failover, replication, or database maintenance activity."},
			SuggestedAction:   "Start with database reachability and primary-node health before restarting the application service.",
			Confidence:        confidenceLabel,
		}

	case "failure_then_timeout":
		return models.IncidentInsight{
			IncidentType:      "service_degradation",
			LikelyRootCause:   incident.RootCauseSummary,
			WhyThisIsLikely:   append(baseWhy, "Failure-related and timeout-related signals appeared in the same incident window."),
			RecommendedChecks: []string{"Check application logs around the first failure timestamp.", "Inspect downstream database or API dependency latency.", "Check queue backlog, thread pool usage, or request saturation for the service.", "Verify whether a recent deploy or config change happened before the incident started."},
			SuggestedAction:   "Investigate dependency health and service saturation first. Do not restart blindly until logs and downstream latency are checked.",
			Confidence:        confidenceLabel,
		}

	case "timeout_only":
		return models.IncidentInsight{
			IncidentType:      "response_timeout",
			LikelyRootCause:   incident.RootCauseSummary,
			WhyThisIsLikely:   append(baseWhy, "Timeout-related signals were observed without a stronger preceding failure pattern."),
			RecommendedChecks: []string{"Check upstream and downstream response times.", "Inspect network connectivity and gateway timeout settings.", "Review CPU, memory, and concurrency saturation for the service.", "Check whether dependency calls are taking longer than normal."},
			SuggestedAction:   "Validate dependency latency and service load before considering restart or scale-up actions.",
			Confidence:        confidenceLabel,
		}

	case "latency_only":
		return models.IncidentInsight{
			IncidentType:      "latency_degradation",
			LikelyRootCause:   incident.RootCauseSummary,
			WhyThisIsLikely:   append(baseWhy, "Latency-related wording dominated the event sequence."),
			RecommendedChecks: []string{"Inspect CPU, memory, and response-time trends for the service.", "Check database query duration and external API call latency.", "Review recent traffic spikes or batch workloads hitting the service.", "Inspect any recent deployment, feature flag, or config changes."},
			SuggestedAction:   "Investigate service load and slow dependencies before taking disruptive recovery actions.",
			Confidence:        confidenceLabel,
		}

	case "failure_only":
		return models.IncidentInsight{
			IncidentType:      "service_failure",
			LikelyRootCause:   incident.RootCauseSummary,
			WhyThisIsLikely:   append(baseWhy, "Failure-related wording dominated the correlated event set."),
			RecommendedChecks: []string{"Inspect service error logs and stack traces.", "Check configuration, secrets, and environment variables.", "Validate downstream dependency availability and credentials.", "Review recent deploy or release activity for regression risk."},
			SuggestedAction:   "Start with application logs and recent change history to isolate direct failure causes.",
			Confidence:        confidenceLabel,
		}

	default:
		return models.IncidentInsight{
			IncidentType:      "unknown",
			LikelyRootCause:   incident.RootCauseSummary,
			WhyThisIsLikely:   baseWhy,
			RecommendedChecks: []string{"Inspect service logs around the incident window.", "Review recent deployment or configuration changes.", "Check resource utilization and downstream dependency health."},
			SuggestedAction:   "Gather more telemetry before concluding root cause.",
			Confidence:        confidenceLabel,
		}
	}
}

func classifySignalType(message string) string {
	msg := strings.ToLower(strings.TrimSpace(message))

	switch {
	case containsKeyword(msg, []string{"database", "db", "sql", "connection refused", "primary node"}):
		return "database"
	case containsKeyword(msg, []string{"timeout", "timed out"}):
		return "timeout"
	case containsKeyword(msg, []string{"latency", "slow", "delay"}):
		return "latency"
	case containsKeyword(msg, []string{"fail", "failure", "error", "exception"}):
		return "failure"
	default:
		return "generic"
	}
}

func classifyStage(index int, total int, signalType string) string {
	if total <= 1 {
		return "only"
	}
	if index == 0 {
		return "first"
	}
	if index == total-1 {
		return "latest"
	}
	if signalType == "timeout" || signalType == "failure" {
		return "escalation"
	}
	if signalType == "latency" {
		return "degradation"
	}
	return "progression"
}

func storyLabel(signalType string, stageType string) string {
	switch {
	case stageType == "first" && signalType == "latency":
		return "Latency increased"
	case stageType == "first" && signalType == "database":
		return "Database issue detected"
	case signalType == "timeout":
		return "Timeouts started"
	case signalType == "failure":
		return "Failures escalated"
	case signalType == "latency":
		return "Latency remained elevated"
	case signalType == "database":
		return "Database dependency remained unstable"
	default:
		return "Signal progression observed"
	}
}

func computeGap(previousTimestamp string, currentTimestamp string) string {
	previousTime, previousErr := time.Parse(time.RFC3339, previousTimestamp)
	currentTime, currentErr := time.Parse(time.RFC3339, currentTimestamp)

	if previousErr != nil || currentErr != nil {
		return ""
	}

	diff := currentTime.Sub(previousTime)
	if diff < 0 {
		diff = -diff
	}

	if diff >= time.Minute {
		return fmt.Sprintf("+%d min", int(diff.Minutes()))
	}

	return fmt.Sprintf("+%d sec", int(diff.Seconds()))
}

func buildTimeline(events []models.Event) []models.TimelineEvent {
	timeline := make([]models.TimelineEvent, 0, len(events))

	for index, event := range events {
		signalType := classifySignalType(event.Message)
		stageType := classifyStage(index, len(events), signalType)

		gap := ""
		if index > 0 {
			gap = computeGap(events[index-1].Timestamp.Format(time.RFC3339), event.Timestamp.Format(time.RFC3339))
		}

		timeline = append(timeline, models.TimelineEvent{
			Event:           event,
			SignalType:      signalType,
			StageType:       stageType,
			GapFromPrevious: gap,
			StoryLabel:      storyLabel(signalType, stageType),
		})
	}

	return timeline
}

func buildNarrative(incident models.Incident, timeline []models.TimelineEvent) string {
	recurringText := ""
	if incident.SeenBefore {
		recurringText = fmt.Sprintf(" This pattern has been seen before %d time(s).", incident.RecurringCount)
	}

	if len(timeline) == 0 {
		return incident.Service + " has no correlated timeline signals yet." + recurringText
	}

	if len(timeline) == 1 {
		return fmt.Sprintf(
			"%s generated a single %s signal. Current evidence suggests: %s.%s",
			incident.Service,
			timeline[0].SignalType,
			incident.RootCauseSummary,
			recurringText,
		)
	}

	firstSignal := timeline[0].SignalType
	latestSignal := timeline[len(timeline)-1].SignalType

	return fmt.Sprintf(
		"%s generated %d correlated signals. The incident started with %s behavior and currently ends with %s behavior. Most likely cause: %s.%s",
		incident.Service,
		len(timeline),
		firstSignal,
		latestSignal,
		incident.RootCauseSummary,
		recurringText,
	)
}

func preferReasoning(current []string, fallback []string) []string {
	if len(current) > 0 {
		return current
	}
	return fallback
}

func buildDecisionCard(incident models.Incident) models.DecisionCard {
	whatChangedLabel := "No recent change linked"
	if incident.WhatChangedType != "" {
		whatChangedLabel = fmt.Sprintf(
			"%s on %s %s",
			incident.WhatChangedType,
			incident.WhatChangedService,
			incident.WhatChangedVersion,
		)
	}

	return models.DecisionCard{
		Title:            incident.Title,
		Cause:            incident.RootCauseSummary,
		Confidence:       incident.Confidence,
		RiskScore:        incident.RiskScore,
		ImpactCount:      incident.ImpactCount,
		WhatChangedLabel: whatChangedLabel,
		Status:           incident.Status,
		Severity:         incident.Severity,
		SeenBefore:       incident.SeenBefore,
		RecurringCount:   incident.RecurringCount,
	}
}

func buildWhatChanged(incident models.Incident) models.WhatChanged {
	return models.WhatChanged{
		Type:        incident.WhatChangedType,
		Service:     incident.WhatChangedService,
		Version:     incident.WhatChangedVersion,
		Description: incident.WhatChangedDescription,
		Timestamp:   formatNullableTime(incident.WhatChangedTimestamp),
	}
}

func buildStatusAudit(records []store.IncidentStatusHistoryRecord) []models.IncidentStatusAudit {
	audit := make([]models.IncidentStatusAudit, 0, len(records))

	for _, record := range records {
		audit = append(audit, models.IncidentStatusAudit{
			ID:             record.ID,
			IncidentID:     record.IncidentID,
			PreviousStatus: record.PreviousStatus,
			NewStatus:      record.NewStatus,
			Note:           record.Note,
			ChangedBy:      record.ChangedBy,
			ChangedAt:      record.ChangedAt.Format(time.RFC3339),
		})
	}

	return audit
}

func appendUnique(values []string, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return values
	}

	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), trimmed) {
			return values
		}
	}

	return append(values, trimmed)
}

func buildEvidence(incident models.Incident, insight models.IncidentInsight, timeline []models.TimelineEvent) []string {
	evidence := []string{}

	evidence = appendUnique(evidence, incident.CorrelationReason)
	evidence = appendUnique(evidence, incident.RootCauseSummary)

	for _, reason := range incident.Reasoning {
		evidence = appendUnique(evidence, reason)
	}

	for _, reason := range insight.WhyThisIsLikely {
		evidence = appendUnique(evidence, reason)
	}

	for index, item := range timeline {
		if index >= 3 {
			break
		}
		label := strings.TrimSpace(item.StoryLabel)
		if label == "" {
			label = "Signal progression observed"
		}
		evidence = appendUnique(evidence, fmt.Sprintf("%s at %s", label, item.Event.Timestamp))
	}

	if incident.WhatChangedType != "" {
		evidence = appendUnique(
			evidence,
			fmt.Sprintf(
				"Recent %s linked to %s %s",
				incident.WhatChangedType,
				incident.WhatChangedService,
				incident.WhatChangedVersion,
			),
		)
	}

	if incident.SeenBefore {
		evidence = appendUnique(
			evidence,
			fmt.Sprintf("Pattern seen before %d time(s)", incident.RecurringCount),
		)
	}

	return evidence
}

func buildRecommendedNextStep(primaryAction *models.Action, insight models.IncidentInsight) string {
	if primaryAction != nil && strings.TrimSpace(primaryAction.Label) != "" {
		return primaryAction.Label
	}

	if strings.TrimSpace(insight.SuggestedAction) != "" {
		return insight.SuggestedAction
	}

	if len(insight.RecommendedChecks) > 0 {
		return insight.RecommendedChecks[0]
	}

	return "Review the strongest correlated evidence before taking recovery action."
}

func buildGraph(incident models.Incident) models.GraphData {
	nodes := []models.GraphNode{
		{
			ID:       incident.Service,
			Label:    incident.Service,
			NodeType: "service",
			Severity: incident.Severity,
		},
	}

	edges := []models.GraphEdge{}
	seenNodes := map[string]bool{
		incident.Service: true,
	}

	for _, dependency := range config.ServiceDependencies[incident.Service] {
		if dependency == "" {
			continue
		}

		if !seenNodes[dependency] {
			nodes = append(nodes, models.GraphNode{
				ID:       dependency,
				Label:    dependency,
				NodeType: "dependency",
			})
			seenNodes[dependency] = true
		}

		edges = append(edges, models.GraphEdge{
			From:     incident.Service,
			To:       dependency,
			Relation: "depends_on",
		})
	}

	for _, impacted := range incident.ImpactedServices {
		if impacted == "" || impacted == incident.Service {
			continue
		}

		if !seenNodes[impacted] {
			nodes = append(nodes, models.GraphNode{
				ID:       impacted,
				Label:    impacted,
				NodeType: "impacted_service",
			})
			seenNodes[impacted] = true
		}

		edges = append(edges, models.GraphEdge{
			From:     incident.Service,
			To:       impacted,
			Relation: "impacts",
		})
	}

	return models.GraphData{
		Nodes: nodes,
		Edges: edges,
	}
}

func (incidentDetailService *IncidentDetailService) GetIncidentDetail(incidentID string) (models.IncidentDetail, bool) {
	incident, found := incidentDetailService.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return models.IncidentDetail{}, false
	}

	events, err := incidentDetailService.eventStore.GetEventsByIDs(incident.EventIDs)
	if err != nil {
		slog.Warn("incident_detail: GetEventsByIDs failed — continuing with empty events", "incident_id", incidentID, "error", err)
		events = []models.Event{}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	pattern := classifyIncidentPattern(events)
	insight := buildInsight(pattern, incident, events)
	timeline := buildTimeline(events)
	impact := buildImpact(incident)
	actions := BuildActions(incident, insight)
	primaryAction := GetPrimaryAction(actions)
	narrative := buildNarrative(incident, timeline)
	graph := buildGraph(incident)
	evidence := buildEvidence(incident, insight, timeline)
	recommendedNextStep := buildRecommendedNextStep(primaryAction, insight)
	resolutionSteps := buildResolutionSteps(incident, insight, events)

	statusAudit := []models.IncidentStatusAudit{}
	if incidentDetailService.historyStore != nil {
		records, err := incidentDetailService.historyStore.GetByIncidentID(incident.TenantID, incident.ID)
		if err == nil {
			statusAudit = buildStatusAudit(records)
		}
	}

	// Enrich with knowledge base if available
	if incidentDetailService.knowledgeBase != nil {
		if entry := incidentDetailService.knowledgeBase.MatchByIncident(incident.Title, incident.RootCauseSummary, incident.Reasoning); entry != nil {
			resolutionSteps = entry.ResolutionSteps
			if len(incident.Reasoning) == 0 || (len(incident.Reasoning) == 1 && incident.Reasoning[0] == "") {
				incident.Reasoning = entry.Reasoning
			}
			evidence = append(evidence, "Prevention: "+entry.Prevention)
		}
	}

	lifecycle := buildLifecycle(incident, statusAudit)

	// Compute real recurring count from the DB — this is accurate even when
	// the seen_before flag was not set correctly at incident creation time.
	realRecurringCount := incidentDetailService.incidentStore.CountSimilarByService(incident.Service, incident.ID)
	// Use whichever value is higher: the stored count or the live count.
	recurringCount := incident.RecurringCount
	if realRecurringCount > recurringCount {
		recurringCount = realRecurringCount
	}
	seenBefore := incident.SeenBefore || recurringCount > 0

	detail := models.IncidentDetail{
		Incident: incident,
		Events:   timeline,
		Summary: models.IncidentSummary{
			EventCount:         len(events),
			Service:            incident.Service,
			Severity:           incident.Severity,
			LatestEventTime:    incident.LastEventTime.Format(time.RFC3339),
			CorrelationScore:   incident.CorrelationScore,
			CorrelationReason:  incident.CorrelationReason,
			CorrelationPattern: incident.CorrelationPattern,
			Confidence:         incident.Confidence,
			RiskScore:          incident.RiskScore,
			RootCauseSummary:   incident.RootCauseSummary,
			RootCauseType:      incident.RootCauseType,
			ImpactedServices:   incident.ImpactedServices,
			ImpactCount:        incident.ImpactCount,
			SeenBefore:         seenBefore,
			RecurringCount:     recurringCount,
			SimilarIncidentID:  incident.SimilarIncidentID,
			LastSeenAt:         formatNullableTime(incident.LastSeenAt),
			PriorityScore:      incident.PriorityScore,
		},
		Insight:             insight,
		Impact:              impact,
		Actions:             actions,
		PrimaryAction:       primaryAction,
		DecisionCard:        buildDecisionCard(incident),
		WhatChanged:         buildWhatChanged(incident),
		Narrative:           narrative,
		StatusAudit:         statusAudit,
		Graph:               graph,
		Evidence:            evidence,
		RecommendedNextStep: recommendedNextStep,
		ResolutionSteps:     resolutionSteps,
		OccurrenceTimes:     buildOccurrenceTimes(events),
		Lifecycle:           lifecycle,
	}

	// Feature 2: Context enrichment — attach recent logs
	incidentDetailService.enrichContextLogs(&detail, incident)

	// Feature 2: Context enrichment — attach recent metrics
	incidentDetailService.enrichContextMetrics(&detail, incident)

	// Enrich with action executions
	incidentDetailService.enrichActionExecutions(&detail, incident)

	// Feature 4: embed real-time dollar breakdown (nil-safe; skipped when service not wired)
	if incidentDetailService.businessImpactSvc != nil {
		if live, err := incidentDetailService.businessImpactSvc.ComputeLiveImpact(incidentID, 0); err == nil {
			detail.LiveBusinessImpact = live
		}
	}

	return detail, true
}

func buildResolutionSteps(incident models.Incident, insight models.IncidentInsight, events []models.Event) []string {
	steps := []string{}

	// Start with the insight's recommended checks
	for _, check := range insight.RecommendedChecks {
		steps = append(steps, check)
	}

	// Add context-specific resolution from the root cause
	rcLower := strings.ToLower(incident.RootCauseSummary)
	switch {
	case strings.Contains(rcLower, "timeout") || strings.Contains(rcLower, "timed out"):
		steps = append(steps, "Check downstream service health and response times")
		steps = append(steps, "Review connection pool settings and increase if saturated")
		steps = append(steps, "Check for recent deployments that may have introduced latency")
	case strings.Contains(rcLower, "connection refused") || strings.Contains(rcLower, "connection reset"):
		steps = append(steps, "Verify the target service is running: systemctl status <service>")
		steps = append(steps, "Check firewall rules and network connectivity")
		steps = append(steps, "Review service logs for crash/restart indicators")
	case strings.Contains(rcLower, "out of memory") || strings.Contains(rcLower, "oom"):
		steps = append(steps, "Check current memory usage: free -h")
		steps = append(steps, "Identify memory-hungry processes: top -o %MEM")
		steps = append(steps, "Consider increasing memory limits or adding swap")
	case strings.Contains(rcLower, "disk") || strings.Contains(rcLower, "no space"):
		steps = append(steps, "Check disk usage: df -h")
		steps = append(steps, "Find large files: du -sh /* | sort -hr | head -20")
		steps = append(steps, "Clean up old logs, temp files, or unused Docker images")
	case strings.Contains(rcLower, "cpu") || strings.Contains(rcLower, "load"):
		steps = append(steps, "Check CPU usage: top or htop")
		steps = append(steps, "Identify CPU-bound processes and check for runaway loops")
		steps = append(steps, "Consider scaling horizontally or optimizing hot code paths")
	case strings.Contains(rcLower, "permission") || strings.Contains(rcLower, "denied") || strings.Contains(rcLower, "auth"):
		steps = append(steps, "Check file/directory permissions and ownership")
		steps = append(steps, "Review authentication credentials and tokens")
		steps = append(steps, "Check if certificates have expired")
	}

	if len(steps) == 0 {
		steps = append(steps, "Review service logs for error patterns")
		steps = append(steps, "Check recent deployments and configuration changes")
		steps = append(steps, "Verify downstream dependencies are healthy")
	}

	return steps
}

func buildOccurrenceTimes(events []models.Event) []string {
	times := make([]string, 0, len(events))
	for _, e := range events {
		if !e.Timestamp.IsZero() {
			times = append(times, e.Timestamp.Format("2006-01-02 15:04:05"))
		}
	}
	return times
}

// buildLifecycle constructs a lifecycle timeline from incident data and status audit.
func buildLifecycle(incident models.Incident, statusAudit []models.IncidentStatusAudit) []models.LifecycleEntry {
	lifecycle := []models.LifecycleEntry{}

	// Always start with "detected"
	lifecycle = append(lifecycle, models.LifecycleEntry{
		Stage:     "detected",
		Timestamp: incident.FirstEventTime.Format(time.RFC3339),
		Duration:  "",
		Actor:     "correlation engine",
	})

	prevTime := incident.FirstEventTime

	// Walk through status audit entries for ack/resolve
	for _, entry := range statusAudit {
		stage := ""
		switch strings.ToLower(entry.NewStatus) {
		case "acknowledged":
			stage = "acknowledged"
		case "resolved":
			stage = "resolved"
		case "open":
			stage = "reopened"
		default:
			continue
		}

		entryTime, err := time.Parse(time.RFC3339, entry.ChangedAt)
		if err != nil {
			// try alternate format
			entryTime, err = time.Parse("2006-01-02T15:04:05Z07:00", entry.ChangedAt)
			if err != nil {
				continue
			}
		}

		duration := ""
		if !prevTime.IsZero() {
			diff := entryTime.Sub(prevTime)
			if diff >= time.Hour {
				duration = fmt.Sprintf("%.1f hours", diff.Hours())
			} else if diff >= time.Minute {
				duration = fmt.Sprintf("%d min", int(diff.Minutes()))
			} else {
				duration = fmt.Sprintf("%d sec", int(diff.Seconds()))
			}
		}

		actor := entry.ChangedBy
		if actor == "" {
			actor = "operator"
		}

		lifecycle = append(lifecycle, models.LifecycleEntry{
			Stage:     stage,
			Timestamp: entry.ChangedAt,
			Duration:  duration,
			Actor:     actor,
		})

		prevTime = entryTime
	}

	// Add "last signal" if the last event is set and differs from the first
	if !incident.LastEventTime.IsZero() && !incident.LastEventTime.Equal(incident.FirstEventTime) {
		duration := ""
		if !prevTime.IsZero() {
			diff := incident.LastEventTime.Sub(prevTime)
			if diff > 0 {
				if diff >= time.Hour {
					duration = fmt.Sprintf("%.1f hours", diff.Hours())
				} else if diff >= time.Minute {
					duration = fmt.Sprintf("%d min", int(diff.Minutes()))
				} else {
					duration = fmt.Sprintf("%d sec", int(diff.Seconds()))
				}
			}
		}
		lifecycle = append(lifecycle, models.LifecycleEntry{
			Stage:     "last signal",
			Timestamp: incident.LastEventTime.Format(time.RFC3339),
			Duration:  duration,
			Actor:     "event stream",
		})
	}

	return lifecycle
}

// enrichContextLogs attaches recent logs from the same service/host.
func (s *IncidentDetailService) enrichContextLogs(detail *models.IncidentDetail, incident models.Incident) {
	if s.logStore == nil {
		return
	}

	thirtyMinAgo := time.Now().Add(-30 * time.Minute)
	q := models.LogQuery{
		TenantID: incident.TenantID,
		Category: "", // we filter in code below
		Limit:    50,
		Offset:   0,
	}

	// Search by service name in the message
	q.Search = incident.Service
	q.From = &thirtyMinAgo

	resp, err := s.logStore.Query(q)
	if err != nil || resp == nil {
		return
	}

	contextLogs := make([]models.ContextLogEntry, 0, 20)
	for _, entry := range resp.Entries {
		cat := strings.ToLower(entry.EventCategory)
		if cat != "error" && cat != "warn" && cat != "warning" {
			continue
		}
		contextLogs = append(contextLogs, models.ContextLogEntry{
			Timestamp: entry.Timestamp.Format(time.RFC3339),
			Category:  entry.EventCategory,
			Message:   entry.Message,
			Source:    entry.LogSource,
		})
		if len(contextLogs) >= 20 {
			break
		}
	}

	detail.ContextLogs = contextLogs
}

// enrichContextMetrics attaches recent agent metrics.
func (s *IncidentDetailService) enrichContextMetrics(detail *models.IncidentDetail, incident models.Incident) {
	if s.agentStore == nil {
		return
	}

	// Try to find an agent for this incident's tenant
	agents, err := s.agentStore.GetAgents(incident.TenantID, 500, 0)
	if err != nil || len(agents) == 0 {
		return
	}

	// Use the first agent (most recently seen)
	agentID := agents[0].ID

	metrics, err := s.agentStore.GetRecentMetrics(agentID, "cpu_memory", 1)
	if err != nil || len(metrics) == 0 {
		return
	}

	m := metrics[0]
	ctx := &models.ContextMetrics{}

	if v, ok := m["cpu_percent"]; ok {
		if f, ok := v.(float64); ok {
			ctx.CPUPercent = f
		}
	}
	if v, ok := m["memory_percent"]; ok {
		if f, ok := v.(float64); ok {
			ctx.MemoryPercent = f
		}
	}
	if v, ok := m["disk_percent"]; ok {
		if f, ok := v.(float64); ok {
			ctx.DiskPercent = f
		}
	}
	if v, ok := m["load_avg_1"]; ok {
		if f, ok := v.(float64); ok {
			ctx.LoadAvg1 = f
		}
	}
	if v, ok := m["timestamp"]; ok {
		if s, ok := v.(string); ok {
			ctx.CollectedAt = s
		}
	}

	detail.ContextMetrics = ctx
}

func buildImpact(incident models.Incident) models.ImpactAnalysis {
	primary := incident.Service

	downstream := config.ServiceDependencies[primary]

	affected := append([]string{primary}, downstream...)

	impactLevel := "low"

	if incident.Severity == "critical" {
		if len(downstream) > 1 {
			impactLevel = "high"
		} else {
			impactLevel = "medium"
		}
	} else if incident.Severity == "high" && len(downstream) > 0 {
		impactLevel = "medium"
	}

	return models.ImpactAnalysis{
		PrimaryService:   primary,
		Downstream:       downstream,
		AffectedServices: affected,
		ImpactLevel:      impactLevel,
		ImpactCount:      len(affected),
	}
}

// SetActionExecutionStore wires in the action execution store.
func (s *IncidentDetailService) SetActionExecutionStore(aes *store.ActionExecutionStore) {
	s.actionExecutionStore = aes
}

// enrichActionExecutions attaches action execution records to the incident detail.
func (s *IncidentDetailService) enrichActionExecutions(detail *models.IncidentDetail, incident models.Incident) {
	if s.actionExecutionStore == nil {
		return
	}

	executions, err := s.actionExecutionStore.GetByIncidentID(incident.ID)
	if err != nil {
		slog.Error("incident_detail: failed to get action executions", "incident_id", incident.ID, "error", err)
		return
	}

	detail.ActionExecutions = executions
}
