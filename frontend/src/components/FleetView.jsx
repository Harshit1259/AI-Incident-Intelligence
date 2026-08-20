/**
 * NeuroOps — Fleet Monitoring
 * High-density operations view built to read 500–2,000 monitored devices
 * at a glance on a single monitor. Three lenses on the same fleet:
 *   • Matrix  — a status wall (one cell per device, worst-first)
 *   • Table   — sortable detail rows
 *   • Groups  — health rollups by site / OS / role / status
 *
 * Health is derived from agent status + staleness (no per-device metric
 * fan-out, so it stays cheap at 2k devices). Live data comes from /agents;
 * a clearly-labelled sample fleet lets you preview the design at scale
 * before your real fleet is connected.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Activity, Cpu, Grid3x3, HardDrive, Layers, RefreshCw,
  Search, Server, Table2, Wifi, WifiOff, X, Zap, ChevronRight,
} from "lucide-react";
import { getAgents } from "../api/agents.js";

/* ── Health model ─────────────────────────────────────────────────── */
const STALE_MS  = 2 * 60 * 1000;   // healthy → warning after 2m silence
const OFFLINE_MS = 10 * 60 * 1000; // → offline after 10m silence

const HEALTH = {
  critical: { label: "Critical", color: "#FF3B3B", dim: "rgba(255,59,59,0.12)", rank: 0 },
  offline:  { label: "Offline",  color: "#64748B", dim: "rgba(100,116,139,0.14)", rank: 1 },
  warning:  { label: "Warning",  color: "#FFBC00", dim: "rgba(255,188,0,0.12)", rank: 2 },
  healthy:  { label: "Healthy",  color: "#00D084", dim: "rgba(0,208,132,0.10)", rank: 3 },
};
const HEALTH_ORDER = ["critical", "offline", "warning", "healthy"];

function deriveHealth(d, now) {
  if (d.status === "error") return "critical";
  const seen = d.last_seen_at ? now - new Date(d.last_seen_at).getTime() : Infinity;
  if (d.status === "stopped" || seen > OFFLINE_MS) return "offline";
  // metric-driven escalation (only present on sample / enriched devices)
  if ((d.cpu ?? 0) >= 92 || (d.mem ?? 0) >= 95) return "critical";
  if (d.status === "inactive" || seen > STALE_MS || (d.cpu ?? 0) >= 80 || (d.mem ?? 0) >= 85) return "warning";
  return "healthy";
}

/* ── Derived facets (work for real + sample devices) ──────────────── */
function siteOf(d)  { return d.site || (d.host_ip ? `subnet ${d.host_ip.split(".").slice(0, 2).join(".")}.x` : "unknown"); }
function roleOf(d)  { return d.role || (d.name ? (d.name.match(/^[a-z]+/i)?.[0] || "node").toLowerCase() : "node"); }

