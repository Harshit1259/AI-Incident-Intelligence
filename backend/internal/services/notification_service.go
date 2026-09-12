// Package services — NotificationService.
//
// The operator notification feed. It answers one question: "is anything
// silently broken in my ingest pipeline right now?"
//
// Design notes:
//
//	Derived, not stored. Every request recomputes the feed from live state.
//	There is no notifications table, no read/unread column, no background job.
//	A notification exists exactly as long as its underlying problem does and
//	vanishes on its own when the problem is fixed. That is the correct model
//	for operational alerts — a stored feed would need reconciliation logic to
//	avoid showing stale problems that were resolved hours ago.
//
//	Best-effort collection. Each collector is independent. If one store errors
//	the feed is still returned with whatever the others produced, and Degraded
//	is set. A single broken query must never blank the whole panel — that would
//	reproduce the exact silent-failure problem this feature exists to fix.
//
//	Stable IDs. Each item's ID is a hash of (kind, source, cause), so the same
//	problem keeps the same ID across polls. That is what makes client-side
//	dismissal work: dismissing an item hides it until it recurs.
package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// Detection thresholds.
//
// These are deliberately conservative. A false "your integration is broken"
// notification is worse than a missed one: it trains operators to ignore the
// bell, which costs more than the feature is worth.
const (
	// agentOfflineAfter — an agent that has not checked in for this long is
	// reported offline. Agents heartbeat far more often than this.
	agentOfflineAfter = 15 * time.Minute

	// sourceSilentAfter — a source that previously delivered and has sent
	// nothing for this long is reported silent.
	sourceSilentAfter = 24 * time.Hour

	// sourceSilentMinEvents — only sources with at least this many lifetime
	// events are eligible for silence detection. Without this, a source
	// registered during a trial and never used would alarm forever.
	sourceSilentMinEvents = 10

	// sourceErrorMaxAge — errors older than this are considered stale history,
	// not a live problem.
	sourceErrorMaxAge = 24 * time.Hour

	// rejectionsCriticalThreshold — rejection groups at or above this count are
	// raised to critical. A handful of rejects is a config typo; thousands is
	// an integration that is entirely broken.
	rejectionsCriticalThreshold = 100

	// maxRejectionGroups caps how many distinct rejection causes we surface.
	maxRejectionGroups = 25

	// maxFeedItems caps the whole feed. Beyond this the panel stops being
	// readable and the operator needs a dedicated screen, not a dropdown.
	maxFeedItems = 100
)

// NotificationService assembles the operator notification feed.
//
// Every dependency is optional. Editions that disable the agent pipeline pass
// a nil agentStore; the corresponding collector is simply skipped rather than
// panicking. Callers must still construct the service via NewNotificationService.
type NotificationService struct {
	deadLetterStore *store.DeadLetterStore
	sourceStore     *store.SourceRegistryStore
	agentStore      *store.AgentStore
}

func NewNotificationService(
	dlq *store.DeadLetterStore,
	sources *store.SourceRegistryStore,
	agents *store.AgentStore,
) *NotificationService {
	return &NotificationService{
		deadLetterStore: dlq,
		sourceStore:     sources,
		agentStore:      agents,
	}
}

// Feed returns every current notification for a tenant, most urgent first.
//
// It never returns an error. A collector that fails is logged and skipped, and
// Degraded is set on the response so the UI can say the list may be incomplete.
func (s *NotificationService) Feed(tenantID string) models.NotificationFeed {
	now := time.Now().UTC()

	feed := models.NotificationFeed{
		Items:       []models.Notification{},
		GeneratedAt: now,
	}

	// Source names are needed by two collectors, so resolve them once.
	// A failure here is not fatal: notifications fall back to the raw source ID.
	sourceNames, sources, sourcesErr := s.loadSources(tenantID)
	if sourcesErr != nil {
		feed.Degraded = true
	}

	items := make([]models.Notification, 0, 16)

	if got, err := s.collectRejections(tenantID, sourceNames, now); err != nil {
		slog.Error("notifications: rejection collector failed", "tenant_id", tenantID, "error", err)
		feed.Degraded = true
	} else {
		items = append(items, got...)
	}

	// Source collectors reuse the list already fetched above.
	if sourcesErr == nil {
		items = append(items, s.collectSourceProblems(sources, now)...)
	}

	if got, err := s.collectOfflineAgents(tenantID, now); err != nil {
		slog.Error("notifications: agent collector failed", "tenant_id", tenantID, "error", err)
		feed.Degraded = true
	} else {
		items = append(items, got...)
	}

	sortNotifications(items)

	if len(items) > maxFeedItems {
		items = items[:maxFeedItems]
	}

	feed.Items = items
	feed.Total = len(items)
	for _, n := range items {
		switch n.Severity {
		case models.NotifySeverityCritical:
			feed.CriticalCount++
		case models.NotifySeverityWarning:
			feed.WarningCount++
		}
	}

	return feed
}

