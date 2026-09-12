package handlers

import (
	"strings"
	"testing"
)

// The test-alert path must reject malformed payloads with a clear error
// instead of reaching the pipeline (it no longer depends on a source token).
func TestSendTestAlertValidatesPayload(t *testing.T) {
	h := NewIngestHandler(nil, nil, nil, nil, nil)
	cases := []struct {
		name, body, wantErr string
	}{
		{"not json", `{`, "invalid JSON"},
		{"no alerts", `{"alerts":[]}`, "alerts array is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.SendTestAlert("acme", "source-1", []byte(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
