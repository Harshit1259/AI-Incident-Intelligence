/**
 * Audit trail — immutable record of every action, change and access event.
 *
 * Two modes from one component:
 *   global   (admin nav)        — filterable, paginated table
 *   scoped   (inside incident)  — compact list of that incident's actions
 *
 * The table is now a real <table> rather than nested CSS grids, so columns
 * align, headers associate with cells for screen readers, and the row layout
 * is defined once in CSS instead of twice in JSX.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, Ban, ChevronRight, LogIn, Pencil, Play, Plus,
  RefreshCw, Search, Shield, Sparkles, Trash2, User,
} from "lucide-react";

import { apiRequest } from "../api/client";
import {
  EmptyState, ErrorState, Grid, Page, PageHeader, Panel, SkeletonRows, StatTile,
} from "./ui/Primitives.jsx";

const PAGE_SIZE = 50;
const REFRESH_MS = 15000;

/* Map an audit action string to an icon and a tone. Destructive and blocked
   actions read as danger; creates as success; everything else is neutral. */
function actionMeta(action) {
  const a = (action || "").toLowerCase();
  if (a.includes("blocked") || a.includes("circuit")) return { Icon: Ban, tone: "tone-danger" };
  if (a.includes("delete")) return { Icon: Trash2, tone: "tone-danger" };
  if (a.includes("create")) return { Icon: Plus, tone: "tone-success" };
  if (a.includes("login")) return { Icon: LogIn, tone: "tone-info" };
  if (a.includes("explain") || a.includes("copilot")) return { Icon: Sparkles, tone: "tone-info" };
  if (a.includes("execute") || a.includes("remediation")) return { Icon: Play, tone: "tone-brand" };
  if (a.includes("update") || a.includes("config")) return { Icon: Pencil, tone: "tone-brand" };
  return { Icon: Activity, tone: "tone-neutral" };
}

function statusTone(s) {
  const v = (s || "").toLowerCase();
  if (v === "success" || v === "succeeded" || v === "ok") return "tone-success";
  if (v === "failed" || v === "error" || v === "blocked") return "tone-danger";
  if (v === "pending" || v === "running") return "tone-warning";
  return "tone-neutral";
}

