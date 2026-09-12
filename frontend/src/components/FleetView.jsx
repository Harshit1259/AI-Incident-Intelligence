/**
 * NeuroOps — Fleet Monitoring
 *
 * High-density operations view built to read 500–2,000 monitored devices at a
 * glance on a single monitor. Three lenses on the same fleet:
 *   • Matrix  — a status wall (one cell per device, worst-first)
 *   • Table   — sortable detail rows
 *   • Groups  — health rollups by site / OS / role / status
 *
 * Health is derived from agent status + staleness (no per-device metric
 * fan-out, so it stays cheap at 2k devices). Live data comes from /agents; a
 * clearly-labelled sample fleet lets you preview the design at scale before
 * your real fleet is connected.
 *
 * The three-lens structure is unchanged — it is the right shape for this job.
 * What changed: every colour now resolves from a health class to a design
 * token instead of a literal, the drawer reuses the shared slide-over, and the
 * hand-rolled hover/scale handlers became CSS.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, ChevronRight, Cpu, Grid3x3, HardDrive, Layers, RefreshCw,
  Search, Server, Table2, Wifi, WifiOff, X, Zap,
} from "lucide-react";

import { getAgents } from "../api/agents.js";
import { EmptyState, Page, PageHeader, Panel, StatTile } from "./ui/Primitives.jsx";

/* ── Health model ─────────────────────────────────────────────────── */
const STALE_MS = 2 * 60 * 1000; // healthy → warning after 2m silence
const OFFLINE_MS = 10 * 60 * 1000; // → offline after 10m silence

/* Ordered worst-first. `cls` sets --health, from which every swatch, bar and
   cell in this view takes its colour. */
const HEALTH = {
  critical: { label: "Critical", cls: "health-critical", rank: 0 },
  offline: { label: "Offline", cls: "health-offline", rank: 1 },
  warning: { label: "Warning", cls: "health-warning", rank: 2 },
  healthy: { label: "Healthy", cls: "health-healthy", rank: 3 },
};
const HEALTH_ORDER = ["critical", "offline", "warning", "healthy"];

function deriveHealth(d, now) {
  if (d.status === "error") return "critical";
  const seen = d.last_seen_at ? now - new Date(d.last_seen_at).getTime() : Infinity;
  if (d.status === "stopped" || seen > OFFLINE_MS) return "offline";
  // Metric-driven escalation (only present on sample / enriched devices).
  if ((d.cpu ?? 0) >= 92 || (d.mem ?? 0) >= 95) return "critical";
  if (d.status === "inactive" || seen > STALE_MS || (d.cpu ?? 0) >= 80 || (d.mem ?? 0) >= 85) return "warning";
  return "healthy";
}

/* ── Derived facets (work for real + sample devices) ──────────────── */
const siteOf = (d) =>
  d.site || (d.host_ip ? `subnet ${d.host_ip.split(".").slice(0, 2).join(".")}.x` : "unknown");
const roleOf = (d) =>
  d.role || (d.name ? (d.name.match(/^[a-z]+/i)?.[0] || "node").toLowerCase() : "node");

