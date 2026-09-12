// Package fingerprint computes a stable identity for an alert: two alerts
// about the same problem get the same fingerprint even when they come from a
// different pod, IP address, port or time.
//
// Not wired into ingest yet (see plan.md, "On hold → Fingerprint quality").
// Sources that send their own fingerprint (Alertmanager, Grafana) keep using
// theirs; this is for the ones that don't.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Alert is the minimum a fingerprint needs.
type Alert struct {
	Name   string // alertname / metric / trigger name
	Labels map[string]string
}

// Fingerprint returns the hex SHA-256 of the alert's normalized name and
// labels:
//  1. keys and values are lower-cased and trimmed; labels that change on
//     every occurrence (values, timestamps, request IDs, links) are dropped;
//  2. instance tokens in values are replaced — pod suffixes are stripped and
//     IPs, UUIDs, timestamps, long numbers and hex IDs become placeholders;
//     host:port loses the port;
//  3. labels are sorted by key;
//  4. every field is written length-prefixed, so no label value can forge
//     another label set ("a=b|c" never collides with a="b", c="").
//
// Kept on purpose: StatefulSet ordinals (kafka-0 ≠ kafka-1) and hostnames
// with numbers (web01 ≠ web02) — those name different machines.
func Fingerprint(a Alert) string {
	buf := make([]byte, 0, 256)
	buf = appendField(buf, normalizeValue(strings.ToLower(strings.TrimSpace(a.Name))))

	keys := make([]string, 0, len(a.Labels))
	values := make(map[string]string, len(a.Labels))
	for k, v := range a.Labels {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" || isVolatileKey(k) {
			continue
		}
		v = normalizeValue(strings.ToLower(strings.TrimSpace(v)))
		if v == "" {
			continue // an empty label is the same as a missing one
		}
		if old, dup := values[k]; dup {
			// "Pod" and "pod" became one key: pick a winner that does not
			// depend on map order.
			if v < old {
				values[k] = v
			}
			continue
		}
		keys = append(keys, k)
		values[k] = v
	}
	sort.Strings(keys)
	for _, k := range keys {
		buf = appendField(buf, k)
		buf = appendField(buf, values[k])
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}

// appendField appends "<len>:<s>" so field boundaries are unambiguous.
func appendField(buf []byte, s string) []byte {
	buf = strconv.AppendInt(buf, int64(len(s)), 10)
	buf = append(buf, ':')
	return append(buf, s...)
}

// ── step 1: labels that are not part of the alert's identity ──────────────

var volatileKeys = map[string]bool{
	// the measured value: 95% and 96% CPU are the same alert
	"value": true, "annotation.value": true, "grafana.value": true,
	"zabbix.item_value": true, "otel.metric.value": true,
	// times
	"timestamp": true, "time": true, "startsat": true, "endsat": true,
	// per-occurrence IDs
	"event_id": true, "request_id": true, "trace_id": true, "span_id": true,
	"traceid": true, "spanid": true, "pod_uid": true, "uid": true, "container_id": true,
	// added by NeuroOps, not by the source
	"neuroops_test": true, "service.original": true, "service.derived_from": true,
}

func isVolatileKey(k string) bool {
	// links (runbook_url, generatorURL, grafana.rule_url, zabbix.url) can
	// carry query strings and IDs, and are not identity
	return volatileKeys[k] || strings.HasSuffix(k, "url")
}

// ── step 2: instance tokens inside values ─────────────────────────────────

var (
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// 2026-09-13, 2026-09-13t10:00:00z, 2026.09.13 (Zabbix) — already lower-cased
	dateRe = regexp.MustCompile(`^\d{4}[-.]\d{2}[-.]\d{2}([t_]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(z|[+-]\d{2}:?\d{2})?)?$`)
	// the time half of "2026-09-13 10:00:00.123+05:30" (words are split on spaces)
	clockRe = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}(\.\d+)?(z|[+-]\d{2}:?\d{2})?$`)
	hexIDRe = regexp.MustCompile(`^[0-9a-f]{12,}$`)
)

// normalizeValue rewrites each whitespace- or "/"-separated word of an
// already lower-cased value.
func normalizeValue(v string) string {
	if v == "" || !strings.ContainsAny(v, "0123456789-:.") {
		return v // fast path: plain words have nothing to strip
	}
	words := strings.Fields(v)
	for i, w := range words {
		if strings.Contains(w, "/") && !strings.Contains(w, "://") {
			parts := strings.Split(w, "/")
			for j, p := range parts {
				parts[j] = normalizeWord(p)
			}
			words[i] = strings.Join(parts, "/")
			continue
		}
		words[i] = normalizeWord(w)
	}
	return strings.Join(words, " ")
}

// normalizeWord handles one token. Order matters: timestamps and IPv6
// contain colons, so they are checked before host:port.
func normalizeWord(w string) string {
	core := strings.Trim(w, `,;()"'`) // brackets stay: [::1]:80 needs them
	if core == "" {
		return w
	}
	if n := normalizeToken(core); n != core {
		return strings.Replace(w, core, n, 1)
	}
	return w
}

