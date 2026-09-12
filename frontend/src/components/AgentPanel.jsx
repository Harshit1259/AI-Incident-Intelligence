/**
 * Agent fleet — connected NeuroOps agents and their live telemetry.
 *
 * Master–detail, matching the incident workspace so the two read as the same
 * product. Emoji markers (🖥, ←, 📊) were replaced with real icons; they were
 * the main reason this screen looked unlike the rest of the app.
 *
 * When no agents exist the page becomes the install guide rather than an empty
 * shell — that is the only useful thing to show at that point.
 */
import { useState, useEffect, useCallback } from "react";
import {
  Activity, AlertTriangle, Bot, Check, Copy, Cpu, HardDrive, MemoryStick,
  Play, Plug, RefreshCw, ScrollText, Server, Settings, Square, Trash2, X,
} from "lucide-react";

import { getAgents, getAgentByID, startAgent, stopAgent, deleteAgent } from "../api/agents.js";
import {
  EmptyState, ErrorState, Grid, Page, PageHeader, Panel, SkeletonRows, StatTile,
} from "./ui/Primitives.jsx";

const STATUS = {
  active: { tone: "tone-success", label: "Active" },
  stopped: { tone: "tone-neutral", label: "Stopped" },
  inactive: { tone: "tone-warning", label: "Inactive" },
  error: { tone: "tone-danger", label: "Error" },
};

const statusCfg = (s) => STATUS[s] || STATUS.inactive;

function ago(dateStr) {
  if (!dateStr) return "never";
  const sec = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (sec < 60) return `${sec}s ago`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
  return `${Math.floor(sec / 86400)}d ago`;
}

/* Utilisation bar. Tone is derived from the value, not passed in — 92% CPU is
   critical no matter which metric it belongs to. */
function UsageBar({ label, icon: Icon, pct }) {
  const p = Math.min(100, Math.max(0, pct || 0));
  const tone = p > 90 ? "tone-danger" : p > 70 ? "tone-warning" : "tone-success";
  return (
    <div className={`usage ${tone}`}>
      <div className="usage-head">
        <span className="usage-label">
          {Icon && <Icon size={11} />} {label}
        </span>
        <span className="usage-value">{p.toFixed(0)}%</span>
      </div>
      <div className="usage-track">
        <div className="usage-fill" style={{ width: `${p}%` }} />
      </div>
    </div>
  );
}

function AgentRow({ agent, selected, onSelect, onRefresh }) {
  const cfg = statusCfg(agent.status);
  const [confirm, setConfirm] = useState(false);
  const [err, setErr] = useState("");

  async function act(fn, e) {
    e.stopPropagation();
    setErr("");
    try {
      await fn(agent.id);
      onRefresh();
    } catch (ex) {
      setErr(ex.message || "Action failed");
    }
  }

  return (
    <div
      className={`agent-row${selected ? " is-active" : ""}`}
      onClick={() => onSelect(agent.id)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => e.key === "Enter" && onSelect(agent.id)}
    >
      <div className="agent-row-main">
        <span className={`agent-dot ${cfg.tone}${agent.status === "active" ? " is-live" : ""}`} aria-hidden="true" />
        <span className="agent-row-text">
          <span className="agent-row-name">{agent.name || agent.id.slice(0, 14)}</span>
          <span className="agent-row-meta">
            {agent.host_ip || "—"} · {agent.os_type || "Unknown OS"} · v{agent.version || "?"}
          </span>
        </span>
        <span className="agent-row-age">{ago(agent.last_seen_at)}</span>
      </div>

      <div className="agent-row-actions">
        {agent.status !== "active" ? (
          <button type="button" className="btn btn-ghost btn-xs" onClick={(e) => act(startAgent, e)}>
            <Play size={10} /> Start
          </button>
        ) : (
          <button type="button" className="btn btn-ghost btn-xs" onClick={(e) => act(stopAgent, e)}>
            <Square size={10} /> Stop
          </button>
        )}
        {confirm ? (
          <>
            <button type="button" className="btn btn-danger btn-xs" onClick={(e) => act(deleteAgent, e)}>
              Confirm
            </button>
            <button
              type="button" className="btn btn-ghost btn-xs"
              onClick={(e) => {
                e.stopPropagation();
                setConfirm(false);
              }}
            >
              <X size={10} />
            </button>
          </>
        ) : (
          <button
            type="button" className="btn btn-ghost btn-xs"
            title="Remove agent"
            onClick={(e) => {
              e.stopPropagation();
              setConfirm(true);
            }}
          >
            <Trash2 size={10} />
          </button>
        )}
      </div>

      {err && <p className="agent-row-error">{err}</p>}
    </div>
  );
}

