package services

import (
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

func TestDeriveHealthScore(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		src  models.SourceConnection
		want string
	}{
		{"never sent data", models.SourceConnection{Status: "connected"}, "waiting"},
		{"receiving", models.SourceConnection{Status: "healthy", LastEventAt: &now}, "healthy"},
		{"some errors", models.SourceConnection{LastEventAt: &now, ErrorCount: 2}, "degraded"},
		{"failing", models.SourceConnection{Status: "error"}, "error"},
	}
	for _, tt := range tests {
		if got := deriveHealthScore(tt.src); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