// The length and character checks in front of each regex are cheap filters:
// most words fail them, and regexes are the bulk of the cost.
func normalizeToken(t string) string {
	n := len(t)
	switch {
	case n >= 10 && (t[4] == '-' || t[4] == '.') && dateRe.MatchString(t),
		n >= 8 && t[2] == ':' && clockRe.MatchString(t):
		return "<ts>"
	case n == 36 && t[8] == '-' && uuidRe.MatchString(t):
		return "<uuid>"
	case strings.ContainsAny(t, ".:") && isIP(t):
		return "<ip>"
	}
	if !strings.Contains(t, ":") {
		// no host:port
	} else if host, port, err := net.SplitHostPort(t); err == nil && isDigits(port) {
		if isIP(host) {
			return "<ip>"
		}
		return normalizeToken(host)
	}
	if isDigits(t) {
		return normalizeNumber(t)
	}
	if n >= 12 && hexIDRe.MatchString(t) && hasDigit(t) && hasLetter(t) {
		return "<hex>" // container IDs, commit SHAs
	}
	return stripPodSuffix(t)
}

// stripPodSuffix removes the parts Kubernetes appends to workload names:
//
//	checkout-7d9f8b6c5-x2x4k → checkout       Deployment pod (template hash + suffix)
//	backup-28391234-t8w2m    → backup         CronJob pod (scheduled minute + suffix)
//	node-exporter-x2x4k      → node-exporter  DaemonSet / bare pod suffix
//	checkout-7d9f8b6c5       → checkout       ReplicaSet name
//
// Hand-written rather than regexes like `^(.+)-…$`, which backtrack over
// every dash and were most of Fingerprint's cost. Bare suffixes (no hash in
// front) are only stripped when they mix letters and digits, so "redis-cache"
// or "web-stdby" survive; StatefulSet ordinals (kafka-0) never match.
func stripPodSuffix(t string) string {
	i := strings.LastIndexByte(t, '-')
	if i <= 0 {
		return t
	}
	rest, last := t[:i], t[i+1:]
	if len(last) == 5 && isK8sSuffix(last) {
		if j := strings.LastIndexByte(rest, '-'); j > 0 {
			mid := rest[j+1:]
			if (len(mid) >= 8 && isDigits(mid)) || (len(mid) >= 6 && len(mid) <= 10 && isK8sSuffix(mid)) {
				return rest[:j]
			}
		}
		if hasDigit(last) && hasLetter(last) {
			return rest
		}
		return t
	}
	if len(last) >= 6 && len(last) <= 10 && isK8sSuffix(last) && hasDigit(last) && hasLetter(last) {
		return rest
	}
	return t
}

// isK8sSuffix reports whether s uses only the alphabet Kubernetes generates
// pod and ReplicaSet suffixes from: no vowels and no 0, 1 or 3, so suffixes
// never spell words like "service" or "cache".
func isK8sSuffix(s string) bool {
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune("bcdfghjklmnpqrstvwxz2456789", rune(s[i])) {
			return false
		}
	}
	return true
}

// normalizeNumber turns Unix timestamps and long IDs into placeholders; short
// numbers (ports, HTTP codes, "10050") are kept.
func normalizeNumber(t string) string {
	switch len(t) {
	case 10, 13, 16, 19: // Unix seconds, ms, µs, ns
		// between 2001-09-09 and 2100 in seconds, scaled to the unit
		if n, err := strconv.ParseInt(t[:10], 10, 64); err == nil && n >= 1_000_000_000 && n < 4_102_444_800 {
			return "<ts>"
		}
	}
	if len(t) >= 6 {
		return "<num>"
	}
	return t
}

func isIP(s string) bool {
	if i := strings.IndexByte(s, '%'); i > 0 { // fe80::1%eth0
		s = s[:i]
	}
	return net.ParseIP(s) != nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func hasDigit(s string) bool { return strings.ContainsAny(s, "0123456789") }

func hasLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'z' {
			return true
		}
	}
	return false
}
