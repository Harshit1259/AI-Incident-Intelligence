package store_test

import (
	"testing"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// TestIncidentListFilter_TenantScopingField verifies that IncidentListFilter
// carries a TenantID field and that it defaults to empty string (not "default")
// so callers are forced to set it explicitly.
func TestIncidentListFilter_TenantScopingField(t *testing.T) {
	var f models.IncidentListFilter
	if f.TenantID != "" {
		t.Fatalf("IncidentListFilter.TenantID should be empty by default, got %q", f.TenantID)
	}
}

// TestIncidentListFilter_TenantIDPreserved verifies that setting TenantID on
// the filter is preserved through the struct lifecycle.
func TestIncidentListFilter_TenantIDPreserved(t *testing.T) {
	f := models.IncidentListFilter{
		TenantID: "acme",
		Page:     1,
		PageSize: 20,
	}
	if f.TenantID != "acme" {
		t.Fatalf("expected TenantID=acme, got %q", f.TenantID)
	}
}

// TestIncidentListFilter_AllFields verifies the complete filter struct can be
// populated without zero-value surprises.
func TestIncidentListFilter_AllFields(t *testing.T) {
	now := time.Now()
	f := models.IncidentListFilter{
		TenantID:  "beta-corp",
		Status:    "open",
		Severity:  "critical",
		Service:   "payments-api",
		Search:    "database",
		From:      &now,
		To:        &now,
		Page:      2,
		PageSize:  50,
		SortBy:    "priority_score",
		SortOrder: "desc",
	}

	if f.TenantID != "beta-corp" {
		t.Fatalf("TenantID mismatch")
	}
	if f.Status != "open" {
		t.Fatalf("Status mismatch")
	}
	if f.Page != 2 {
		t.Fatalf("Page mismatch")
	}
}
