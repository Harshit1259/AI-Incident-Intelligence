package handlers

import (
	"errors"
	"strings"
	"testing"
)

func zbxProblem() zabbixPayload {
	return zabbixPayload{
		EventID: "1234", EventValue: "1", EventStatus: "PROBLEM", UpdateStatus: "0",
		TriggerID: "5678", TriggerName: "High CPU utilization",
		EventName: "High CPU utilization (over 90% for 5m)", Severity: "4",
		Host: "db01", HostName: "DB Server 01", HostIP: "10.0.0.5", HostGroup: "Linux servers",
		Tags:      []zabbixTag{{Tag: "service", Value: "billing"}, {Tag: "env", Value: "prod"}},
		ItemValue: "93.4", OpData: "Current utilization: 93.4 %",
		ZabbixURL: "https://zabbix.example.com/tr_events.php?eventid=1234",
	}
}

func TestZabbixProblem(t *testing.T) {
	e, kind, err := zabbixToEvent(zbxProblem(), "acme")
	if err != nil || kind != zabbixProblem {
		t.Fatalf("kind=%v err=%v", kind, err)
	}
	checks := map[string][2]string{
		"ID":          {e.ID, "zbx-acme-1234"},
		"Fingerprint": {e.Fingerprint, "zbx:5678:db01"},
		"Service":     {e.Service, "billing"},
		"Resource":    {e.Resource, "db01"},
		"Environment": {e.Environment, "prod"},
		"Severity":    {e.Severity, "high"},
		"Title":       {e.Title, "High CPU utilization (over 90% for 5m)"},
		"AlertStatus": {e.AlertStatus, "firing"},
		"Schema":      {e.IngestSchema, "zabbix"},
		"alertname":   {e.Labels["alertname"], "High CPU utilization"},
		"value":       {e.Labels[ZabbixValueLabel], "93.4"},
		"url":         {e.Labels[ZabbixURLLabel], "https://zabbix.example.com/tr_events.php?eventid=1234"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestZabbixRecoverySharesEventIDAndFingerprint(t *testing.T) {
	p := zbxProblem()
	problem, _, _ := zabbixToEvent(p, "acme")
	p.EventValue, p.EventStatus = "0", "RESOLVED"
	recovery, kind, err := zabbixToEvent(p, "acme")
	if err != nil || kind != zabbixRecovery || recovery.AlertStatus != "resolved" {
		t.Fatalf("kind=%v status=%q err=%v", kind, recovery.AlertStatus, err)
	}
	if recovery.ID != problem.ID || recovery.Fingerprint != problem.Fingerprint {
		t.Error("recovery must reuse the problem's event ID and fingerprint so auto-close can pair them")
	}
}

func TestZabbixUpdate(t *testing.T) {
	p := zbxProblem()
	p.UpdateStatus, p.UpdateAction, p.UpdateMessage, p.UpdateUser = "1", "acknowledged", "looking into it", "John Smith"
	_, kind, err := zabbixToEvent(p, "acme")
	if err != nil || kind != zabbixUpdate {
		t.Fatalf("kind=%v err=%v", kind, err)
	}
	if got := zabbixUpdateNote(p); got != `Zabbix: John Smith acknowledged the problem: "looking into it"` {
		t.Errorf("note = %q", got)
	}
	p.UpdateUser = "Inaccessible user" // what Zabbix sends to a restricted user
	if got := zabbixUpdateNote(p); !strings.HasPrefix(got, "Zabbix: a Zabbix user acknowledged") {
		t.Errorf("note = %q", got)
	}
}

func TestZabbixSeverityMap(t *testing.T) {
	for sev, want := range map[string]string{"5": "critical", "4": "high", "3": "medium", "2": "low", "1": "info", "0": "info", "9": "medium", "": "medium"} {
		p := zbxProblem()
		p.Severity = sev
		e, _, _ := zabbixToEvent(p, "acme")
		if e.Severity != want {
			t.Errorf("severity %q → %q, want %q", sev, e.Severity, want)
		}
	}
}

func TestZabbixServiceOrder(t *testing.T) {
	p := zbxProblem()
	p.Tags = nil
	if e, _, _ := zabbixToEvent(p, "acme"); e.Service != "Linux servers" {
		t.Errorf("no service tag: service = %q, want host group", e.Service)
	}
	p.HostGroup = "{TRIGGER.HOSTGROUP.NAME}"
	if e, _, _ := zabbixToEvent(p, "acme"); e.Service != "db01" {
		t.Errorf("no tag or group: service = %q, want host", e.Service)
	}
}

func TestZabbixTestButton(t *testing.T) {
	p := zabbixPayload{EventID: "{EVENT.ID}", TriggerID: "{TRIGGER.ID}", Host: "{HOST.HOST}"}
	if _, _, err := zabbixToEvent(p, "acme"); !errors.Is(err, errZabbixTest) {
		t.Fatalf("unexpanded macros should be treated as the Zabbix Test button, got %v", err)
	}
}

func TestZabbixRequiredFields(t *testing.T) {
	p := zbxProblem()
	p.TriggerID = ""
	if _, _, err := zabbixToEvent(p, "acme"); err == nil || !strings.Contains(err.Error(), "trigger_id") {
		t.Fatalf("missing trigger_id: err = %v", err)
	}
}
