import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, Ban, ChevronRight, LogIn, Pencil, Play, Plus,
  RefreshCw, Search, Shield, Sparkles, Trash2, User,
} from "lucide-react";
import { apiRequest } from "../api/client";

const PAGE_SIZE = 50;
const REFRESH_MS = 15000;

/* Map an audit action string to an icon + color family. */
function actionMeta(action) {
  const a = (action || "").toLowerCase();
  if (a.includes("blocked") || a.includes("circuit")) return { Icon: Ban, color: "var(--red)", dim: "var(--red-dim)" };
  if (a.includes("delete")) return { Icon: Trash2, color: "var(--red)", dim: "var(--red-dim)" };
  if (a.includes("create")) return { Icon: Plus, color: "var(--green)", dim: "var(--green-dim)" };
  if (a.includes("login")) return { Icon: LogIn, color: "var(--cyan)", dim: "var(--cyan-dim)" };
  if (a.includes("explain") || a.includes("copilot")) return { Icon: Sparkles, color: "var(--cyan)", dim: "var(--cyan-dim)" };
  if (a.includes("execute") || a.includes("remediation")) return { Icon: Play, color: "var(--blue-lt)", dim: "var(--blue-dim)" };
  if (a.includes("update") || a.includes("config")) return { Icon: Pencil, color: "var(--blue-lt)", dim: "var(--blue-dim)" };
  return { Icon: Activity, color: "var(--t2)", dim: "rgba(255,255,255,0.05)" };
}

function statusColor(s) {
  const v = (s || "").toLowerCase();
  if (v === "success" || v === "succeeded" || v === "ok") return "var(--green)";
  if (v === "failed" || v === "error" || v === "blocked") return "var(--red)";
  if (v === "pending" || v === "running") return "var(--amber)";
  return "var(--t3)";
}

