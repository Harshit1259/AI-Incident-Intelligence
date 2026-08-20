package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
)

const testSecret = "test-jwt-secret-that-is-32-chars-minimum"

func makeToken(t *testing.T, claims models.TokenClaims) string {
	t.Helper()
	tok, err := middleware.GenerateToken(claims, testSecret)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return tok
}

func TestRequireAuth_ValidToken(t *testing.T) {
	claims := models.TokenClaims{
		UserID:   "u1",
		TenantID: "tenant-a",
		Role:     models.RoleAdmin,
		Exp:      time.Now().Add(time.Hour).Unix(),
	}
	tok := makeToken(t, claims)

	called := false
	handler := middleware.RequireAuth(testSecret, func(w http.ResponseWriter, r *http.Request) {
		called = true
		got, ok := middleware.ClaimsFromContext(r)
		if !ok {
			t.Error("claims not found in context")
		}
		if got.TenantID != "tenant-a" {
			t.Errorf("TenantID: got %q, want tenant-a", got.TenantID)
		}
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	handler(rr, r)

	if !called {
		t.Error("inner handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rr.Code)
	}
}

func TestRequireAuth_MissingToken(t *testing.T) {
	handler := middleware.RequireAuth(testSecret, okHandler)

	rr := httptest.NewRecorder()
	handler(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != api.ErrCodeTokenMissing {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeTokenMissing)
	}
}

func TestRequireAuth_InvalidSignature(t *testing.T) {
	tok := makeToken(t, models.TokenClaims{
		UserID: "u1", TenantID: "t1", Role: models.RoleAdmin,
		Exp: time.Now().Add(time.Hour).Unix(),
	})

	handler := middleware.RequireAuth("wrong-secret-also-at-least-32-chars-xxx", okHandler)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	handler(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != api.ErrCodeTokenInvalid {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeTokenInvalid)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	tok := makeToken(t, models.TokenClaims{
		UserID: "u1", TenantID: "t1", Role: models.RoleAdmin,
		Exp: time.Now().Add(-time.Hour).Unix(), // already expired
	})

	handler := middleware.RequireAuth(testSecret, okHandler)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	handler(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != api.ErrCodeTokenExpired {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeTokenExpired)
	}
}

func TestRequireAuth_MalformedHeader(t *testing.T) {
	handler := middleware.RequireAuth(testSecret, okHandler)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Token notbearer")
	rr := httptest.NewRecorder()
	handler(rr, r)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status: got %d, want 401", rr.Code)
	}
}
