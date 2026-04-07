package services

import "ai-incident-platform/backend/internal/models"

func CalculateConfidence(incident models.Incident) int {
	score := 0

	// correlation strength
	score += incident.CorrelationScore / 2

	// pattern strength
	switch incident.CorrelationPattern {
	case "database":
		score += 20
	case "timeout":
		score += 15
	case "failure":
		score += 15
	case "latency":
		score += 10
	default:
		score += 5
	}

	// recurring boost
	if incident.SeenBefore {
		score += 15
	}

	// change correlation boost
	if incident.WhatChangedType != "" {
		score += 15
	}

	// cap
	if score > 100 {
		score = 100
	}

	return score
}
