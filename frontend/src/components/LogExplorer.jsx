// LogExplorer.jsx — Centralized log viewer

import { useState, useEffect, useCallback, useRef } from "react";
import { queryLogs, getLogStats, createLogStream } from "../api/logs.js";

// ─── Config ───────────────────────────────────────────────────────────────────

const LEVEL = {
  error:   { color: "#FF3B3B", bg: "rgba(255,59,59,0.13)",   label: "ERROR" },
  warn:    { color: "#FFBC00", bg: "rgba(255,188,0,0.12)",    label: "WARN"  },
  warning: { color: "#FFBC00", bg: "rgba(255,188,0,0.12)",    label: "WARN"  },
  info:    { color: "#7a9dc9", bg: "rgba(122,157,201,0.08)",  label: "INFO"  },
  debug:   { color: "#818cf8", bg: "rgba(129,140,248,0.09)",  label: "DEBUG" },
  trace:   { color: "#a78bfa", bg: "rgba(167,139,250,0.09)",  label: "TRACE" },
};

function lvl(cat) { return LEVEL[(cat || "info").toLowerCase()] || LEVEL.info; }

function fmtTime(ts) {
  if (!ts) return "—";
  return new Date(ts).toLocaleTimeString([], { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}
function fmtDate(ts) {
  if (!ts) return "";
  return new Date(ts).toLocaleDateString([], { month: "short", day: "numeric" });
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function LevelBadge({ category }) {
  const c = lvl(category);
  return (
    <span style={{
      fontSize: "0.62rem", fontWeight: 800, letterSpacing: "0.08em",
      padding: "2px 7px", borderRadius: 4,
      color: c.color, background: c.bg, flexShrink: 0,
      fontFamily: "JetBrains Mono, monospace",
    }}>
      {c.label}
    </span>
  );
}

function StatPill({ label, value, color }) {
  return (
    <div style={{
      display: "flex", flexDirection: "column", alignItems: "center",
      padding: "10px 18px", borderRadius: 10,
      background: "rgba(255,255,255,0.04)",
      border: "1px solid var(--border)",
      minWidth: 80,
    }}>
      <span style={{ fontSize: "1.45rem", fontWeight: 800, color: color || "var(--t1)", lineHeight: 1 }}>
        {value}
      </span>
      <span style={{ fontSize: "0.62rem", color: "var(--t3)", marginTop: 4, textTransform: "uppercase", letterSpacing: "0.08em" }}>
        {label}
      </span>
    </div>
  );
}

function LogRow({ entry, expanded, onToggle }) {
  const c = lvl(entry.event_category);
  return (
    <div
      onClick={onToggle}
      style={{
        borderBottom: "1px solid var(--border)",
        borderLeft: `3px solid ${(c.label === "ERROR" || c.label === "WARN") ? c.color : "transparent"}`,
        background: expanded ? "rgba(0,102,255,0.04)" : "transparent",
        cursor: "pointer",
        transition: "background 0.12s",
      }}
    >
      {/* Main row */}
      <div style={{
        display: "grid",
        gridTemplateColumns: "130px 58px 130px 90px 1fr",
        gap: 12, alignItems: "center",
        padding: "8px 16px",
      }}>
        {/* Timestamp */}
        <div style={{ fontFamily: "JetBrains Mono, monospace", fontSize: "0.7rem" }}>
          <span style={{ color: "var(--t3)" }}>{fmtDate(entry.timestamp)} </span>
          <span style={{ color: "var(--t2)" }}>{fmtTime(entry.timestamp)}</span>
        </div>
        {/* Level */}
        <LevelBadge category={entry.event_category} />
        {/* Host */}
        <span style={{
          fontSize: "0.72rem", color: "var(--t2)",
          overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
          fontFamily: "JetBrains Mono, monospace",
        }} title={entry.host_ip}>
          {entry.host_ip || "—"}
        </span>
        {/* Tag */}
        <span style={{
          fontSize: "0.68rem", color: "var(--t3)",
          overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
        }}>
          {entry.log_tag || "—"}
        </span>
        {/* Message */}
        <span style={{
          fontSize: "0.78rem", color: expanded ? "var(--t1)" : c.label === "ERROR" ? "#fca5a5" : c.label === "WARN" ? "#fcd34d" : "var(--t1)",
          overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
          fontFamily: "JetBrains Mono, monospace",
        }}>
          {entry.message || "—"}
        </span>
      </div>

      {/* Expanded detail */}
      {expanded && (
        <div style={{
          margin: "0 16px 12px", padding: "12px 14px",
          background: "rgba(0,0,0,0.25)", borderRadius: 8,
          border: "1px solid var(--border)",
        }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "8px 16px", marginBottom: 10 }}>
            {[
              ["Agent ID", entry.agent_id],
              ["Source",   entry.log_source],
              ["Host IP",  entry.host_ip],
              ["Type",     entry.event_type || "log"],
              ["Tag",      entry.log_tag],
              ["Category", entry.event_category || "info"],
            ].map(([k, v]) => (
              <div key={k} style={{ fontSize: "0.72rem" }}>
                <span style={{ color: "var(--t3)", marginRight: 6 }}>{k}</span>
                <span style={{ color: "var(--t1)", fontFamily: "JetBrains Mono, monospace" }}>{v || "—"}</span>
              </div>
            ))}
          </div>
          <div style={{ fontSize: "0.67rem", color: "var(--t3)", marginBottom: 4 }}>FULL MESSAGE</div>
          <pre style={{
            margin: 0, fontSize: "0.73rem", color: "var(--t1)",
            fontFamily: "JetBrains Mono, monospace",
            whiteSpace: "pre-wrap", wordBreak: "break-all",
            background: "rgba(0,0,0,0.2)", padding: "8px 10px", borderRadius: 6,
            lineHeight: 1.6, maxHeight: 160, overflowY: "auto",
          }}>
            {entry.message}
          </pre>
        </div>
      )}
    </div>
  );
}

function EmptyState() {
  return (
    <div style={{
      display: "flex", flexDirection: "column", alignItems: "center",
      justifyContent: "center", gap: 12, padding: "56px 24px", textAlign: "center",
    }}>
      <div style={{
        width: 56, height: 56, borderRadius: 14,
        background: "rgba(0,200,232,0.08)", border: "1px solid rgba(0,200,232,0.2)",
        display: "flex", alignItems: "center", justifyContent: "center", fontSize: 24,
      }}>📋</div>
      <div style={{ fontSize: "1rem", fontWeight: 700, color: "var(--t1)" }}>No logs found</div>
      <div style={{ fontSize: "0.8rem", color: "var(--t3)", maxWidth: 380, lineHeight: 1.6 }}>
        Install the NeurOps agent on your hosts to start collecting logs.
        Logs appear here in real-time across all connected infrastructure.
      </div>
    </div>
  );
}

// ─── Main Component ────────────────────────────────────────────────────────────

const PAGE_SIZE = 100;

export default function LogExplorer() {
  const [entries,    setEntries]    = useState([]);
  const [total,      setTotal]      = useState(0);
  const [loading,    setLoading]    = useState(true);
  const [stats,      setStats]      = useState(null);
  const [expandedID, setExpandedID] = useState(null);
  const [streaming,  setStreaming]  = useState(false);
  const [streamCount, setStreamCount] = useState(0);
  const stopRef = useRef(null);

  const [filters, setFilters] = useState({ host: "", category: "", tag: "", search: "" });
  const [offset,  setOffset]  = useState(0);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [logData, statsData] = await Promise.all([
        queryLogs({ ...filters, limit: PAGE_SIZE, offset }).catch(() => ({ entries: [], total: 0 })),
        getLogStats(24).catch(() => null),
      ]);
      setEntries(logData.entries || []);
      setTotal(logData.total || 0);
      setStats(statsData);
    } finally {
      setLoading(false);
    }
  }, [filters, offset]);

  useEffect(() => { load(); }, [load]);

  function toggleStream() {
    if (streaming) {
      if (stopRef.current) stopRef.current();
      stopRef.current = null;
      setStreaming(false);
      setStreamCount(0);
    } else {
      const lastId = entries.length > 0 ? Math.max(...entries.map(e => e.id || 0)) : 0;
      const stop = createLogStream(lastId, (entry) => {
        setEntries(prev => { const n = [entry, ...prev]; if (n.length > 500) n.length = 500; return n; });
        setStreamCount(c => c + 1);
      }, () => {});
      stopRef.current = stop;
      setStreaming(true);
    }
  }

  useEffect(() => () => { if (stopRef.current) stopRef.current(); }, []);

  function setFilter(key, val) {
    setOffset(0);
    setFilters(f => ({ ...f, [key]: val }));
  }

  function clearFilters() {
    setOffset(0);
    setFilters({ host: "", category: "", tag: "", search: "" });
  }

  const cats = stats?.by_category || {};
  const errorCount   = cats.error   || 0;
  const warnCount    = (cats.warn   || 0) + (cats.warning || 0);
  const infoCount    = cats.info    || 0;
  const hostCount    = (stats?.by_host || []).length;
  const hosts        = [...new Set((stats?.by_host || []).map(h => h.host_ip))];
  const tags         = [...new Set((stats?.by_tag  || []).map(t => t.tag))];
  const hasMore      = offset + entries.length < total;
  const activeFilter = Object.values(filters).some(Boolean);

  const inputStyle = {
    height: 32, padding: "0 10px", fontSize: "0.77rem",
    background: "rgba(255,255,255,0.05)", border: "1px solid var(--border-strong)",
    borderRadius: 7, color: "var(--t1)", outline: "none",
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 0 }}>

      {/* ── Header ───────────────────────────────────────────────── */}
      <div style={{
        display: "flex", alignItems: "flex-start", justifyContent: "space-between",
        flexWrap: "wrap", gap: 12, marginBottom: 20,
      }}>
        <div>
          <div className="lux-eyebrow">LOG EXPLORER</div>
          <h2 style={{ margin: "0.2rem 0 0.25rem", fontSize: "1.35rem" }}>Centralized Log Viewer</h2>
          <div className="lux-muted">All agents · All hosts · Real-time streaming with search and filter</div>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          {streaming && (
            <div style={{
              display: "flex", alignItems: "center", gap: 6,
              padding: "5px 10px", borderRadius: 6,
              background: "rgba(0,208,132,0.1)", border: "1px solid rgba(0,208,132,0.25)",
              fontSize: "0.72rem", color: "#00D084",
            }}>
              <span style={{
                width: 7, height: 7, borderRadius: "50%", background: "#00D084",
                animation: "pulse 1.2s ease-in-out infinite",
              }} />
              Live · {streamCount} new
            </div>
          )}
          <button
            onClick={toggleStream}
            style={{
              padding: "6px 14px", fontSize: "0.77rem", fontWeight: 600,
              borderRadius: 7, cursor: "pointer", transition: "all 0.14s",
              background: streaming ? "rgba(255,59,59,0.12)" : "rgba(0,208,132,0.1)",
              border: `1px solid ${streaming ? "rgba(255,59,59,0.3)" : "rgba(0,208,132,0.3)"}`,
              color: streaming ? "#FF3B3B" : "#00D084",
            }}
          >
            {streaming ? "⏹ Stop Stream" : "▶ Live Stream"}
          </button>
          <button className="lux-secondary-btn" onClick={load} disabled={streaming}>Refresh</button>
        </div>
      </div>

      {/* ── Stat pills ────────────────────────────────────────────── */}
      <div style={{ display: "flex", gap: 10, marginBottom: 18, flexWrap: "wrap" }}>
        <StatPill label="Total"    value={stats?.total_logs || 0} />
        <StatPill label="Errors"   value={errorCount}  color="#FF3B3B" />
        <StatPill label="Warnings" value={warnCount}   color="#FFBC00" />
        <StatPill label="Info"     value={infoCount}   color="#7a9dc9" />
        <StatPill label="Hosts"    value={hostCount}   color="#00C8E8" />
        <StatPill label="Errors/1h" value={stats?.recent_errors || 0} color={stats?.recent_errors > 0 ? "#FF3B3B" : "var(--t3)"} />
      </div>

      {/* ── Filter bar ────────────────────────────────────────────── */}
      <div style={{
        display: "flex", gap: 8, marginBottom: 4, flexWrap: "wrap", alignItems: "center",
        padding: "10px 14px", borderRadius: 10,
        background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)",
      }}>
        <input
          style={{ ...inputStyle, flex: 1, minWidth: 160 }}
          placeholder="🔍  Search messages…"
          value={filters.search}
          onChange={e => setFilter("search", e.target.value)}
        />
        <select style={{ ...inputStyle, paddingRight: 6 }} value={filters.category} onChange={e => setFilter("category", e.target.value)}>
          <option value="">All levels</option>
          <option value="error">Error</option>
          <option value="warn">Warning</option>
          <option value="info">Info</option>
          <option value="debug">Debug</option>
        </select>
        <select style={{ ...inputStyle, paddingRight: 6 }} value={filters.host} onChange={e => setFilter("host", e.target.value)}>
          <option value="">All hosts</option>
          {hosts.map(h => <option key={h} value={h}>{h}</option>)}
        </select>
        <select style={{ ...inputStyle, paddingRight: 6 }} value={filters.tag} onChange={e => setFilter("tag", e.target.value)}>
          <option value="">All tags</option>
          {tags.map(t => <option key={t} value={t}>{t}</option>)}
        </select>
        {activeFilter && (
          <button onClick={clearFilters} style={{
            ...inputStyle, padding: "0 10px", cursor: "pointer",
            color: "var(--t3)", background: "transparent",
          }}>
            Clear ✕
          </button>
        )}
      </div>

      {/* ── Table header ─────────────────────────────────────────── */}
      <div style={{
        display: "grid",
        gridTemplateColumns: "130px 58px 130px 90px 1fr",
        gap: 12, padding: "6px 16px",
        borderBottom: "1px solid var(--border)",
        borderLeft: "3px solid transparent",
        marginBottom: 0,
      }}>
        {["Timestamp", "Level", "Host", "Tag", "Message"].map(h => (
          <span key={h} style={{
            fontSize: "0.62rem", fontWeight: 700, color: "var(--t4)",
            textTransform: "uppercase", letterSpacing: "0.09em",
          }}>{h}</span>
        ))}
      </div>

      {/* ── Log rows ─────────────────────────────────────────────── */}
      <div style={{
        background: "var(--surface-1)", border: "1px solid var(--border)",
        borderRadius: "0 0 10px 10px", overflow: "hidden",
        minHeight: 200,
      }}>
        {loading && !streaming ? (
          <div style={{ padding: "2rem", textAlign: "center", color: "var(--t3)", fontSize: "0.8rem" }}>
            Loading logs…
          </div>
        ) : entries.length === 0 ? (
          <EmptyState />
        ) : (
          entries.map(entry => (
            <LogRow
              key={entry.id || Math.random()}
              entry={entry}
              expanded={expandedID === entry.id}
              onToggle={() => setExpandedID(expandedID === entry.id ? null : entry.id)}
            />
          ))
        )}
      </div>

      {/* ── Pagination ────────────────────────────────────────────── */}
      {!streaming && entries.length > 0 && (
        <div style={{
          display: "flex", justifyContent: "space-between", alignItems: "center",
          padding: "10px 4px", marginTop: 6,
        }}>
          <span style={{ fontSize: "0.72rem", color: "var(--t3)" }}>
            {offset + 1}–{offset + entries.length} of {total} logs
          </span>
          <div style={{ display: "flex", gap: 6 }}>
            <button className="lux-secondary-btn small" disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>← Prev</button>
            <button className="lux-secondary-btn small" disabled={!hasMore}
              onClick={() => setOffset(offset + PAGE_SIZE)}>Next →</button>
          </div>
        </div>
      )}

      <style>{`@keyframes pulse { 0%,100%{opacity:1} 50%{opacity:0.4} }`}</style>
    </div>
  );
}
