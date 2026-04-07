package services

import (
	"math"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

const changeWindow = 30 * time.Minute

func MatchBestChange(incident models.Incident, changes []store.ChangeRecord) *store.ChangeRecord {
	if len(changes) == 0 {
		return nil
	}

	incidentTime, err := time.Parse(time.RFC3339, incident.FirstEventTime)
	if err != nil {
		return nil
	}

	bestScore := -1.0
	var bestChange *store.ChangeRecord

	for index := range changes {
		change := changes[index]

		changeTime, err := time.Parse(time.RFC3339, change.Timestamp)
		if err != nil {
			continue
		}

		timeDiff := math.Abs(incidentTime.Sub(changeTime).Minutes())
		if timeDiff > changeWindow.Minutes() {
			continue
		}

		score := 0.0

		score += (1.0 - (timeDiff / changeWindow.Minutes())) * 60

		if change.Service == incident.Service {
			score += 30
		}

		switch change.Type {
		case "deployment":
			score += 20
		case "config":
			score += 15
		case "infra":
			score += 10
		}

		if score > bestScore {
			bestScore = score
			bestChange = &changes[index]
		}
	}

	return bestChange
}
