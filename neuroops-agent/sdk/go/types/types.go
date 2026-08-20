/*
 * NeurOps Agent — Go SDK Types
 * Copyright (c) NeurOps 2025. All rights reserved.
 *
 * Strongly-typed wrappers used throughout the Go plugin SDK and the
 * core agent.  Mirrors the Motadata MotadataMap / MotadataString
 * pattern but with idiomatic Go naming.
 *
 * Central type is NeurOpsMap (map[string]any) with helper methods
 * for safe, type-coercing value extraction — eliminating boilerplate
 * type-assertion chains in every plugin.
 */

package types

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// ── Primitive types ───────────────────────────────────────────────────────────

type (
	NeurOpsString  string
	NeurOpsInt     int64
	NeurOpsFloat   float64
	NeurOpsBool    bool
	NeurOpsMap     map[string]any
	NeurOpsStrMap  map[string]string
	NeurOpsList    []string
	NeurOpsKB      float64
	NeurOpsMB      float64
	NeurOpsGB      float64
)

// ── NeurOpsMap helpers ────────────────────────────────────────────────────────

// Contains reports whether key exists in the map.
func (m NeurOpsMap) Contains(key string) bool {
	_, ok := m[key]
	return ok
}

// Delete removes a key if present.
func (m NeurOpsMap) Delete(key string) { delete(m, key) }

// IsNotEmpty reports whether the map has at least one entry.
func (m NeurOpsMap) IsNotEmpty() bool { return len(m) > 0 }

// GetString returns the value at key as a native Go string.
// Handles string, NeurOpsString, int, float64, []uint8 automatically.
func (m NeurOpsMap) GetString(key string) string {
	if !m.Contains(key) {
		return ""
	}
	return toString(m[key])
}

// GetInt returns the value at key as int64, coercing numeric types.
func (m NeurOpsMap) GetInt(key string) int64 {
	if !m.Contains(key) {
		return 0
	}
	return toInt64(m[key])
}

// GetFloat returns the value at key as float64.
func (m NeurOpsMap) GetFloat(key string) float64 {
	if !m.Contains(key) {
		return 0
	}
	return toFloat64(m[key])
}

// GetBool returns the value at key as bool.
// Recognises native bool, "yes"/"no", "true"/"false", 1/0.
func (m NeurOpsMap) GetBool(key string) bool {
	if !m.Contains(key) {
		return false
	}
	v := m[key]
	switch t := v.(type) {
	case bool:
		return t
	case string:
		lc := strings.ToLower(t)
		return lc == "yes" || lc == "true" || lc == "1"
	case int, int64, float64:
		return toInt64(v) != 0
	}
	return false
}

// GetMap returns the value at key as a NeurOpsMap.
func (m NeurOpsMap) GetMap(key string) NeurOpsMap {
	if !m.Contains(key) {
		return nil
	}
	switch t := m[key].(type) {
	case NeurOpsMap:
		return t
	case map[string]any:
		return NeurOpsMap(t)
	}
	return nil
}

// GetSlice returns the value at key as []NeurOpsMap.
func (m NeurOpsMap) GetSlice(key string) []NeurOpsMap {
	if !m.Contains(key) {
		return nil
	}
	switch t := m[key].(type) {
	case []NeurOpsMap:
		return t
	case []any:
		result := make([]NeurOpsMap, 0, len(t))
		for _, item := range t {
			if mm, ok := item.(map[string]any); ok {
				result = append(result, NeurOpsMap(mm))
			}
		}
		return result
	}
	return nil
}

// GetList returns the value at key as []string.
func (m NeurOpsMap) GetList(key string) []string {
	if !m.Contains(key) {
		return nil
	}
	switch t := m[key].(type) {
	case []string:
		return t
	case NeurOpsList:
		return []string(t)
	case []any:
		result := make([]string, 0, len(t))
		for _, v := range t {
			result = append(result, toString(v))
		}
		return result
	}
	return nil
}

// Merge copies all entries from other into m and returns m.
func (m NeurOpsMap) Merge(other NeurOpsMap) NeurOpsMap {
	for k, v := range other {
		m[k] = v
	}
	return m
}

// Copy returns a shallow copy of the map.
func (m NeurOpsMap) Copy() NeurOpsMap {
	result := make(NeurOpsMap, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Keys returns all keys in the map.
func (m NeurOpsMap) Keys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ToJSON serialises the map to a compact JSON string.
func (m NeurOpsMap) ToJSON() string {
	data, _ := json.Marshal(m)
	return string(data)
}

// ToJSONPretty serialises the map to indented JSON.
func (m NeurOpsMap) ToJSONPretty() string {
	data, _ := json.MarshalIndent(m, "", "  ")
	return string(data)
}

// ── Unit conversion helpers ───────────────────────────────────────────────────

func (v NeurOpsKB) ToBytes() float64 { return float64(v * 1024) }
func (v NeurOpsMB) ToBytes() float64 { return float64(v * 1024 * 1024) }
func (v NeurOpsGB) ToBytes() float64 { return float64(v * 1024 * 1024 * 1024) }

// ── Result envelope builders ──────────────────────────────────────────────────

// Succeed wraps a result map in the standard success envelope.
func Succeed(result NeurOpsMap) NeurOpsMap {
	return NeurOpsMap{
		"status": "succeed",
		"result": result,
	}
}

// Fail wraps an error message in the standard failure envelope.
func Fail(message, errorCode string) NeurOpsMap {
	return NeurOpsMap{
		"status":     "fail",
		"error":      message,
		"error.code": errorCode,
	}
}

// ── Metric event builder ──────────────────────────────────────────────────────

// MetricEvent builds a complete telemetry event envelope ready for the publisher.
func MetricEvent(metricType, objectType, objectIP, agentID string, metrics NeurOpsMap) NeurOpsMap {
	event := NeurOpsMap{
		"event.type":  "metric",
		"metric.type": metricType,
		"object.type": objectType,
		"object.ip":   objectIP,
		"agent.id":    agentID,
		"timestamp":   time.Now().Unix(),
	}
	return event.Merge(metrics)
}

// LogEvent builds a log event envelope.
func LogEvent(source, tag, message, agentID string) NeurOpsMap {
	return NeurOpsMap{
		"event.type":   "log",
		"log.source":   source,
		"log.tag":      tag,
		"log.message":  message,
		"agent.id":     agentID,
		"timestamp":    time.Now().Unix(),
		"timestamp.ms": time.Now().UnixMilli(),
	}
}

// ── Internal type coercions ───────────────────────────────────────────────────

func toString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case NeurOpsString:
		return string(t)
	case []uint8:
		return string(t)
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt64(v any) int64 {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return int64(rv.Float())
	case reflect.String:
		n, _ := strconv.ParseInt(strings.TrimSpace(rv.String()), 10, 64)
		return n
	case reflect.Bool:
		if rv.Bool() {
			return 1
		}
		return 0
	}
	return 0
}

func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return round2(rv.Float())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint())
	case reflect.String:
		f, _ := strconv.ParseFloat(strings.TrimSpace(rv.String()), 64)
		return round2(f)
	}
	return 0
}

func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
