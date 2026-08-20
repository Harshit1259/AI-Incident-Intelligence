package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-incident-platform/backend/internal/api"
)

func TestWriteError_IncludesCodeField(t *testing.T) {
	rr := httptest.NewRecorder()
	api.WriteError(rr, http.StatusNotFound, "resource not found")

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", rr.Code)
	}

	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body.Code == "" {
		t.Error("Code field must not be empty")
	}
	if body.Code != api.ErrCodeNotFound {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeNotFound)
	}
	if body.Error == "" {
		t.Error("Error field must not be empty")
	}
}

func TestWriteErrorCode_UsesSpecificCode(t *testing.T) {
	rr := httptest.NewRecorder()
	api.WriteErrorCode(rr, http.StatusForbidden, "tenant suspended", api.ErrCodeTenantSuspended)

	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != api.ErrCodeTenantSuspended {
		t.Errorf("Code: got %q, want %q", body.Code, api.ErrCodeTenantSuspended)
	}
}

func TestWriteErrorCode_IncludesRequestID(t *testing.T) {
	rr := httptest.NewRecorder()
	// Simulate what RequestID middleware does.
	rr.Header().Set("X-Request-ID", "req-test-123")

	api.WriteErrorCode(rr, http.StatusBadRequest, "bad input", api.ErrCodeBadRequest)

	var body api.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.RequestID != "req-test-123" {
		t.Errorf("RequestID: got %q, want %q", body.RequestID, "req-test-123")
	}
}

func TestWriteError_StatusMapping(t *testing.T) {
	cases := []struct {
		status   int
		wantCode string
	}{
		{400, api.ErrCodeBadRequest},
		{401, api.ErrCodeTokenInvalid},
		{403, api.ErrCodeForbidden},
		{404, api.ErrCodeNotFound},
		{405, api.ErrCodeMethodNotAllowed},
		{409, api.ErrCodeConflict},
		{429, api.ErrCodeRateLimitExceeded},
		{503, api.ErrCodeServiceUnavailable},
		{500, api.ErrCodeInternal},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			rr := httptest.NewRecorder()
			api.WriteError(rr, tc.status, "msg")

			var body api.ErrorResponse
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Code != tc.wantCode {
				t.Errorf("status %d: Code got %q, want %q", tc.status, body.Code, tc.wantCode)
			}
		})
	}
}

func TestWriteJSON_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	api.WriteJSON(rr, http.StatusOK, map[string]string{"key": "val"})

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
}