// loadSources fetches the tenant's sources once and returns both a
// id → display-name lookup and the full records.
func (s *NotificationService) loadSources(tenantID string) (map[string]string, []models.SourceConnection, error) {
	if s.sourceStore == nil {
		return map[string]string{}, nil, nil
	}

	sources, err := s.sourceStore.List(tenantID)
	if err != nil {
		slog.Error("notifications: failed to list sources", "tenant_id", tenantID, "error", err)
		return map[string]string{}, nil, err
	}

	names := make(map[string]string, len(sources))
	for _, src := range sources {
		names[src.ID] = src.Name
	}
	return names, sources, nil
}

// ── Collector: rejected payloads (dead-letter queue) ─────────────────────────

func (s *NotificationService) collectRejections(
	tenantID string,
	sourceNames map[string]string,
	now time.Time,
) ([]models.Notification, error) {
	if s.deadLetterStore == nil {
		return nil, nil
	}

	groups, err := s.deadLetterStore.ListGrouped(tenantID, maxRejectionGroups)
	if err != nil {
		return nil, err
	}

	out := make([]models.Notification, 0, len(groups))
	for _, g := range groups {
		name := displayName(sourceNames, g.SourceID, g.SourceType)

		severity := models.NotifySeverityWarning
		if g.Count >= rejectionsCriticalThreshold {
			severity = models.NotifySeverityCritical
		}

		out = append(out, models.Notification{
			ID:          notificationID(models.NotifyKindIngestRejected, g.SourceID, g.Endpoint, g.Error),
			Kind:        models.NotifyKindIngestRejected,
			Severity:    severity,
			Title:       fmt.Sprintf("%s rejected from %s", pluralAlerts(g.Count), name),
			Detail:      firstNonBlank(g.Error, "payload failed validation"),
			Source:      name,
			Count:       g.Count,
			FirstSeen:   g.FirstSeen.UTC(),
			LastSeen:    g.LastSeen.UTC(),
			Evidence:    g.SamplePayload,
			ActionLabel: "View integration",
			ActionView:  "integrations",
		})
	}
	return out, nil
}

// ── Collector: source errors and silence ─────────────────────────────────────

func (s *NotificationService) collectSourceProblems(
	sources []models.SourceConnection,
	now time.Time,
) []models.Notification {
	out := make([]models.Notification, 0, 8)

	for _, src := range sources {
		name := firstNonBlank(src.Name, src.ID)

		// Live delivery error.
		if src.ErrorCount > 0 && src.LastErrorAt != nil && now.Sub(*src.LastErrorAt) <= sourceErrorMaxAge {
			first := *src.LastErrorAt
			if src.CreatedAt.Before(first) {
				first = src.CreatedAt
			}
			out = append(out, models.Notification{
				ID:          notificationID(models.NotifyKindSourceError, src.ID, src.LastError),
				Kind:        models.NotifyKindSourceError,
				Severity:    models.NotifySeverityCritical,
				Title:       fmt.Sprintf("%s is failing to deliver", name),
				Detail:      firstNonBlank(src.LastError, "delivery error"),
				Source:      name,
				Count:       src.ErrorCount,
				FirstSeen:   first.UTC(),
				LastSeen:    src.LastErrorAt.UTC(),
				ActionLabel: "View integration",
				ActionView:  "integrations",
			})
			continue // one notification per source; an erroring source is not also "silent"
		}

		// Went quiet after previously working.
		if src.TotalEvents >= sourceSilentMinEvents &&
			src.LastEventAt != nil &&
			now.Sub(*src.LastEventAt) >= sourceSilentAfter {

			out = append(out, models.Notification{
				ID:       notificationID(models.NotifyKindSourceSilent, src.ID),
				Kind:     models.NotifyKindSourceSilent,
				Severity: models.NotifySeverityWarning,
				Title:    fmt.Sprintf("%s has stopped sending", name),
				Detail: fmt.Sprintf("No events received for %s — last delivery %s.",
					humanDuration(now.Sub(*src.LastEventAt)),
					src.LastEventAt.UTC().Format("2 Jan 15:04 MST")),
				Source:      name,
				Count:       1,
				FirstSeen:   src.LastEventAt.UTC(),
				LastSeen:    src.LastEventAt.UTC(),
				ActionLabel: "View integration",
				ActionView:  "integrations",
			})
		}
	}
	return out
}

