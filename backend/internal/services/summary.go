package services

import (
	"fmt"
	"strings"

	"ai-incident-platform/backend/internal/models"
)

func GenerateIncidentSummary(incident models.Incident) string {
	service := strings.Title(incident.Service)

	switch incident.RootCauseType {

	case "database_failure":
		return fmt.Sprintf(
			"%s is degraded due to database connectivity failure, likely caused by connection issues or primary node unavailability",
			service,
		)

	case "response_timeout":
		return fmt.Sprintf(
			"%s is experiencing timeouts, likely due to slow downstream dependencies or service saturation",
			service,
		)

	case "service_degradation":
		return fmt.Sprintf(
			"%s is showing progressive degradation with failures followed by timeouts, indicating dependency or internal processing issues",
			service,
		)

	case "latency_degradation":
		return fmt.Sprintf(
			"%s is experiencing latency increase, likely due to rising load or slow external calls",
			service,
		)

	case "service_failure":
		return fmt.Sprintf(
			"%s is failing due to application errors or misconfiguration",
			service,
		)

	default:
		return fmt.Sprintf(
			"%s is experiencing an incident with correlated failure signals",
			service,
		)
	}
}
