package ingest

import (
	"encoding/json"
	"testing"
)

func TestSpanExternalID(t *testing.T) {
	tests := []struct{ trace, span, want string }{
		{"t1", "s1", "t1-s1"},
		{"t1", "", "t1"},
		{"", "s1", ""},
		{"", "", ""}, // used to be "-", shared by every log without a trace
	}
	for _, tt := range tests {
		if got := spanExternalID(tt.trace, tt.span); got != tt.want {
			t.Errorf("spanExternalID(%q,%q) = %q, want %q", tt.trace, tt.span, got, tt.want)
		}
	}
}

// Logs without trace context must not share an ID — they used to overwrite
// each other in the events table.
func TestLogsWithoutTraceGetDistinctEventIDs(t *testing.T) {
	var p OTLPLogsPayload
	body := `{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},
	  "scopeLogs":[{"logRecords":[
	    {"timeUnixNano":"1789207200000000000","severityNumber":17,"body":{"stringValue":"db timeout"}},
	    {"timeUnixNano":"1789207201000000000","severityNumber":17,"body":{"stringValue":"db timeout"}}]}]}]}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	evs := NormalizeOTLPLogs(p, "acme")
	if len(evs) != 2 {
		t.Fatalf("want 2 events, got %d", len(evs))
	}
	for _, ie := range evs {
		if ie.ExternalID != "" {
			t.Errorf("log without trace context has ExternalID %q, want empty", ie.ExternalID)
		}
	}
	a, b := IngestEventToEvent(evs[0]), IngestEventToEvent(evs[1])
	if a.ID == b.ID {
		t.Errorf("two logs share event ID %q", a.ID)
	}
	if a.Fingerprint != "" {
		t.Errorf("ToEvent set Fingerprint %q; ProcessEvent must compute it", a.Fingerprint)
	}
}

// Two failing spans in one trace are two events, not one overwritten row.
func TestErrorSpansInOneTraceGetDistinctIDs(t *testing.T) {
	var p OTLPTracesPayload
	body := `{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"api"}}]},
	  "scopeSpans":[{"spans":[
	    {"traceId":"abc","spanId":"s1","name":"GET /a","status":{"code":2}},
	    {"traceId":"abc","spanId":"s2","name":"GET /b","status":{"code":2}}]}]}]}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	evs := NormalizeOTLPTraces(p, "acme")
	if len(evs) != 2 {
		t.Fatalf("want 2 error-span events, got %d", len(evs))
	}
	if evs[0].ExternalID == evs[1].ExternalID {
		t.Errorf("spans share ExternalID %q", evs[0].ExternalID)
	}
}