function fmtAbs(v) {
  if (!v) return "—";
  return new Date(v).toLocaleString([], {
    month: "short", day: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
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
  try {
    return JSON.parse(raw);
  } catch {
    return { detail: String(raw) };
  }
}

/* Normalise the two backend shapes into one row model. */
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

const RESOURCE_FILTERS = [
  ["", "All resources"],
  ["action", "Actions"],
  ["policy", "Policy"],
  ["incident", "Incidents"],
  ["config", "Config"],
  ["user", "Users"],
];

function AuditRow({ row, expanded, onToggle }) {
  const { Icon, tone } = actionMeta(row.action);
  return (
    <>
      <tr className={`audit-row${expanded ? " is-expanded" : ""}`} onClick={onToggle}>
        <td className="audit-cell-time">
          <span className="audit-time-abs">{fmtAbs(row.time)}</span>
          <span className="audit-time-rel">{fmtRel(row.time)}</span>
        </td>
        <td>
          <span className="audit-actor">
            <span className="audit-actor-avatar" aria-hidden="true">
              <User size={11} />
            </span>
            <span className="audit-actor-name">{row.actor}</span>
          </span>
        </td>
        <td>
          <span className="audit-action">
            <span className={`audit-action-icon ${tone}`} aria-hidden="true">
              <Icon size={12} />
            </span>
            <code className="audit-action-name">{row.action}</code>
          </span>
        </td>
        <td className="audit-cell-resource">
          {row.resourceType ? <span className="audit-resource-type">{row.resourceType}</span> : "—"}
          {row.resourceId && <span className="audit-resource-id"> · {row.resourceId}</span>}
        </td>
        <td className="audit-cell-status">
          {row.status ? (
            <span className={`pill is-plain ${statusTone(row.status)}`}>{row.status}</span>
          ) : (
            <span className="audit-ip">{row.ip || "—"}</span>
          )}
        </td>
        <td className="audit-cell-chevron">
          <ChevronRight size={13} className={expanded ? "is-rotated" : ""} />
        </td>
      </tr>
      {expanded && row.details && (
        <tr className="audit-detail-row">
          <td colSpan={6}>
            <dl className="audit-detail">
              {Object.entries(row.details).map(([k, v]) => (
                <div key={k} className="audit-detail-item">
                  <dt>{k}</dt>
                  <dd>{typeof v === "object" ? JSON.stringify(v, null, 2) : String(v)}</dd>
                </div>
              ))}
            </dl>
          </td>
        </tr>
      )}
    </>
  );
}

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
    return () => {
      aliveRef.current = false;
      window.clearInterval(id);
    };
  }, [load]);

  const visible = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) =>
      `${r.actor} ${r.action} ${r.resourceType} ${r.resourceId} ${r.ip}`.toLowerCase().includes(q),
    );
  }, [rows, search]);

  const actors = useMemo(() => new Set(rows.map((r) => r.actor)).size, [rows]);
  const hasMore = isGlobal && offset + rows.length < total;

  /* ── Incident-scoped: compact card list inside the incident detail ──────*/
  if (!isGlobal) {
    return (
      <div className="detail-stack is-tight">
        {error ? (
          <ErrorState message={error} onRetry={load} />
        ) : visible.length === 0 ? (
          <EmptyState
            icon={Activity}
            title="No actions executed"
            message="Actions taken on this incident will be recorded here."
          />
        ) : (
          visible.map((row) => {
            const { Icon, tone } = actionMeta(row.action);
            return (
              <article key={row.key} className="exec-card">
                <header className="exec-head">
                  <h3 className="exec-title">
                    <span className={`audit-action-icon ${tone}`} aria-hidden="true">
                      <Icon size={12} />
                    </span>
                    {row.action}
                  </h3>
                  <span className={`pill is-plain ${statusTone(row.status)}`}>{row.status || "unknown"}</span>
                </header>
                {row.details?.message && <p className="detail-prose">{row.details.message}</p>}
                <footer className="exec-meta">
                  {row.actor} · {fmtAbs(row.time)}
                </footer>
              </article>
            );
          })
        )}
      </div>
    );
  }

  /* ── Global audit trail ────────────────────────────────────────────────*/
  return (
    <Page>
      <PageHeader
        title="Audit trail"
        meta="Immutable record of every action, change and access event · refreshes every 15s"
        actions={
          <button
            type="button"
            className="btn btn-ghost"
            onClick={() => {
              setLoading(true);
              load();
            }}
          >
            <RefreshCw size={13} className={loading ? "is-spinning" : ""} /> Refresh
          </button>
        }
      />

      <Grid cols={3}>
        <StatTile label="Total events" icon={Shield} tone="neutral" value={total.toLocaleString()} sub="recorded" loading={loading && !rows.length} />
        <StatTile label="On this page" icon={Activity} tone="brand" value={rows.length} sub={`page size ${PAGE_SIZE}`} loading={loading && !rows.length} />
        <StatTile label="Distinct actors" icon={User} tone="info" value={actors} sub="on this page" loading={loading && !rows.length} />
      </Grid>

      <Panel flush>
        <div className="table-toolbar">
          <div className="search-field">
            <Search size={13} className="search-field-icon" />
            <input
              className="form-input"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search actor, action, resource, IP…"
              aria-label="Search audit events"
            />
          </div>
          <select
            className="form-select"
            value={resourceType}
            aria-label="Filter by resource type"
            onChange={(e) => {
              setOffset(0);
              setResourceType(e.target.value);
            }}
          >
            {RESOURCE_FILTERS.map(([v, l]) => (
              <option key={v} value={v}>{l}</option>
            ))}
          </select>
        </div>

        {loading && rows.length === 0 ? (
          <div className="table-skeletons">
            <SkeletonRows count={6} height={40} />
          </div>
        ) : error ? (
          <ErrorState message={error} onRetry={load} />
        ) : visible.length === 0 ? (
          <EmptyState
            icon={Shield}
            title="No audit events"
            message={
              search || resourceType
                ? "No events match the current filter."
                : "Actions, policy changes and access events appear here as they happen."
            }
          />
        ) : (
          <div className="table-scroll">
            <table className="audit-table">
              <thead>
                <tr>
                  <th scope="col">Time</th>
                  <th scope="col">Actor</th>
                  <th scope="col">Action</th>
                  <th scope="col">Resource</th>
                  <th scope="col" className="audit-cell-status">Status / IP</th>
                  <th scope="col"><span className="sr-only">Expand</span></th>
                </tr>
              </thead>
              <tbody>
                {visible.map((row) => (
                  <AuditRow
                    key={row.key}
                    row={row}
                    expanded={expanded === row.key}
                    onToggle={() => setExpanded(expanded === row.key ? null : row.key)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {!loading && !error && total > 0 && (
        <div className="table-pager">
          <span className="table-pager-label">
            {offset + 1}–{offset + rows.length} of {total.toLocaleString()} events
          </span>
          <div className="table-pager-buttons">
            <button
              type="button" className="btn btn-ghost btn-sm"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            >
              Previous
            </button>
            <button
              type="button" className="btn btn-ghost btn-sm"
              disabled={!hasMore}
              onClick={() => setOffset(offset + PAGE_SIZE)}
            >
              Next
            </button>
          </div>
        </div>
      )}
    </Page>
  );
}
