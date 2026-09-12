package services

import (
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

func ptrTime(t time.Time) *time.Time { return &t }

// ── collectSourceProblems ────────────────────────────────────────────────────

func TestCollectSourceProblems_ReportsLiveError(t *testing.T) {
	now := time.Now().UTC()
	svc := &NotificationService{}

	got := svc.collectSourceProblems([]models.SourceConnection{{
		ID:          "src-1",
		Name:        "Datadog-prod",
		CreatedAt:   now.Add(-72 * time.Hour),
		ErrorCount:  3,
		LastError:   "signature verification failed",
		LastErrorAt: ptrTime(now.Add(-10 * time.Minute)),
	}}, now)

	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}
	if got[0].Kind != models.NotifyKindSourceError {
		t.Errorf("kind = %q, want %q", got[0].Kind, models.NotifyKindSourceError)
	}
	if got[0].Severity != models.NotifySeverityCritical {
		t.Errorf("severity = %q, want critical", got[0].Severity)
	}
	if got[0].Detail != "signature verification failed" {
		t.Errorf("detail = %q, want the source's last error", got[0].Detail)
	}
	// FirstSeen must not be after LastSeen, or the UI renders a negative age.
	if got[0].FirstSeen.After(got[0].LastSeen) {
		t.Errorf("FirstSeen %v is after LastSeen %v", got[0].FirstSeen, got[0].LastSeen)
	}
}

func TestCollectSourceProblems_IgnoresStaleError(t *testing.T) {
	now := time.Now().UTC()
	svc := &NotificationService{}

	got := svc.collectSourceProblems([]models.SourceConnection{{
		ID:          "src-1",
		Name:        "Old-source",
		ErrorCount:  3,
		LastError:   "fixed days ago",
		LastErrorAt: ptrTime(now.Add(-sourceErrorMaxAge - time.Hour)),
	}}, now)

	if len(got) != 0 {
		t.Fatalf("stale error should not notify, got %d items", len(got))
	}
}

func TestCollectSourceProblems_ReportsSilence(t *testing.T) {
	now := time.Now().UTC()
	svc := &NotificationService{}

	got := svc.collectSourceProblems([]models.SourceConnection{{
		ID:          "src-2",
		Name:        "prom-eu",
		TotalEvents: 5000,
		LastEventAt: ptrTime(now.Add(-sourceSilentAfter - time.Hour)),
	}}, now)

	if len(got) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(got))
	}
	if got[0].Kind != models.NotifyKindSourceSilent {
		t.Errorf("kind = %q, want %q", got[0].Kind, models.NotifyKindSourceSilent)
	}
	if got[0].Severity != models.NotifySeverityWarning {
		t.Errorf("severity = %q, want warning", got[0].Severity)
	}
}

// A source registered but never really used must never alarm — this is the
// guard that keeps trial accounts from showing a permanent red dot.
func TestCollectSourceProblems_IgnoresNeverUsedSource(t *testing.T) {
	now := time.Now().UTC()
	svc := &NotificationService{}

	got := svc.collectSourceProblems([]models.SourceConnection{{
		ID:          "src-3",
		Name:        "never-used",
		TotalEvents: sourceSilentMinEvents - 1,
		LastEventAt: ptrTime(now.Add(-30 * 24 * time.Hour)),
	}}, now)

	if len(got) != 0 {
		t.Fatalf("under-used source should not notify, got %d items", len(got))
	}
}

// An erroring source must produce exactly one notification, not one for the
// error and a second for the silence the error causes.
func TestCollectSourceProblems_ErrorSuppressesSilence(t *testing.T) {
	now := time.Now().UTC()
	svc := &NotificationService{}

	got := svc.collectSourceProblems([]models.SourceConnection{{
		ID:          "src-4",
		Name:        "broken-and-quiet",
		TotalEvents: 9000,
		LastEventAt: ptrTime(now.Add(-sourceSilentAfter - time.Hour)),
		ErrorCount:  12,
		LastError:   "connection refused",
		LastErrorAt: ptrTime(now.Add(-time.Minute)),
	}}, now)

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 notification, got %d", len(got))
	}
	if got[0].Kind != models.NotifyKindSourceError {
		t.Errorf("kind = %q, want the error to win over silence", got[0].Kind)
	}
}

func TestCollectSourceProblems_HealthySourceIsQuiet(t *testing.T) {
	now := time.Now().UTC()
	svc := &NotificationService{}

	got := svc.collectSourceProblems([]models.SourceConnection{{
		ID:          "src-5",
		Name:        "healthy",
		TotalEvents: 50000,
		LastEventAt: ptrTime(now.Add(-30 * time.Second)),
	}}, now)

	if len(got) != 0 {
		t.Fatalf("healthy source should produce no notifications, got %d", len(got))
	}
}

// ── Stable IDs ───────────────────────────────────────────────────────────────

