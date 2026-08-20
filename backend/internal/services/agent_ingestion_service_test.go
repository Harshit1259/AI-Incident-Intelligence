package services

import "testing"

func TestMatchLogSeverity_Critical(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"kernel panic at 0x1234", "critical"},
		{"panic: runtime error: index out of range", "critical"},
		{"fatal error: all goroutines are asleep", "critical"},
		{"out of memory: killed process 5678 (java)", "critical"},
		{"oom-killer invoked on behalf of process 1234", "critical"},
	}
	for _, tt := range tests {
		got := matchLogSeverity(tt.msg)
		if got != tt.want {
			t.Errorf("matchLogSeverity(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestMatchLogSeverity_High(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"connection refused to database", "high"},
		{"connection reset by peer during TLS handshake", "high"},
		{"segfault at 0x7fff12345678", "high"},
		{"permission denied for user admin", "high"},
		{"no space left on device /dev/sda1", "high"},
	}
	for _, tt := range tests {
		got := matchLogSeverity(tt.msg)
		if got != tt.want {
			t.Errorf("matchLogSeverity(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestMatchLogSeverity_Medium(t *testing.T) {
	tests := []struct {
		msg  string
		want string
	}{
		{"request timeout after 30s", "medium"},
		{"operation timed out connecting to redis", "medium"},
		{"too many open files (ulimit)", "medium"},
		{"service failed to start: unit nginx.service", "medium"},
	}
	for _, tt := range tests {
		got := matchLogSeverity(tt.msg)
		if got != tt.want {
			t.Errorf("matchLogSeverity(%q) = %q, want %q", tt.msg, got, tt.want)
		}
	}
}

func TestMatchLogSeverity_Skip(t *testing.T) {
	tests := []struct {
		msg string
	}{
		{"systemd[1]: Started session 42 of user root"},
		{"| CORE | WARN | pkg/collector/runner.go:123"},
		{"session opened for user admin by uid=0"},
		{"logrotate: rotating /var/log/syslog"},
		{"0 errors found in processing"},
		{"errors=0 warnings=0"},
		{"dbus-daemon[789]: activating service name"},
		{"snapd[123]: Starting service"},
	}
	for _, tt := range tests {
		got := matchLogSeverity(tt.msg)
		if got != "" {
			t.Errorf("matchLogSeverity(%q) = %q, should be skipped (empty)", tt.msg, got)
		}
	}
}

func TestMatchLogSeverity_BenignMessage(t *testing.T) {
	got := matchLogSeverity("normal log message nothing wrong here")
	if got != "" {
		t.Errorf("normal message should not match any severity, got %q", got)
	}
}

func TestExtractServiceFromLog(t *testing.T) {
	tests := []struct {
		msg      string
		fallback string
		want     string
	}{
		{"FATAL: payments-api: connection refused", "host1", "payments-api"},
		{"ERROR: checkout-service: timeout exceeded", "host1", "checkout-service"},
		{"CRITICAL: auth-gateway: certificate expired", "host1", "auth-gateway"},
		{"some random log without service pattern", "host1", "host1"},
		{"WARNING: data-pipeline: batch failed", "host2", "data-pipeline"},
	}
	for _, tt := range tests {
		got := extractServiceFromLog(tt.msg, tt.fallback)
		if got != tt.want {
			t.Errorf("extractServiceFromLog(%q, %q) = %q, want %q", tt.msg, tt.fallback, got, tt.want)
		}
	}
}

func TestBuildLogTitle(t *testing.T) {
	title := buildLogTitle("critical", "payments-api", "connection refused to database")
	if title == "" {
		t.Error("buildLogTitle should return a non-empty title")
	}
	if len(title) > 200 {
		t.Errorf("title too long: %d chars", len(title))
	}
}
