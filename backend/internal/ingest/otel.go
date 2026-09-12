// Package ingest contains all ingest-path normalisation logic.
// otel.go implements OTLP/JSON deserialization and semantic normalization
// according to the OpenTelemetry specification (v1.24).
//
// OTLP/JSON key structures:
//
//	ExportLogsServiceRequest   → resourceLogs[].scopeLogs[].logRecords[]
//	ExportMetricsServiceRequest → resourceMetrics[].scopeMetrics[].metrics[]
//	ExportTraceServiceRequest  → resourceSpans[].scopeSpans[].spans[]
//
// We convert every signal into a models.IngestEvent so the existing
// correlation pipeline requires zero changes.
package ingest

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ── OTel AnyValue ─────────────────────────────────────────────────────────────

// otlpAnyValue is an OTel AnyValue union decoded from JSON.
// We only use the string form for attribute extraction; non-string types are
// converted via fmt.Sprint.
type otlpAnyValue struct {
	StringValue *string          `json:"stringValue"`
	IntValue    *json.RawMessage `json:"intValue"`    // int64 encoded as JSON string
	DoubleValue *float64         `json:"doubleValue"`
	BoolValue   *bool            `json:"boolValue"`
	ArrayValue  *struct {
		Values []otlpAnyValue `json:"values"`
	} `json:"arrayValue"`
}

func (v otlpAnyValue) String() string {
	if v.StringValue != nil {
		return *v.StringValue
	}
	if v.IntValue != nil {
		// intValue is encoded as a JSON string in OTel: "42"
		var s string
		if err := json.Unmarshal(*v.IntValue, &s); err == nil {
			return s
		}
		return string(*v.IntValue)
	}
	if v.DoubleValue != nil {
		return fmt.Sprintf("%g", *v.DoubleValue)
	}
	if v.BoolValue != nil {
		if *v.BoolValue {
			return "true"
		}
		return "false"
	}
	return ""
}

type otlpKV struct {
	Key   string       `json:"key"`
	Value otlpAnyValue `json:"value"`
}

// flattenAttrs converts a []otlpKV slice to a map[string]string.
func flattenAttrs(kvs []otlpKV) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[kv.Key] = kv.Value.String()
	}
	return m
}

// mergeAttrs merges b into a (a wins on key collision).
func mergeAttrs(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	for k, v := range b {
		out[k] = v
	}
	for k, v := range a {
		out[k] = v
	}
	return out
}

// ── Resource ─────────────────────────────────────────────────────────────────

type otlpResource struct {
	Attributes []otlpKV `json:"attributes"`
}

// extractResourceAttrs returns a flat map of all resource attributes.
func extractResourceAttrs(r otlpResource) map[string]string {
	return flattenAttrs(r.Attributes)
}

// serviceFromResource extracts the service name from OTel semantic conventions.
// Fallback chain: service.name → k8s.deployment.name → k8s.pod.name → "unknown"
func serviceFromResource(attrs map[string]string) string {
	for _, key := range []string{"service.name", "k8s.deployment.name", "k8s.pod.name"} {
		if v, ok := attrs[key]; ok && v != "" {
			return v
		}
	}
	return "unknown-service"
}

// environmentFromResource returns the deployment environment.
func environmentFromResource(attrs map[string]string) string {
	if v, ok := attrs["deployment.environment"]; ok && v != "" {
		return v
	}
	if v, ok := attrs["deployment.environment.name"]; ok && v != "" {
		return v
	}
	return "prod"
}

// ── Severity mapping ─────────────────────────────────────────────────────────

// OTel severity numbers per spec (https://opentelemetry.io/docs/specs/otel/logs/data-model/):
//  1–4   TRACE
//  5–8   DEBUG
//  9–12  INFO
//  13–16 WARN
//  17–20 ERROR
//  21–24 FATAL

func otelSeverityToOurs(severityNumber int, severityText string) string {
	switch {
	case severityNumber >= 21:
		return "critical"
	case severityNumber >= 17:
		return "high"
	case severityNumber >= 13:
		return "medium"
	case severityNumber >= 9:
		return "low"
	case severityNumber >= 1:
		return "low"
	}
	// Fall back to text-based mapping.
	return normalizeSeverity(severityText)
}