// Client-side dismissal depends on the same problem yielding the same ID on
// every poll. If this breaks, dismissed items silently reappear.
func TestNotificationID_IsStableAndDistinct(t *testing.T) {
	a := notificationID(models.NotifyKindSourceError, "src-1", "boom")
	b := notificationID(models.NotifyKindSourceError, "src-1", "boom")
	if a != b {
		t.Errorf("same inputs produced different IDs: %q vs %q", a, b)
	}

	c := notificationID(models.NotifyKindSourceError, "src-2", "boom")
	if a == c {
		t.Errorf("different sources collided on ID %q", a)
	}

	// The separator must prevent ("ab","c") and ("a","bc") colliding.
	if notificationID("ab", "c") == notificationID("a", "bc") {
		t.Error("ID parts are not unambiguously separated")
	}
}

// ── Sorting ──────────────────────────────────────────────────────────────────

func TestSortNotifications_CriticalFirstThenNewest(t *testing.T) {
	now := time.Now().UTC()
	items := []models.Notification{
		{ID: "w-old", Severity: models.NotifySeverityWarning, LastSeen: now.Add(-2 * time.Hour)},
		{ID: "c-old", Severity: models.NotifySeverityCritical, LastSeen: now.Add(-3 * time.Hour)},
		{ID: "w-new", Severity: models.NotifySeverityWarning, LastSeen: now},
		{ID: "c-new", Severity: models.NotifySeverityCritical, LastSeen: now.Add(-time.Minute)},
	}
	sortNotifications(items)

	want := []string{"c-new", "c-old", "w-new", "w-old"}
	for i, id := range want {
		if items[i].ID != id {
			t.Errorf("position %d = %q, want %q (full order: %v)", i, items[i].ID, id, ids(items))
		}
	}
}

// Two items sharing a timestamp must not reshuffle between polls, or the panel
// visibly jitters on every refresh.
func TestSortNotifications_IsDeterministicOnTies(t *testing.T) {
	ts := time.Now().UTC()
	build := func() []models.Notification {
		return []models.Notification{
			{ID: "zzz", Severity: models.NotifySeverityWarning, LastSeen: ts},
			{ID: "aaa", Severity: models.NotifySeverityWarning, LastSeen: ts},
			{ID: "mmm", Severity: models.NotifySeverityWarning, LastSeen: ts},
		}
	}
	first, second := build(), build()
	sortNotifications(first)
	sortNotifications(second)

	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("tie order not deterministic: %v vs %v", ids(first), ids(second))
		}
	}
	if first[0].ID != "aaa" {
		t.Errorf("ties should break on ID ascending, got %v", ids(first))
	}
}

func ids(items []models.Notification) []string {
	out := make([]string, len(items))
	for i, n := range items {
		out[i] = n.ID
	}
	return out
}

// ── Display helpers ──────────────────────────────────────────────────────────

// Sources can be deleted while their dead-letter rows survive, so the name
// lookup must degrade instead of rendering a blank row.
func TestDisplayName_FallsBackWhenSourceDeleted(t *testing.T) {
	names := map[string]string{"src-1": "Datadog-prod"}

	if got := displayName(names, "src-1", "datadog"); got != "Datadog-prod" {
		t.Errorf("got %q, want the registered name", got)
	}
	if got := displayName(names, "src-gone", "datadog"); got != "datadog" {
		t.Errorf("got %q, want fallback to source type", got)
	}
	if got := displayName(names, "src-gone", ""); got != "src-gone" {
		t.Errorf("got %q, want fallback to source ID", got)
	}
	if got := displayName(names, "", ""); got != "unknown source" {
		t.Errorf("got %q, want the generic label", got)
	}
}

func TestHumanDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{30 * time.Second, "less than a minute"},
		{time.Minute, "1 minute"},
		{45 * time.Minute, "45 minutes"},
		{time.Hour, "1 hour"},
		{5 * time.Hour, "5 hours"},
		{72 * time.Hour, "3 days"},
	}
	for _, c := range cases {
		if got := humanDuration(c.in); got != c.want {
			t.Errorf("humanDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPluralAlerts(t *testing.T) {
	if got := pluralAlerts(1); got != "1 alert" {
		t.Errorf("got %q, want %q", got, "1 alert")
	}
	if got := pluralAlerts(4000); got != "4000 alerts" {
		t.Errorf("got %q, want %q", got, "4000 alerts")
	}
}

// ── Feed assembly ────────────────────────────────────────────────────────────

// A service with no stores wired must return an empty, non-nil feed — the JSON
// must serialise as [] so the client never has to guard against null.
func TestFeed_EmptyServiceReturnsUsableFeed(t *testing.T) {
	feed := (&NotificationService{}).Feed("tenant-1")

	if feed.Items == nil {
		t.Error("Items is nil; must be an empty slice so JSON renders []")
	}
	if feed.Total != 0 || feed.CriticalCount != 0 || feed.WarningCount != 0 {
		t.Errorf("counts should be zero, got %+v", feed)
	}
	if feed.Degraded {
		t.Error("no wired stores is not a degraded state")
	}
	if feed.GeneratedAt.IsZero() {
		t.Error("GeneratedAt was not set")
	}
}
