/**
 * Log Explorer — centralised, streamable log viewer.
 *
 * Restructured as a real table so the five columns actually align (they were
 * two independently declared CSS grids before, header and body, which drifted
 * apart at narrow widths). Filters moved into the table toolbar, and the
 * inline <style> block that injected a @keyframes at render time is gone —
 * the animation lives in the stylesheet with the rest.
 */
import { useState, useEffect, useCallback, useRef } from "react";
import {
  AlertTriangle, Info, RefreshCw, Radio, ScrollText, Search, Server, Square, X,
} from "lucide-react";

import { queryLogs, getLogStats, createLogStream } from "../api/logs.js";
import {
  EmptyState, Grid, Page, PageHeader, Panel, SkeletonRows, StatTile,
} from "./ui/Primitives.jsx";

const PAGE_SIZE = 100;

/* Level → tone. Debug and trace share "neutral": they are not warnings, and
   giving each its own colour was adding hues without adding meaning. */
const LEVEL = {
  error: { tone: "tone-danger", label: "ERROR" },
  warn: { tone: "tone-warning", label: "WARN" },
  warning: { tone: "tone-warning", label: "WARN" },
  info: { tone: "tone-info", label: "INFO" },
  debug: { tone: "tone-neutral", label: "DEBUG" },
  trace: { tone: "tone-neutral", label: "TRACE" },
};

const lvl = (cat) => LEVEL[(cat || "info").toLowerCase()] || LEVEL.info;

