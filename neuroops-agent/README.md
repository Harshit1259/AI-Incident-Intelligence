# NeurOps Agent

**Intelligent Observability Agent for the NeurOps Platform**

The NeurOps Agent runs on every monitored host and collects metrics, logs,
traces, and network flows, shipping all telemetry to the NeurOps central
product via ZeroMQ for alerting, correlation, and AI-driven remediation.

---

## Architecture

```
Monitored Host
│
├── neuroops-agent          (Go binary — core coordinator)
│   ├── Metric Collector    → CPU, memory, disk, network, processes, services
│   ├── Log Collector       → file tailing, position tracking, multiline
│   ├── Trace Collector     → OTLP/HTTP proxy (port 4318)
│   ├── Plugin Engine       → Go + Python plugins in plugins/
│   ├── Health Monitor      → HTTP /health (port 8765)
│   ├── ZMQ Publisher       → PUSH → NeurOps product (port 9441)
│   └── ZMQ Subscriber      → SUB  ← NeurOps product (port 9440)
│
└── neuroops-upgrader       (Go binary — watchdog + auto-upgrade)
```

### Data Flow

```
Device / Application
        │
        ▼
Plugin / Collector
  (SSH, SNMP, HTTP, DB, OTLP, file tail, psutil)
        │
        ▼  JSON event envelope
Plugin Engine / Collector
        │
        ▼  base64(JSON) over ZMQ PUSH
NeurOps Product
  (alerting, storage, correlation, AI analysis, dashboards)
```

### Event Envelope (all telemetry types)

```json
{
  "event.type":  "metric",
  "metric.type": "cpu_memory",
  "object.type": "Linux",
  "object.ip":   "192.168.1.100",
  "agent.id":    "a3f1c2d4-...",
  "timestamp":   1719000000,
  "system.cpu.used.percent": 42.5,
  "system.memory.used.bytes": 3221225472
}
```

---

## Requirements

| Component     | Version  |
|---------------|----------|
| Ubuntu        | 18.04 / 20.04 / 22.04 / 24.04 |
| Go (build)    | 1.22+    |
| Python (runtime) | 3.9 (embedded, no system Python needed) |
| ZeroMQ        | 4.x (libzmq5) |

---

## Quick Install

```bash
# Interactive install (prompts for product server IP and ports)
sudo bash install.sh

# Silent / bulk install
sudo bash scripts/bulk-install.sh <product_ip> <pub_port> <sub_port>

# Example
sudo bash scripts/bulk-install.sh 192.168.10.5 9441 9440
```

---

## Build from Source

```bash
# Clone
git clone https://github.com/neuroops/agent.git
cd agent

# Build agent + upgrader for the host OS
make build

# Cross-compile for all Ubuntu versions
make build-all

# Run tests
make test

# Create distribution package
make package
# → dist/neuroops-agent-1.0.0-linux-amd64.tar.gz
```

---

## Configuration

All configuration is in `config/agent.json`.  
The agent watches this file and **hot-reloads** on change.

### Key settings

| Key | Default | Description |
|-----|---------|-------------|
| `agent.neuroops.product.host` | `127.0.0.1` | NeurOps product server IP |
| `agent.neuroops.event.publisher.port` | `9441` | Agent → product (ZMQ PUSH) |
| `agent.neuroops.event.subscriber.port` | `9440` | Product → agent (ZMQ SUB) |
| `agent.metric.agent.enabled` | `true` | Enable metric collection |
| `agent.log.agent.enabled` | `false` | Enable log collection |
| `agent.trace.agent.enabled` | `false` | Enable trace forwarding |
| `agent.health.port` | `8765` | Health HTTP endpoint port |
| `metric.agent.cpu.memory.metric.poll.seconds` | `300` | CPU/memory poll interval |

---

## Writing a Plugin

### Python Metric Plugin

```python
# plugins/metric/my_app/plugin.py
from neuroopssdk.plugin import MetricPlugin

class MyAppPlugin(MetricPlugin):

    def discover(self, context):
        return self.succeed({
            "object.name": "My Application v2.1",
            "object.type": "Application"
        })

    def collect(self, context):
        # Collect metrics from your application
        return self.succeed({
            "myapp.queue.depth": 42,
            "myapp.request.rate": 1250.5,
            "myapp.error.rate": 0.02,
        })
```

