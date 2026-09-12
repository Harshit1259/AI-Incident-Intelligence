package models

import "time"

// Notification severities, ordered most to least urgent.
const (
	NotifySeverityCritical = "critical"
	NotifySeverityWarning  = "warning"
	NotifySeverityInfo     = "info"
)

// Notification kinds. Each kind is produced by one collector in
// NotificationService and maps to a distinct remediation for the operator.
const (
	// NotifyKindIngestRejected — payloads that reached us but failed validation
	// and were parked in the dead-letter queue.
	NotifyKindIngestRejected = "ingest_rejected"
	// NotifyKindSourceError — a registered source whose last delivery errored.
	NotifyKindSourceError = "source_error"
	// NotifyKindSourceSilent — a source that used to deliver and has gone quiet.
	NotifyKindSourceSilent = "source_silent"
	// NotifyKindAgentOffline — a NeuroOps agent that has stopped checking in.
	NotifyKindAgentOffline = "agent_offline"
)

// Notification is one actionable item in the operator notification feed.
//
// Notifications are DERIVED, not stored: every request recomputes them from
// live state. That means there is no read/unread column — a notification
// exists exactly as long as the underlying problem does, and disappears on
// its own once the problem is fixed.
//
// ID is a deterministic hash of (kind, source, cause). The same underlying
// problem therefore keeps the same ID across polls, which is what lets the
// client dismiss an item and have that dismissal stick.
type Notification struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Severity string `json:"severity"`

	// Title is the one-line summary shown in the feed.
	Title string `json:"title"`
	// Detail is the cause — an error string, or a plain-English explanation.
	Detail string `json:"detail"`
	// Source names the origin (source name, agent name) for grouping in the UI.
	Source string `json:"source"`

	// Count is how many underlying occurrences this item represents.
	// 4,000 identical rejections collapse into one item with Count=4000.
	Count int `json:"count"`

	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// Evidence is one representative raw payload, truncated. Empty when the
	// notification kind has no payload to show.
	Evidence string `json:"evidence,omitempty"`

	// ActionLabel / ActionView drive the "go fix it" link. ActionView is a
	// frontend view key (e.g. "integrations", "agents").
	ActionLabel string `json:"action_label,omitempty"`
	ActionView  string `json:"action_view,omitempty"`
}

// NotificationFeed is the response body of GET /api/v1/notifications.
// Items is always non-nil so the client never has to guard against null.
type NotificationFeed struct {
	Items         []Notification `json:"items"`
	Total         int            `json:"total"`
	CriticalCount int            `json:"critical_count"`
	WarningCount  int            `json:"warning_count"`
	GeneratedAt   time.Time      `json:"generated_at"`

	// Degraded is true when at least one collector failed. The feed is still
	// returned — a broken collector must not blank the whole panel — but the
	// client can show that the list may be incomplete.
	Degraded bool `json:"degraded"`
}
