package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
)

// ─── Helpers ─────────────────────────────────────────────────────────────────

// okHandler always returns 200 — used as the target behind RBAC middleware.
var okHandler http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// requestWithRole builds an *http.Request whose context carries JWT claims for
// the given role.  Pass an empty string to simulate an unauthenticated request.
func requestWithRole(role string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if role == "" {
		return r // no claims → simulates missing/invalid JWT
	}
	claims := models.TokenClaims{
		UserID:   "test-user",
		TenantID: "test-tenant",
		Role:     role,
	}
	ctx := context.WithValue(r.Context(), models.ClaimsContextKey, claims)
	return r.WithContext(ctx)
}

func runHandler(handler http.HandlerFunc, r *http.Request) int {
	rr := httptest.NewRecorder()
	handler(rr, r)
	return rr.Code
}

// ─── RoleSatisfies unit tests ────────────────────────────────────────────────

func TestRoleSatisfies(t *testing.T) {
	tests := []struct {
		userRole string
		minRole  string
		want     bool
	}{
		// Admin satisfies everything
		{models.RoleAdmin, models.RoleAdmin, true},
		{models.RoleAdmin, models.RoleOperator, true},
		{models.RoleAdmin, models.RoleViewer, true},

		// Operator satisfies operator and viewer, but not admin
		{models.RoleOperator, models.RoleAdmin, false},
		{models.RoleOperator, models.RoleOperator, true},
		{models.RoleOperator, models.RoleViewer, true},

		// Viewer satisfies only viewer
		{models.RoleViewer, models.RoleAdmin, false},
		{models.RoleViewer, models.RoleOperator, false},
		{models.RoleViewer, models.RoleViewer, true},

		// Unknown role never satisfies anything
		{"superuser", models.RoleViewer, false},
		{"", models.RoleViewer, false},

		// Unknown minRole is never satisfiable
		{models.RoleAdmin, "superuser", false},
	}

	for _, tt := range tests {
		t.Run(tt.userRole+"→"+tt.minRole, func(t *testing.T) {
			got := models.RoleSatisfies(tt.userRole, tt.minRole)
			if got != tt.want {
				t.Errorf("RoleSatisfies(%q, %q) = %v, want %v", tt.userRole, tt.minRole, got, tt.want)
			}
		})
	}
}

// ─── RequireMinRole middleware tests ─────────────────────────────────────────

// TestRequireMinRole_Admin verifies that only admin can pass an admin gate.
func TestRequireMinRole_Admin(t *testing.T) {
	guard := middleware.RequireMinRole(models.RoleAdmin)(okHandler)

	if code := runHandler(guard, requestWithRole(models.RoleAdmin)); code != http.StatusOK {
		t.Errorf("admin → admin gate: got %d, want 200", code)
	}
	if code := runHandler(guard, requestWithRole(models.RoleOperator)); code != http.StatusForbidden {
		t.Errorf("operator → admin gate: got %d, want 403", code)
	}
	if code := runHandler(guard, requestWithRole(models.RoleViewer)); code != http.StatusForbidden {
		t.Errorf("viewer → admin gate: got %d, want 403", code)
	}
	if code := runHandler(guard, requestWithRole("")); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated → admin gate: got %d, want 401", code)
	}
}

// TestRequireMinRole_Operator verifies that admin and operator pass, viewer does not.
func TestRequireMinRole_Operator(t *testing.T) {
	guard := middleware.RequireMinRole(models.RoleOperator)(okHandler)

	if code := runHandler(guard, requestWithRole(models.RoleAdmin)); code != http.StatusOK {
		t.Errorf("admin → operator gate: got %d, want 200", code)
	}
	if code := runHandler(guard, requestWithRole(models.RoleOperator)); code != http.StatusOK {
		t.Errorf("operator → operator gate: got %d, want 200", code)
	}
	if code := runHandler(guard, requestWithRole(models.RoleViewer)); code != http.StatusForbidden {
		t.Errorf("viewer → operator gate: got %d, want 403", code)
	}
	if code := runHandler(guard, requestWithRole("")); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated → operator gate: got %d, want 401", code)
	}
}

// TestRequireMinRole_Viewer verifies that any authenticated role passes a viewer gate.
func TestRequireMinRole_Viewer(t *testing.T) {
	guard := middleware.RequireMinRole(models.RoleViewer)(okHandler)

	for _, role := range []string{models.RoleAdmin, models.RoleOperator, models.RoleViewer} {
		if code := runHandler(guard, requestWithRole(role)); code != http.StatusOK {
			t.Errorf("%s → viewer gate: got %d, want 200", role, code)
		}
	}
	if code := runHandler(guard, requestWithRole("")); code != http.StatusUnauthorized {
		t.Errorf("unauthenticated → viewer gate: got %d, want 401", code)
	}
}

// ─── Authorization matrix tests (by persona) ─────────────────────────────────