function fmtTime(ts) {
  if (!ts) return "—";
  return new Date(ts).toLocaleTimeString([], {
    hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
}

function fmtDate(ts) {
  if (!ts) return "";
  return new Date(ts).toLocaleDateString([], { month: "short", day: "numeric" });
}

function LogRow({ entry, expanded, onToggle }) {
  const c = lvl(entry.event_category);
  const isAlert = c.label === "ERROR" || c.label === "WARN";

  return (
    <>
      <tr
        className={`log-row ${c.tone}${expanded ? " is-expanded" : ""}${isAlert ? " is-alert" : ""}`}
        onClick={onToggle}
      >
        <td className="log-cell-time">
          <span className="log-date">{fmtDate(entry.timestamp)}</span>{" "}
          <span className="log-time">{fmtTime(entry.timestamp)}</span>
        </td>
        <td className="log-cell-level">
          <span className={`log-level ${c.tone}`}>{c.label}</span>
        </td>
        <td className="log-cell-host" title={entry.host_ip}>{entry.host_ip || "—"}</td>
        <td className="log-cell-tag">{entry.log_tag || "—"}</td>
        <td className="log-cell-message">{entry.message || "—"}</td>
      </tr>
      {expanded && (
        <tr className="log-detail-row">
          <td colSpan={5}>
            <div className="log-detail">
              <dl className="log-detail-grid">
                {[
                  ["Agent ID", entry.agent_id],
                  ["Source", entry.log_source],
                  ["Host IP", entry.host_ip],
                  ["Type", entry.event_type || "log"],
                  ["Tag", entry.log_tag],
                  ["Category", entry.event_category || "info"],
                ].map(([k, v]) => (
                  <div key={k} className="log-detail-item">
                    <dt>{k}</dt>
                    <dd>{v || "—"}</dd>
                  </div>
                ))}
              </dl>
              <h4 className="detail-section-label">Full message</h4>
              <pre className="log-message-full">{entry.message}</pre>
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

export default function LogExplorer() {
  const [entries, setEntries] = useState([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [stats, setStats] = useState(null);
  const [expandedID, setExpandedID] = useState(null);
  const [streaming, setStreaming] = useState(false);
  const [streamCount, setStreamCount] = useState(0);
  const stopRef = useRef(null);

  const [filters, setFilters] = useState({ host: "", category: "", tag: "", search: "" });
  const [offset, setOffset] = useState(0);

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

  useEffect(() => {
    load();
  }, [load]);

  function toggleStream() {
    if (streaming) {
      if (stopRef.current) stopRef.current();
      stopRef.current = null;
      setStreaming(false);
      setStreamCount(0);
      return;
    }
    const lastId = entries.length > 0 ? Math.max(...entries.map((e) => e.id || 0)) : 0;
    stopRef.current = createLogStream(
      lastId,
      (entry) => {
        setEntries((prev) => {
          const n = [entry, ...prev];
          if (n.length > 500) n.length = 500;
          return n;
        });
        setStreamCount((c) => c + 1);
      },
      () => {},
    );
    setStreaming(true);
  }

  useEffect(
    () => () => {
      if (stopRef.current) stopRef.current();
    },
    [],
  );

  function setFilter(key, val) {
    setOffset(0);
    setFilters((f) => ({ ...f, [key]: val }));
  }

  function clearFilters() {
    setOffset(0);
    setFilters({ host: "", category: "", tag: "", search: "" });
  }

  const cats = stats?.by_category || {};
  const errorCount = cats.error || 0;
  const warnCount = (cats.warn || 0) + (cats.warning || 0);
  const infoCount = cats.info || 0;
  const hosts = [...new Set((stats?.by_host || []).map((h) => h.host_ip))];
  const tags = [...new Set((stats?.by_tag || []).map((t) => t.tag))];
  const hasMore = offset + entries.length < total;
  const activeFilter = Object.values(filters).some(Boolean);

  return (
    <Page>
      <PageHeader
        title="Log explorer"
        meta="All agents, all hosts · search, filter and stream in real time"
        actions={
          <>
            {streaming && (
              <span className="stream-badge tone-success">
                <span className="stream-dot" aria-hidden="true" />
                Live · {streamCount} new
              </span>
            )}
            <button
              type="button"
              className={streaming ? "btn btn-danger" : "btn btn-success"}
              onClick={toggleStream}
            >
              {streaming ? <Square size={12} /> : <Radio size={12} />}
              {streaming ? "Stop stream" : "Live stream"}
            </button>
            <button type="button" className="btn btn-ghost" onClick={load} disabled={streaming}>
              <RefreshCw size={13} className={loading ? "is-spinning" : ""} /> Refresh
            </button>
          </>
        }
      />

      <Grid cols={4}>
        <StatTile label="Total logs" icon={ScrollText} tone="neutral" value={(stats?.total_logs || 0).toLocaleString()} sub="last 24h" />
        <StatTile label="Errors" icon={AlertTriangle} tone={errorCount > 0 ? "danger" : "success"} value={errorCount.toLocaleString()} sub={`${stats?.recent_errors || 0} in last hour`} />
        <StatTile label="Warnings" icon={Info} tone={warnCount > 0 ? "warning" : "neutral"} value={warnCount.toLocaleString()} sub={`${infoCount.toLocaleString()} info`} />
        <StatTile label="Hosts" icon={Server} tone="info" value={hosts.length} sub="reporting logs" />
      </Grid>

      <Panel flush>
        <div className="table-toolbar">
          <div className="search-field">
            <Search size={13} className="search-field-icon" />
            <input
              className="form-input"
              placeholder="Search messages…"
              value={filters.search}
              aria-label="Search log messages"
              onChange={(e) => setFilter("search", e.target.value)}
            />
          </div>
          <select className="form-select" value={filters.category} aria-label="Filter by level" onChange={(e) => setFilter("category", e.target.value)}>
            <option value="">All levels</option>
            <option value="error">Error</option>
            <option value="warn">Warning</option>
            <option value="info">Info</option>
            <option value="debug">Debug</option>
          </select>
          <select className="form-select" value={filters.host} aria-label="Filter by host" onChange={(e) => setFilter("host", e.target.value)}>
            <option value="">All hosts</option>
            {hosts.map((h) => (
              <option key={h} value={h}>{h}</option>
            ))}
          </select>
          <select className="form-select" value={filters.tag} aria-label="Filter by tag" onChange={(e) => setFilter("tag", e.target.value)}>
            <option value="">All tags</option>
            {tags.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
          {activeFilter && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={clearFilters}>
              <X size={11} /> Clear
            </button>
          )}
        </div>

        {loading && !streaming ? (
          <div className="table-skeletons">
            <SkeletonRows count={8} height={30} />
          </div>
        ) : entries.length === 0 ? (
          <EmptyState
            icon={ScrollText}
            title="No logs found"
            message={
              activeFilter
                ? "No entries match the current filter."
                : "Install the NeuroOps agent on your hosts to start collecting logs."
            }
          />
        ) : (
          <div className="table-scroll">
            <table className="log-table">
              <thead>
                <tr>
                  <th scope="col">Timestamp</th>
                  <th scope="col">Level</th>
                  <th scope="col">Host</th>
                  <th scope="col">Tag</th>
                  <th scope="col">Message</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry, i) => (
                  <LogRow
                    key={entry.id ?? `row-${i}`}
                    entry={entry}
                    expanded={expandedID === entry.id}
                    onToggle={() => setExpandedID(expandedID === entry.id ? null : entry.id)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {!streaming && entries.length > 0 && (
        <div className="table-pager">
          <span className="table-pager-label">
            {offset + 1}–{offset + entries.length} of {total.toLocaleString()} logs
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
