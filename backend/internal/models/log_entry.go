package models

import "time"

// LogEntry represents a single log entry stored in the log_entries table.
type LogEntry struct {
	ID            int64     `json:"id"`
	TenantID      string    `json:"tenant_id"`
	AgentID       string    `json:"agent_id"`
	HostIP        string    `json:"host_ip"`
	LogSource     string    `json:"log_source"`
	LogTag        string    `json:"log_tag"`
	EventType     string    `json:"event_type"`
	EventCategory string    `json:"event_category"`
	Message       string    `json:"message"`
	RawJSON       string    `json:"raw_json,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
}

// LogQuery holds the filter parameters for querying log entries.
type LogQuery struct {
	TenantID string
	AgentID  string
	HostIP   string
	Category string // info, warn, error
	Tag      string
	Search   string // substring search in message
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

// LogQueryResponse wraps the query results with pagination info.
type LogQueryResponse struct {
	Entries []LogEntry `json:"entries"`
	Total   int        `json:"total"`
	HasMore bool       `json:"has_more"`
	Query   LogQuery   `json:"-"`
}

// LogStats holds aggregate statistics for the log explorer dashboard.
type LogStats struct {
	TotalLogs    int            `json:"total_logs"`
	ByCategory   map[string]int `json:"by_category"`
	ByHost       []HostLogCount `json:"by_host"`
	ByTag        []TagLogCount  `json:"by_tag"`
	RecentErrors int            `json:"recent_errors"`
}

// HostLogCount holds the log count for a single host.
type HostLogCount struct {
	HostIP string `json:"host_ip"`
	Count  int    `json:"count"`
}

// TagLogCount holds the log count for a single tag.
type TagLogCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}
