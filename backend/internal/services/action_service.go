package services

import (
	"fmt"
	"sort"
	"strings"

	"ai-incident-platform/backend/internal/models"
)

// knowledgeBaseForActions is a package-level reference set from main.
var knowledgeBaseForActions *KnowledgeBase

// SetKnowledgeBaseForActions sets the knowledge base used by BuildActions.
func SetKnowledgeBaseForActions(kb *KnowledgeBase) {
	knowledgeBaseForActions = kb
}

func BuildActions(incident models.Incident, insight models.IncidentInsight) []models.Action {
	actions := make([]models.Action, 0)

	// Try knowledge base for resolution-step-based actions
	if knowledgeBaseForActions != nil {
		if entry := knowledgeBaseForActions.MatchByIncident(incident.Title, incident.RootCauseSummary, incident.Reasoning); entry != nil {
			for i, step := range entry.ResolutionSteps {
				if i >= 5 {
					break // cap at 5 KB actions
				}
				actionType := "diagnostic"
				riskLevel := "low"
				requiresApproval := false
				if strings.Contains(strings.ToLower(step), "kill") || strings.Contains(strings.ToLower(step), "restart") || strings.Contains(strings.ToLower(step), "delete") {
					actionType = "remediation"
					riskLevel = "medium"
					requiresApproval = true
				}
				actions = append(actions, models.Action{
					ID:               fmt.Sprintf("kb-step-%d", i+1),
					Label:            truncateLabel(step, 60),
					Description:      step,
					Type:             actionType,
					Severity:         normalizeActionSeverity(incident.Severity),
					RiskLevel:        riskLevel,
					RequiresApproval: requiresApproval,
				})
			}
		}
	}

	// Always add pattern-based actions as fallback/supplement
	actions = append(actions, buildPrimaryDiagnosticAction(incident, insight))
	actions = append(actions, buildSecondaryDiagnosticAction(incident, insight))
	actions = append(actions, buildRemediationActions(incident, insight)...)

	if incident.SeenBefore {
		actions = append(actions, models.Action{
			ID:               "compare-last-incident",
			Label:            "Compare With Previous Incident",
			Description:      fmt.Sprintf("Review similar incident %s and compare symptoms, timing, and recovery path before taking disruptive action", safeIncidentID(incident.SimilarIncidentID)),
			Type:             "investigation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		})
	}

	if incident.WhatChangedType != "" {
		actions = append(actions, models.Action{
			ID:               "review-recent-change",
			Label:            "Review Recent Change",
			Description:      buildWhatChangedDescription(incident),
			Type:             "investigation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		})
	}

	if incident.ImpactCount >= 3 {
		actions = append(actions, models.Action{
			ID:               "notify-impacted-owners",
			Label:            "Notify Impacted Service Owners",
			Description:      fmt.Sprintf("Notify downstream owners because %d services are in the current blast radius", incident.ImpactCount),
			Type:             "coordination",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		})
	}

	if incident.RiskScore >= 80 {
		actions = append(actions, models.Action{
			ID:               "protect-business-flow",
			Label:            "Protect Business-Critical Flow",
			Description:      "Apply mitigation planning first because current incident risk is high and the blast radius may expand quickly",
			Type:             "coordination",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		})
	}

	actions = deduplicateActions(actions)
	actions = prioritizeActions(actions, incident, insight)

	// Always append a verification action as the final step — this lets
	// the operator confirm the incident is resolved before closing the ticket.
	actions = append(actions, buildVerificationAction(incident, insight))

	return actions
}

func GetPrimaryAction(actions []models.Action) *models.Action {
	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		if action.Type == "diagnostic" && strings.EqualFold(action.RiskLevel, "low") && !action.RequiresApproval {
			selected := action
			return &selected
		}
	}

	for _, action := range actions {
		if (action.Type == "investigation" || action.Type == "coordination") && strings.EqualFold(action.RiskLevel, "low") && !action.RequiresApproval {
			selected := action
			return &selected
		}
	}

	selected := actions[0]
	return &selected
}

func buildPrimaryDiagnosticAction(incident models.Incident, insight models.IncidentInsight) models.Action {
	svc := strings.TrimSpace(incident.Service)
	if svc == "" {
		svc = "app"
	}
	switch insight.IncidentType {
	case "database_failure":
		return models.Action{
			ID:               "check-db-health",
			Label:            "Check Database Health",
			Description:      fmt.Sprintf("Run: pg_isready -h localhost -p 5432 && psql -U postgres -c 'SELECT count(*) FROM pg_stat_activity;'\nAlso check: systemctl status postgresql\nLook for refused connections, auth failures, or max_connections exhaustion."),
			Type:             "diagnostic",
			Severity:         "high",
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "response_timeout":
		return models.Action{
			ID:               "check-downstream-latency",
			Label:            "Check Slow Dependency Path",
			Description:      fmt.Sprintf("Run: curl -w '%%{time_total}s\\n' -o /dev/null -s http://localhost:8080/health\nAlso: ss -tnp | grep ESTABLISHED | wc -l\nAnd: netstat -s | grep -i timeout\nMap which downstream call is adding latency."),
			Type:             "diagnostic",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "service_degradation":
		return models.Action{
			ID:               "inspect-service-logs",
			Label:            "Inspect Failure Progression in Logs",
			Description:      fmt.Sprintf("Run: journalctl -u %s -n 200 --no-pager | grep -E 'ERROR|WARN|panic|fatal'\nOr: tail -200 /var/log/%s/error.log\nFind the first error timestamp and correlate with the incident window.", svc, svc),
			Type:             "diagnostic",
			Severity:         "high",
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "latency_degradation":
		return models.Action{
			ID:               "check-load-and-latency",
			Label:            "Check CPU / Memory / Load Pressure",
			Description:      "Run: top -bn1 | head -20\nAnd: free -h\nAnd: uptime\nAnd: iostat -x 1 3\nLook for CPU steal, iowait >5%, or memory swap usage indicating resource saturation.",
			Type:             "diagnostic",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "service_failure":
		return models.Action{
			ID:               "inspect-error-logs",
			Label:            "Inspect Error Logs and Stack Traces",
			Description:      fmt.Sprintf("Run: journalctl -u %s --since '30 minutes ago' --no-pager\nOr: grep -E 'Exception|ERROR|panic' /var/log/%s/*.log | tail -50\nIdentify the exception type and the first failure line.", svc, svc),
			Type:             "diagnostic",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	default:
		return models.Action{
			ID:               "triage-incident",
			Label:            "Start Structured Triage",
			Description:      "Run: top -bn1 | head -5 && df -h && free -h\nReview the incident timeline and correlated signals in the platform before taking any action. Document findings before each step.",
			Type:             "investigation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	}
}

func buildSecondaryDiagnosticAction(incident models.Incident, insight models.IncidentInsight) models.Action {
	switch insight.IncidentType {
	case "database_failure":
		return models.Action{
			ID:               "check-db-network-path",
			Label:            "Check DB Network Path",
			Description:      "Validate network reachability between the service and database tier, including DNS, routing, and intermediate connectivity",
			Type:             "diagnostic",
			Severity:         "high",
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "response_timeout":
		return models.Action{
			ID:               "check-service-saturation",
			Label:            "Check Service Saturation",
			Description:      "Inspect thread pools, concurrency limits, queue build-up, worker starvation, and request backlog",
			Type:             "diagnostic",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "service_degradation":
		return models.Action{
			ID:               "review-recent-deploys",
			Label:            "Review Recent Deployment or Config Change",
			Description:      "Verify whether recent releases, flags, or config changes align with the start of degradation",
			Type:             "investigation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "latency_degradation":
		return models.Action{
			ID:               "review-query-and-external-latency",
			Label:            "Review Query and External Call Latency",
			Description:      "Check slow queries and external dependency calls that may be increasing service response time",
			Type:             "diagnostic",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	case "service_failure":
		return models.Action{
			ID:               "validate-config-and-secrets",
			Label:            "Validate Config and Secrets",
			Description:      "Check environment variables, secrets, token validity, and release-time configuration drift",
			Type:             "investigation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	default:
		return models.Action{
			ID:               "inspect-correlated-events",
			Label:            "Inspect Correlated Events",
			Description:      "Review each correlated event to determine whether the issue is escalating, repeating, or change-related",
			Type:             "investigation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		}
	}
}

func buildRemediationActions(incident models.Incident, insight models.IncidentInsight) []models.Action {
	actions := make([]models.Action, 0)

	switch insight.IncidentType {
	case "database_failure":
		actions = append(actions, models.Action{
			ID:               "restart-db-client-connections",
			Label:            "Restart DB Client Connections",
			Description:      "Restart connection pools or database clients only after database reachability and listener checks are complete",
			Type:             "remediation",
			Severity:         "high",
			RiskLevel:        "medium",
			RequiresApproval: true,
		})
		if incident.ImpactCount >= 3 || incident.RiskScore >= 80 {
			actions = append(actions, models.Action{
				ID:               "failover-db-path",
				Label:            "Evaluate Database Failover Path",
				Description:      "Evaluate controlled database failover or traffic reroute because blast radius and business risk are elevated",
				Type:             "remediation",
				Severity:         "critical",
				RiskLevel:        "high",
				RequiresApproval: true,
			})
		}

	case "response_timeout":
		actions = append(actions, models.Action{
			ID:               "scale-service-capacity",
			Label:            "Scale Service Capacity",
			Description:      "Increase service replicas or worker capacity if latency analysis confirms saturation instead of dependency failure",
			Type:             "remediation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "medium",
			RequiresApproval: true,
		})
		if incident.WhatChangedType != "" {
			actions = append(actions, models.Action{
				ID:               "rollback-recent-change",
				Label:            "Consider Rolling Back Recent Change",
				Description:      "Evaluate rollback because incident timing aligns with a recent deployment or configuration change",
				Type:             "remediation",
				Severity:         normalizeActionSeverity(incident.Severity),
				RiskLevel:        "high",
				RequiresApproval: true,
			})
		}

	case "service_degradation":
		actions = append(actions, models.Action{
			ID:               "restart-affected-service",
			Label:            "Restart Affected Service",
			Description:      "Restart the affected service only after checking logs, dependency health, and recent changes",
			Type:             "remediation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "high",
			RequiresApproval: true,
		})
		actions = append(actions, models.Action{
			ID:               "rollback-release",
			Label:            "Rollback Release or Flag",
			Description:      "Rollback recent release, feature flag, or config shift if degradation clearly started after change introduction",
			Type:             "remediation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "high",
			RequiresApproval: true,
		})

	case "latency_degradation":
		actions = append(actions, models.Action{
			ID:               "shed-or-throttle-load",
			Label:            "Throttle or Shed Non-Critical Load",
			Description:      "Reduce non-critical load to protect service availability while root cause is isolated",
			Type:             "remediation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "medium",
			RequiresApproval: true,
		})

	case "service_failure":
		actions = append(actions, models.Action{
			ID:               "restart-service-instance",
			Label:            "Restart Service Instance",
			Description:      "Restart the service only after validating that configuration, secrets, and dependencies are not the primary cause",
			Type:             "remediation",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "high",
			RequiresApproval: true,
		})

	default:
		actions = append(actions, models.Action{
			ID:               "manual-triage-escalation",
			Label:            "Escalate to Manual Triage",
			Description:      "Escalate for operator-led diagnosis because the current evidence does not support a safe automated remediation path",
			Type:             "generic",
			Severity:         normalizeActionSeverity(incident.Severity),
			RiskLevel:        "low",
			RequiresApproval: false,
		})
	}

	return actions
}

// buildVerificationAction returns the final "confirm resolution" action.
// It is always appended last so operators know exactly what to check before
// marking the incident resolved.
func buildVerificationAction(incident models.Incident, insight models.IncidentInsight) models.Action {
	svc := strings.TrimSpace(incident.Service)
	if svc == "" {
		svc = "app"
	}

	var verifyCmd string
	switch insight.IncidentType {
	case "database_failure":
		verifyCmd = fmt.Sprintf(
			"1. pg_isready -h localhost -p 5432  (expect: accepting connections)\n"+
				"2. psql -U postgres -c 'SELECT count(*) FROM pg_stat_activity WHERE state=$$active$$;'\n"+
				"3. Check error rate in your monitoring — confirm it returned to baseline.\n"+
				"4. If all healthy → mark incident as Resolved.",
		)
	case "response_timeout", "latency_degradation":
		verifyCmd = fmt.Sprintf(
			"1. curl -w '%%{time_total}s\\n' -o /dev/null -s http://localhost:8080/health\n"+
				"   (expect: <200ms response time)\n"+
				"2. tail -20 /var/log/%s/access.log | grep -v '2[0-9][0-9]'\n"+
				"   (expect: no 5xx errors)\n"+
				"3. Check p99 latency in monitoring — confirm it dropped to baseline.\n"+
				"4. If all healthy → mark incident as Resolved.", svc,
		)
	case "service_degradation", "service_failure":
		verifyCmd = fmt.Sprintf(
			"1. systemctl is-active %s  (expect: active)\n"+
				"2. journalctl -u %s --since '5 minutes ago' --no-pager | grep -c ERROR\n"+
				"   (expect: 0 errors)\n"+
				"3. curl -sf http://localhost:8080/health && echo OK\n"+
				"   (expect: 200 OK)\n"+
				"4. If all checks pass → mark incident as Resolved.", svc, svc,
		)
	default:
		verifyCmd = fmt.Sprintf(
			"1. Check service health: curl -sf http://localhost:8080/health && echo OK\n"+
				"2. Check system load: uptime  (expect: load avg <4.0)\n"+
				"3. Check error logs: journalctl -u %s --since '5 minutes ago' | grep -c ERROR\n"+
				"   (expect: 0)\n"+
				"4. Confirm in monitoring that the alert has cleared.\n"+
				"5. If all checks pass → mark incident as Resolved.", svc,
		)
	}

	return models.Action{
		ID:               "verify-resolution",
		Label:            "Verify Incident Is Resolved",
		Description:      verifyCmd,
		Type:             "verification",
		Severity:         normalizeActionSeverity(incident.Severity),
		RiskLevel:        "low",
		RequiresApproval: false,
	}
}

func truncateLabel(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Find last space before max to avoid cutting mid-word
	lastSpace := strings.LastIndex(s[:max], " ")
	if lastSpace > max/2 {
		return s[:lastSpace] + "..."
	}
	return s[:max] + "..."
}

func normalizeActionSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "medium":
		return "medium"
	default:
		return "low"
	}
}

func buildWhatChangedDescription(incident models.Incident) string {
	changeType := strings.TrimSpace(incident.WhatChangedType)
	changeService := strings.TrimSpace(incident.WhatChangedService)
	changeVersion := strings.TrimSpace(incident.WhatChangedVersion)

	if changeType == "" {
		return "Review change history aligned with the incident window"
	}

	if changeService == "" {
		changeService = incident.Service
	}

	if changeVersion != "" {
		return fmt.Sprintf("Review the recent %s on %s (%s) because it aligns with the incident window", changeType, changeService, changeVersion)
	}

	return fmt.Sprintf("Review the recent %s on %s because it aligns with the incident window", changeType, changeService)
}

func safeIncidentID(value string) string {
	if strings.TrimSpace(value) == "" {
		return "previous similar incident"
	}
	return value
}

func deduplicateActions(actions []models.Action) []models.Action {
	seen := map[string]bool{}
	result := make([]models.Action, 0, len(actions))

	for _, action := range actions {
		if strings.TrimSpace(action.ID) == "" {
			continue
		}
		if seen[action.ID] {
			continue
		}
		seen[action.ID] = true
		result = append(result, action)
	}

	return result
}

func prioritizeActions(actions []models.Action, incident models.Incident, insight models.IncidentInsight) []models.Action {
	type scoredAction struct {
		action models.Action
		score  int
	}

	scored := make([]scoredAction, 0, len(actions))

	for _, action := range actions {
		score := 0

		switch action.Type {
		case "diagnostic":
			score += 120
		case "investigation":
			score += 95
		case "coordination":
			score += 75
		case "remediation":
			score += 45
		default:
			score += 20
		}

		switch strings.ToLower(strings.TrimSpace(action.RiskLevel)) {
		case "low":
			score += 25
		case "medium":
			score += 8
		case "high":
			score -= 20
		}

		if !action.RequiresApproval {
			score += 15
		} else {
			score -= 10
		}

		if incident.WhatChangedType != "" && action.ID == "review-recent-change" {
			score += 45
		}

		if incident.SeenBefore && action.ID == "compare-last-incident" {
			score += 35
		}

		if incident.ImpactCount >= 3 && action.ID == "notify-impacted-owners" {
			score += 30
		}

		if incident.RiskScore >= 80 && action.ID == "protect-business-flow" {
			score += 40
		}

		if insight.IncidentType == "database_failure" && (action.ID == "check-db-health" || action.ID == "check-db-network-path") {
			score += 30
		}

		if insight.IncidentType == "response_timeout" && (action.ID == "check-downstream-latency" || action.ID == "check-service-saturation") {
			score += 25
		}

		if insight.IncidentType == "service_failure" && action.ID == "inspect-error-logs" {
			score += 25
		}

		if strings.Contains(strings.ToLower(action.Label), "restart") {
			score -= 15
		}

		if strings.Contains(strings.ToLower(action.Label), "rollback") {
			score -= 5
		}

		scored = append(scored, scoredAction{
			action: action,
			score:  score,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	result := make([]models.Action, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.action)
	}

	return result
}
