package models

import "time"

// MuteAll is the single-element list value that means "every alert name" or
// "every device" in an AlertMute.
const MuteAll = "all"

// PrometheusValueLabel is the event label the Prometheus webhook copies an
// alert's "value" annotation into, so value ranges on mutes can read it.
// Customers opt in by adding `value: "{{ $value }}"` to their alert rules.
const PrometheusValueLabel = "annotation.value"

// PrometheusRunbookLabel is the event label the Prometheus webhook copies an
// alert's "runbook_url" annotation into, so responders get the link the rule
// author wrote (kube-prometheus-stack rules ship one on almost every alert).
const PrometheusRunbookLabel = "annotation.runbook_url"

// MuteDuration is how long every mute lasts. It is fixed: a mute cannot be
// extended, only ended early.
const MuteDuration = 7 * 24 * time.Hour

// Mute status values, derived from the timestamps rather than stored.
const (
	MuteStatusActive  = "active"
	MuteStatusExpired = "expired"
	MuteStatusUnmuted = "unmuted"
)

// AlertMute is an admin-created rule that stops matching alerts from creating
// or updating incidents. Matching alerts are still stored and are recorded in
// the mute log so they can be reviewed.
//
// Matching is AND across fields. AlertNames and Devices are either a list of
// exact values or []string{"all"}; Service and Environment are optional ("" =
// any). A value range applies only when ValueMin or ValueMax is set, and an
// alert without a readable value never matches a ranged mute.
type AlertMute struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenant_id"`
	AlertNames  []string `json:"alert_names"`
	Devices     []string `json:"devices"`
	Service     string   `json:"service"`
	Environment string   `json:"environment"`
	ValueMin    *float64 `json:"value_min,omitempty"`
	ValueMax    *float64 `json:"value_max,omitempty"`
	Reason      string   `json:"reason"`
	// IncidentID is set when the mute was created from "Mute incident".
	// The incident itself is never changed by a mute.
	IncidentID    string     `json:"incident_id,omitempty"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	EndsAt        time.Time  `json:"ends_at"`
	UnmutedAt     *time.Time `json:"unmuted_at,omitempty"`
	UnmutedBy     string     `json:"unmuted_by,omitempty"`
	MatchCount    int64      `json:"match_count"`
	LastMatchedAt *time.Time `json:"last_matched_at,omitempty"`
	Status        string     `json:"status"`
}

// StatusAt derives the mute's status at the given time.
func (m AlertMute) StatusAt(now time.Time) string {
	switch {
	case m.UnmutedAt != nil:
		return MuteStatusUnmuted
	case !now.Before(m.EndsAt):
		return MuteStatusExpired
	default:
		return MuteStatusActive
	}
}

// AlertMuteRequest is the body of POST /api/v1/mutes and /api/v1/mutes/preview.
type AlertMuteRequest struct {
	AlertNames  []string `json:"alert_names"`
	Devices     []string `json:"devices"`
	Service     string   `json:"service"`
	Environment string   `json:"environment"`
	ValueMin    *float64 `json:"value_min"`
	ValueMax    *float64 `json:"value_max"`
	Reason      string   `json:"reason"`
	IncidentID  string   `json:"incident_id"`
}

// AlertMuteLogEntry is one alert that a mute stopped.
type AlertMuteLogEntry struct {
	ID         int64     `json:"id"`
	TenantID   string    `json:"tenant_id"`
	MuteID     string    `json:"mute_id"`
	EventID    string    `json:"event_id"`
	AlertName  string    `json:"alert_name"`
	Device     string    `json:"device"`
	Service    string    `json:"service"`
	Value      string    `json:"value"`
	ReceivedAt time.Time `json:"received_at"`
}

// AlertMuteList is the response of GET /api/v1/mutes.
type AlertMuteList struct {
	Items       []AlertMute `json:"items"`
	ActiveCount int         `json:"active_count"`
	Muted7d     int         `json:"muted_7d"`
}

// AlertMutePreview is the response of POST /api/v1/mutes/preview: how many
// alerts from the last 7 days the mute would have caught.
type AlertMutePreview struct {
	Matched    int            `json:"matched"`
	Scanned    int            `json:"scanned"`
	WindowDays int            `json:"window_days"`
	ByDevice   map[string]int `json:"by_device"`
	// Truncated is true when the scan hit its row limit, so Matched is a floor.
	Truncated bool `json:"truncated"`
}

// AlertMuteSuggestion pre-fills the mute form from an alert or an incident.
type AlertMuteSuggestion struct {
	AlertNames  []string `json:"alert_names"`
	Devices     []string `json:"devices"`
	Service     string   `json:"service"`
	Environment string   `json:"environment"`
	// KnownDevices are other devices that sent the same alert names in the
	// last 7 days, offered as extra choices.
	KnownDevices []string `json:"known_devices"`
	IncidentID   string   `json:"incident_id,omitempty"`
	// Supported is false when none of the source alerts came through a path
	// mutes cover (Prometheus/Grafana, OTel metrics and Zabbix).
	Supported bool `json:"supported"`
}