function fmtAbs(v) {
  if (!v) return "—";
  const d = new Date(v);
  return d.toLocaleString([], { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit" });
}
function fmtRel(v) {
  if (!v) return "";
  const s = Math.floor((Date.now() - new Date(v).getTime()) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function parseDetails(raw) {
  if (!raw) return null;
  if (typeof raw === "object") return raw;
  try { return JSON.parse(raw); } catch { return { detail: String(raw) }; }
}

/* Normalize the two backend shapes into one row model. */
function normalizeGlobal(e) {
  return {
    key: `g-${e.id}`,
    time: e.created_at,
    actor: e.actor || "system",
    action: e.action || "event",
    resourceType: e.resource_type || "",
    resourceId: e.resource_id || "",
    ip: e.ip_address || "",
    status: null,
    details: parseDetails(e.details_json),
  };
}
function normalizeIncident(a, i) {
  return {
    key: `i-${a.action_id || "action"}-${a.executed_at || i}`,
    time: a.executed_at,
    actor: a.approved ? "Approved" : "Not approved",
    action: a.action_id || "action.executed",
    resourceType: "action",
    resourceId: a.action_id || "",
    ip: "",
    status: a.status || "unknown",
    details: a.message ? { message: a.message } : null,
  };
}

function Row({ row, expanded, onToggle }) {
  const { Icon, color, dim } = actionMeta(row.action);
  return (
    <div
      onClick={onToggle}
      style={{
        borderBottom: "1px solid var(--border)",
        background: expanded ? "rgba(0,102,255,0.04)" : "transparent",
        cursor: "pointer", transition: "background 0.12s",
      }}
    >
      <div style={{ display: "grid", gridTemplateColumns: "150px 150px 1fr 150px 120px 20px", gap: 12, alignItems: "center", padding: "10px 16px" }}>
        {/* Time */}
        <div style={{ fontSize: 11, fontFamily: "JetBrains Mono, monospace" }}>
          <div style={{ color: "var(--t2)" }}>{fmtAbs(row.time)}</div>
          <div style={{ color: "var(--t4)", fontSize: 10 }}>{fmtRel(row.time)}</div>
        </div>
        {/* Actor */}
        <div style={{ display: "flex", alignItems: "center", gap: 6, minWidth: 0 }}>
          <span style={{ width: 22, height: 22, borderRadius: 6, background: "var(--surface-3)", display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 }}>
            <User size={11} color="var(--t3)" />
          </span>
          <span style={{ fontSize: 12, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{row.actor}</span>
        </div>
        {/* Action */}
        <div style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
          <span style={{ width: 24, height: 24, borderRadius: 7, background: dim, display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 }}>
            <Icon size={12} color={color} />
          </span>
          <code style={{ fontSize: 12, color: "var(--t1)", background: "none", padding: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{row.action}</code>
        </div>
        {/* Resource */}
        <div style={{ fontSize: 12, color: "var(--t3)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
          {row.resourceType ? <span style={{ color: "var(--t2)" }}>{row.resourceType}</span> : "—"}
          {row.resourceId ? <span style={{ color: "var(--t4)" }}> · {row.resourceId}</span> : ""}
        </div>
        {/* Status or IP */}
        <div style={{ fontSize: 11, textAlign: "right" }}>
          {row.status
            ? <span style={{ fontWeight: 700, color: statusColor(row.status), textTransform: "uppercase", letterSpacing: "0.04em" }}>{row.status}</span>
            : <span style={{ color: "var(--t4)", fontFamily: "JetBrains Mono, monospace" }}>{row.ip || "—"}</span>}
        </div>
        <ChevronRight size={13} color="var(--t4)" style={{ transform: expanded ? "rotate(90deg)" : "none", transition: "transform 0.15s" }} />
      </div>

      {expanded && row.details && (
        <div style={{ margin: "0 16px 12px 16px", padding: "12px 14px", background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: "var(--r3)" }}>
          {Object.entries(row.details).map(([k, v]) => (
            <div key={k} style={{ display: "flex", gap: 10, padding: "3px 0", fontSize: 12, alignItems: "flex-start" }}>
              <span style={{ color: "var(--t3)", minWidth: 120, flexShrink: 0 }}>{k}</span>
              <span style={{ color: "var(--t1)", fontFamily: "JetBrains Mono, monospace", fontSize: 11, whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
                {typeof v === "object" ? JSON.stringify(v, null, 2) : String(v)}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

const RESOURCE_FILTERS = [
  ["", "All resources"], ["action", "Actions"], ["policy", "Policy"],
  ["incident", "Incidents"], ["config", "Config"], ["user", "Users"],
];

export default function AuditPanel({ incidentId }) {
  const isGlobal = !incidentId;
  const [rows, setRows] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [offset, setOffset] = useState(0);
  const [resourceType, setResourceType] = useState("");
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState(null);
  const aliveRef = useRef(true);

  const load = useCallback(async () => {
    try {
      if (isGlobal) {
        const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
        if (resourceType) params.set("resource_type", resourceType);
        const data = await apiRequest(`/audit?${params}`);
        if (!aliveRef.current) return;
        setRows((data.entries || []).map(normalizeGlobal));
        setTotal(data.total || 0);
      } else {
        const data = await apiRequest(`/actions/audit?incident_id=${incidentId}`);
        if (!aliveRef.current) return;
        const arr = Array.isArray(data) ? data : [];
        setRows(arr.map(normalizeIncident));
        setTotal(arr.length);
      }
      setError("");
    } catch (err) {
      if (!aliveRef.current) return;
      setRows([]);
      setError(err.message || "Failed to load audit log.");
    } finally {
      if (aliveRef.current) setLoading(false);
    }
  }, [isGlobal, incidentId, offset, resourceType]);

  useEffect(() => {
    aliveRef.current = true;
    setLoading(true);
    load();
    const id = window.setInterval(load, REFRESH_MS);
    return () => { aliveRef.current = false; window.clearInterval(id); };
  }, [load]);

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter(r => `${r.actor} ${r.action} ${r.resourceType} ${r.resourceId} ${r.ip}`.toLowerCase().includes(q));
  }, [rows, search]);

  const actors = useMemo(() => new Set(rows.map(r => r.actor)).size, [rows]);
  const hasMore = isGlobal && offset + rows.length < total;

  /* ── Incident-scoped (inside an incident) — compact card list ── */
  if (!isGlobal) {
    return (
      <div className="detail-section">
        <div className="detail-subtitle" style={{ marginBottom: 10 }}>Action Audit</div>
        {error && <div className="toast danger" style={{ marginBottom: 10 }}>{error}</div>}
        {!error && visible.length === 0 && (
          <div className="lux-muted" style={{ fontSize: 13 }}>No actions executed for this incident yet.</div>
        )}
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {visible.map(row => {
            const { Icon, color, dim } = actionMeta(row.action);
            return (
              <div key={row.key} className="action-item">
                <div className="action-item-top">
                  <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{ width: 24, height: 24, borderRadius: 7, background: dim, display: "flex", alignItems: "center", justifyContent: "center" }}>
                      <Icon size={12} color={color} />
                    </span>
                    <strong style={{ fontSize: 13, color: "var(--t1)" }}>{row.action}</strong>
                  </span>
                  <span className="badge" style={{ color: statusColor(row.status), background: "var(--surface-3)" }}>{(row.status || "unknown").toUpperCase()}</span>
                </div>
                {row.details?.message && <p style={{ fontSize: 12, color: "var(--t2)", margin: "8px 0 0", lineHeight: 1.6 }}>{row.details.message}</p>}
                <div className="action-meta" style={{ marginTop: 6 }}>{row.actor} · {fmtAbs(row.time)}</div>
              </div>
            );
          })}
        </div>
      </div>
    );
  }

  /* ── Global Audit Trail (Admin nav) ── */
  return (
    <div style={{ padding: "24px 28px", animation: "fadeIn .22s ease" }}>
      {/* Header */}
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 16, marginBottom: 18, flexWrap: "wrap" }}>
        <div>
          <div className="lux-eyebrow" style={{ marginBottom: 4 }}>Administration</div>
          <h1 style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 24, fontWeight: 800, letterSpacing: "-0.03em", color: "var(--t1)", margin: 0, lineHeight: 1.1 }}>
            <Shield size={20} color="var(--blue-lt)" /> Audit Trail
          </h1>
          <div style={{ fontSize: 12, color: "var(--t4)", marginTop: 5 }}>
            Immutable record of every action, change, and access event · auto-refreshes every 15s
          </div>
        </div>
        <button className="btn btn-ghost btn-sm" onClick={() => { setLoading(true); load(); }} style={{ gap: 6 }}>
          <RefreshCw size={12} style={{ animation: loading ? "rotateSpin .8s linear infinite" : "none" }} /> Refresh
        </button>
      </div>

      {/* KPIs */}
      <div style={{ display: "flex", gap: 10, marginBottom: 16, flexWrap: "wrap" }}>
        {[
          { label: "Total Events", value: total.toLocaleString(), color: "var(--t1)" },
          { label: "On This Page", value: rows.length, color: "var(--blue-lt)" },
          { label: "Distinct Actors", value: actors, color: "var(--cyan)" },
        ].map(k => (
          <div key={k.label} style={{ flex: 1, minWidth: 120, padding: "12px 16px", background: "var(--surface-1)", border: "1px solid var(--border)", borderRadius: "var(--r4)" }}>
            <div style={{ fontSize: 24, fontWeight: 800, letterSpacing: "-0.03em", color: k.color, lineHeight: 1 }}>{k.value}</div>
            <div style={{ fontSize: 10, fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.07em", marginTop: 8 }}>{k.label}</div>
          </div>
        ))}
      </div>

      {/* Table card */}
      <div className="card" style={{ padding: 0 }}>
        {/* Filter bar */}
        <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "12px 16px", borderBottom: "1px solid var(--border)", flexWrap: "wrap" }}>
          <div style={{ position: "relative", flex: 1, minWidth: 200 }}>
            <Search size={13} style={{ position: "absolute", left: 11, top: "50%", transform: "translateY(-50%)", color: "var(--t3)", pointerEvents: "none" }} />
            <input value={search} onChange={e => setSearch(e.target.value)} placeholder="Search actor, action, resource, IP…"
              style={{ width: "100%", height: 32, background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 8, padding: "0 12px 0 32px", fontSize: 12, color: "var(--t1)", outline: "none" }} />
          </div>
          <select className="form-select" value={resourceType} onChange={e => { setOffset(0); setResourceType(e.target.value); }} style={{ width: "auto", padding: "7px 32px 7px 12px", fontSize: 12 }}>
            {RESOURCE_FILTERS.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
          </select>
        </div>

        {/* Column header */}
        <div style={{ display: "grid", gridTemplateColumns: "150px 150px 1fr 150px 120px 20px", gap: 12, padding: "8px 16px", borderBottom: "1px solid var(--border)" }}>
          {["Time", "Actor", "Action", "Resource", "Status / IP", ""].map((h, i) => (
            <span key={i} style={{ fontSize: 10, fontWeight: 700, color: "var(--t4)", textTransform: "uppercase", letterSpacing: "0.09em", textAlign: i === 4 ? "right" : "left" }}>{h}</span>
          ))}
        </div>

        {/* Rows */}
        {loading && rows.length === 0 ? (
          <div style={{ padding: "14px 16px", display: "flex", flexDirection: "column", gap: 8 }}>
            {[1, 2, 3, 4, 5, 6].map(i => <div key={i} className="skeleton" style={{ height: 40, borderRadius: 8 }} />)}
          </div>
        ) : error ? (
          <div className="empty-state" style={{ padding: "48px 24px" }}>
            <div className="empty-icon"><Shield size={22} /></div>
            <div className="empty-title">Couldn’t load the audit log</div>
            <div className="empty-desc">{error}</div>
          </div>
        ) : visible.length === 0 ? (
          <div className="empty-state" style={{ padding: "56px 24px" }}>
            <div className="empty-icon"><Shield size={22} /></div>
            <div className="empty-title">No audit events</div>
            <div className="empty-desc">{search || resourceType ? "No events match the current filter." : "Actions, policy changes, and access events will appear here as they happen."}</div>
          </div>
        ) : (
          visible.map(row => (
            <Row key={row.key} row={row} expanded={expanded === row.key} onToggle={() => setExpanded(expanded === row.key ? null : row.key)} />
          ))
        )}
      </div>

      {/* Pagination */}
      {!loading && !error && total > 0 && (
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "12px 4px" }}>
          <span style={{ fontSize: 12, color: "var(--t3)" }}>
            {offset + 1}–{offset + rows.length} of {total.toLocaleString()} events
          </span>
          <div style={{ display: "flex", gap: 6 }}>
            <button className="btn btn-ghost btn-sm" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}>← Prev</button>
            <button className="btn btn-ghost btn-sm" disabled={!hasMore} onClick={() => setOffset(offset + PAGE_SIZE)}>Next →</button>
          </div>
        </div>
      )}
    </div>
  );
}
