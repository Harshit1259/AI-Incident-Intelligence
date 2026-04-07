package services

import (
	"fmt"
	"strings"

	"ai-incident-platform/backend/internal/models"
)

func BuildExplanation(detail models.IncidentDetail) string {
	incident := detail.Incident
	summary := detail.Summary

	parts := make([]string, 0)

	serviceName := humanizeServiceName(incident.Service)
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "This service"
	}

	severityLabel := strings.ToLower(strings.TrimSpace(incident.Severity))
	if severityLabel == "" {
		severityLabel = "active"
	}

	parts = append(parts, fmt.Sprintf("%s is experiencing a %s incident", serviceName, severityLabel))

	if strings.TrimSpace(summary.RootCauseSummary) != "" {
		parts = append(parts, sentenceCase(summary.RootCauseSummary))
	}

	if detail.WhatChanged.Type != "" {
		changeService := humanizeServiceName(detail.WhatChanged.Service)
		if strings.TrimSpace(changeService) == "" {
			changeService = serviceName
		}

		parts = append(parts, fmt.Sprintf("This started after a %s on %s", detail.WhatChanged.Type, changeService))
	}

	if summary.ImpactCount > 1 {
		parts = append(parts, fmt.Sprintf("It is affecting %d services", summary.ImpactCount))
	} else if summary.ImpactCount == 1 {
		parts = append(parts, "It is affecting 1 service")
	}

	if summary.SeenBefore {
		parts = append(parts, fmt.Sprintf("This issue has been seen before %s", pluralizeTimes(summary.RecurringCount)))
	}

	return strings.Join(parts, ". ") + "."
}

func humanizeServiceName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	parts := strings.FieldsFunc(trimmed, splitServiceName)
	for index, part := range parts {
		if len(part) == 0 {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}

	return strings.Join(parts, " ")
}

func splitServiceName(r rune) bool {
	if r == '-' {
		return true
	}
	if r == '_' {
		return true
	}
	return false
}

func sentenceCase(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	return strings.ToUpper(trimmed[:1]) + trimmed[1:]
}

func pluralizeTimes(value int) string {
	if value == 1 {
		return "1 time"
	}
	return fmt.Sprintf("%d times", value)
}
