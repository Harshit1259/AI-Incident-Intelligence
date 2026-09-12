package fingerprint

import (
	"regexp"
	"testing"
)

func alert(name string, kv ...string) Alert {
	labels := make(map[string]string, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		labels[kv[i]] = kv[i+1]
	}
	return Alert{Name: name, Labels: labels}
}

// Each row is two alerts and whether they are the same problem. A fingerprint
// that merges everything is as broken as one that merges nothing, so the
// "different" rows matter as much as the "same" ones.
func TestFingerprint(t *testing.T) {
	tests := []struct {
		name string
		a, b Alert
		same bool
	}{
		// ── the drill's edge cases ──────────────────────────────────────
		{"1 replicaset hash + pod suffix",
			alert("PodCrashLooping", "pod", "checkout-7d9f8b6c5-x2x4k"),
			alert("PodCrashLooping", "pod", "checkout-7d9f8b6c5-p9q2m"), true},
		{"2 new rollout, different template hash",
			alert("PodCrashLooping", "pod", "checkout-7d9f8b6c5-x2x4k"),
			alert("PodCrashLooping", "pod", "checkout-5c8d7f9b4-b7k2q"), true},
		{"3 UUID",
			alert("RequestFailed", "detail", "request 3f2a8c1e-9b4d-4e6f-a1b2-c3d4e5f6a7b8 failed"),
			alert("RequestFailed", "detail", "request 0a1b2c3d-4e5f-6789-abcd-ef0123456789 failed"), true},
		{"4 IPv4",
			alert("NodeDown", "instance", "10.0.3.17"),
			alert("NodeDown", "instance", "10.0.3.18"), true},
		{"5 IPv6, bare and bracketed with port",
			alert("NodeDown", "instance", "fe80::1"),
			alert("NodeDown", "instance", "[2001:db8::2]:9100"), true},
		{"6 port",
			alert("TargetDown", "instance", "web01:9100"),
			alert("TargetDown", "instance", "web01:9200"), true},
		{"7 timestamps: ISO vs Unix seconds",
			alert("BackupLate", "detail", "last run 2026-09-13T10:00:00Z"),
			alert("BackupLate", "detail", "last run 1789239068"), true},
		{"8 job suffix (CronJob pod)",
			alert("JobFailed", "pod", "backup-28391234-t8w2m"),
			alert("JobFailed", "pod", "backup-28391299-k9zq4"), true},
		{"9 StatefulSet ordinal is kept",
			alert("BrokerDown", "pod", "kafka-0"),
			alert("BrokerDown", "pod", "kafka-1"), false},
		{"10 numbered hostname is kept",
			alert("HighCPU", "instance", "web01"),
			alert("HighCPU", "instance", "web02"), false},
		{"11 label order",
			Alert{Name: "HighCPU", Labels: map[string]string{"a": "1", "b": "2", "c": "3", "d": "4"}},
			Alert{Name: "HighCPU", Labels: map[string]string{"d": "4", "c": "3", "b": "2", "a": "1"}}, true},
		{"12 different alert name",
			alert("HighCPU", "instance", "web01"),
			alert("DiskFull", "instance", "web01"), false},

		// ── more cases worth pinning ────────────────────────────────────
		{"measured value is not identity (95% vs 96% CPU)",
			alert("HighCPU", "instance", "web01", "annotation.value", "95"),
			alert("HighCPU", "instance", "web01", "annotation.value", "96"), true},
		{"runbook / rule links are not identity",
			alert("HighCPU", "runbook_url", "https://a/x?id=1", "grafana.rule_url", "https://g/1"),
			alert("HighCPU", "runbook_url", "https://a/x?id=2"), true},
		{"case and whitespace",
			alert(" HighCPU ", "Service", " Checkout "),
			alert("highcpu", "service", "checkout"), true},
		{"empty label equals missing label",
			alert("HighCPU", "service", "checkout", "team", ""),
			alert("HighCPU", "service", "checkout"), true},
		{"a label value cannot forge another label",
			alert("X", "a", "b|c=d"),
			alert("X", "a", "b", "c", "d"), false},
		{"different service",
			alert("HighCPU", "service", "checkout"),
			alert("HighCPU", "service", "payments"), false},
		{"DaemonSet pod suffix",
			alert("ExporterDown", "pod", "node-exporter-x2x4k"),
			alert("ExporterDown", "pod", "node-exporter-q7m4z"), true},
		{"ReplicaSet name label",
			alert("ReplicasMismatch", "replicaset", "checkout-7d9f8b6c5"),
			alert("ReplicasMismatch", "replicaset", "checkout-5c8d7f9b4"), true},
		{"words that look like suffixes are kept",
			alert("Down", "service", "redis-cache"),
			alert("Down", "service", "redis-store"), false},
		{"IPv6 with zone",
			alert("NodeDown", "instance", "fe80::1%eth0"),
			alert("NodeDown", "instance", "fe80::2%eth1"), true},
		{"timestamps: date + time with a space, and milliseconds",
			alert("BackupLate", "detail", "at 2026-09-13 10:00:00.123+05:30"),
			alert("BackupLate", "detail", "at 1789239068123"), false}, // "<ts> <ts>" vs "<ts>": word count differs
		{"timestamps: two date-times with a space",
			alert("BackupLate", "detail", "at 2026-09-13 10:00:00"),
			alert("BackupLate", "detail", "at 2026-09-14 23:59:59"), true},
		{"Zabbix-style date",
			alert("Late", "detail", "since 2026.09.13"),
			alert("Late", "detail", "since 2026.09.14"), true},
		{"long numeric IDs",
			alert("OrderStuck", "detail", "order 12345678 stuck"),
			alert("OrderStuck", "detail", "order 87654321 stuck"), true},
		{"short numbers are kept (HTTP status)",
			alert("Errors", "code", "500"),
			alert("Errors", "code", "503"), false},
		{"container ID",
			alert("OOMKilled", "detail", "container 4f2a9c8b1d3e oom"),
			alert("OOMKilled", "detail", "container 9e8d7c6b5a4f oom"), true},
		{"namespace/pod path",
			alert("PodCrashLooping", "target", "shop/checkout-7d9f8b6c5-x2x4k"),
			alert("PodCrashLooping", "target", "shop/checkout-5c8d7f9b4-b7k2q"), true},
		{"nil labels",
			Alert{Name: "HighCPU"},
			Alert{Name: "HighCPU", Labels: map[string]string{}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa, fb := Fingerprint(tt.a), Fingerprint(tt.b)
			if (fa == fb) != tt.same {
				t.Errorf("same = %v, want %v\n a: %+v\n b: %+v", fa == fb, tt.same, tt.a, tt.b)
			}
		})
	}
}

