package ingest

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Label keys written when the service is derived, so the original value is
// never lost and the UI can explain where the service name came from.
const (
	LabelServiceOriginal    = "service.original"
	LabelServiceDerivedFrom = "service.derived_from"
)

// Kubernetes pod-name suffixes use this alphabet (no vowels, no 0/1/3), which
// keeps names like "api-https" or "db-02" from being mistaken for suffixes.
const k8sSafe = `[bcdfghjklmnpqrstvwxz2456789]`

var (
	replicaSetPodSuffix = regexp.MustCompile(`-` + k8sSafe + `{6,10}-` + k8sSafe + `{5}$`)
	podSuffix           = regexp.MustCompile(`-` + k8sSafe + `{5}$`)
)

// exporterKind reports whether a Prometheus `service` label names the
// exporter that produced the metric rather than the application, and which
// kind of exporter it is ("node", "probe", "k8s"), or "" for a real service.
//
// kube-prometheus-stack sets service to the Kubernetes Service of the
// exporter (e.g. "kube-prometheus-stack-kube-state-metrics"), so every
// kube-state-metrics alert in a cluster would otherwise share one service.
func exporterKind(service string) string {
	s := strings.ToLower(strings.TrimSpace(service))
	switch {
	case s == "":
		return ""
	case strings.HasSuffix(s, "node-exporter"):
		return "node"
	case strings.HasSuffix(s, "blackbox-exporter"):
		return "probe"
	case strings.HasSuffix(s, "kube-state-metrics"), s == "kubelet", s == "cadvisor",
		strings.HasPrefix(s, "kube-prometheus-stack-"):
		return "k8s"
	}
	return ""
}

// DeriveAlertService returns the service an alert should be grouped under.
//
// A real `service` label is used as-is. When it names an exporter, the
// service is built from the labels that identify what the alert is about:
//   - node-exporter:   node/<node or instance host>
//   - blackbox probes: probe/<target host>
//   - Kubernetes:      <namespace>/<deployment|statefulset|daemonset|container|pod|job_name>,
//     else <namespace>, else the job label
//
// derivedFrom names the labels used ("" when the service label was kept), so
// callers can record it alongside the original value.
func DeriveAlertService(labels map[string]string) (service, derivedFrom string) {
	original := strings.TrimSpace(labels["service"])
	kind := exporterKind(original)
	if kind == "" {
		return original, ""
	}

	switch kind {
	case "node":
		if n := strings.TrimSpace(labels["node"]); n != "" {
			return "node/" + n, "node"
		}
		if h := hostOf(labels["instance"]); h != "" {
			return "node/" + h, "instance"
		}
	case "probe":
		if h := hostOf(labels["instance"]); h != "" {
			return "probe/" + h, "instance"
		}
	}

	ns := strings.TrimSpace(labels["namespace"])
	for _, key := range []string{"deployment", "statefulset", "daemonset", "container", "pod", "job_name"} {
		v := strings.TrimSpace(labels[key])
		if v == "" {
			continue
		}
		if key == "pod" {
			v = stripPodSuffix(v)
		}
		if ns != "" {
			return ns + "/" + v, "namespace/" + key
		}
		return v, key
	}
	if ns != "" {
		return ns, "namespace"
	}
	if job := strings.TrimSpace(labels["job"]); job != "" {
		return job, "job"
	}
	return original, "" // nothing better available; keep what we had
}

// stripPodSuffix turns "checkout-7d4b9c8f6d-x2k9p" or "agent-x2k9p" into the
// workload name. StatefulSet ordinals ("kafka-2") are kept.
func stripPodSuffix(pod string) string {
	if replicaSetPodSuffix.MatchString(pod) {
		return replicaSetPodSuffix.ReplaceAllString(pod, "")
	}
	return podSuffix.ReplaceAllString(pod, "")
}

// hostOf returns the host of "10.0.0.5:9100", "web01" or "https://shop.example.com/health".
func hostOf(instance string) string {
	instance = strings.TrimSpace(instance)
	if instance == "" {
		return ""
	}
	if strings.Contains(instance, "://") {
		if u, err := url.Parse(instance); err == nil && u.Hostname() != "" {
			return u.Hostname()
		}
	}
	if host, _, err := net.SplitHostPort(instance); err == nil {
		return host // "host:port", "1.2.3.4:9100", "[2001:db8::1]:9100"
	}
	return instance // no port (including bare IPv6)
}