// TestAuthorizationMatrix exercises the full permission matrix by persona.
// Each row expresses: given this role + this gate → expect this HTTP status.
func TestAuthorizationMatrix(t *testing.T) {
	adminGate    := middleware.RequireMinRole(models.RoleAdmin)
	operatorGate := middleware.RequireMinRole(models.RoleOperator)
	viewerGate   := middleware.RequireMinRole(models.RoleViewer)

	tests := []struct {
		name     string
		gate     func(http.HandlerFunc) http.HandlerFunc
		persona  string // role string, or "" for unauthenticated
		wantCode int
		comment  string
	}{
		// ── Admin gates ──────────────────────────────────────────────────
		{"admin/admin-writes-policy", adminGate, models.RoleAdmin, 200, "admin can write policies"},
		{"operator/admin-writes-policy", adminGate, models.RoleOperator, 403, "operator cannot write policies"},
		{"viewer/admin-writes-policy", adminGate, models.RoleViewer, 403, "viewer cannot write policies"},
		{"anon/admin-writes-policy", adminGate, "", 401, "unauthenticated blocked from admin gate"},

		// ── Operator gates ───────────────────────────────────────────────
		{"admin/operator-ack-incident", operatorGate, models.RoleAdmin, 200, "admin can ack incidents"},
		{"operator/operator-ack-incident", operatorGate, models.RoleOperator, 200, "operator can ack incidents"},
		{"viewer/operator-ack-incident", operatorGate, models.RoleViewer, 403, "viewer cannot ack incidents"},
		{"anon/operator-ack-incident", operatorGate, "", 401, "unauthenticated blocked"},

		{"admin/operator-write-slo", operatorGate, models.RoleAdmin, 200, "admin can write SLOs"},
		{"operator/operator-write-slo", operatorGate, models.RoleOperator, 200, "operator can write SLOs"},
		{"viewer/operator-write-slo", operatorGate, models.RoleViewer, 403, "viewer cannot write SLOs"},

		{"admin/operator-execute-action", operatorGate, models.RoleAdmin, 200, "admin can execute actions"},
		{"operator/operator-execute-action", operatorGate, models.RoleOperator, 200, "operator can execute actions"},
		{"viewer/operator-execute-action", operatorGate, models.RoleViewer, 403, "viewer cannot execute actions"},

		{"admin/operator-billing-read", operatorGate, models.RoleAdmin, 200, "admin can read billing"},
		{"operator/operator-billing-read", operatorGate, models.RoleOperator, 200, "operator can read billing"},
		{"viewer/operator-billing-read", operatorGate, models.RoleViewer, 403, "viewer cannot read billing"},

		{"admin/operator-audit-log", operatorGate, models.RoleAdmin, 200, "admin can read audit log"},
		{"operator/operator-audit-log", operatorGate, models.RoleOperator, 200, "operator can read audit log"},
		{"viewer/operator-audit-log", operatorGate, models.RoleViewer, 403, "viewer cannot read audit log"},

		// ── Viewer gates (any authenticated role passes) ──────────────────
		{"admin/viewer-list-incidents", viewerGate, models.RoleAdmin, 200, "admin can list incidents"},
		{"operator/viewer-list-incidents", viewerGate, models.RoleOperator, 200, "operator can list incidents"},
		{"viewer/viewer-list-incidents", viewerGate, models.RoleViewer, 200, "viewer can list incidents"},
		{"anon/viewer-list-incidents", viewerGate, "", 401, "unauthenticated blocked even from viewer gate"},

		{"admin/viewer-slo-read", viewerGate, models.RoleAdmin, 200, "admin can read SLOs"},
		{"operator/viewer-slo-read", viewerGate, models.RoleOperator, 200, "operator can read SLOs"},
		{"viewer/viewer-slo-read", viewerGate, models.RoleViewer, 200, "viewer can read SLOs"},

		{"admin/viewer-risk-dashboard", viewerGate, models.RoleAdmin, 200, "admin can view risk dashboard"},
		{"operator/viewer-risk-dashboard", viewerGate, models.RoleOperator, 200, "operator can view risk dashboard"},
		{"viewer/viewer-risk-dashboard", viewerGate, models.RoleViewer, 200, "viewer can view risk dashboard"},

		// ── Unknown / legacy role ─────────────────────────────────────────
		// Claims are present (authenticated) but role is unrecognised → 403 not 401.
		// The distinction matters: 401 = no identity, 403 = insufficient permissions.
		{"unknown-role/viewer-gate", viewerGate, "superuser", 403, "unknown role with valid JWT is forbidden, not unauthenticated"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guarded := tt.gate(okHandler)
			r := requestWithRole(tt.persona)
			got := runHandler(guarded, r)
			if got != tt.wantCode {
				t.Errorf("%s: got %d, want %d — %s", tt.name, got, tt.wantCode, tt.comment)
			}
		})
	}
}

// TestValidRole ensures only known roles are accepted.
func TestValidRole(t *testing.T) {
	valid := []string{models.RoleAdmin, models.RoleOperator, models.RoleViewer}
	for _, r := range valid {
		if !models.ValidRole(r) {
			t.Errorf("ValidRole(%q) = false, want true", r)
		}
	}
	invalid := []string{"superuser", "", "root", "guest", "ADMIN"}
	for _, r := range invalid {
		if models.ValidRole(r) {
			t.Errorf("ValidRole(%q) = true, want false", r)
		}
	}
}
