package handlers

import (
	"testing"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"ai-incident-platform/backend/internal/ingest"
)

func strAttr(k, v string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}}}
}

var testResource = &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
	strAttr("service.name", "checkout"), strAttr("host.name", "web01"),
}}

// Round trip: protobuf (as the Collector sends it) → our converter → the real
// JSON mapper → events.
func mapProto(t *testing.T, sourceType string, msg proto.Message) []string {
	t.Helper()
	raw, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	js, err := otlpProtobufToJSON(sourceType, raw)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	mapper, _ := ingest.NewMapperRegistry().Get(sourceType)
	events, err := mapper.Map(js, "acme")
	if err != nil {
		t.Fatalf("map: %v (json %s)", err, js)
	}
	var out []string
	for _, e := range events {
		out = append(out, e.Service+"|"+e.ExternalID+"|"+e.Title)
	}
	return out
}

func TestProtobufTracesKeepHexIDs(t *testing.T) {
	traceID := []byte{0x5b, 0x8e, 0xff, 0xf7, 0x98, 0x03, 0x81, 0x03, 0xd2, 0x69, 0xb6, 0x33, 0x81, 0x3f, 0xc6, 0x0c}
	spanID := []byte{0xee, 0xe1, 0x9b, 0x7e, 0xc3, 0xc1, 0xb1, 0x74}
	msg := &tracepb.TracesData{ResourceSpans: []*tracepb.ResourceSpans{{
		Resource: testResource,
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			TraceId: traceID, SpanId: spanID, Name: "POST /checkout",
			Kind:   tracepb.Span_SPAN_KIND_SERVER,
			Status: &tracepb.Status{Code: tracepb.Status_STATUS_CODE_ERROR, Message: "db timeout"},
			StartTimeUnixNano: 1789207200000000000, EndTimeUnixNano: 1789207201000000000,
		}}}},
	}}}
	got := mapProto(t, "otel-traces", msg)
	want := "checkout|5b8efff798038103d269b633813fc60c-eee19b7ec3c1b174|Trace error: POST /checkout"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want [%s]", got, want)
	}
}

func TestProtobufMetrics(t *testing.T) {
	msg := &metricspb.MetricsData{ResourceMetrics: []*metricspb.ResourceMetrics{{
		Resource: testResource,
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{{
			Name: "system.cpu.utilization",
			Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{
				TimeUnixNano: 1789207200000000000,
				Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 0.93},
			}}}},
		}}}},
	}}}
	got := mapProto(t, "otel-metrics", msg)
	if len(got) != 1 || got[0] != "checkout||Metric: system.cpu.utilization = 0.93" {
		t.Fatalf("got %v", got)
	}
}

func TestProtobufLogs(t *testing.T) {
	msg := &logspb.LogsData{ResourceLogs: []*logspb.ResourceLogs{{
		Resource: testResource,
		ScopeLogs: []*logspb.ScopeLogs{{LogRecords: []*logspb.LogRecord{{
			TimeUnixNano:   1789207200000000000,
			SeverityNumber: logspb.SeverityNumber_SEVERITY_NUMBER_ERROR,
			Body:           &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "db timeout"}},
		}}}},
	}}}
	got := mapProto(t, "otel-logs", msg)
	if len(got) != 1 || got[0][:9] != "checkout|" {
		t.Fatalf("got %v", got)
	}
}

func TestProtobufRejectsGarbage(t *testing.T) {
	if _, err := otlpProtobufToJSON("otel-traces", []byte{0xff, 0xff, 0xff}); err == nil {
		t.Error("expected an error for an invalid protobuf body")
	}
}
