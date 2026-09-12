package ingest

import "testing"

func TestDeriveAlertService(t *testing.T) {
	ksm := "kube-prometheus-stack-kube-state-metrics"
	tests := []struct {
		name       string
		labels     map[string]string
		want, from string
	}{
		{"real service kept", map[string]string{"service": "checkout", "namespace": "shop", "container": "api"}, "checkout", ""},
		{"no service label", map[string]string{"alertname": "X"}, "", ""},
		{"crashloop → namespace/container", map[string]string{"service": ksm, "namespace": "shop", "container": "checkout", "pod": "checkout-7d4b9c8f6d-x2k9p"}, "shop/checkout", "namespace/container"},
		{"deployment wins over container", map[string]string{"service": ksm, "namespace": "shop", "deployment": "payments", "container": "app"}, "shop/payments", "namespace/deployment"},
		{"statefulset", map[string]string{"service": "kube-state-metrics", "namespace": "data", "statefulset": "kafka"}, "data/kafka", "namespace/statefulset"},
		{"pod suffix stripped", map[string]string{"service": ksm, "namespace": "shop", "pod": "checkout-7d4b9c8f6d-x2k9p"}, "shop/checkout", "namespace/pod"},
		{"statefulset ordinal kept", map[string]string{"service": ksm, "namespace": "data", "pod": "kafka-2"}, "data/kafka-2", "namespace/pod"},
		{"daemonset pod suffix", map[string]string{"service": "kubelet", "namespace": "kube-system", "pod": "fluentd-x2k9p"}, "kube-system/fluentd", "namespace/pod"},
		{"job", map[string]string{"service": ksm, "namespace": "batch", "job_name": "nightly-report"}, "batch/nightly-report", "namespace/job_name"},
		{"only namespace", map[string]string{"service": ksm, "namespace": "shop"}, "shop", "namespace"},
		{"only job", map[string]string{"service": ksm, "job": "kube-state-metrics"}, "kube-state-metrics", "job"},
		{"nothing better: keep original", map[string]string{"service": ksm}, ksm, ""},
		{"other helm release name", map[string]string{"service": "prom-kube-state-metrics", "namespace": "shop", "container": "api"}, "shop/api", "namespace/container"},
		{"node-exporter → node label", map[string]string{"service": "kube-prometheus-stack-prometheus-node-exporter", "node": "worker-3", "instance": "10.0.0.5:9100"}, "node/worker-3", "node"},
		{"node-exporter → instance host", map[string]string{"service": "node-exporter", "instance": "10.0.0.5:9100"}, "node/10.0.0.5", "instance"},
		{"blackbox → probe target", map[string]string{"service": "blackbox-exporter", "instance": "https://shop.example.com/health"}, "probe/shop.example.com", "instance"},
		{"app named like a word is kept", map[string]string{"service": "api-https", "namespace": "shop"}, "api-https", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, from := DeriveAlertService(tt.labels)
			if got != tt.want || from != tt.from {
				t.Errorf("DeriveAlertService = (%q, %q), want (%q, %q)", got, from, tt.want, tt.from)
			}
		})
	}
}

func TestHostOf(t *testing.T) {
	for in, want := range map[string]string{
		"10.0.0.5:9100":              "10.0.0.5",
		"web01":                      "web01",
		"https://shop.example.com/x": "shop.example.com",
		"[2001:db8::1]:9100":         "2001:db8::1",
		"2001:db8::1":                "2001:db8::1",
		"":                           "",
	} {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
}
