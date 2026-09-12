package handlers

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"

	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// otlpProtobufToJSON converts an OTLP/HTTP protobuf body into OTLP JSON so the
// existing JSON mappers handle it unchanged.
//
// The body of an Export{Logs,Metrics,Trace}ServiceRequest is decoded into the
// matching {Logs,Metrics,Traces}Data message: both have the resource list as
// field 1, so they are wire-compatible, and the *Data packages do not pull in
// gRPC.
//
// protojson differs from OTLP JSON in two ways that are corrected here:
// enums are written as numbers (UseEnumNumbers), and trace/span IDs — bytes
// fields, which protojson writes as base64 — are rewritten as hex.
func otlpProtobufToJSON(sourceType string, body []byte) ([]byte, error) {
	var msg proto.Message
	switch sourceType {
	case "otel-logs":
		msg = &logspb.LogsData{}
	case "otel-metrics":
		msg = &metricspb.MetricsData{}
	case "otel-traces":
		msg = &tracepb.TracesData{}
	default:
		return nil, fmt.Errorf("protobuf is not supported for %s", sourceType)
	}
	if err := proto.Unmarshal(body, msg); err != nil {
		return nil, fmt.Errorf("invalid OTLP protobuf: %w", err)
	}
	js, err := protojson.MarshalOptions{UseEnumNumbers: true}.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode OTLP as JSON: %w", err)
	}

	var doc any
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, err
	}
	hexIDs(doc)
	return json.Marshal(doc)
}

// otlpIDKeys are the OTLP bytes fields that OTLP JSON encodes as hex.
var otlpIDKeys = map[string]bool{"traceId": true, "spanId": true, "parentSpanId": true}

func hexIDs(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if s, ok := child.(string); ok && otlpIDKeys[k] {
				if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
					t[k] = hex.EncodeToString(raw)
				}
				continue
			}
			hexIDs(child)
		}
	case []any:
		for _, child := range t {
			hexIDs(child)
		}
	}
}
