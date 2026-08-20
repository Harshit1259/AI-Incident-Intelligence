// AgentPanel.jsx — NeurOps Agent Fleet Management

import { useState, useEffect, useCallback } from "react";
import { getAgents, getAgentByID, startAgent, stopAgent, deleteAgent } from "../api/agents.js";

// ─── Config ───────────────────────────────────────────────────────────────────

const STATUS = {
  active:   { color: "#00D084", label: "Active",   dot: "#00D084" },
  stopped:  { color: "#64748b", label: "Stopped",  dot: "#64748b" },
  inactive: { color: "#FFBC00", label: "Inactive", dot: "#FFBC00" },
  error:    { color: "#FF3B3B", label: "Error",    dot: "#FF3B3B" },
};

function sCfg(s) { return STATUS[s] || STATUS.inactive; }

function ago(dateStr) {
  if (!dateStr) return "never";
  const sec = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (sec < 60)    return `${sec}s ago`;
  if (sec < 3600)  return `${Math.floor(sec / 60)}m ago`;
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`;
  return `${Math.floor(sec / 86400)}d ago`;
}

// ─── Mini metric bar ──────────────────────────────────────────────────────────

function MiniBar({ pct, color }) {
  const p = Math.min(100, Math.max(0, pct || 0));
  const c = p > 90 ? "#FF3B3B" : p > 70 ? "#FFBC00" : color || "#00D084";
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
      <div style={{ flex: 1, height: 4, background: "rgba(255,255,255,0.07)", borderRadius: 2 }}>
        <div style={{ width: `${p}%`, height: "100%", background: c, borderRadius: 2, transition: "width 0.4s" }} />
      </div>
      <span style={{ fontSize: "0.63rem", color: c, minWidth: 30, textAlign: "right", fontWeight: 700 }}>
        {p.toFixed(0)}%
      </span>
    </div>
  );
}

// ─── Agent Card ───────────────────────────────────────────────────────────────

function AgentCard({ agent, selected, onSelect, onRefresh }) {
  const cfg = sCfg(agent.status);
  const [confirm, setConfirm] = useState(false);
  const [err, setErr]         = useState("");

  async function act(fn, e) {
    e.stopPropagation();
    setErr("");
    try { await fn(agent.id); onRefresh(); }
    catch (ex) { setErr(ex.message || "Action failed"); }
  }

  const btnBase = {
    padding: "3px 10px", fontSize: "0.68rem", fontWeight: 600,
    borderRadius: 5, cursor: "pointer", border: "1px solid",
    transition: "all 0.12s",
  };

  return (
    <div
      onClick={() => onSelect(agent.id)}
      style={{
        padding: "12px 14px",
        borderBottom: "1px solid var(--border)",
        background: selected ? "rgba(0,102,255,0.06)" : "transparent",
        borderLeft: selected ? "3px solid #0066FF" : "3px solid transparent",
        cursor: "pointer", transition: "background 0.12s",
      }}
    >
      {/* Top row: status dot + name + actions */}
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        {/* Status dot */}
        <div style={{
          width: 8, height: 8, borderRadius: "50%",
          background: cfg.dot, flexShrink: 0,
          boxShadow: agent.status === "active" ? `0 0 6px ${cfg.dot}` : "none",
        }} />

        {/* Name + meta */}
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ fontSize: "0.83rem", fontWeight: 700, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            {agent.name || agent.id.slice(0, 14)}
          </div>
          <div style={{ fontSize: "0.67rem", color: "var(--t3)", marginTop: 1 }}>
            {agent.host_ip || "—"} · {agent.os_type || "Unknown OS"} · v{agent.version || "?"}
          </div>
        </div>

        {/* Last seen */}
        <span style={{ fontSize: "0.65rem", color: "var(--t4)", flexShrink: 0 }}>
          {ago(agent.last_seen_at)}
        </span>

        {/* Action buttons */}
        <div style={{ display: "flex", gap: 4, flexShrink: 0 }} onClick={e => e.stopPropagation()}>
          {agent.status !== "active" ? (
            <button style={{ ...btnBase, color: "#00D084", borderColor: "rgba(0,208,132,0.3)", background: "rgba(0,208,132,0.08)" }}
              onClick={e => act(startAgent, e)}>▶ Start</button>
          ) : (
            <button style={{ ...btnBase, color: "#FFBC00", borderColor: "rgba(255,188,0,0.3)", background: "rgba(255,188,0,0.07)" }}
              onClick={e => act(stopAgent, e)}>⏸ Stop</button>
          )}
          {confirm ? (
            <>
              <button style={{ ...btnBase, color: "#FF3B3B", borderColor: "rgba(255,59,59,0.4)", background: "rgba(255,59,59,0.12)" }}
                onClick={e => act(deleteAgent, e)}>Confirm</button>
              <button style={{ ...btnBase, color: "var(--t3)", borderColor: "var(--border-strong)", background: "transparent" }}
                onClick={e => { e.stopPropagation(); setConfirm(false); }}>✕</button>
            </>
          ) : (
            <button style={{ ...btnBase, color: "var(--t4)", borderColor: "transparent", background: "transparent" }}
              onClick={e => { e.stopPropagation(); setConfirm(true); }}>✕</button>
          )}
        </div>
      </div>

      {/* Error */}
      {err && <div style={{ fontSize: "0.68rem", color: "#FF3B3B", marginTop: 5 }}>{err}</div>}
    </div>
  );
}

// ─── Agent Detail ─────────────────────────────────────────────────────────────

function AgentDetail({ agentID }) {
  const [data,    setData]    = useState(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!agentID) return;
    let alive = true;
    setLoading(true);
    getAgentByID(agentID)
      .then(d => { if (alive) setData(d); })
      .catch(() => { if (alive) setData(null); })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [agentID]);

  if (loading) return <div style={{ padding: 24, color: "var(--t3)", fontSize: "0.8rem" }}>Loading…</div>;
  if (!data)   return <div style={{ padding: 24, color: "var(--t3)", fontSize: "0.8rem" }}>Agent not found.</div>;

  const info    = data.agent || data;
  const metrics = data.recent_metrics || {};
  const cpuMem  = metrics.cpu_memory?.[0];
  const disk    = metrics.disk?.[0];
  const load    = metrics.system_load?.[0];
  const sysInfo = metrics.system_info?.[0];
  const cfg     = sCfg(info.status);

  const metaItems = [
    ["Hostname",     sysInfo?.["system.hostname"] || info.name],
    ["IP Address",   info.host_ip],
    ["OS",           sysInfo?.["system.os"] || info.os_type],
    ["Architecture", sysInfo?.["system.architecture"]],
    ["Kernel",       sysInfo?.["system.kernel.version"]],
    ["CPU Cores",    sysInfo?.["system.cpu.count"] || cpuMem?.["system.cpu.core.count"]],
    ["Version",      info.version ? `v${info.version}` : null],
    ["Last Seen",    info.last_seen_at ? ago(info.last_seen_at) : null],
    ["Registered",   info.registered_at ? new Date(info.registered_at).toLocaleDateString() : null],
  ];

  return (
    <div style={{ padding: "20px 22px", overflowY: "auto", height: "100%" }}>
      {/* Agent name + status */}
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 18 }}>
        <div style={{
          width: 38, height: 38, borderRadius: 10,
          background: `${cfg.color}18`, border: `1px solid ${cfg.color}40`,
          display: "flex", alignItems: "center", justifyContent: "center", fontSize: 18,
        }}>
          🖥
        </div>
        <div>
          <div style={{ fontSize: "1rem", fontWeight: 700, color: "var(--t1)" }}>
            {info.name || info.id}
          </div>
          <div style={{ fontSize: "0.72rem", color: cfg.color, marginTop: 2, fontWeight: 600 }}>
            ● {cfg.label}
          </div>
        </div>
      </div>

      {/* Live metrics */}
      {cpuMem && (
        <div style={{
          padding: "12px 14px", borderRadius: 10, marginBottom: 16,
          background: "rgba(0,102,255,0.05)", border: "1px solid rgba(0,102,255,0.12)",
        }}>
          <div className="lux-eyebrow" style={{ marginBottom: 10 }}>LIVE METRICS</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {[
              ["CPU Usage",    cpuMem["system.cpu.used.percent"],    "#0066FF"],
              ["Memory",       cpuMem["system.memory.used.percent"],  "#00C8E8"],
              disk   ? ["Disk",   disk["disk.used.percent"],          "#a78bfa"] : null,
            ].filter(Boolean).map(([label, pct, color]) => (
              <div key={label}>
                <div style={{ fontSize: "0.67rem", color: "var(--t3)", marginBottom: 3 }}>{label}</div>
                <MiniBar pct={pct} color={color} />
              </div>
            ))}
            {load && (
              <div style={{ display: "flex", justifyContent: "space-between", marginTop: 4 }}>
                {[["Load 1m", load["system.load.1"]], ["Load 5m", load["system.load.5"]], ["Load 15m", load["system.load.15"]]].map(([k, v]) => (
                  <div key={k} style={{ textAlign: "center" }}>
                    <div style={{ fontSize: "1rem", fontWeight: 800, color: "var(--t1)" }}>{(v || 0).toFixed(2)}</div>
                    <div style={{ fontSize: "0.62rem", color: "var(--t3)" }}>{k}</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* System info grid */}
      <div className="lux-eyebrow" style={{ marginBottom: 10 }}>SYSTEM INFO</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 0 }}>
        {metaItems.filter(([, v]) => v).map(([k, v]) => (
          <div key={k} style={{
            display: "flex", justifyContent: "space-between", alignItems: "center",
            padding: "7px 0", borderBottom: "1px solid var(--border)",
            fontSize: "0.77rem",
          }}>
            <span style={{ color: "var(--t3)" }}>{k}</span>
            <span style={{ color: "var(--t1)", fontFamily: "JetBrains Mono, monospace", fontSize: "0.72rem" }}>{v}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Setup Guide ──────────────────────────────────────────────────────────────

function SetupGuide() {
  const [copied, setCopied] = useState("");
  const ingestURL   = `${window.location.origin}/api/v1/ingest/agent`;
  const registerURL = `${window.location.origin}/api/v1/agents/register`;

  function copy(text, key) {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(key);
      setTimeout(() => setCopied(""), 1800);
    });
  }

  const STEPS = [
    { n: 1, text: "Download and install the NeurOps agent on your host." },
    { n: 2, text: "Configure agent.json — set neuroops.product.host to your server URL." },
    { n: 3, text: "Start the agent: sudo systemctl start neuroops-agent" },
    { n: 4, text: "The agent auto-registers and begins sending metrics, logs, and traces." },
  ];

  const COLLECTS = [
    ["📊", "CPU, Memory, Disk, Network"],
    ["📝", "System Logs (syslog, auth)"],
    ["🔍", "Distributed Traces (OTLP)"],
    ["⚙️",  "Process & Service Health"],
    ["📈", "Load Averages & Uptime"],
    ["🔌", "Custom Plugins (Go/Python)"],
  ];

  return (
    <div style={{ padding: "24px", overflowY: "auto" }}>
      <div className="lux-eyebrow" style={{ marginBottom: 6 }}>GETTING STARTED</div>
      <div style={{ fontSize: "1rem", fontWeight: 700, color: "var(--t1)", marginBottom: 16 }}>
        Install the NeurOps Agent
      </div>

      {/* Steps */}
      <div style={{ display: "flex", flexDirection: "column", gap: 8, marginBottom: 20 }}>
        {STEPS.map(s => (
          <div key={s.n} style={{ display: "flex", gap: 12, alignItems: "flex-start" }}>
            <div style={{
              width: 22, height: 22, borderRadius: "50%", flexShrink: 0,
              background: "rgba(0,102,255,0.15)", border: "1px solid rgba(0,102,255,0.3)",
              display: "flex", alignItems: "center", justifyContent: "center",
              fontSize: "0.65rem", fontWeight: 800, color: "#0066FF",
            }}>{s.n}</div>
            <div style={{ fontSize: "0.78rem", color: "var(--t2)", lineHeight: 1.6, paddingTop: 2 }}>{s.text}</div>
          </div>
        ))}
      </div>

      {/* Endpoints */}
      {[["Ingest Endpoint", ingestURL, "ingest"], ["Register Endpoint", registerURL, "register"]].map(([label, url, key]) => (
        <div key={key} style={{ marginBottom: 10 }}>
          <div style={{ fontSize: "0.65rem", color: "var(--t3)", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.08em" }}>{label}</div>
          <div style={{
            display: "flex", alignItems: "center", gap: 8,
            padding: "7px 10px", borderRadius: 7,
            background: "rgba(0,0,0,0.2)", border: "1px solid var(--border)",
          }}>
            <code style={{ flex: 1, fontSize: "0.68rem", color: "var(--cyan)", wordBreak: "break-all", background: "none", padding: 0 }}>
              {url}
            </code>
            <button onClick={() => copy(url, key)} style={{
              padding: "3px 9px", fontSize: "0.65rem", fontWeight: 600,
              borderRadius: 5, cursor: "pointer",
              background: copied === key ? "rgba(0,208,132,0.15)" : "rgba(255,255,255,0.06)",
              border: `1px solid ${copied === key ? "rgba(0,208,132,0.3)" : "var(--border-strong)"}`,
              color: copied === key ? "#00D084" : "var(--t2)", flexShrink: 0,
            }}>
              {copied === key ? "Copied!" : "Copy"}
            </button>
          </div>
        </div>
      ))}

      {/* What it collects */}
      <div className="lux-eyebrow" style={{ margin: "18px 0 10px" }}>WHAT AGENTS COLLECT</div>
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 6 }}>
        {COLLECTS.map(([icon, text]) => (
          <div key={text} style={{
            display: "flex", gap: 8, alignItems: "center",
            padding: "7px 10px", borderRadius: 7,
            background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)",
            fontSize: "0.73rem", color: "var(--t2)",
          }}>
            <span>{icon}</span> {text}
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Main Panel ───────────────────────────────────────────────────────────────

export default function AgentPanel() {
  const [agents,     setAgents]     = useState([]);
  const [loading,    setLoading]    = useState(true);
  const [selectedID, setSelectedID] = useState(null);
  const [lastRefresh, setLastRefresh] = useState(null);

  const load = useCallback(async () => {
    try {
      const d = await getAgents();
      setAgents(Array.isArray(d) ? d : []);
      setLastRefresh(new Date());
    } catch {
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

  const active   = agents.filter(a => a.status === "active").length;
  const inactive = agents.filter(a => a.status === "inactive" || a.status === "stopped").length;
  const errored  = agents.filter(a => a.status === "error").length;

  const KPIS = [
    { label: "Total Agents", value: agents.length,   color: "var(--t1)" },
    { label: "Active",       value: active,           color: "#00D084" },
    { label: "Inactive",     value: inactive,         color: "#FFBC00" },
    { label: "Error",        value: errored,          color: errored > 0 ? "#FF3B3B" : "var(--t3)" },
  ];

  return (
    <div className="p3-panel">

      {/* ── Header ───────────────────────────────────────────────── */}
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">AGENT FLEET</div>
          <h2 style={{ margin: "0.2rem 0 0.2rem", fontSize: "1.35rem" }}>Connected Agents</h2>
          <div className="lux-muted">
            Real-time telemetry from your infrastructure · auto-refreshes every 30s
          </div>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          {lastRefresh && (
            <span style={{ fontSize: "0.67rem", color: "var(--t4)" }}>
              Updated {lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" })}
            </span>
          )}
          <button className="lux-secondary-btn" onClick={load}>Refresh</button>
        </div>
      </div>

      {/* ── KPI strip ─────────────────────────────────────────────── */}
      <div style={{ display: "flex", gap: 10, marginBottom: 20, flexWrap: "wrap" }}>
        {KPIS.map(k => (
          <div key={k.label} style={{
            flex: 1, minWidth: 80, padding: "12px 16px", borderRadius: 10,
            background: "rgba(255,255,255,0.04)", border: "1px solid var(--border)",
          }}>
            <div style={{ fontSize: "1.6rem", fontWeight: 800, color: k.color, lineHeight: 1 }}>{k.value}</div>
            <div style={{ fontSize: "0.62rem", color: "var(--t3)", marginTop: 4, textTransform: "uppercase", letterSpacing: "0.08em" }}>{k.label}</div>
          </div>
        ))}
      </div>

      {/* ── Two-panel layout ─────────────────────────────────────── */}
      <div style={{
        display: "grid", gridTemplateColumns: agents.length === 0 ? "1fr" : "300px 1fr",
        gap: 16, minHeight: 440,
      }}>

        {/* Agent list */}
        <div style={{
          background: "var(--surface-1)", border: "1px solid var(--border)",
          borderRadius: 10, overflow: "hidden",
        }}>
          {loading ? (
            <div style={{ padding: 24, color: "var(--t3)", fontSize: "0.8rem" }}>Loading agents…</div>
          ) : agents.length === 0 ? (
            <SetupGuide />
          ) : (
            <>
              <div style={{
                padding: "8px 14px",
                borderBottom: "1px solid var(--border)",
                fontSize: "0.65rem", color: "var(--t4)",
                textTransform: "uppercase", letterSpacing: "0.09em",
              }}>
                {agents.length} agent{agents.length !== 1 ? "s" : ""}
              </div>
              <div style={{ overflowY: "auto", maxHeight: 520 }}>
                {agents.map(a => (
                  <AgentCard
                    key={a.id}
                    agent={a}
                    selected={a.id === selectedID}
                    onSelect={id => setSelectedID(prev => prev === id ? null : id)}
                    onRefresh={load}
                  />
                ))}
              </div>
            </>
          )}
        </div>

        {/* Detail pane */}
        {agents.length > 0 && (
          <div style={{
            background: "var(--surface-1)", border: "1px solid var(--border)",
            borderRadius: 10, overflow: "hidden",
          }}>
            {selectedID ? (
              <AgentDetail agentID={selectedID} />
            ) : (
              <div style={{
                display: "flex", flexDirection: "column", alignItems: "center",
                justifyContent: "center", height: "100%", gap: 10, padding: 32,
                color: "var(--t3)", textAlign: "center",
              }}>
                <div style={{ fontSize: 32 }}>←</div>
                <div style={{ fontSize: "0.83rem" }}>Select an agent to view live metrics and system details</div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