/* Mounted with key={agentID} by the parent, so a new selection remounts this
   component. That makes the initial useState values the reset — no setState
   inside the effect body, and no stale data flashing from the previous agent. */
function AgentDetail({ agentID }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!agentID) return undefined;
    let alive = true;
    getAgentByID(agentID)
      .then((d) => {
        if (alive) setData(d);
      })
      .catch(() => {
        if (alive) setData(null);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [agentID]);

  if (loading) return <SkeletonRows count={4} height={54} />;
  if (!data) return <EmptyState icon={Bot} title="Agent not found" message="It may have been removed." />;

  const info = data.agent || data;
  const metrics = data.recent_metrics || {};
  const cpuMem = metrics.cpu_memory?.[0];
  const disk = metrics.disk?.[0];
  const load = metrics.system_load?.[0];
  const sysInfo = metrics.system_info?.[0];
  const cfg = statusCfg(info.status);

  const metaItems = [
    ["Hostname", sysInfo?.["system.hostname"] || info.name],
    ["IP address", info.host_ip],
    ["OS", sysInfo?.["system.os"] || info.os_type],
    ["Architecture", sysInfo?.["system.architecture"]],
    ["Kernel", sysInfo?.["system.kernel.version"]],
    ["CPU cores", sysInfo?.["system.cpu.count"] || cpuMem?.["system.cpu.core.count"]],
    ["Version", info.version ? `v${info.version}` : null],
    ["Last seen", info.last_seen_at ? ago(info.last_seen_at) : null],
    ["Registered", info.registered_at ? new Date(info.registered_at).toLocaleDateString() : null],
  ].filter(([, v]) => v);

  return (
    <div className="agent-detail">
      <header className="agent-detail-head">
        <span className={`agent-detail-avatar ${cfg.tone}`} aria-hidden="true">
          <Server size={18} />
        </span>
        <div>
          <h2 className="agent-detail-name">{info.name || info.id}</h2>
          <span className={`pill is-plain ${cfg.tone}`}>{cfg.label}</span>
        </div>
      </header>

      {cpuMem && (
        <section className="detail-block tone-brand">
          <h3 className="detail-block-title">Live metrics</h3>
          <div className="usage-stack">
            <UsageBar label="CPU" icon={Cpu} pct={cpuMem["system.cpu.used.percent"]} />
            <UsageBar label="Memory" icon={MemoryStick} pct={cpuMem["system.memory.used.percent"]} />
            {disk && <UsageBar label="Disk" icon={HardDrive} pct={disk["disk.used.percent"]} />}
          </div>
          {load && (
            <div className="load-row">
              {[
                ["1m", load["system.load.1"]],
                ["5m", load["system.load.5"]],
                ["15m", load["system.load.15"]],
              ].map(([k, v]) => (
                <div key={k} className="load-stat">
                  <span className="load-value">{(v || 0).toFixed(2)}</span>
                  <span className="load-label">load {k}</span>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      <section className="detail-block">
        <h3 className="detail-block-title">System info</h3>
        <dl className="kv-list">
          {metaItems.map(([k, v]) => (
            <div key={k} className="kv-item">
              <dt>{k}</dt>
              <dd>{v}</dd>
            </div>
          ))}
        </dl>
      </section>
    </div>
  );
}

const INSTALL_STEPS = [
  "Download and install the NeuroOps agent on your host.",
  "Configure agent.json — set neuroops.product.host to your server URL.",
  "Start the agent: sudo systemctl start neuroops-agent",
  "The agent auto-registers and begins sending metrics, logs and traces.",
];

const COLLECTS = [
  { icon: Cpu, label: "CPU, memory, disk, network" },
  { icon: ScrollText, label: "System logs (syslog, auth)" },
  { icon: Activity, label: "Distributed traces (OTLP)" },
  { icon: Settings, label: "Process & service health" },
  { icon: Server, label: "Load averages & uptime" },
  { icon: Plug, label: "Custom plugins (Go / Python)" },
];

function SetupGuide() {
  const [copied, setCopied] = useState("");
  const endpoints = [
    ["Ingest endpoint", `${window.location.origin}/api/v1/ingest/agent`, "ingest"],
    ["Register endpoint", `${window.location.origin}/api/v1/agents/register`, "register"],
  ];

  function copy(text, key) {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key);
      setTimeout(() => setCopied(""), 1800);
    });
  }

  return (
    <div className="ui-split">
      <Panel title="Install the NeuroOps agent" sub="four steps to first telemetry">
        <ol className="setup-steps">
          {INSTALL_STEPS.map((step, i) => (
            <li key={i} className="setup-step">
              <span className="setup-step-index">{i + 1}</span>
              <span className="setup-step-text">{step}</span>
            </li>
          ))}
        </ol>

        <div className="detail-section is-divided">
          <h4 className="detail-section-label">Endpoints</h4>
          {endpoints.map(([label, url, key]) => (
            <div key={key} className="endpoint">
              <div className="endpoint-text">
                <p className="endpoint-label">{label}</p>
                <div className="endpoint-url">
                  <code>{url}</code>
                </div>
              </div>
              <button type="button" className="btn btn-outline btn-xs copy-btn" onClick={() => copy(url, key)}>
                {copied === key ? <Check size={11} /> : <Copy size={11} />}
                {copied === key ? "Copied" : "Copy"}
              </button>
            </div>
          ))}
        </div>
      </Panel>

      <Panel title="What agents collect" sub="out of the box, no configuration">
        <ul className="collect-list">
          {COLLECTS.map(({ icon: Icon, label }) => (
            <li key={label} className="collect-item">
              <span className="collect-icon" aria-hidden="true">
                {Icon && <Icon size={12} />}
              </span>
              {label}
            </li>
          ))}
        </ul>
      </Panel>
    </div>
  );
}

export default function AgentPanel() {
  const [agents, setAgents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectedID, setSelectedID] = useState(null);
  const [lastRefresh, setLastRefresh] = useState(null);

  const load = useCallback(async () => {
    try {
      const d = await getAgents();
      setAgents(Array.isArray(d) ? d : []);
      setLastRefresh(new Date());
      setError("");
    } catch (e) {
      // Previously swallowed, so a failed request rendered the install guide —
      // telling an operator with a live fleet that they had no agents.
      setError(e.message || "Could not load agents");
      setAgents([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const t = setInterval(load, 30000);
    return () => clearInterval(t);
  }, [load]);

  const active = agents.filter((a) => a.status === "active").length;
  const inactive = agents.filter((a) => a.status === "inactive" || a.status === "stopped").length;
  const errored = agents.filter((a) => a.status === "error").length;

  return (
    <Page>
      <PageHeader
        title="Agent fleet"
        meta={`Real-time telemetry from your infrastructure · auto-refreshes every 30s${
          lastRefresh ? ` · updated ${lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : ""
        }`}
        actions={
          <button type="button" className="btn btn-ghost" onClick={load}>
            <RefreshCw size={13} className={loading ? "is-spinning" : ""} /> Refresh
          </button>
        }
      />

      <Grid cols={4}>
        <StatTile label="Total agents" icon={Bot} tone="neutral" value={agents.length} sub="registered" loading={loading} />
        <StatTile label="Active" icon={Activity} tone="success" value={active} sub="reporting in" loading={loading} />
        <StatTile label="Inactive" icon={Square} tone={inactive > 0 ? "warning" : "neutral"} value={inactive} sub="stopped or silent" loading={loading} />
        <StatTile label="Errored" icon={AlertTriangle} tone={errored > 0 ? "danger" : "neutral"} value={errored} sub="need attention" loading={loading} />
      </Grid>

      {error ? (
        <ErrorState message={error} onRetry={load} />
      ) : loading ? (
        <SkeletonRows count={4} height={64} />
      ) : agents.length === 0 ? (
        <SetupGuide />
      ) : (
        <div className="agent-layout">
          <Panel title={`${agents.length} agent${agents.length !== 1 ? "s" : ""}`} flush className="agent-list-panel">
            <div className="agent-list">
              {agents.map((a) => (
                <AgentRow
                  key={a.id}
                  agent={a}
                  selected={a.id === selectedID}
                  onSelect={(id) => setSelectedID((prev) => (prev === id ? null : id))}
                  onRefresh={load}
                />
              ))}
            </div>
          </Panel>

          <Panel flush className="agent-detail-panel">
            {selectedID ? (
              <AgentDetail key={selectedID} agentID={selectedID} />
            ) : (
              <EmptyState
                icon={Server}
                title="Select an agent"
                message="Pick a host to see live CPU, memory, disk and system details."
              />
            )}
          </Panel>
        </div>
      )}
    </Page>
  );
}