// ── Unix nano to time.Time ────────────────────────────────────────────────────

func unixNanoToTime(ns string) time.Time {
	if ns == "" {
		return time.Now().UTC()
	}
	var n int64
	_, _ = fmt.Sscan(ns, &n)
	if n == 0 {
		return time.Now().UTC()
	}
	return time.Unix(0, n).UTC()
}

// ── OTLP Logs ─────────────────────────────────────────────────────────────────

type OTLPLogsPayload struct {
	ResourceLogs []struct {
		Resource otlpResource `json:"resource"`
		ScopeLogs []struct {
			Scope struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"scope"`
			LogRecords []struct {
				TimeUnixNano         string       `json:"timeUnixNano"`
				ObservedTimeUnixNano string       `json:"observedTimeUnixNano"`
				SeverityNumber       int          `json:"severityNumber"`
				SeverityText         string       `json:"severityText"`
				Body                 otlpAnyValue `json:"body"`
				Attributes           []otlpKV     `json:"attributes"`
				TraceID              string       `json:"traceId"`
				SpanID               string       `json:"spanId"`
				Flags                uint32       `json:"flags"`
			} `json:"logRecords"`
		} `json:"scopeLogs"`
	} `json:"resourceLogs"`
}

// NormalizeOTLPLogs converts an OTLP/JSON logs payload into IngestEvents.
// Each log record that represents an error/warning becomes one IngestEvent.
// Lower-severity log records are also converted so operators can see them.
func NormalizeOTLPLogs(payload OTLPLogsPayload, tenantID string) []models.IngestEvent {
	var events []models.IngestEvent

	for _, rl := range payload.ResourceLogs {
		resAttrs := extractResourceAttrs(rl.Resource)
		service := serviceFromResource(resAttrs)
		environment := environmentFromResource(resAttrs)

		for _, sl := range rl.ScopeLogs {
			scopeName := sl.Scope.Name

			for _, lr := range sl.LogRecords {
				recAttrs := flattenAttrs(lr.Attributes)
				allAttrs := mergeAttrs(resAttrs, recAttrs)

				ts := unixNanoToTime(lr.TimeUnixNano)
				if ts.IsZero() {
					ts = unixNanoToTime(lr.ObservedTimeUnixNano)
				}

				body := lr.Body.String()
				severity := otelSeverityToOurs(lr.SeverityNumber, lr.SeverityText)
				title := buildOTelTitle(recAttrs, body, service, "log")

				attrsJSON := marshalAttrs(allAttrs)

				events = append(events, models.IngestEvent{
					TenantID:     tenantID,
					Source:       "otel",
					ExternalID:   spanExternalID(lr.TraceID, lr.SpanID),
					Service:      service,
					Resource:     allAttrs["k8s.pod.name"],
					Environment:  environment,
					Severity:     severity,
					SignalType:   "log",
					Title:        title,
					Message:      body,
					Labels:       allAttrs,
					Timestamp:    ts,
					TraceID:      lr.TraceID,
					SpanID:       lr.SpanID,
					ScopeName:    scopeName,
					IngestSchema: "otel-logs",
					AttrsJSON:    attrsJSON,
				})
			}
		}
	}

	return events
}

// ── OTLP Metrics ─────────────────────────────────────────────────────────────

type OTLPMetricsPayload struct {
	ResourceMetrics []struct {
		Resource     otlpResource `json:"resource"`
		ScopeMetrics []struct {
			Scope struct {
				Name string `json:"name"`
			} `json:"scope"`
			Metrics []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Unit        string `json:"unit"`
				Gauge       *struct {
					DataPoints []otlpNumberDataPoint `json:"dataPoints"`
				} `json:"gauge"`
				Sum *struct {
					DataPoints []otlpNumberDataPoint `json:"dataPoints"`
				} `json:"sum"`
				Histogram *struct {
					DataPoints []struct {
						Attributes []otlpKV `json:"attributes"`
						TimeUnixNano string  `json:"timeUnixNano"`
						Count       string  `json:"count"`
						Sum         *float64 `json:"sum"`
					} `json:"dataPoints"`
				} `json:"histogram"`
			} `json:"metrics"`
		} `json:"scopeMetrics"`
	} `json:"resourceMetrics"`
}

type otlpNumberDataPoint struct {
	Attributes   []otlpKV     `json:"attributes"`
	TimeUnixNano string       `json:"timeUnixNano"`
	AsDouble     *float64     `json:"asDouble"`
	AsInt        *json.RawMessage `json:"asInt"`
	Exemplars    []struct{} `json:"exemplars"`
}

func (dp *otlpNumberDataPoint) valueString() string {
	if dp.AsDouble != nil {
		return fmt.Sprintf("%g", *dp.AsDouble)
	}
	if dp.AsInt != nil {
		var s string
		if err := json.Unmarshal(*dp.AsInt, &s); err == nil {
			return s
		}
	}
	return "0"
}

// NormalizeOTLPMetrics converts an OTLP/JSON metrics payload into IngestEvents.
// Each metric data point becomes one event so the correlation engine can process them.
func NormalizeOTLPMetrics(payload OTLPMetricsPayload, tenantID string) []models.IngestEvent {
	var events []models.IngestEvent

	for _, rm := range payload.ResourceMetrics {
		resAttrs := extractResourceAttrs(rm.Resource)
		service := serviceFromResource(resAttrs)
		environment := environmentFromResource(resAttrs)

		for _, sm := range rm.ScopeMetrics {
			scopeName := sm.Scope.Name

			for _, m := range sm.Metrics {
				var dataPoints []struct {
					attrs []otlpKV
					ts    string
					val   string
				}

				if m.Gauge != nil {
					for _, dp := range m.Gauge.DataPoints {
						dataPoints = append(dataPoints, struct {
							attrs []otlpKV
							ts    string
							val   string
						}{dp.Attributes, dp.TimeUnixNano, dp.valueString()})
					}
				}
				if m.Sum != nil {
					for _, dp := range m.Sum.DataPoints {
						dataPoints = append(dataPoints, struct {
							attrs []otlpKV
							ts    string
							val   string
						}{dp.Attributes, dp.TimeUnixNano, dp.valueString()})
					}
				}

				for _, dp := range dataPoints {
					dpAttrs := flattenAttrs(dp.attrs)
					allAttrs := mergeAttrs(resAttrs, dpAttrs)
					allAttrs["otel.metric.name"] = m.Name
					allAttrs["otel.metric.value"] = dp.val
					if m.Unit != "" {
						allAttrs["otel.metric.unit"] = m.Unit
					}

					ts := unixNanoToTime(dp.ts)
					title := fmt.Sprintf("Metric: %s = %s%s", m.Name, dp.val, unitSuffix(m.Unit))
					msg := m.Description
					if msg == "" {
						msg = title
					}

					// Derive severity from HTTP status code if present.
					severity := metricSeverity(dpAttrs)

					events = append(events, models.IngestEvent{
						TenantID:     tenantID,
						Source:       "otel",
						Service:      service,
						Environment:  environment,
						Severity:     severity,
						SignalType:   "metric",
						Title:        title,
						Message:      msg,
						Labels:       allAttrs,
						Timestamp:    ts,
						ScopeName:    scopeName,
						IngestSchema: "otel-metrics",
					})
				}
			}
		}
	}

	return events
}

// ── OTLP Traces ──────────────────────────────────────────────────────────────

type OTLPTracesPayload struct {
	ResourceSpans []struct {
		Resource   otlpResource `json:"resource"`
		ScopeSpans []struct {
			Scope struct {
				Name string `json:"name"`
			} `json:"scope"`
			Spans []struct {
				TraceID           string   `json:"traceId"`
				SpanID            string   `json:"spanId"`
				ParentSpanID      string   `json:"parentSpanId"`
				Name              string   `json:"name"`
				Kind              int      `json:"kind"`
				StartTimeUnixNano string   `json:"startTimeUnixNano"`
				EndTimeUnixNano   string   `json:"endTimeUnixNano"`
				Attributes        []otlpKV `json:"attributes"`
				Status            struct {
					Code    int    `json:"code"`    // 0=UNSET, 1=OK, 2=ERROR
					Message string `json:"message"`
				} `json:"status"`
				Events []struct {
					TimeUnixNano string   `json:"timeUnixNano"`
					Name         string   `json:"name"`
					Attributes   []otlpKV `json:"attributes"`
				} `json:"events"`
			} `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

// NormalizeOTLPTraces converts an OTLP/JSON traces payload into IngestEvents.
// Only spans with status=ERROR (code=2) produce events; clean spans are skipped
// to avoid noise in the incident feed.
func NormalizeOTLPTraces(payload OTLPTracesPayload, tenantID string) []models.IngestEvent {
	var events []models.IngestEvent

	for _, rs := range payload.ResourceSpans {
		resAttrs := extractResourceAttrs(rs.Resource)
		service := serviceFromResource(resAttrs)
		environment := environmentFromResource(resAttrs)

		for _, ss := range rs.ScopeSpans {
			scopeName := ss.Scope.Name

			for _, span := range ss.Spans {
				// Only ingest spans that carry errors.
				if span.Status.Code != 2 {
					continue
				}

				spanAttrs := flattenAttrs(span.Attributes)
				allAttrs := mergeAttrs(resAttrs, spanAttrs)
				allAttrs["otel.span.name"] = span.Name
				allAttrs["otel.span.kind"] = spanKindName(span.Kind)

				startTS := unixNanoToTime(span.StartTimeUnixNano)

				statusMsg := span.Status.Message
				if statusMsg == "" {
					statusMsg = spanAttrs["error.message"]
				}
				if statusMsg == "" {
					statusMsg = fmt.Sprintf("span %q failed", span.Name)
				}

				title := fmt.Sprintf("Trace error: %s", span.Name)
				severity := traceSeverity(spanAttrs)

				events = append(events, models.IngestEvent{
					TenantID:     tenantID,
					Source:       "otel",
					ExternalID:   spanExternalID(span.TraceID, span.SpanID),
					Service:      service,
					Environment:  environment,
					Severity:     severity,
					SignalType:   "trace",
					Title:        title,
					Message:      statusMsg,
					Labels:       allAttrs,
					Timestamp:    startTS,
					TraceID:      span.TraceID,
					SpanID:       span.SpanID,
					ScopeName:    scopeName,
					IngestSchema: "otel-traces",
				})
			}
		}
	}

	return events
}

// ── helpers ───────────────────────────────────────────────────────────────────

// spanExternalID identifies the span a log or trace event came from. It is
// empty when there is no trace context: the old value "traceId-spanId" became
// "-" for every log without a trace, and because ExternalID doubles as the
// event ID those logs overwrote each other (across tenants too). Traces used
// the trace ID alone, so several failing spans in one trace collided.
func spanExternalID(traceID, spanID string) string {
	switch {
	case traceID != "" && spanID != "":
		return traceID + "-" + spanID
	case traceID != "":
		return traceID
	default:
		return ""
	}
}

func buildOTelTitle(attrs map[string]string, body, service, signalType string) string {
	// Check common alert-title attributes first.
	for _, k := range []string{"exception.message", "error.type", "event.name", "alertname"} {
		if v := attrs[k]; v != "" {
			return fmt.Sprintf("%s on %s", v, service)
		}
	}
	if body != "" {
		if len(body) > 120 {
			return body[:120] + "…"
		}
		return body
	}
	return fmt.Sprintf("OTel %s from %s", signalType, service)
}

func metricSeverity(attrs map[string]string) string {
	// HTTP 5xx → high; HTTP 4xx → medium; error attr → high
	if code := attrs["http.status_code"]; code != "" {
		if strings.HasPrefix(code, "5") {
			return "high"
		}
		if strings.HasPrefix(code, "4") {
			return "medium"
		}
	}
	if attrs["error"] == "true" || attrs["error.type"] != "" {
		return "high"
	}
	return "low"
}

func traceSeverity(attrs map[string]string) string {
	if code := attrs["http.status_code"]; code != "" {
		if strings.HasPrefix(code, "5") {
			return "critical"
		}
		if strings.HasPrefix(code, "4") {
			return "high"
		}
	}
	return "high"
}

func spanKindName(kind int) string {
	names := []string{"UNSPECIFIED", "INTERNAL", "SERVER", "CLIENT", "PRODUCER", "CONSUMER"}
	if kind >= 0 && kind < len(names) {
		return names[kind]
	}
	return "UNKNOWN"
}

func unitSuffix(unit string) string {
	if unit == "" {
		return ""
	}
	return " " + unit
}

func marshalAttrs(m map[string]string) string {
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}
