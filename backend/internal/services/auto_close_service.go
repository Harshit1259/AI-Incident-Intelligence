// auto_close_service.go — close incidents when their alerts recover.
//
// Rules:
//  1. Each alert in an incident is firing, resolved, or "unknown" (sources
//     that never send a resolve, e.g. OTel).
//  2. When every alert in an open incident is resolved, a quiet period starts.
//  3. If any alert joins or fires again during it, the close is cancelled.
//  4. When the quiet period ends and every alert is still resolved, the
//     incident is resolved automatically with a note saying why.
//
// Incidents holding "unknown" alerts are never auto-closed. A resolve for an
// alert that is not in any open incident is ignored.
package services

import (
	"fmt"
	"log/slog"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// DefaultAutoCloseQuietPeriod is how long an incident must stay fully
// recovered before it is closed. Overridable with AUTO_CLOSE_QUIET_PERIOD.
const DefaultAutoCloseQuietPeriod = 5 * time.Minute

const autoCloseActor = "neuroops-auto-close"

type autoCloseRepo interface {
	OpenIncidentsWithEvent(tenantID, eventID string) ([]string, error)
	AlertCounts(tenantID, incidentID string) (total, unresolved int, err error)
	MarkRecovered(p store.AutoClosePending) error
	ClearRecovered(incidentID string) error
	Pending(incidentID string) (*store.AutoClosePending, error)
	ClaimDue(now time.Time, limit int) ([]store.AutoClosePending, error)
	IncidentIsOpen(tenantID, incidentID string) (bool, error)
}

// incidentAutoResolver resolves an incident on the system's behalf.
type incidentAutoResolver interface {
	UpdateIncidentStatusBy(incidentID, action, actor, note string) (models.Incident, error)
}

type AutoCloseService struct {
	store    autoCloseRepo
	resolver incidentAutoResolver
	quiet    time.Duration
	now      func() time.Time
}

func NewAutoCloseService(s *store.IncidentAutoCloseStore, resolver *IncidentService, quiet time.Duration) *AutoCloseService {
	return newAutoCloseService(s, resolver, quiet, time.Now)
}

func newAutoCloseService(s autoCloseRepo, resolver incidentAutoResolver, quiet time.Duration, now func() time.Time) *AutoCloseService {
	if quiet <= 0 {
		quiet = DefaultAutoCloseQuietPeriod
	}
	return &AutoCloseService{store: s, resolver: resolver, quiet: quiet, now: now}
}

// AlertResolved handles a "resolved" message for an alert already stored with
// status resolved. It starts the quiet period for every open incident whose
// alerts are now all resolved.
func (s *AutoCloseService) AlertResolved(e models.Event) {
	incidentIDs, err := s.store.OpenIncidentsWithEvent(e.TenantID, e.ID)
	if err != nil {
		slog.Error("auto_close: lookup incidents for resolved alert", "event_id", e.ID, "error", err)
		return
	}
	for _, id := range incidentIDs {
		total, unresolved, err := s.store.AlertCounts(e.TenantID, id)
		if err != nil {
			slog.Error("auto_close: count alerts", "incident_id", id, "error", err)
			continue
		}
		if total == 0 || unresolved > 0 {
			continue
		}
		now := s.now()
		p := store.AutoClosePending{
			IncidentID: id, TenantID: e.TenantID,
			RecoveredAt: now, ClosesAt: now.Add(s.quiet), AlertCount: total,
		}
		if err := s.store.MarkRecovered(p); err != nil {
			slog.Error("auto_close: start quiet period", "incident_id", id, "error", err)
			continue
		}
		slog.Info("auto_close: all alerts recovered, quiet period started",
			"incident_id", id, "tenant_id", e.TenantID, "closes_at", p.ClosesAt)
	}
}

// IncidentActive cancels a pending close because an alert joined or fired
// again. Safe to call for incidents with nothing pending.
func (s *AutoCloseService) IncidentActive(incidentID string) {
	if err := s.store.ClearRecovered(incidentID); err != nil {
		slog.Error("auto_close: cancel pending close", "incident_id", incidentID, "error", err)
	}
}

// PendingCloseAt returns when the incident will auto-close, or nil.
func (s *AutoCloseService) PendingCloseAt(incidentID string) *time.Time {
	p, err := s.store.Pending(incidentID)
	if err != nil || p == nil {
		return nil
	}
	return &p.ClosesAt
}

// CloseDue resolves incidents whose quiet period has ended, re-checking that
// they are still open and fully recovered. Returns how many were closed.
func (s *AutoCloseService) CloseDue() int {
	due, err := s.store.ClaimDue(s.now(), 100)
	if err != nil {
		slog.Error("auto_close: claim due incidents", "error", err)
		return 0
	}
	closed := 0
	for _, p := range due {
		open, err := s.store.IncidentIsOpen(p.TenantID, p.IncidentID)
		if err != nil || !open {
			continue // closed by hand meanwhile, or unreadable — nothing to do
		}
		total, unresolved, err := s.store.AlertCounts(p.TenantID, p.IncidentID)
		if err != nil || total == 0 || unresolved > 0 {
			continue // an alert fired again; its next resolve restarts the period
		}
		note := fmt.Sprintf("Resolved automatically: all %d alert(s) recovered and stayed quiet for %s.",
			total, s.quiet.Round(time.Second))
		if _, err := s.resolver.UpdateIncidentStatusBy(p.IncidentID, "resolve", autoCloseActor, note); err != nil {
			slog.Error("auto_close: resolve incident", "incident_id", p.IncidentID, "error", err)
			continue
		}
		slog.Info("auto_close: incident resolved", "incident_id", p.IncidentID, "tenant_id", p.TenantID, "alerts", total)
		closed++
	}
	return closed
}
