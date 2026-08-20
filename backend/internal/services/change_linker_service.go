package services

import (
	"log/slog"
	"regexp"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// ChangeLinkerService links GitHub/GitLab/deploy events to open incidents.
type ChangeLinkerService struct {
	incidentStore *store.IncidentStore
	changeStore   *store.ChangeStore
	ciStore       *store.ChangeIntelligenceStore // optional; nil = no enriched persistence
}

func NewChangeLinkerService(is *store.IncidentStore, cs *store.ChangeStore) *ChangeLinkerService {
	return &ChangeLinkerService{
		incidentStore: is,
		changeStore:   cs,
	}
}

// SetChangeIntelligenceStore wires in the enriched store so webhooks persist
// author, commit SHA, environment, etc. alongside the bare change record.
func (s *ChangeLinkerService) SetChangeIntelligenceStore(ci *store.ChangeIntelligenceStore) {
	s.ciStore = ci
}

// semverPattern matches semver-like version strings (v1.2.3, 1.2.3, 1.2.3-beta).
var semverPattern = regexp.MustCompile(`^v?\d+\.\d+`)

// LinkDeployEnriched stores an enriched ChangeEvent, links it to any open
// incident, and back-fills the incident's WhatChanged fields.
// Returns the incident that was updated, or nil.
func (s *ChangeLinkerService) LinkDeployEnriched(c models.ChangeEvent) *models.Incident {
	// Persist via the enriched store when available, else fall back to bare store.
	if s.ciStore != nil {
		if _, err := s.ciStore.AddEnrichedChange(c); err != nil {
			slog.Error("change_linker: enriched add failed", "service", c.Service, "error", err)
			// fallthrough to bare store
		} else {
			return s.linkToIncident(c.TenantID, c.Service, c.Type, c.Version, c.Description, c.Timestamp)
		}
	}
	if err := s.changeStore.AddChange(c.TenantID, c.Service, c.Type, c.Version, c.Description, c.Timestamp); err != nil {
		slog.Error("change_linker: failed to add change record (enriched fallback)", "service", c.Service, "error", err)
	}
	return s.linkToIncident(c.TenantID, c.Service, c.Type, c.Version, c.Description, c.Timestamp)
}

// LinkDeploy stores the change record and auto-links it to any open incident
// for the same service within the last 30 minutes.
// Returns the incident that was updated, or nil if none was found.
func (s *ChangeLinkerService) LinkDeploy(tenantID, service, changeType, version, description string, ts time.Time) *models.Incident {
	// 1. Persist the change record.
	if err := s.changeStore.AddChange(tenantID, service, changeType, version, description, ts); err != nil {
		slog.Error("change_linker: failed to add change record", "service", service, "error", err)
	}
	return s.linkToIncident(tenantID, service, changeType, version, description, ts)
}

// linkToIncident finds the matching open incident and updates WhatChanged fields.
func (s *ChangeLinkerService) linkToIncident(tenantID, service, changeType, version, description string, ts time.Time) *models.Incident {

	// Look for an open incident within the last 30 minutes.
	windowStart := ts.Add(-30 * time.Minute)
	incident := s.incidentStore.FindOpenIncidentForService(service, windowStart)
	if incident == nil {
		return nil
	}

	confidence := computeLinkConfidence(service, incident, ts, changeType, version)

	incident.WhatChangedType = changeType
	incident.WhatChangedService = service
	incident.WhatChangedVersion = version
	incident.WhatChangedDescription = description
	incident.WhatChangedTimestamp = &ts
	incident.WhatChangedConfidence = confidence

	if err := s.incidentStore.UpdateIncident(*incident); err != nil {
		slog.Error("change_linker: failed to update incident with change data", "incident_id", incident.ID, "error", err)
		return nil
	}

	slog.Info("change_linker: linked deploy to incident", "service", service, "version", version, "change_type", changeType, "incident_id", incident.ID, "confidence", confidence)
	return incident
}

// computeLinkConfidence returns 0-100 score for how confident we are that the
// given change caused the given incident.
//
// Scoring factors:
//   - Temporal proximity: closer to incident start = higher score (max 60)
//   - Change type quality: deployment > release > commit (max 20)
//   - Version format: semver or full SHA8+ = structured version (max 10)
//   - Service name match quality: exact vs. normalised match (max 10)
func computeLinkConfidence(service string, incident *models.Incident, changeTS time.Time, changeType, version string) int {
	score := 0

	// — Temporal proximity (0-60) ———————————————
	incidentStart := incident.FirstEventTime
	var gap time.Duration
	if !incidentStart.IsZero() {
		gap = changeTS.Sub(incidentStart)
		if gap < 0 {
			gap = -gap // change may have come just before or just after
		}
	}

	switch {
	case gap <= 2*time.Minute:
		score += 60
	case gap <= 5*time.Minute:
		score += 50
	case gap <= 10*time.Minute:
		score += 40
	case gap <= 15*time.Minute:
		score += 30
	case gap <= 20*time.Minute:
		score += 20
	default:
		score += 10
	}

	// — Change type quality (0-20) ——————————————
	switch strings.ToLower(changeType) {
	case "deployment":
		score += 20
	case "release":
		score += 15
	case "commit":
		score += 8
	default:
		score += 5
	}

	// — Version quality (0-10) ——————————————————
	if semverPattern.MatchString(version) {
		score += 10 // structured semver
	} else if len(version) >= 8 {
		score += 7 // git SHA or equivalent
	} else if version != "" {
		score += 3
	}

	// — Service name match quality (0-10) ———————
	// Exact match already guaranteed by FindOpenIncidentForService; check for
	// a canonical prefix match (e.g. "auth" matching "auth-service").
	incSvc := strings.ToLower(strings.TrimSpace(incident.Service))
	chSvc := strings.ToLower(strings.TrimSpace(service))
	if incSvc == chSvc {
		score += 10
	} else if strings.HasPrefix(incSvc, chSvc) || strings.HasPrefix(chSvc, incSvc) {
		score += 6
	}

	if score > 100 {
		score = 100
	}
	return score
}