function ago(ts) {
  if (!ts) return "never";
  const s = Math.floor((Date.now() - new Date(ts).getTime()) / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

const usageTone = (v) => (v >= 90 ? "tone-danger" : v >= 75 ? "tone-warning" : "tone-brand");

/* ── Sample fleet — preview the design at scale (clearly labelled) ── */
const SITES = ["us-east-1", "us-west-2", "eu-west-1", "ap-south-1", "on-prem-dc1"];
const ROLES = ["web", "api", "db", "cache", "worker", "k8s-node", "edge", "lb", "queue"];
const OSES = ["Ubuntu 22.04", "Debian 12", "RHEL 9", "Amazon Linux 2", "Windows Server 2022"];

function makeSampleFleet(n) {
  const now = Date.now();
  const out = [];
  for (let i = 0; i < n; i++) {
    const role = ROLES[i % ROLES.length];
    const site = SITES[i % SITES.length];
    // Realistic skew: ~84% healthy, rest spread across warn/crit/offline.
    const roll = Math.random();
    let status = "active";
    let cpu = 8 + Math.random() * 45;
    let mem = 20 + Math.random() * 45;
    let seenAgo = Math.random() * 90_000;
    if (roll > 0.985) {
      status = "error";
      cpu = 90 + Math.random() * 9;
      mem = 80 + Math.random() * 18;
    } else if (roll > 0.955) {
      status = "stopped";
      seenAgo = OFFLINE_MS + Math.random() * 3_600_000;
    } else if (roll > 0.9) {
      cpu = 80 + Math.random() * 12;
      mem = 70 + Math.random() * 20;
    } else if (roll > 0.875) {
      status = "inactive";
      seenAgo = STALE_MS + Math.random() * 200_000;
    }
    out.push({
      id: `dev-${String(i + 1).padStart(4, "0")}`,
      name: `${role}-${site.split("-")[0]}-${String((i % 240) + 1).padStart(2, "0")}`,
      host_ip: `10.${(i >> 8) & 255}.${i & 255}.${((i * 7) % 254) + 1}`,
      os_type: OSES[i % OSES.length],
      version: "1.4.0",
      status,
      site,
      role,
      cpu: Math.round(cpu),
      mem: Math.round(mem),
      last_seen_at: new Date(now - seenAgo).toISOString(),
    });
  }
  return out;
}

/* ── Fleet health donut ───────────────────────────────────────────── */
function FleetDonut({ counts, total }) {
  const segs = HEALTH_ORDER.map((k, i) => {
    const before = HEALTH_ORDER.slice(0, i).reduce((s, kk) => s + (counts[kk] || 0), 0);
    const start = (before / (total || 1)) * 360;
    const end = ((before + (counts[k] || 0)) / (total || 1)) * 360;
    return `var(--h-${k}) ${start}deg ${end}deg`;
  });
  const healthyPct = total ? Math.round(((counts.healthy || 0) / total) * 100) : 0;

  return (
    <div className="fleet-donut-block">
      <div className="donut fleet-donut" style={{ background: `conic-gradient(${segs.join(",")})` }}>
        <div className="donut-hole">
          <span className="donut-value">{healthyPct}%</span>
          <span className="donut-label">healthy</span>
        </div>
      </div>
      <ul className="fleet-legend">
        {HEALTH_ORDER.map((k) => (
          <li key={k} className={`fleet-legend-row ${HEALTH[k].cls}`}>
            <span className="fleet-swatch" aria-hidden="true" />
            <span className="fleet-legend-label">{HEALTH[k].label}</span>
            <span className="fleet-legend-count">{(counts[k] || 0).toLocaleString()}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/* ── Device drawer ────────────────────────────────────────────────── */
function DeviceDrawer({ device, onClose }) {
  useEffect(() => {
    if (!device) return undefined;
    const onKey = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [device, onClose]);

  if (!device) return null;

  const h = HEALTH[device._health];
  const rows = [
    ["IP address", device.host_ip],
    ["Operating system", device.os_type],
    ["Role", roleOf(device)],
    ["Site / segment", siteOf(device)],
    ["Agent version", device.version ? `v${device.version}` : null],
    ["Last seen", device.last_seen_at ? `${ago(device.last_seen_at)} ago` : "never"],
    ["Device ID", device.id],
  ].filter(([, v]) => v);

  const meters = [
    ["CPU", device.cpu, Cpu],
    ["Memory", device.mem, HardDrive],
  ].filter(([, v]) => v != null);

  return (
    <>
      <div className="notif-scrim" onClick={onClose} aria-hidden="true" />
      <aside className="notif-panel" role="dialog" aria-modal="true" aria-label="Device details">
        <header className="notif-panel-head device-drawer-head">
          <span className={`device-avatar ${h.cls}`} aria-hidden="true">
            <Server size={18} />
          </span>
          <div className="device-drawer-title">
            <span className="device-name">{device.name || device.id}</span>
            <span className={`device-health ${h.cls}`}>{h.label}</span>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} title="Close">
            <X size={15} />
          </button>
        </header>

        <div className="notif-panel-body device-drawer-body">
          {meters.length > 0 && (
            <div className="usage-stack">
              {meters.map(([label, pct, Icon]) => (
                <div key={label} className={`usage ${usageTone(pct)}`}>
                  <div className="usage-head">
                    <span className="usage-label">
                      {Icon && <Icon size={12} />} {label}
                    </span>
                    <span className="usage-value">{pct}%</span>
                  </div>
                  <div className="usage-track">
                    <div className="usage-fill" style={{ width: `${pct}%` }} />
                  </div>
                </div>
              ))}
            </div>
          )}

          <h3 className="detail-section-label device-drawer-section">System details</h3>
          <dl className="kv-list">
            {rows.map(([k, v]) => (
              <div key={k} className="kv-item">
                <dt>{k}</dt>
                <dd>{v}</dd>
              </div>
            ))}
          </dl>
        </div>
      </aside>
    </>
  );
}

/* ── Density presets for the matrix ───────────────────────────────── */
const DENSITY = {
  compact: { cell: 13, gap: 3, label: false },
  normal: { cell: 20, gap: 4, label: false },
  large: { cell: 34, gap: 5, label: true },
};

/* ── Lenses ───────────────────────────────────────────────────────── */
function MatrixView({ devices, dz, onSelect, onHover }) {
  return (
    <div className="matrix-scroll" onMouseLeave={() => onHover(null)}>
      <div
        className="matrix-grid"
        style={{ "--cell": `${dz.cell}px`, "--cell-gap": `${dz.gap}px` }}
      >
        {devices.map((d) => {
          const h = HEALTH[d._health];
          return (
            <button
              key={d.id}
              type="button"
              className={`matrix-cell ${h.cls}${dz.cell > 24 ? " is-large" : ""}`}
              onClick={() => onSelect(d)}
              onMouseEnter={() => onHover(d)}
              onFocus={() => onHover(d)}
              title={dz.label ? undefined : `${d.name || d.id} · ${h.label}`}
            >
              {dz.label ? (d.name || "").replace(/^[a-z]+-/i, "").slice(0, 5) : null}
            </button>
          );
        })}
      </div>
    </div>
  );
}

function TableView({ devices, onSelect }) {
  // Cap rendered rows for very large fleets; matrix is the dense lens.
  const rows = devices.slice(0, 600);
  return (
    <div className="lens-scroll">
      <table className="data-table fleet-table">
        <thead>
          <tr>
            <th scope="col">Device</th>
            <th scope="col">Status</th>
            <th scope="col">IP</th>
            <th scope="col">OS</th>
            <th scope="col">Role</th>
            <th scope="col">CPU</th>
            <th scope="col">Mem</th>
            <th scope="col">Last seen</th>
            <th scope="col"><span className="sr-only">Open</span></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((d) => {
            const h = HEALTH[d._health];
            return (
              <tr key={d.id} className="fleet-table-row" onClick={() => onSelect(d)}>
                <td className="cell-strong-plain">{d.name || d.id}</td>
                <td>
                  <span className={`device-health ${h.cls}`}>
                    <span className="fleet-dot" aria-hidden="true" /> {h.label}
                  </span>
                </td>
                <td className="cell-mono">{d.host_ip || "—"}</td>
                <td>{d.os_type || "—"}</td>
                <td>{roleOf(d)}</td>
                <td className={d.cpu >= 85 ? "tone-danger cell-strong" : d.cpu >= 70 ? "tone-warning cell-strong" : ""}>
                  {d.cpu != null ? `${d.cpu}%` : "—"}
                </td>
                <td className={d.mem >= 90 ? "tone-danger cell-strong" : d.mem >= 75 ? "tone-warning cell-strong" : ""}>
                  {d.mem != null ? `${d.mem}%` : "—"}
                </td>
                <td className="cell-muted">{ago(d.last_seen_at)} ago</td>
                <td className="cell-chevron"><ChevronRight size={13} /></td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {devices.length > rows.length && (
        <p className="lens-footnote">
          Showing the first {rows.length.toLocaleString()} of {devices.length.toLocaleString()} — refine
          your search, or use the Matrix lens for the whole fleet.
        </p>
      )}
    </div>
  );
}

function GroupsView({ groups, onSelect }) {
  const [open, setOpen] = useState(() => new Set());

  return (
    <div className="lens-scroll group-list">
      {groups.map((g) => {
        const isOpen = open.has(g.key);
        const healthyPct = Math.round((g.healthy / g.total) * 100);
        return (
          <section key={g.key} className="group-card">
            <button
              type="button"
              className="group-head"
              aria-expanded={isOpen}
              onClick={() =>
                setOpen((s) => {
                  const n = new Set(s);
                  if (n.has(g.key)) n.delete(g.key);
                  else n.add(g.key);
                  return n;
                })
              }
            >
              <ChevronRight size={14} className={`group-chevron${isOpen ? " is-open" : ""}`} />
              <span className="group-headings">
                <span className="group-name">{g.key}</span>
                <span className="group-sub">
                  {g.total.toLocaleString()} devices · {healthyPct}% healthy
                </span>
              </span>
              <span className="group-bar">
                {HEALTH_ORDER.map(
                  (k) =>
                    g[k] > 0 && (
                      <span
                        key={k}
                        className={`group-bar-seg ${HEALTH[k].cls}`}
                        style={{ width: `${(g[k] / g.total) * 100}%` }}
                        title={`${HEALTH[k].label}: ${g[k]}`}
                      />
                    ),
                )}
              </span>
              <span className="group-counts">
                {g.critical > 0 && <span className="health-critical group-count">{g.critical} crit</span>}
                {g.offline > 0 && <span className="health-offline group-count">{g.offline} off</span>}
              </span>
            </button>

            {isOpen && (
              <div className="group-devices">
                {g.devices.slice(0, 120).map((d) => (
                  <button key={d.id} type="button" className="group-device" onClick={() => onSelect(d)}>
                    <span className={`fleet-dot ${HEALTH[d._health].cls}`} aria-hidden="true" />
                    <span className="group-device-name">{d.name || d.id}</span>
                    {d.cpu != null && <span className="group-device-cpu">{d.cpu}%</span>}
                  </button>
                ))}
                {g.devices.length > 120 && (
                  <p className="group-more">+{(g.devices.length - 120).toLocaleString()} more…</p>
                )}
              </div>
            )}
          </section>
        );
      })}
    </div>
  );
}

/* ── Main view ────────────────────────────────────────────────────── */
export default function FleetView() {
  const [source, setSource] = useState("live"); // live | 500 | 2000
  const [live, setLive] = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(null);

  const [mode, setMode] = useState("matrix"); // matrix | table | groups
  const [density, setDensity] = useState("normal");
  const [groupBy, setGroupBy] = useState("site");
  const [sortBy, setSortBy] = useState("health");
  const [q, setQ] = useState("");
  const [healthFilter, setHealthFilter] = useState(null);
  const [selected, setSelected] = useState(null);
  const [hover, setHover] = useState(null);
  const tipRef = useRef({ x: 0, y: 0 });

  const loadLive = useCallback(async () => {
    setRefreshing(true);
    try {
      const d = await getAgents();
      setLive(Array.isArray(d) ? d : []);
      setLastRefresh(new Date());
    } catch {
      setLive([]);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    loadLive();
    const t = setInterval(loadLive, 30_000);
    return () => clearInterval(t);
  }, [loadLive]);

  // Sample fleets are memoised so they stay stable across re-renders.
  const sample500 = useMemo(() => makeSampleFleet(500), []);
  const sample2000 = useMemo(() => makeSampleFleet(2000), []);

  const rawDevices = source === "500" ? sample500 : source === "2000" ? sample2000 : live;
  const isSample = source !== "live";

  const devices = useMemo(() => {
    const now = Date.now();
    return rawDevices.map((d) => ({ ...d, _health: deriveHealth(d, now) }));
  }, [rawDevices]);

  const counts = useMemo(() => {
    const c = { critical: 0, offline: 0, warning: 0, healthy: 0 };
    for (const d of devices) c[d._health]++;
    return c;
  }, [devices]);

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    const list = devices.filter((d) => {
      if (healthFilter && d._health !== healthFilter) return false;
      if (needle) {
        const hay = `${d.name || ""} ${d.host_ip || ""} ${d.os_type || ""} ${roleOf(d)} ${siteOf(d)}`.toLowerCase();
        if (!hay.includes(needle)) return false;
      }
      return true;
    });
    const cmp = {
      health: (a, b) =>
        HEALTH[a._health].rank - HEALTH[b._health].rank || (a.name || "").localeCompare(b.name || ""),
      name: (a, b) => (a.name || "").localeCompare(b.name || ""),
      cpu: (a, b) => (b.cpu ?? -1) - (a.cpu ?? -1),
      seen: (a, b) => new Date(b.last_seen_at || 0) - new Date(a.last_seen_at || 0),
    };
    return [...list].sort(cmp[sortBy] || cmp.health);
  }, [devices, q, healthFilter, sortBy]);

  const groups = useMemo(() => {
    if (mode !== "groups") return [];
    const keyer = {
      site: siteOf,
      os: (d) => d.os_type || "Unknown",
      role: roleOf,
      status: (d) => HEALTH[d._health].label,
    };
    const fn = keyer[groupBy] || siteOf;
    const map = new Map();
    for (const d of filtered) {
      const k = fn(d);
      if (!map.has(k)) {
        map.set(k, { key: k, total: 0, critical: 0, offline: 0, warning: 0, healthy: 0, devices: [] });
      }
      const g = map.get(k);
      g.total++;
      g[d._health]++;
      g.devices.push(d);
    }
    return [...map.values()].sort(
      (a, b) => b.critical + b.offline - (a.critical + a.offline) || b.total - a.total,
    );
  }, [filtered, mode, groupBy]);

  const dz = DENSITY[density];
  const trackTip = (e) => {
    tipRef.current = { x: e.clientX, y: e.clientY };
  };

  const toggleHealth = (k) => setHealthFilter((p) => (p === k ? null : k));

  return (
    <div className="ui-page fade-in" onMouseMove={hover ? trackTip : undefined}>
      <PageHeader
        title="Fleet monitoring"
        meta={`${devices.length.toLocaleString()} devices · ${
          isSample
            ? "sample preview"
            : `auto-refresh 30s${lastRefresh ? ` · updated ${lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : ""}`
        }`}
        actions={
          <>
            <div className="segmented" role="group" aria-label="Data source">
              {[
                ["live", "Live"],
                ["500", "Sample 500"],
                ["2000", "Sample 2K"],
              ].map(([k, lbl]) => (
                <button
                  key={k}
                  type="button"
                  className={`segmented-item${source === k ? " is-active" : ""}`}
                  onClick={() => {
                    setSource(k);
                    setSelected(null);
                  }}
                >
                  {lbl}
                </button>
              ))}
            </div>
            <button type="button" className="btn btn-ghost" onClick={loadLive} disabled={refreshing}>
              <RefreshCw size={13} className={refreshing ? "is-spinning" : ""} /> Refresh
            </button>
          </>
        }
      />

      {isSample && (
        <div className="callout tone-warning callout-block">
          <Zap size={13} className="callout-icon" />
          <p className="callout-text">
            <strong>Sample fleet</strong> — synthetic devices for previewing the design at scale.
            Switch to <em>Live</em> for your connected agents.
          </p>
        </div>
      )}

      <div className="ui-grid cols-5 fleet-kpis">
        <StatTile
          label="Total devices" icon={Server} tone="brand"
          value={devices.length.toLocaleString()} sub={healthFilter ? "filtered" : "all devices"}
          onClick={() => setHealthFilter(null)}
        />
        <StatTile label="Healthy" icon={Wifi} tone="success" value={counts.healthy.toLocaleString()} sub="reporting normally" onClick={() => toggleHealth("healthy")} />
        <StatTile label="Warning" icon={Activity} tone="warning" value={counts.warning.toLocaleString()} sub="stale or loaded" onClick={() => toggleHealth("warning")} />
        <StatTile label="Critical" icon={Zap} tone="danger" value={counts.critical.toLocaleString()} sub="need attention" onClick={() => toggleHealth("critical")} />
        <StatTile label="Offline" icon={WifiOff} tone="neutral" value={counts.offline.toLocaleString()} sub="not checking in" onClick={() => toggleHealth("offline")} />
      </div>

      <div className="fleet-layout">
        <aside className="fleet-rail">
          <Panel title="Fleet health" sub={`${devices.length.toLocaleString()} devices`}>
            <FleetDonut counts={counts} total={devices.length} />
          </Panel>

          <Panel title="View">
            <div className="lens-picker">
              {[
                ["matrix", "Matrix", Grid3x3],
                ["table", "Table", Table2],
                ["groups", "Groups", Layers],
              ].map(([k, lbl, Icon]) => (
                <button
                  key={k}
                  type="button"
                  className={`lens-option${mode === k ? " is-active" : ""}`}
                  onClick={() => setMode(k)}
                >
                  {Icon && <Icon size={15} />} {lbl}
                </button>
              ))}
            </div>

            {mode === "matrix" && (
              <div className="form-group">
                <label className="form-label" htmlFor="fleet-density">Density</label>
                <div className="segmented is-full" id="fleet-density">
                  {["compact", "normal", "large"].map((k) => (
                    <button
                      key={k}
                      type="button"
                      className={`segmented-item${density === k ? " is-active" : ""}`}
                      onClick={() => setDensity(k)}
                    >
                      {k}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {mode === "groups" && (
              <div className="form-group">
                <label className="form-label" htmlFor="fleet-groupby">Group by</label>
                <select id="fleet-groupby" className="form-select" value={groupBy} onChange={(e) => setGroupBy(e.target.value)}>
                  <option value="site">Site / segment</option>
                  <option value="os">Operating system</option>
                  <option value="role">Role</option>
                  <option value="status">Health status</option>
                </select>
              </div>
            )}

            <div className="form-group">
              <label className="form-label" htmlFor="fleet-sort">Sort by</label>
              <select id="fleet-sort" className="form-select" value={sortBy} onChange={(e) => setSortBy(e.target.value)}>
                <option value="health">Worst first</option>
                <option value="name">Name</option>
                <option value="cpu">CPU load</option>
                <option value="seen">Last seen</option>
              </select>
            </div>
          </Panel>

          <Panel title="Legend">
            <ul className="fleet-legend">
              {HEALTH_ORDER.map((k) => (
                <li key={k} className={`fleet-legend-row ${HEALTH[k].cls}`}>
                  <span className="fleet-swatch" aria-hidden="true" />
                  <span className="fleet-legend-label">{HEALTH[k].label}</span>
                </li>
              ))}
            </ul>
          </Panel>
        </aside>

        <Panel flush className="fleet-workspace">
          <div className="table-toolbar">
            <div className="search-field">
              <Search size={13} className="search-field-icon" />
              <input
                className="form-input"
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Search by name, IP, OS, role, site…"
                aria-label="Search devices"
              />
            </div>
            <span className="fleet-count">
              <strong>{filtered.length.toLocaleString()}</strong> shown
              {healthFilter && (
                <>
                  {" · "}
                  <button type="button" className="link-action" onClick={() => setHealthFilter(null)}>
                    clear filter
                  </button>
                </>
              )}
            </span>
          </div>

          {loading && source === "live" ? (
            <div className="matrix-scroll">
              <div className="matrix-grid" style={{ "--cell": "20px", "--cell-gap": "4px" }}>
                {Array.from({ length: 120 }, (_, i) => (
                  <div key={i} className="skeleton matrix-cell" />
                ))}
              </div>
            </div>
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={Server}
              title={source === "live" ? "No agents connected" : "No devices match"}
              message={
                source === "live"
                  ? "Install the NeuroOps agent on your hosts, or preview the design with a sample fleet using the switch above."
                  : "Adjust your search or health filter to see devices."
              }
            />
          ) : mode === "matrix" ? (
            <MatrixView devices={filtered} dz={dz} onSelect={setSelected} onHover={setHover} />
          ) : mode === "table" ? (
            <TableView devices={filtered} onSelect={setSelected} />
          ) : (
            <GroupsView groups={groups} onSelect={setSelected} />
          )}
        </Panel>
      </div>

      {hover && (
        <div
          className={`fleet-tooltip ${HEALTH[hover._health].cls}`}
          style={{
            left: Math.min(tipRef.current.x + 14, window.innerWidth - 230),
            top: tipRef.current.y + 14,
          }}
        >
          <div className="fleet-tooltip-head">
            <span className="fleet-swatch" aria-hidden="true" />
            <span className="fleet-tooltip-name">{hover.name || hover.id}</span>
          </div>
          <p className="fleet-tooltip-ip">{hover.host_ip || "—"}</p>
          <p className="fleet-tooltip-meta">
            {hover.os_type} · {HEALTH[hover._health].label} · {ago(hover.last_seen_at)} ago
          </p>
          {hover.cpu != null && (
            <p className="fleet-tooltip-usage">
              CPU {hover.cpu}% · MEM {hover.mem}%
            </p>
          )}
        </div>
      )}

      <DeviceDrawer device={selected} onClose={() => setSelected(null)} />
    </div>
  );
}
