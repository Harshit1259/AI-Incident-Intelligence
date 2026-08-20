package services

// verification_service.go — Closed-Loop Action Verification
//
// Flow:
//  1. StartVerification  — captures a "before" system snapshot and creates a
//     pending record (no checks run yet).
//  2. CompleteVerification — captures "after" snapshot, runs all checks,
//     derives proof items from before→after delta, computes confidence change,
//     auto-triggers rollback when confidence drops ≥ 20 pts, gates auto-close.
//  3. TriggerRollback    — operator-initiated; marks rollback_triggered.
//  4. AttachProof        — appends an operator- or system-supplied proof item.
//  5. VerifyIncident / VerifyAction — legacy one-shot convenience wrappers
//     (preserved for backward compatibility).

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// VerificationService handles closed-loop verification of incident resolution.
type VerificationService struct {
	store         *store.VerificationStore
	logStore      *store.LogStore
	agentStore    *store.AgentStore
	incidentStore *store.IncidentStore
}

// NewVerificationService creates a new VerificationService.
func NewVerificationService(
	vs *store.VerificationStore,
	ls *store.LogStore,
	as *store.AgentStore,
	is *store.IncidentStore,
) *VerificationService {
	return &VerificationService{
		store:         vs,
		logStore:      ls,
		agentStore:    as,
		incidentStore: is,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Public API — closed-loop lifecycle
// ─────────────────────────────────────────────────────────────────────────────

// StartVerification captures a before-snapshot and persists a pending record.
// Call this immediately before the remediation action runs.
func (s *VerificationService) StartVerification(incidentID, executionID string) (*models.VerificationRecord, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	now := time.Now()
	snap := s.captureSnapshot(incident, now)

	record := models.VerificationRecord{
		ID:               fmt.Sprintf("vr-%d", now.UnixNano()),
		TenantID:         incident.TenantID,
		IncidentID:       incidentID,
		ExecutionID:      executionID,
		Strategy:         "closed_loop",
		Status:           "pending",
		Checks:           []models.VerificationCheck{},
		Result:           "pending",
		BeforeSnapshot:   &snap,
		ConfidenceBefore: snap.IncidentConfidence,
		ProofItems:       []models.ProofItem{},
		CreatedAt:        now,
	}

	if err := s.store.Create(record); err != nil {
		return nil, fmt.Errorf("verification: create record: %w", err)
	}
	return &record, nil
}

// CompleteVerification captures an after-snapshot, runs checks, derives proof
// items from the before→after delta, and gates auto-close.
// Auto-rollback is triggered when confidence drops ≥ 20 points.
func (s *VerificationService) CompleteVerification(vrID string) (*models.VerificationRecord, error) {
	record, err := s.store.GetByID(vrID)
	if err != nil {
		return nil, err
	}

	incident, found := s.incidentStore.GetIncidentByID(record.IncidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", record.IncidentID)
	}

	now := time.Now()
	afterSnap := s.captureSnapshot(incident, now)
	record.AfterSnapshot = &afterSnap
	record.ConfidenceAfter = afterSnap.IncidentConfidence

	// Run verification checks.
	checks := []models.VerificationCheck{
		s.checkNoNewErrors(incident),
		s.checkMetricStable(incident),
		s.checkIncidentAge(incident),
	}
	if record.BeforeSnapshot != nil {
		checks = append(checks, s.checkConfidenceImproved(*record.BeforeSnapshot, afterSnap))
	}
	record.Checks = checks

	// Derive proof items from before→after delta.
	record.ProofItems = append(record.ProofItems, s.deriveProofItems(record.BeforeSnapshot, afterSnap)...)

	// Compute result and status.
	passedCount := 0
	for _, c := range checks {
		if c.Status == "passed" {
			passedCount++
		}
	}
	switch {
	case passedCount == len(checks):
		record.Result = "resolved"
		record.Status = "passed"
	case passedCount > 0:
		record.Result = "partially_resolved"
		record.Status = "passed"
	default:
		record.Result = "not_resolved"
		record.Status = "failed"
	}

	// Auto-rollback: confidence dropped ≥ 20 pts after the action.
	if record.BeforeSnapshot != nil {
		confidenceDrop := record.ConfidenceBefore - record.ConfidenceAfter
		if confidenceDrop >= 20 {
			record.RollbackTriggered = true
			slog.Warn("verification: auto-rollback triggered", "verification_id", record.ID, "confidence_drop", confidenceDrop)
		}
	}

	// Auto-close eligibility: all checks passed AND at least one improved proof item.
	hasImprovedProof := false
	for _, p := range record.ProofItems {
		if p.Status == "improved" {
			hasImprovedProof = true
			break
		}
	}
	record.AutoCloseEligible = record.Result == "resolved" && hasImprovedProof

	record.VerifiedAt = &now

	if err := s.store.Update(*record); err != nil {
		slog.Error("verification: failed to update record", "verification_id", vrID, "error", err)
	}
	return record, nil
}

// TriggerRollback marks the verification record as rollback_triggered.
func (s *VerificationService) TriggerRollback(vrID string) (*models.VerificationRecord, error) {
	record, err := s.store.GetByID(vrID)
	if err != nil {
		return nil, err
	}

	record.RollbackTriggered = true
	record.Result = "rolled_back"
	record.Status = "failed"

	now := time.Now()
	record.VerifiedAt = &now

	if err := s.store.Update(*record); err != nil {
		return nil, fmt.Errorf("verification: update rollback: %w", err)
	}
	return record, nil
}

// AttachProof appends an operator- or system-supplied proof item to the record.
func (s *VerificationService) AttachProof(vrID string, item models.ProofItem) (*models.VerificationRecord, error) {
	record, err := s.store.GetByID(vrID)
	if err != nil {
		return nil, err
	}

	if item.CapturedAt == "" {
		item.CapturedAt = time.Now().Format(time.RFC3339)
	}
	record.ProofItems = append(record.ProofItems, item)

	// Recheck auto-close eligibility whenever proof is added.
	hasImprovedProof := false
	for _, p := range record.ProofItems {
		if p.Status == "improved" {
			hasImprovedProof = true
			break
		}
	}
	record.AutoCloseEligible = record.Result == "resolved" && hasImprovedProof

	if err := s.store.Update(*record); err != nil {
		return nil, fmt.Errorf("verification: attach proof: %w", err)
	}
	return record, nil
}

// GetByIncidentID returns all verification records for an incident.
func (s *VerificationService) GetByIncidentID(incidentID string) ([]models.VerificationRecord, error) {
	return s.store.GetByIncidentID(incidentID)
}

// ─────────────────────────────────────────────────────────────────────────────
// Legacy one-shot convenience methods (backward compat)
// ─────────────────────────────────────────────────────────────────────────────

// VerifyIncident runs a one-shot verification for an incident.
func (s *VerificationService) VerifyIncident(incidentID string) (*models.VerificationRecord, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	now := time.Now()
	beforeSnap := s.captureSnapshot(incident, now.Add(-5*time.Minute))
	afterSnap := s.captureSnapshot(incident, now)

	checks := []models.VerificationCheck{
		s.checkNoNewErrors(incident),
		s.checkMetricStable(incident),
		s.checkIncidentAge(incident),
		s.checkConfidenceImproved(beforeSnap, afterSnap),
	}

	passedCount := 0
	for _, c := range checks {
		if c.Status == "passed" {
			passedCount++
		}
	}

	result, status := resolveStatus(passedCount, len(checks))
	proofItems := s.deriveProofItems(&beforeSnap, afterSnap)

	hasImprovedProof := false
	for _, p := range proofItems {
		if p.Status == "improved" {
			hasImprovedProof = true
			break
		}
	}

	record := models.VerificationRecord{
		ID:                fmt.Sprintf("vr-%d", now.UnixNano()),
		TenantID:          incident.TenantID,
		IncidentID:        incidentID,
		Strategy:          "composite",
		Status:            status,
		Checks:            checks,
		Result:            result,
		BeforeSnapshot:    &beforeSnap,
		AfterSnapshot:     &afterSnap,
		ConfidenceBefore:  beforeSnap.IncidentConfidence,
		ConfidenceAfter:   afterSnap.IncidentConfidence,
		ProofItems:        proofItems,
		AutoCloseEligible: result == "resolved" && hasImprovedProof,
		VerifiedAt:        &now,
		CreatedAt:         now,
	}

	if err := s.store.Create(record); err != nil {
		slog.Error("verification: failed to save record for incident", "incident_id", incidentID, "error", err)
	}
	return &record, nil
}

// VerifyAction runs a one-shot verification for a specific action execution.
func (s *VerificationService) VerifyAction(incidentID, executionID string) (*models.VerificationRecord, error) {
	incident, found := s.incidentStore.GetIncidentByID(incidentID)
	if !found {
		return nil, fmt.Errorf("incident not found: %s", incidentID)
	}

	now := time.Now()
	beforeSnap := s.captureSnapshot(incident, now.Add(-5*time.Minute))
	afterSnap := s.captureSnapshot(incident, now)

	checks := []models.VerificationCheck{
		s.checkNoNewErrors(incident),
		s.checkMetricStable(incident),
		s.checkConfidenceImproved(beforeSnap, afterSnap),
	}

	passedCount := 0
	for _, c := range checks {
		if c.Status == "passed" {
			passedCount++
		}
	}

	result, status := resolveStatus(passedCount, len(checks))
	proofItems := s.deriveProofItems(&beforeSnap, afterSnap)

	record := models.VerificationRecord{
		ID:               fmt.Sprintf("vr-%d", now.UnixNano()),
		TenantID:         incident.TenantID,
		IncidentID:       incidentID,
		ExecutionID:      executionID,
		Strategy:         "action_verify",
		Status:           status,
		Checks:           checks,
		Result:           result,
		BeforeSnapshot:   &beforeSnap,
		AfterSnapshot:    &afterSnap,
		ConfidenceBefore: beforeSnap.IncidentConfidence,
		ConfidenceAfter:  afterSnap.IncidentConfidence,
		ProofItems:       proofItems,
		VerifiedAt:       &now,
		CreatedAt:        now,
	}

	if err := s.store.Create(record); err != nil {
		slog.Error("verification: failed to save action verification", "execution_id", executionID, "error", err)
	}
	return &record, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Snapshot capture
// ─────────────────────────────────────────────────────────────────────────────

func (s *VerificationService) captureSnapshot(incident models.Incident, at time.Time) models.VerificationSnapshot {
	snap := models.VerificationSnapshot{
		CapturedAt:         at.Format(time.RFC3339),
		IncidentConfidence: incident.Confidence,
	}

	// Error count from log store.
	if s.logStore != nil {
		fiveMinAgo := at.Add(-5 * time.Minute)
		q := models.LogQuery{
			TenantID: incident.TenantID,
			Category: "error",
			Search:   incident.Service,
			From:     &fiveMinAgo,
			Limit:    100,
		}
		if resp, err := s.logStore.Query(q); err == nil {
			snap.ErrorCount = resp.Total
			if snap.ErrorCount > 0 {
				snap.ErrorRatePct = float64(snap.ErrorCount) / 5.0 // errors per minute
			}
		}
	}

	// CPU / memory from agent store.
	if s.agentStore != nil {
		agents, err := s.agentStore.GetAgents(incident.TenantID, 500, 0)
		if err == nil && len(agents) > 0 {
			metrics, err := s.agentStore.GetRecentMetrics(agents[0].ID, "cpu_memory", 1)
			if err == nil && len(metrics) > 0 {
				m := metrics[0]
				if v, ok := m["cpu_percent"].(float64); ok {
					snap.CPUPercent = v
				}
				if v, ok := m["memory_percent"].(float64); ok {
					snap.MemoryPercent = v
				}
				if v, ok := m["latency_ms_p99"].(float64); ok {
					snap.LatencyMsP99 = v
				}
			}
		}
	}

	return snap
}

// ─────────────────────────────────────────────────────────────────────────────
// Check functions
// ─────────────────────────────────────────────────────────────────────────────

func (s *VerificationService) checkNoNewErrors(incident models.Incident) models.VerificationCheck {
	check := models.VerificationCheck{
		Name:   "no_new_errors",
		Type:   "no_new_errors",
		Status: "skipped",
		Detail: "log store not available",
	}
	if s.logStore == nil {
		return check
	}

	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	q := models.LogQuery{
		TenantID: incident.TenantID,
		Category: "error",
		Search:   incident.Service,
		From:     &fiveMinAgo,
		Limit:    1,
	}
	resp, err := s.logStore.Query(q)
	if err != nil {
		check.Status = "skipped"
		check.Detail = fmt.Sprintf("query error: %v", err)
		return check
	}
	if resp.Total == 0 {
		check.Status = "passed"
		check.Detail = "No new error logs in the last 5 minutes"
	} else {
		check.Status = "failed"
		check.Detail = fmt.Sprintf("%d error log(s) found in the last 5 minutes", resp.Total)
	}
	return check
}

func (s *VerificationService) checkMetricStable(incident models.Incident) models.VerificationCheck {
	check := models.VerificationCheck{
		Name:   "metric_stable",
		Type:   "metric_below_threshold",
		Status: "skipped",
		Detail: "agent store not available",
	}
	if s.agentStore == nil {
		return check
	}

	agents, err := s.agentStore.GetAgents(incident.TenantID, 500, 0)
	if err != nil || len(agents) == 0 {
		check.Detail = "no agents found for tenant"
		return check
	}
	metrics, err := s.agentStore.GetRecentMetrics(agents[0].ID, "cpu_memory", 1)
	if err != nil || len(metrics) == 0 {
		check.Detail = "no recent metrics available"
		return check
	}

	m := metrics[0]
	cpuPercent, memPercent := 0.0, 0.0
	if v, ok := m["cpu_percent"].(float64); ok {
		cpuPercent = v
	}
	if v, ok := m["memory_percent"].(float64); ok {
		memPercent = v
	}

	if cpuPercent < 85 && memPercent < 90 {
		check.Status = "passed"
		check.Detail = fmt.Sprintf("CPU: %.1f%%, Memory: %.1f%% — within thresholds", cpuPercent, memPercent)
	} else {
		check.Status = "failed"
		check.Detail = fmt.Sprintf("CPU: %.1f%%, Memory: %.1f%% — exceeds thresholds (CPU<85%%, Mem<90%%)", cpuPercent, memPercent)
	}
	return check
}

func (s *VerificationService) checkIncidentAge(incident models.Incident) models.VerificationCheck {
	check := models.VerificationCheck{
		Name:   "incident_age",
		Type:   "service_healthy",
		Status: "skipped",
		Detail: "unable to determine incident age",
	}
	if incident.LastEventTime.IsZero() {
		return check
	}
	sinceLastEvent := time.Since(incident.LastEventTime)
	if sinceLastEvent > 30*time.Minute {
		check.Status = "passed"
		check.Detail = fmt.Sprintf("No new events for %d minutes — incident likely resolved",
			int(sinceLastEvent.Minutes()))
	} else {
		check.Status = "failed"
		check.Detail = fmt.Sprintf("Last event was %d minutes ago — still within active window",
			int(sinceLastEvent.Minutes()))
	}
	return check
}

func (s *VerificationService) checkConfidenceImproved(before, after models.VerificationSnapshot) models.VerificationCheck {
	delta := after.IncidentConfidence - before.IncidentConfidence
	check := models.VerificationCheck{
		Name: "confidence_improved",
		Type: "confidence_delta",
	}
	switch {
	case delta >= 10:
		check.Status = "passed"
		check.Detail = fmt.Sprintf("Confidence improved by %d pts (%d → %d)", delta, before.IncidentConfidence, after.IncidentConfidence)
	case delta >= 0:
		check.Status = "passed"
		check.Detail = fmt.Sprintf("Confidence stable at %d (delta: %+d)", after.IncidentConfidence, delta)
	default:
		check.Status = "failed"
		check.Detail = fmt.Sprintf("Confidence dropped by %d pts (%d → %d) — action may have degraded the system",
			-delta, before.IncidentConfidence, after.IncidentConfidence)
	}
	return check
}

// ─────────────────────────────────────────────────────────────────────────────
// Proof item derivation from before→after delta
// ─────────────────────────────────────────────────────────────────────────────

func (s *VerificationService) deriveProofItems(before *models.VerificationSnapshot, after models.VerificationSnapshot) []models.ProofItem {
	if before == nil {
		return nil
	}
	now := after.CapturedAt
	var items []models.ProofItem

	// Error rate comparison.
	if before.ErrorRatePct > 0 || after.ErrorRatePct > 0 {
		status := proofStatus(before.ErrorRatePct, after.ErrorRatePct, true /* lower is better */)
		items = append(items, models.ProofItem{
			Type:       "metric_drop",
			Label:      "Error rate",
			Value:      after.ErrorRatePct,
			Threshold:  before.ErrorRatePct,
			Unit:       "errors/min",
			Status:     status,
			CapturedAt: now,
		})
	}

	// CPU comparison.
	if before.CPUPercent > 0 || after.CPUPercent > 0 {
		status := proofStatus(before.CPUPercent, after.CPUPercent, true)
		items = append(items, models.ProofItem{
			Type:       "metric_drop",
			Label:      "CPU utilization",
			Value:      after.CPUPercent,
			Threshold:  before.CPUPercent,
			Unit:       "%",
			Status:     status,
			CapturedAt: now,
		})
	}

	// Memory comparison.
	if before.MemoryPercent > 0 || after.MemoryPercent > 0 {
		status := proofStatus(before.MemoryPercent, after.MemoryPercent, true)
		items = append(items, models.ProofItem{
			Type:       "metric_drop",
			Label:      "Memory utilization",
			Value:      after.MemoryPercent,
			Threshold:  before.MemoryPercent,
			Unit:       "%",
			Status:     status,
			CapturedAt: now,
		})
	}

	// Latency comparison.
	if before.LatencyMsP99 > 0 || after.LatencyMsP99 > 0 {
		status := proofStatus(before.LatencyMsP99, after.LatencyMsP99, true)
		items = append(items, models.ProofItem{
			Type:       "latency_drop",
			Label:      "Latency p99",
			Value:      after.LatencyMsP99,
			Threshold:  before.LatencyMsP99,
			Unit:       "ms",
			Status:     status,
			CapturedAt: now,
		})
	}

	// Error count cleared.
	if before.ErrorCount > 0 && after.ErrorCount == 0 {
		items = append(items, models.ProofItem{
			Type:       "error_clear",
			Label:      "Error logs cleared",
			Value:      0,
			Threshold:  float64(before.ErrorCount),
			Unit:       "errors",
			Status:     "improved",
			CapturedAt: now,
		})
	}

	return items
}

// proofStatus returns "improved", "degraded", or "neutral" comparing before→after.
// lowerIsBetter inverts the comparison for metrics like error rate.
func proofStatus(before, after float64, lowerIsBetter bool) string {
	const threshold = 0.05 // 5 % relative change required to be "improved"
	if before == 0 && after == 0 {
		return "neutral"
	}
	base := before
	if base == 0 {
		base = after
	}
	relativeDelta := (after - before) / base
	switch {
	case lowerIsBetter && relativeDelta < -threshold:
		return "improved"
	case !lowerIsBetter && relativeDelta > threshold:
		return "improved"
	case lowerIsBetter && relativeDelta > threshold:
		return "degraded"
	case !lowerIsBetter && relativeDelta < -threshold:
		return "degraded"
	default:
		return "neutral"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared helpers
// ─────────────────────────────────────────────────────────────────────────────

func resolveStatus(passedCount, total int) (result, status string) {
	switch {
	case passedCount == total:
		return "resolved", "passed"
	case passedCount > 0:
		return "partially_resolved", "passed"
	default:
		return "not_resolved", "failed"
	}
}

// summarizeResult returns a human-readable label for a verification result.
func summarizeResult(r string) string {
	switch strings.ToLower(r) {
	case "resolved":
		return "Resolved"
	case "partially_resolved":
		return "Partially resolved"
	case "rolled_back":
		return "Rolled back"
	default:
		return "Not resolved"
	}
}

// keep linter happy — summarizeResult is intentionally exported for future use
var _ = summarizeResult
