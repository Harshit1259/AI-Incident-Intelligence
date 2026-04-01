package services

import (
	"sort"
	"strings"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type IncidentDetailService struct {
	incidentStore *store.IncidentStore
	eventStore    *store.EventStore
}

func NewIncidentDetailService(incidentStore *store.IncidentStore, eventStore *store.EventStore) *IncidentDetailService {
	return &IncidentDetailService{
		incidentStore: incidentStore,
		eventStore:    eventStore,
	}
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

	for _, event := range events {
		message := strings.ToLower(event.Message)

		if containsKeyword(message, []string{"fail", "failure", "error"}) {
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
	switch pattern {
	case "failure_then_timeout":
		return models.IncidentInsight{
			IncidentType: "service_degradation",
			LikelyRootCause: incident.Service + " is showing progressive degradation: failure signals were followed by timeout behavior. This usually points to a slow or failing downstream dependency, internal saturation, or blocked request processing inside the service.",
			WhyThisIsLikely: []string{
				"A failure-related event occurred before or alongside a timeout-related event.",
				"Both events are grouped under the same service and severity, which suggests a single ongoing issue.",
				"The event sequence indicates degradation rather than an isolated one-off error.",
			},
			RecommendedChecks: []string{
				"Check application logs around the first failure timestamp.",
				"Inspect downstream database or API dependency latency.",
				"Check queue backlog, thread pool usage, or request saturation for the service.",
				"Verify whether a recent deploy or config change happened before the incident started.",
			},
			SuggestedAction: "Investigate dependency health and service saturation first. Do not restart blindly until logs and downstream latency are checked.",
			Confidence:      "high",
		}

	case "timeout_only":
		return models.IncidentInsight{
			IncidentType: "response_timeout",
			LikelyRootCause: incident.Service + " is experiencing timeout behavior without enough supporting failure context. This usually indicates slow downstream calls, network instability, or overloaded service processing.",
			WhyThisIsLikely: []string{
				"Timeout-related signals were observed in correlated events.",
				"No stronger preceding failure pattern was found in the current event set.",
				"Timeout-only patterns commonly indicate latency or dependency issues rather than immediate hard failure.",
			},
			RecommendedChecks: []string{
				"Check upstream and downstream response times.",
				"Inspect network connectivity and gateway timeout settings.",
				"Review CPU, memory, and concurrency saturation for the service.",
				"Check whether dependency calls are taking longer than normal.",
			},
			SuggestedAction: "Validate dependency latency and service load before considering restart or scale-up actions.",
			Confidence:      "medium",
		}

	case "latency_only":
		return models.IncidentInsight{
			IncidentType: "latency_degradation",
			LikelyRootCause: incident.Service + " is showing latency degradation. This commonly points to early resource saturation, inefficient downstream queries, or slow external service responses before full failure begins.",
			WhyThisIsLikely: []string{
				"Latency-related wording was detected in incident events.",
				"No timeout or hard failure pattern dominated the event group.",
				"Latency spikes often appear before user-visible failures or timeouts.",
			},
			RecommendedChecks: []string{
				"Inspect CPU, memory, and response-time trends for the service.",
				"Check database query duration and external API call latency.",
				"Review recent traffic spikes or batch workloads hitting the service.",
				"Inspect any recent deployment, feature flag, or config changes.",
			},
			SuggestedAction: "Investigate service load and slow dependencies before taking disruptive recovery actions.",
			Confidence:      "medium",
		}

	case "failure_only":
		return models.IncidentInsight{
			IncidentType: "service_failure",
			LikelyRootCause: incident.Service + " is reporting failure signals without a clear timeout or latency progression. This usually indicates application exceptions, configuration errors, or immediate dependency failures.",
			WhyThisIsLikely: []string{
				"Failure-related wording was detected in the correlated event set.",
				"No stronger timeout or latency progression was found.",
				"The pattern suggests direct failure rather than gradual degradation.",
			},
			RecommendedChecks: []string{
				"Inspect service error logs and stack traces.",
				"Check configuration, secrets, and environment variables.",
				"Validate downstream dependency availability and credentials.",
				"Review recent deploy or release activity for regression risk.",
			},
			SuggestedAction: "Start with application logs and recent change history to isolate direct failure causes.",
			Confidence:      "medium",
		}

	default:
		return models.IncidentInsight{
			IncidentType: "unknown",
			LikelyRootCause: incident.Service + " has correlated signals, but the current event pattern is not strong enough to identify a specific cause with confidence.",
			WhyThisIsLikely: []string{
				"The correlated event set does not match a known RCA template strongly enough.",
				"Additional telemetry such as logs, metrics, deployment events, or dependency health would improve the diagnosis.",
			},
			RecommendedChecks: []string{
				"Inspect service logs around the incident window.",
				"Review recent deployment or configuration changes.",
				"Check resource utilization and downstream dependency health.",
			},
			SuggestedAction: "Gather more telemetry before concluding root cause.",
			Confidence:      "low",
		}
	}
}

func (incidentDetailService *IncidentDetailService) GetIncidentDetail(incidentID string) (models.IncidentDetail, bool) {
	incident, found := incidentDetailService.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return models.IncidentDetail{}, false
	}

	events := incidentDetailService.eventStore.GetEventsByIDs(incident.EventIDs)

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp < events[j].Timestamp
	})

	pattern := classifyIncidentPattern(events)
	insight := buildInsight(pattern, incident, events)

	detail := models.IncidentDetail{
		Incident: incident,
		Events:   events,
		Summary: models.IncidentSummary{
			EventCount:      len(events),
			Service:         incident.Service,
			Severity:        incident.Severity,
			LatestEventTime: incident.LastEventTime,
		},
		Insight: insight,
	}

	return detail, true
}