// ── Collector: offline agents ────────────────────────────────────────────────

func (s *NotificationService) collectOfflineAgents(
	tenantID string,
	now time.Time,
) ([]models.Notification, error) {
	if s.agentStore == nil {
		return nil, nil
	}

	agents, err := s.agentStore.GetAgents(tenantID, 1000, 0)
	if err != nil {
		return nil, err
	}

	out := make([]models.Notification, 0, 4)
	for _, a := range agents {
		// A never-seen agent has a zero LastSeenAt; report it against its
		// registration time instead so the age reads sensibly.
		lastSeen := a.LastSeenAt
		if lastSeen.IsZero() {
			lastSeen = a.RegisteredAt
		}
		if lastSeen.IsZero() {
			continue // no usable timestamp — nothing meaningful to report
		}

		silentFor := now.Sub(lastSeen.UTC())
		if silentFor < agentOfflineAfter {
			continue
		}

		name := firstNonBlank(a.Name, a.ID)
		host := ""
		if a.HostIP != "" {
			host = " (" + a.HostIP + ")"
		}

		out = append(out, models.Notification{
			ID:          notificationID(models.NotifyKindAgentOffline, a.ID),
			Kind:        models.NotifyKindAgentOffline,
			Severity:    models.NotifySeverityWarning,
			Title:       fmt.Sprintf("Agent %s is offline", name),
			Detail:      fmt.Sprintf("No check-in for %s%s.", humanDuration(silentFor), host),
			Source:      name,
			Count:       1,
			FirstSeen:   lastSeen.UTC(),
			LastSeen:    lastSeen.UTC(),
			ActionLabel: "View agents",
			ActionView:  "agents",
		})
	}
	return out, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// severityRank orders severities for sorting; lower is more urgent.
func severityRank(sev string) int {
	switch sev {
	case models.NotifySeverityCritical:
		return 0
	case models.NotifySeverityWarning:
		return 1
	default:
		return 2
	}
}

// sortNotifications orders by severity, then most recent, then ID.
// The ID tiebreak keeps the order stable across polls when two items share a
// timestamp — without it the list would visibly reshuffle on every refresh.
func sortNotifications(items []models.Notification) {
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := severityRank(items[i].Severity), severityRank(items[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if !items[i].LastSeen.Equal(items[j].LastSeen) {
			return items[i].LastSeen.After(items[j].LastSeen)
		}
		return items[i].ID < items[j].ID
	})
}

// notificationID builds a deterministic short ID from the parts that identify
// a distinct problem. Same problem → same ID on every poll.
func notificationID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

// displayName resolves a source ID to its registered name, falling back to the
// source type and finally to a generic label. Sources can be deleted while
// their dead-letter rows survive, so the lookup must tolerate a miss.
func displayName(names map[string]string, sourceID, sourceType string) string {
	if n, ok := names[sourceID]; ok && n != "" {
		return n
	}
	if sourceType != "" {
		return sourceType
	}
	if sourceID != "" {
		return sourceID
	}
	return "unknown source"
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func pluralAlerts(n int) string {
	if n == 1 {
		return "1 alert"
	}
	return fmt.Sprintf("%d alerts", n)
}

// humanDuration renders a duration the way an operator reads it: coarse and
// rounded, never "1h13m42.8s".
func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	if d < time.Hour {
		return plural(int(d.Minutes()), "minute")
	}
	if d < 48*time.Hour {
		return plural(int(d.Hours()), "hour")
	}
	return plural(int(d.Hours()/24), "day")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