// Shows exactly what each value becomes — easier to read than hashes when a
// case above fails.
func TestNormalizeValue(t *testing.T) {
	tests := []struct{ in, want string }{
		{"checkout-7d9f8b6c5-x2x4k", "checkout"},
		{"backup-28391234-t8w2m", "backup"},
		{"node-exporter-x2x4k", "node-exporter"},
		{"checkout-7d9f8b6c5", "checkout"},
		{"kafka-0", "kafka-0"},
		{"web01", "web01"},
		{"web01:9100", "web01"},
		{"10.0.3.17:9100", "<ip>"},
		{"[2001:db8::2]:9100", "<ip>"},
		{"fe80::1%eth0", "<ip>"},
		{"3f2a8c1e-9b4d-4e6f-a1b2-c3d4e5f6a7b8", "<uuid>"},
		{"2026-09-13t10:00:00z", "<ts>"},
		{"2026-09-13 10:00:00.123+05:30", "<ts> <ts>"},
		{"1789239068", "<ts>"},
		{"1789239068123", "<ts>"},
		{"12345678", "<num>"},
		{"10050", "10050"},
		{"4f2a9c8b1d3e", "<hex>"},
		{"redis-cache", "redis-cache"},
		{"cpu on 10.0.0.5, pod api-6f7d8c9b5-k2m4p (ready)", "cpu on <ip>, pod api (ready)"},
		{"shop/checkout-7d9f8b6c5-x2x4k", "shop/checkout"},
	}
	for _, tt := range tests {
		if got := normalizeValue(tt.in); got != tt.want {
			t.Errorf("normalizeValue(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestFingerprintIsSHA256Hex(t *testing.T) {
	a := alert("HighCPU", "instance", "web01")
	f := Fingerprint(a)
	if !hex64.MatchString(f) {
		t.Fatalf("fingerprint %q is not 64 hex chars", f)
	}
	for i := 0; i < 50; i++ { // map iteration order must never leak in
		if Fingerprint(a) != f {
			t.Fatal("fingerprint is not deterministic")
		}
	}
}

func BenchmarkFingerprint(b *testing.B) {
	a := Alert{Name: "KubePodCrashLooping", Labels: map[string]string{
		"namespace": "shop", "pod": "checkout-7d9f8b6c5-x2x4k", "container": "app",
		"instance": "10.0.3.17:8080", "job": "kube-state-metrics", "severity": "critical",
		"service": "checkout", "annotation.value": "5", "cluster": "prod-eu-1",
	}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Fingerprint(a)
	}
}

func BenchmarkFingerprintPlainLabels(b *testing.B) {
	a := Alert{Name: "HighCPU", Labels: map[string]string{
		"service": "checkout", "severity": "critical", "team": "payments", "env": "prod",
	}}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Fingerprint(a)
	}
}