function ago(ts) {
  if (!ts) return "never";
  const s = Math.floor((Date.now() - new Date(ts).getTime()) / 1000);
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

/* ── Sample fleet — preview the design at scale (clearly labelled) ── */
const SITES = ["us-east-1", "us-west-2", "eu-west-1", "ap-south-1", "on-prem-dc1"];
const ROLES = ["web", "api", "db", "cache", "worker", "k8s-node", "edge", "lb", "queue"];
const OSES  = ["Ubuntu 22.04", "Debian 12", "RHEL 9", "Amazon Linux 2", "Windows Server 2022"];

function makeSampleFleet(n) {
  const now = Date.now();
  const out = [];
  for (let i = 0; i < n; i++) {
    const role = ROLES[i % ROLES.length];
    const site = SITES[i % SITES.length];
    // Realistic skew: ~84% healthy, rest spread across warn/crit/offline.
    const roll = Math.random();
    let status = "active", cpu = 8 + Math.random() * 45, mem = 20 + Math.random() * 45, seenAgo = Math.random() * 90_000;
    if (roll > 0.985)      { status = "error";    cpu = 90 + Math.random() * 9; mem = 80 + Math.random() * 18; }
    else if (roll > 0.955) { status = "stopped";  seenAgo = OFFLINE_MS + Math.random() * 3_600_000; }
    else if (roll > 0.90)  { status = "active";   cpu = 80 + Math.random() * 12; mem = 70 + Math.random() * 20; }
    else if (roll > 0.875) { status = "inactive"; seenAgo = STALE_MS + Math.random() * 200_000; }
    out.push({
      id: `dev-${String(i + 1).padStart(4, "0")}`,
      name: `${role}-${site.split("-")[0]}-${String((i % 240) + 1).padStart(2, "0")}`,
      host_ip: `10.${(i >> 8) & 255}.${i & 255}.${(i * 7) % 254 + 1}`,
      os_type: OSES[i % OSES.length],
      version: "1.4.0",
      status,
      site, role,
      cpu: Math.round(cpu), mem: Math.round(mem),
      last_seen_at: new Date(now - seenAgo).toISOString(),
    });
  }
  return out;
}

/* ── Small UI atoms ───────────────────────────────────────────────── */
function Donut({ counts, total }) {
  const segs = HEALTH_ORDER.map((k, i) => {
    const before = HEALTH_ORDER.slice(0, i).reduce((s, kk) => s + (counts[kk] || 0), 0);
    const start = (before / (total || 1)) * 360;
    const end = ((before + (counts[k] || 0)) / (total || 1)) * 360;
    return `${HEALTH[k].color} ${start}deg ${end}deg`;
  });
  const healthyPct = total ? Math.round(((counts.healthy || 0) / total) * 100) : 0;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
      <div style={{ position: "relative", width: 96, height: 96, flexShrink: 0 }}>
        <div style={{ width: 96, height: 96, borderRadius: "50%", background: `conic-gradient(${segs.join(",")})` }} />
        <div style={{
          position: "absolute", inset: 0, margin: "auto", width: 62, height: 62, borderRadius: "50%",
          background: "var(--surface-1)", display: "flex", flexDirection: "column",
          alignItems: "center", justifyContent: "center",
        }}>
          <div style={{ fontSize: 20, fontWeight: 800, color: "var(--t1)", lineHeight: 1 }}>{healthyPct}%</div>
          <div style={{ fontSize: 8, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.06em", marginTop: 2 }}>healthy</div>
        </div>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 7, flex: 1 }}>
        {HEALTH_ORDER.map((k) => (
          <div key={k} style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <span style={{ width: 8, height: 8, borderRadius: 2, background: HEALTH[k].color, flexShrink: 0 }} />
            <span style={{ fontSize: 11, color: "var(--t3)", flex: 1 }}>{HEALTH[k].label}</span>
            <span style={{ fontSize: 12, fontWeight: 700, color: "var(--t1)" }}>{(counts[k] || 0).toLocaleString()}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function Kpi({ label, value, color, icon, active, onClick }) {
  const Icon = icon;
  return (
    <button
      onClick={onClick}
      style={{
        flex: 1, minWidth: 0, textAlign: "left", cursor: onClick ? "pointer" : "default",
        background: active ? color.dim : "var(--surface-1)",
        border: `1px solid ${active ? color.color + "66" : "var(--border)"}`,
        borderRadius: 12, padding: "12px 14px", position: "relative", overflow: "hidden",
        transition: "border-color .14s, background .14s",
      }}
    >
      <div style={{ position: "absolute", left: 0, top: 0, bottom: 0, width: 3, background: color.color }} />
      <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 8 }}>
        <Icon size={12} color={color.color} />
        <span style={{ fontSize: 10, fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.07em" }}>{label}</span>
      </div>
      <div style={{ fontSize: 24, fontWeight: 800, letterSpacing: "-0.03em", color: color.solid || "var(--t1)", lineHeight: 1 }}>{value}</div>
    </button>
  );
}

/* ── Detail drawer ────────────────────────────────────────────────── */
function DeviceDrawer({ device, onClose }) {
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
    ["CPU", device.cpu, Cpu, "#0066FF"],
    ["Memory", device.mem, HardDrive, "#00C8E8"],
  ].filter(([, v]) => v != null);

  return (
    <>
      <div onClick={onClose} style={{ position: "fixed", inset: 0, background: "rgba(2,6,14,0.55)", zIndex: 40, animation: "fadeIn .15s ease" }} />
      <aside style={{
        position: "fixed", top: 0, right: 0, bottom: 0, width: 380, maxWidth: "92vw", zIndex: 41,
        background: "var(--surface-1)", borderLeft: "1px solid var(--border-strong)",
        boxShadow: "var(--shadow-lg)", display: "flex", flexDirection: "column",
        animation: "fadeIn .18s ease",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "18px 20px", borderBottom: "1px solid var(--border)" }}>
          <div style={{ width: 38, height: 38, borderRadius: 10, background: h.dim, border: `1px solid ${h.color}40`, display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0 }}>
            <Server size={18} color={h.color} />
          </div>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ fontSize: 15, fontWeight: 700, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{device.name || device.id}</div>
            <div style={{ fontSize: 11, fontWeight: 700, color: h.color, marginTop: 2 }}>● {h.label}</div>
          </div>
          <button className="icon-btn" onClick={onClose}><X size={16} /></button>
        </div>

        <div style={{ padding: 20, overflowY: "auto" }}>
          {meters.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 12, marginBottom: 18 }}>
              {meters.map(([label, pct, ic, color]) => {
                const Icon = ic;
                const c = pct >= 90 ? "#FF3B3B" : pct >= 75 ? "#FFBC00" : color;
                return (
                  <div key={label}>
                    <div style={{ display: "flex", justifyContent: "space-between", marginBottom: 4 }}>
                      <span style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 11, color: "var(--t3)" }}><Icon size={12} color={color} /> {label}</span>
                      <span style={{ fontSize: 12, fontWeight: 700, color: c }}>{pct}%</span>
                    </div>
                    <div style={{ height: 6, background: "var(--surface-3)", borderRadius: 99, overflow: "hidden" }}>
                      <div style={{ width: `${pct}%`, height: "100%", background: c, borderRadius: 99, transition: "width .4s" }} />
                    </div>
                  </div>
                );
              })}
            </div>
          )}
          <div className="lux-eyebrow" style={{ marginBottom: 8 }}>System details</div>
          {rows.map(([k, v]) => (
            <div key={k} style={{ display: "flex", justifyContent: "space-between", gap: 12, padding: "8px 0", borderBottom: "1px solid var(--border)", fontSize: 12 }}>
              <span style={{ color: "var(--t3)" }}>{k}</span>
              <span style={{ color: "var(--t1)", fontFamily: "JetBrains Mono, monospace", fontSize: 11, textAlign: "right", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{v}</span>
            </div>
          ))}
        </div>
      </aside>
    </>
  );
}

/* ── Density presets for the matrix ───────────────────────────────── */
const DENSITY = {
  compact: { cell: 13, gap: 3, label: false },
  normal:  { cell: 20, gap: 4, label: false },
  large:   { cell: 34, gap: 5, label: true },
};

/* ── Main view ────────────────────────────────────────────────────── */
export default function FleetView() {
  const [source, setSource]   = useState("live");   // live | 500 | 2000
  const [live, setLive]       = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(null);

  const [mode, setMode]       = useState("matrix");  // matrix | table | groups
  const [density, setDensity] = useState("normal");
  const [groupBy, setGroupBy] = useState("site");
  const [sortBy, setSortBy]   = useState("health");
  const [q, setQ]             = useState("");
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
    } catch { setLive([]); }
    finally { setLoading(false); setRefreshing(false); }
  }, []);

  useEffect(() => {
    loadLive();
    const t = setInterval(loadLive, 30_000);
    return () => clearInterval(t);
  }, [loadLive]);

  // Sample fleets are memoised so they stay stable across re-renders.
  const sample500  = useMemo(() => makeSampleFleet(500), []);
  const sample2000 = useMemo(() => makeSampleFleet(2000), []);

  const rawDevices = source === "500" ? sample500 : source === "2000" ? sample2000 : live;
  const isSample = source !== "live";

  // Attach derived health once.
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
    let list = devices.filter((d) => {
      if (healthFilter && d._health !== healthFilter) return false;
      if (needle) {
        const hay = `${d.name || ""} ${d.host_ip || ""} ${d.os_type || ""} ${roleOf(d)} ${siteOf(d)}`.toLowerCase();
        if (!hay.includes(needle)) return false;
      }
      return true;
    });
    const cmp = {
      health: (a, b) => HEALTH[a._health].rank - HEALTH[b._health].rank || (a.name || "").localeCompare(b.name || ""),
      name:   (a, b) => (a.name || "").localeCompare(b.name || ""),
      cpu:    (a, b) => (b.cpu ?? -1) - (a.cpu ?? -1),
      seen:   (a, b) => new Date(b.last_seen_at || 0) - new Date(a.last_seen_at || 0),
    };
    return [...list].sort(cmp[sortBy] || cmp.health);
  }, [devices, q, healthFilter, sortBy]);

  const groups = useMemo(() => {
    if (mode !== "groups") return [];
    const keyer = { site: siteOf, os: (d) => d.os_type || "Unknown", role: roleOf, status: (d) => HEALTH[d._health].label };
    const fn = keyer[groupBy] || siteOf;
    const map = new Map();
    for (const d of filtered) {
      const k = fn(d);
      if (!map.has(k)) map.set(k, { key: k, total: 0, critical: 0, offline: 0, warning: 0, healthy: 0, devices: [] });
      const g = map.get(k);
      g.total++; g[d._health]++; g.devices.push(d);
    }
    return [...map.values()].sort((a, b) => (b.critical + b.offline) - (a.critical + a.offline) || b.total - a.total);
  }, [filtered, mode, groupBy]);

  const dz = DENSITY[density];

  function trackTip(e) { tipRef.current = { x: e.clientX, y: e.clientY }; }

  return (
    <div style={{ padding: "20px 24px", animation: "fadeIn .22s ease" }} onMouseMove={hover ? trackTip : undefined}>

      {/* ── Header ─────────────────────────────────────────────────── */}
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 16, marginBottom: 18, flexWrap: "wrap" }}>
        <div>
          <div className="lux-eyebrow" style={{ marginBottom: 4 }}>Infrastructure</div>
          <h1 style={{ fontSize: 24, fontWeight: 800, letterSpacing: "-0.03em", color: "var(--t1)", margin: 0, lineHeight: 1.1 }}>Fleet Monitoring</h1>
          <div style={{ fontSize: 12, color: "var(--t4)", marginTop: 5 }}>
            {devices.length.toLocaleString()} devices · {isSample ? "sample preview" : `updated ${lastRefresh ? lastRefresh.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }) : "—"} · auto-refresh 30s`}
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          {/* Source switch */}
          <div style={{ display: "flex", background: "var(--surface-1)", border: "1px solid var(--border)", borderRadius: 9, padding: 2 }}>
            {[["live", "Live"], ["500", "Sample 500"], ["2000", "Sample 2K"]].map(([k, lbl]) => (
              <button key={k} onClick={() => { setSource(k); setSelected(null); }} style={{
                padding: "5px 11px", fontSize: 11, fontWeight: 600, borderRadius: 7, border: "none", cursor: "pointer",
                background: source === k ? "var(--blue)" : "transparent",
                color: source === k ? "#fff" : "var(--t3)", transition: "all .14s",
              }}>{lbl}</button>
            ))}
          </div>
          <button className="btn btn-ghost btn-sm" onClick={loadLive} disabled={refreshing} style={{ gap: 6 }}>
            <RefreshCw size={12} style={{ animation: refreshing ? "rotateSpin .8s linear infinite" : "none" }} /> Refresh
          </button>
        </div>
      </div>

      {isSample && (
        <div style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 14px", marginBottom: 14, borderRadius: 10, background: "var(--amber-dim)", border: "1px solid rgba(255,188,0,0.25)", fontSize: 12, color: "#ffd95c" }}>
          <Zap size={13} /> <strong>Sample fleet</strong> — synthetic devices for previewing the design at scale. Switch to <em>Live</em> for your connected agents.
        </div>
      )}

      {/* ── KPI band ───────────────────────────────────────────────── */}
      <div style={{ display: "flex", gap: 10, marginBottom: 16, flexWrap: "wrap" }}>
        <Kpi label="Total Devices" value={devices.length.toLocaleString()} icon={Server} color={{ color: "#0066FF", dim: "var(--blue-dim)", solid: "var(--t1)" }} active={!healthFilter} onClick={() => setHealthFilter(null)} />
        <Kpi label="Healthy"  value={counts.healthy.toLocaleString()}  icon={Wifi}     color={{ color: HEALTH.healthy.color,  dim: HEALTH.healthy.dim,  solid: HEALTH.healthy.color }}  active={healthFilter === "healthy"}  onClick={() => setHealthFilter((p) => p === "healthy" ? null : "healthy")} />
        <Kpi label="Warning"  value={counts.warning.toLocaleString()}  icon={Activity} color={{ color: HEALTH.warning.color,  dim: HEALTH.warning.dim,  solid: HEALTH.warning.color }}  active={healthFilter === "warning"}  onClick={() => setHealthFilter((p) => p === "warning" ? null : "warning")} />
        <Kpi label="Critical" value={counts.critical.toLocaleString()} icon={Zap}      color={{ color: HEALTH.critical.color, dim: HEALTH.critical.dim, solid: HEALTH.critical.color }} active={healthFilter === "critical"} onClick={() => setHealthFilter((p) => p === "critical" ? null : "critical")} />
        <Kpi label="Offline"  value={counts.offline.toLocaleString()}  icon={WifiOff}  color={{ color: HEALTH.offline.color,  dim: HEALTH.offline.dim,  solid: HEALTH.offline.color }}  active={healthFilter === "offline"}  onClick={() => setHealthFilter((p) => p === "offline" ? null : "offline")} />
      </div>

      {/* ── Body: rail + workspace ─────────────────────────────────── */}
      <div style={{ display: "grid", gridTemplateColumns: "260px 1fr", gap: 16, alignItems: "start" }}>

        {/* Left rail */}
        <div style={{ display: "flex", flexDirection: "column", gap: 14, position: "sticky", top: 12 }}>
          <div className="card" style={{ padding: 18 }}>
            <div className="lux-eyebrow" style={{ marginBottom: 14 }}>Fleet Health</div>
            <Donut counts={counts} total={devices.length} />
          </div>

          <div className="card" style={{ padding: 16 }}>
            <div className="lux-eyebrow" style={{ marginBottom: 10 }}>View</div>
            <div style={{ display: "flex", gap: 4, marginBottom: 14 }}>
              {[["matrix", "Matrix", Grid3x3], ["table", "Table", Table2], ["groups", "Groups", Layers]].map(([k, lbl, ic]) => {
                const Icon = ic;
                return (
                  <button key={k} onClick={() => setMode(k)} style={{
                    flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 4, padding: "9px 4px",
                    borderRadius: 8, cursor: "pointer", fontSize: 10, fontWeight: 600,
                    background: mode === k ? "var(--blue-dim)" : "var(--surface-2)",
                    border: `1px solid ${mode === k ? "rgba(0,102,255,0.35)" : "var(--border)"}`,
                    color: mode === k ? "var(--blue-lt)" : "var(--t3)", transition: "all .14s",
                  }}><Icon size={15} /> {lbl}</button>
                );
              })}
            </div>

            {mode === "matrix" && (
              <>
                <div style={{ fontSize: 10, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 6 }}>Density</div>
                <div style={{ display: "flex", gap: 4, marginBottom: 12 }}>
                  {["compact", "normal", "large"].map((k) => (
                    <button key={k} onClick={() => setDensity(k)} style={{
                      flex: 1, padding: "5px 0", fontSize: 10, fontWeight: 600, borderRadius: 6, cursor: "pointer", textTransform: "capitalize",
                      background: density === k ? "var(--surface-3)" : "transparent",
                      border: `1px solid ${density === k ? "var(--border-strong)" : "var(--border)"}`,
                      color: density === k ? "var(--t1)" : "var(--t3)",
                    }}>{k}</button>
                  ))}
                </div>
              </>
            )}

            {mode === "groups" && (
              <>
                <div style={{ fontSize: 10, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 6 }}>Group by</div>
                <select className="form-select" value={groupBy} onChange={(e) => setGroupBy(e.target.value)} style={{ padding: "6px 10px", fontSize: 12, marginBottom: 12 }}>
                  <option value="site">Site / segment</option>
                  <option value="os">Operating system</option>
                  <option value="role">Role</option>
                  <option value="status">Health status</option>
                </select>
              </>
            )}

            <div style={{ fontSize: 10, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.06em", marginBottom: 6 }}>Sort by</div>
            <select className="form-select" value={sortBy} onChange={(e) => setSortBy(e.target.value)} style={{ padding: "6px 10px", fontSize: 12 }}>
              <option value="health">Worst first</option>
              <option value="name">Name</option>
              <option value="cpu">CPU load</option>
              <option value="seen">Last seen</option>
            </select>
          </div>

          <div className="card" style={{ padding: 16 }}>
            <div className="lux-eyebrow" style={{ marginBottom: 10 }}>Legend</div>
            {HEALTH_ORDER.map((k) => (
              <div key={k} style={{ display: "flex", alignItems: "center", gap: 8, padding: "3px 0", fontSize: 11, color: "var(--t3)" }}>
                <span style={{ width: 12, height: 12, borderRadius: 3, background: HEALTH[k].color, flexShrink: 0 }} />
                {HEALTH[k].label}
              </div>
            ))}
          </div>
        </div>

        {/* Workspace */}
        <div className="card" style={{ padding: 0, minHeight: 480 }}>
          {/* Filter bar */}
          <div style={{ display: "flex", alignItems: "center", gap: 10, padding: "12px 16px", borderBottom: "1px solid var(--border)", flexWrap: "wrap" }}>
            <div style={{ position: "relative", flex: 1, minWidth: 200 }}>
              <Search size={13} style={{ position: "absolute", left: 11, top: "50%", transform: "translateY(-50%)", color: "var(--t3)", pointerEvents: "none" }} />
              <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search by name, IP, OS, role, site…"
                style={{ width: "100%", height: 32, background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 8, padding: "0 12px 0 32px", fontSize: 12, color: "var(--t1)", outline: "none" }} />
            </div>
            <span style={{ fontSize: 12, color: "var(--t3)" }}>
              <strong style={{ color: "var(--t1)" }}>{filtered.length.toLocaleString()}</strong> shown
              {healthFilter && <> · <button onClick={() => setHealthFilter(null)} style={{ background: "none", border: "none", color: "var(--blue-lt)", cursor: "pointer", fontSize: 12, fontWeight: 600 }}>clear filter</button></>}
            </span>
          </div>

          {loading && source === "live" ? (
            <div style={{ padding: 40, display: "grid", gridTemplateColumns: "repeat(auto-fill, 20px)", gap: 4 }}>
              {Array.from({ length: 120 }).map((_, i) => <div key={i} className="skeleton" style={{ width: 20, height: 20, borderRadius: 4 }} />)}
            </div>
          ) : filtered.length === 0 ? (
            <div className="empty-state" style={{ padding: "60px 24px" }}>
              <div className="empty-icon"><Server size={22} /></div>
              <div className="empty-title">{source === "live" ? "No agents connected" : "No devices match"}</div>
              <div className="empty-desc">
                {source === "live"
                  ? "Install the NeuroOps agent on your hosts, or preview the design with a sample fleet using the switch above."
                  : "Adjust your search or health filter to see devices."}
              </div>
            </div>
          ) : mode === "matrix" ? (
            <MatrixView devices={filtered} dz={dz} onSelect={setSelected} onHover={setHover} />
          ) : mode === "table" ? (
            <TableView devices={filtered} onSelect={setSelected} />
          ) : (
            <GroupsView groups={groups} onSelect={setSelected} />
          )}
        </div>
      </div>

      {/* Floating tooltip for matrix hover */}
      {hover && (
        <div style={{
          position: "fixed", left: Math.min(tipRef.current.x + 14, window.innerWidth - 230),
          top: tipRef.current.y + 14, zIndex: 50, pointerEvents: "none",
          background: "var(--surface-3)", border: `1px solid ${HEALTH[hover._health].color}55`,
          borderRadius: 8, padding: "8px 11px", boxShadow: "var(--shadow-md)", width: 210,
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 4 }}>
            <span style={{ width: 8, height: 8, borderRadius: 2, background: HEALTH[hover._health].color }} />
            <span style={{ fontSize: 12, fontWeight: 700, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{hover.name || hover.id}</span>
          </div>
          <div style={{ fontSize: 11, color: "var(--t3)", fontFamily: "JetBrains Mono, monospace" }}>{hover.host_ip || "—"}</div>
          <div style={{ fontSize: 11, color: "var(--t3)", marginTop: 3 }}>{hover.os_type} · {HEALTH[hover._health].label} · {ago(hover.last_seen_at)} ago</div>
          {hover.cpu != null && <div style={{ fontSize: 11, color: "var(--t2)", marginTop: 3 }}>CPU {hover.cpu}% · MEM {hover.mem}%</div>}
        </div>
      )}

      <DeviceDrawer device={selected} onClose={() => setSelected(null)} />
    </div>
  );
}

/* ── Matrix (status wall) ─────────────────────────────────────────── */
function MatrixView({ devices, dz, onSelect, onHover }) {
  return (
    <div style={{ padding: 16, overflow: "auto", maxHeight: "calc(100vh - 280px)" }}
      onMouseLeave={() => onHover(null)}>
      <div style={{ display: "grid", gridTemplateColumns: `repeat(auto-fill, ${dz.cell}px)`, gap: dz.gap, justifyContent: "start" }}>
        {devices.map((d) => {
          const h = HEALTH[d._health];
          return (
            <button
              key={d.id}
              onClick={() => onSelect(d)}
              onMouseEnter={() => onHover(d)}
              title={dz.label ? undefined : `${d.name || d.id} · ${h.label}`}
              style={{
                width: dz.cell, height: dz.cell, borderRadius: dz.cell > 24 ? 6 : 3,
                background: h.color, border: "none", cursor: "pointer", padding: 0,
                opacity: d._health === "healthy" ? 0.82 : 1,
                boxShadow: d._health === "critical" ? `0 0 0 1px ${h.color}, 0 0 6px ${h.color}aa` : "none",
                display: "flex", alignItems: "center", justifyContent: "center",
                fontSize: 8, fontWeight: 700, color: "rgba(0,0,0,0.55)", overflow: "hidden",
                transition: "transform .1s",
              }}
              onMouseOver={(e) => (e.currentTarget.style.transform = "scale(1.18)")}
              onMouseOut={(e) => (e.currentTarget.style.transform = "scale(1)")}
            >
              {dz.label ? (d.name || "").replace(/^[a-z]+-/i, "").slice(0, 5) : null}
            </button>
          );
        })}
      </div>
    </div>
  );
}

/* ── Table ────────────────────────────────────────────────────────── */
function TableView({ devices, onSelect }) {
  // Cap rendered rows for very large fleets; matrix is the dense lens.
  const rows = devices.slice(0, 600);
  return (
    <div style={{ overflow: "auto", maxHeight: "calc(100vh - 280px)" }}>
      <table className="data-table">
        <thead style={{ position: "sticky", top: 0, background: "var(--surface-1)", zIndex: 1 }}>
          <tr>
            <th>Device</th><th>Status</th><th>IP</th><th>OS</th><th>Role</th><th>CPU</th><th>Mem</th><th>Last seen</th><th></th>
          </tr>
        </thead>
        <tbody>
          {rows.map((d) => {
            const h = HEALTH[d._health];
            return (
              <tr key={d.id} onClick={() => onSelect(d)} style={{ cursor: "pointer" }}>
                <td style={{ color: "var(--t1)", fontWeight: 600 }}>{d.name || d.id}</td>
                <td><span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 11, fontWeight: 700, color: h.color }}><span style={{ width: 7, height: 7, borderRadius: "50%", background: h.color }} /> {h.label}</span></td>
                <td style={{ fontFamily: "JetBrains Mono, monospace", fontSize: 11 }}>{d.host_ip || "—"}</td>
                <td>{d.os_type || "—"}</td>
                <td>{roleOf(d)}</td>
                <td style={{ color: d.cpu >= 85 ? "var(--red)" : d.cpu >= 70 ? "var(--amber)" : "var(--t2)", fontWeight: 600 }}>{d.cpu != null ? `${d.cpu}%` : "—"}</td>
                <td style={{ color: d.mem >= 90 ? "var(--red)" : d.mem >= 75 ? "var(--amber)" : "var(--t2)", fontWeight: 600 }}>{d.mem != null ? `${d.mem}%` : "—"}</td>
                <td style={{ fontSize: 11 }}>{ago(d.last_seen_at)} ago</td>
                <td><ChevronRight size={13} color="var(--t4)" /></td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {devices.length > rows.length && (
        <div style={{ padding: "12px 16px", fontSize: 12, color: "var(--t3)", textAlign: "center" }}>
          Showing first {rows.length.toLocaleString()} of {devices.length.toLocaleString()} — refine your search or use the Matrix lens for the full fleet.
        </div>
      )}
    </div>
  );
}

/* ── Groups (rollups) ─────────────────────────────────────────────── */
function GroupsView({ groups, onSelect }) {
  const [open, setOpen] = useState(() => new Set());
  return (
    <div style={{ overflow: "auto", maxHeight: "calc(100vh - 280px)", padding: 14 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        {groups.map((g) => {
          const isOpen = open.has(g.key);
          const healthyPct = Math.round((g.healthy / g.total) * 100);
          return (
            <div key={g.key} style={{ background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 12, overflow: "hidden" }}>
              <button onClick={() => setOpen((s) => { const n = new Set(s); n.has(g.key) ? n.delete(g.key) : n.add(g.key); return n; })}
                style={{ width: "100%", display: "flex", alignItems: "center", gap: 12, padding: "12px 16px", background: "none", border: "none", cursor: "pointer", textAlign: "left" }}>
                <ChevronRight size={14} color="var(--t3)" style={{ transform: isOpen ? "rotate(90deg)" : "none", transition: "transform .15s", flexShrink: 0 }} />
                <div style={{ minWidth: 160 }}>
                  <div style={{ fontSize: 13, fontWeight: 700, color: "var(--t1)" }}>{g.key}</div>
                  <div style={{ fontSize: 11, color: "var(--t3)" }}>{g.total.toLocaleString()} devices · {healthyPct}% healthy</div>
                </div>
                {/* stacked health bar */}
                <div style={{ flex: 1, display: "flex", height: 10, borderRadius: 99, overflow: "hidden", background: "var(--surface-3)", minWidth: 120 }}>
                  {HEALTH_ORDER.map((k) => g[k] > 0 && (
                    <div key={k} title={`${HEALTH[k].label}: ${g[k]}`} style={{ width: `${(g[k] / g.total) * 100}%`, background: HEALTH[k].color }} />
                  ))}
                </div>
                <div style={{ display: "flex", gap: 10, flexShrink: 0 }}>
                  {g.critical > 0 && <span style={{ fontSize: 12, fontWeight: 700, color: HEALTH.critical.color }}>{g.critical} crit</span>}
                  {g.offline > 0 && <span style={{ fontSize: 12, fontWeight: 700, color: HEALTH.offline.color }}>{g.offline} off</span>}
                </div>
              </button>
              {isOpen && (
                <div style={{ borderTop: "1px solid var(--border)", padding: 12, display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))", gap: 6 }}>
                  {g.devices.slice(0, 120).map((d) => {
                    const h = HEALTH[d._health];
                    return (
                      <button key={d.id} onClick={() => onSelect(d)} style={{
                        display: "flex", alignItems: "center", gap: 8, padding: "7px 10px", borderRadius: 8,
                        background: "var(--surface-1)", border: "1px solid var(--border)", cursor: "pointer", textAlign: "left",
                      }}>
                        <span style={{ width: 8, height: 8, borderRadius: "50%", background: h.color, flexShrink: 0 }} />
                        <span style={{ flex: 1, minWidth: 0, fontSize: 12, color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{d.name || d.id}</span>
                        {d.cpu != null && <span style={{ fontSize: 10, color: "var(--t3)" }}>{d.cpu}%</span>}
                      </button>
                    );
                  })}
                  {g.devices.length > 120 && <div style={{ gridColumn: "1/-1", fontSize: 11, color: "var(--t3)", padding: "4px 8px" }}>+{(g.devices.length - 120).toLocaleString()} more…</div>}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
