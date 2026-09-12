package handlers

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFreshAlertmanagerSample(t *testing.T) {
	now := time.Date(2026, 9, 12, 16, 0, 0, 0, time.UTC)
	in := `{"alerts":[{"status":"firing","labels":{"alertname":"X"},"annotations":{"summary":"CPU high"},
		"startsAt":"2024-01-15T10:00:00Z","fingerprint":"57c6d9296de2ad39"}]}`
	out, err := freshAlertmanagerSample([]byte(in), now)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Alerts []struct {
			StartsAt    string            `json:"startsAt"`
			Fingerprint string            `json:"fingerprint"`
			Labels      map[string]string `json:"labels"`
			Annotations map[string]string `json:"annotations"`
		} `json:"alerts"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	a := doc.Alerts[0]
	if a.StartsAt != "2026-09-12T16:00:00Z" || a.Fingerprint != "" ||
		a.Labels["neuroops_test"] != "true" || a.Annotations["summary"] != "[Test] CPU high" {
		t.Errorf("unexpected rewritten alert: %+v", a)
	}
}

func TestFreshOTLPSample(t *testing.T) {
	now := time.Unix(1789207200, 0)
	in := `{"resourceSpans":[{"scopeSpans":[{"spans":[{"name":"x","startTimeUnixNano":"1","endTimeUnixNano":"2"}]}]}]}`
	out, err := freshOTLPSample([]byte(in), now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(out), `"1789207200000000000"`) != 2 {
		t.Errorf("timestamps not rewritten: %s", out)
	}
}

func TestIngestSampleRejectsUnsupportedIntegration(t *testing.T) {
	m := &MarketplaceSampleIngester{}
	if _, err := m.IngestSample("acme", "", "nagios", []byte(`{}`)); err == nil {
		t.Error("nagios is not a supported integration; expected an error")
	}
}
