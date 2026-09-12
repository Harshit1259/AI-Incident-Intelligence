package handlers

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"
)

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadOTLPBody(t *testing.T) {
	payload := []byte(`{"resourceMetrics":[]}`)
	tests := []struct {
		name        string
		body        []byte
		encoding    string
		contentType string
		wantStatus  int // 0 = success
		wantBody    string
	}{
		{"plain json", payload, "", "application/json", 0, string(payload)},
		{"gzip json (Collector default)", gzipBytes(t, payload), "gzip", "application/json", 0, string(payload)},
		{"encoding header case-insensitive", gzipBytes(t, payload), "GZIP", "application/json", 0, string(payload)},
		{"identity", payload, "identity", "application/json", 0, string(payload)},
		{"protobuf body passed through for conversion", []byte{0x0a, 0x00}, "", "application/x-protobuf", 0, "\n\x00"},
		{"unknown encoding", payload, "br", "application/json", http.StatusUnsupportedMediaType, ""},
		{"corrupt gzip", []byte("not gzip"), "gzip", "application/json", http.StatusBadRequest, ""},
		{"gzip bomb capped", gzipBytes(t, bytes.Repeat([]byte("a"), maxOTLPDecompressedBytes+10)), "gzip", "application/json", http.StatusRequestEntityTooLarge, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/otel/metrics", bytes.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			if tt.encoding != "" {
				req.Header.Set("Content-Encoding", tt.encoding)
			}
			got, status, err := readOTLPBody(req)
			if tt.wantStatus == 0 {
				if err != nil || string(got) != tt.wantBody {
					t.Fatalf("got %q, err %v; want %q", got, err, tt.wantBody)
				}
				return
			}
			if err == nil || status != tt.wantStatus {
				t.Fatalf("status = %d, err = %v; want %d", status, err, tt.wantStatus)
			}
		})
	}
}