### Go Metric Plugin

```go
// plugins/metric/my_device/plugin.go
package main

import "github.com/neuroops/agent/sdk/go/plugin"

type MyDevice struct{}

func (d *MyDevice) Discover(ctx plugin.Context) plugin.Result {
    return plugin.Succeed(plugin.Result{"object.name": "My Device"})
}

func (d *MyDevice) Collect(ctx plugin.Context) plugin.Result {
    return plugin.Succeed(plugin.Result{"device.temperature": 42.5})
}

func main() { plugin.RunMetric(&MyDevice{}) }
```

Build the Go plugin:
```bash
cd plugins/metric/my_device
GOPATH=/neuroops/gopath /neuroops/go/bin/go build -o plugin .
```

---

## Health & Monitoring

```bash
# Quick health check
curl http://localhost:8765/health

# Full detailed health
curl http://localhost:8765/health/detail | python3 -m json.tool

# Prometheus metrics
curl http://localhost:8765/metrics
```

---

## Commands Received from Product (ZMQ)

| `event.type` | Description |
|---|---|
| `metric.poll` | Trigger immediate metric collection |
| `plugin.run` | Execute a named runbook plugin |
| `config.reload` | Hot-reload `agent.json` |
| `agent.shutdown` | Graceful shutdown |
| `agent.upgrade` | Download + apply upgrade |
| `discovery` | Re-run discovery on all metric plugins |
| `agent.ping` | Health ping (agent responds with `agent.pong`) |

---

## Directory Layout

```
neuroops-agent/
├── cmd/
│   ├── agent/main.go          — Agent entry point
│   └── upgrader/main.go       — Upgrader / watchdog entry point
├── internal/
│   ├── agent/agent.go         — Core coordinator
│   ├── config/config.go       — Configuration management
│   ├── transport/             — ZeroMQ publisher & subscriber
│   ├── collectors/            — Metric, log, trace collectors
│   ├── plugin/engine.go       — Plugin discovery & execution
│   ├── health/monitor.go      — HTTP health endpoint
│   └── logger/logger.go       — Structured JSON logger
├── sdk/
│   ├── go/                    — Go SDK (types, plugin interfaces, clients)
│   └── python/neuroopssdk/    — Python SDK
├── plugins/
│   └── examples/              — Example metric + runbook plugins
├── config/
│   ├── agent.json             — Default configuration
│   └── otel-config.yaml       — OpenTelemetry collector config
├── services/
│   ├── neuroops-agent.service     — systemd unit
│   └── neuroops-upgrader.service  — systemd unit (watchdog)
├── scripts/
│   ├── install.sh             — Interactive installer
│   └── bulk-install.sh        — Silent/automated installer
├── Makefile
├── Dockerfile
└── go.mod
```

---

## Supported Collection Types

| Type | How | Protocols / Libraries |
|------|-----|-----------------------|
| Host metrics | Built-in | gopsutil (CPU, mem, disk, net, proc, svc) |
| Logs | File tailing | inotify / fsnotify |
| Traces | OTLP proxy | OpenTelemetry HTTP (port 4318) |
| Network flows | Packet capture | pcap (flow.agent) |
| Linux/Unix remote | SSH | golang.org/x/crypto/ssh, paramiko |
| Network devices | SNMP | gosnmp, pysnmp |
| REST APIs | HTTP | net/http, requests |
| VMware | SOAP | govmomi, pyvmomi |
| Windows remote | WinRM | masterzen/winrm, pywinrm |
| Databases | JDBC-equiv | MySQL, PostgreSQL, MSSQL, Oracle, HANA, DB2, MongoDB |
| Cloud (AWS) | SDK | boto3 |
| Cloud (Azure) | SDK | adal + azure-sdk |
| Citrix XenServer | XML-RPC | go-xen-api-client, XenAPI |

---

## License

Copyright (c) NeurOps 2025. All rights reserved.  
Proprietary and confidential. Unauthorised copying or distribution is prohibited.
