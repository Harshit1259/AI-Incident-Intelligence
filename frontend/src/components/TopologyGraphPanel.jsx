// TopologyGraphPanel.jsx — Real topology & dependency intelligence
// Multi-layer graph (service/infra/deployment/team) + causal RCA panel with
// per-evidence confidence scores and blast-radius visualization.

import { useEffect, useState, useCallback, useMemo } from "react";
import { Page, PageHeader } from "./ui/Primitives.jsx";
import {
  getTopologyGraph,
  getLiveTopologyGraph,
  getCausalRCA,
  getBlastRadius,
  discoverTopology,
} from "../api/phase3.js";

// ─── Config ───────────────────────────────────────────────────────────────────

const NODE_CFG = {
  service:          { label: "Service",      color: "var(--cat-blue)",   bg: "var(--cat-blue-dim)",   border: "var(--cat-blue-line)"   },
  dependency:       { label: "Dependency",   color: "var(--cat-slate)",  bg: "var(--cat-slate-dim)",  border: "var(--cat-slate-line)"  },
  impacted_service: { label: "Impacted",     color: "var(--cat-amber)",  bg: "var(--cat-amber-dim)",  border: "var(--cat-amber-line)"  },
  alert_origin:     { label: "Alert source", color: "var(--cat-red)",    bg: "var(--cat-red-dim)",    border: "var(--cat-red-line)"    },
  change:           { label: "Change event", color: "var(--cat-violet)", bg: "var(--cat-violet-dim)", border: "var(--cat-violet-line)" },
  evidence:         { label: "Evidence",     color: "var(--cat-green)",  bg: "var(--cat-green-dim)",  border: "var(--cat-green-line)"  },
  infra:            { label: "Infra / host", color: "var(--cat-orange)", bg: "var(--cat-orange-dim)", border: "var(--cat-orange-line)" },
  database:         { label: "Database",     color: "var(--cat-purple)", bg: "var(--cat-purple-dim)", border: "var(--cat-purple-line)" },
  queue:            { label: "Queue",        color: "var(--cat-lime)",   bg: "var(--cat-lime-dim)",   border: "var(--cat-lime-line)"   },
  cache:            { label: "Cache",        color: "var(--cat-cyan)",   bg: "var(--cat-cyan-dim)",   border: "var(--cat-cyan-line)"   },
  external:         { label: "External",     color: "var(--cat-slate)",  bg: "var(--cat-slate-dim)",  border: "var(--cat-slate-line)"  },
  team:             { label: "Team / owner", color: "var(--cat-pink)",   bg: "var(--cat-pink-dim)",   border: "var(--cat-pink-line)"   },
  deployment:       { label: "Deployment",   color: "var(--cat-lime)",   bg: "var(--cat-lime-dim)",   border: "var(--cat-lime-line)"   },
  owner:            { label: "Owner",        color: "var(--cat-pink)",   bg: "var(--cat-pink-dim)",   border: "var(--cat-pink-line)"   },
};

const CONF_COLOR = (c) => (c >= 70 ? "var(--green)" : c >= 40 ? "var(--amber)" : "var(--red)");
const TIER_COLOR = (t) =>
  ({ critical: "var(--red)", internal: "var(--t3)", external: "var(--amber)", infra: "var(--cat-orange)" })[t] ||
  "var(--t3)";

// ─── Sub-components ───────────────────────────────────────────────────────────

function NodeCard({ node }) {
  const cfg = NODE_CFG[node.node_type] || NODE_CFG.dependency;
  return (
    <div style={{ padding: "10px 12px", borderRadius: 10, background: cfg.bg, border: `1px solid ${cfg.border}`, minWidth: 150, maxWidth: 210 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 5, marginBottom: 4 }}>
        <div style={{ width: 7, height: 7, borderRadius: "50%", background: cfg.color, flexShrink: 0 }} />
        <span style={{ fontSize: "0.63rem", color: cfg.color, textTransform: "uppercase", letterSpacing: "0.08em", fontWeight: 700 }}>
          {cfg.label}
        </span>
      </div>
      <div style={{ fontWeight: 700, fontSize: "0.84rem", color: "var(--t1)", marginBottom: 5, wordBreak: "break-all" }}>
        {node.display_name || node.label || node.name || node.id}
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 3 }}>
        {node.tier && node.tier !== "internal" && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "rgba(0,0,0,0.25)", color: TIER_COLOR(node.tier) }}>
            {node.tier}
          </span>
        )}
        {node.owner_team && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "rgba(255,255,255,0.05)", color: "var(--t2)" }}>
            {node.owner_team}
          </span>
        )}
        {node.is_customer_facing && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "rgba(239,68,68,0.15)", color: "var(--red)" }}>
            customer-facing
          </span>
        )}
        {node.severity && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "rgba(255,255,255,0.05)", color: "var(--t2)" }}>
            {node.severity}
          </span>
        )}
        {node.health?.status && node.health.status !== "unknown" && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5,
            background: node.health.status === "healthy" ? "rgba(52,211,153,0.12)" : "rgba(248,113,113,0.12)",
            color: node.health.status === "healthy" ? "var(--green)" : "var(--red)" }}>
            {node.health.status}
          </span>
        )}
        {node.alert_count > 0 && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "rgba(248,113,113,0.14)", color: "var(--red)" }}>
            {node.alert_count} alerts
          </span>
        )}
        {node.evidence_count > 0 && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "var(--green-dim)", color: "var(--green)" }}>
            {node.evidence_count} ev
          </span>
        )}
        {node.change_linked && (
          <span style={{ fontSize: "0.6rem", padding: "1px 6px", borderRadius: 5, background: "var(--cat-violet-dim)", color: "var(--cat-violet)" }}>
            change
          </span>
        )}
      </div>
    </div>
  );
}

function EdgeRow({ edge }) {
  const conf = Math.min(100, Math.max(0, edge.confidence || 0));
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", padding: "8px 12px", borderRadius: 8, background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)" }}>
      <code style={{ fontSize: "0.74rem", color: "var(--cyan)", background: "rgba(5,13,29,0.6)", padding: "2px 6px", borderRadius: 5, wordBreak: "break-all" }}>
        {edge.from_node_name || edge.from_node_id || edge.from}
      </code>
      <span style={{ color: "var(--cyan)", fontWeight: 800 }}>→</span>
      <code style={{ fontSize: "0.74rem", color: "var(--cyan)", background: "rgba(5,13,29,0.6)", padding: "2px 6px", borderRadius: 5, wordBreak: "break-all" }}>
        {edge.to_node_name || edge.to_node_id || edge.to}
      </code>
      <span style={{ fontSize: "0.67rem", color: "var(--t3)", padding: "1px 6px", borderRadius: 5, background: "rgba(255,255,255,0.04)", border: "1px solid var(--border)" }}>
        {edge.relation}
      </span>
      <div style={{ display: "flex", alignItems: "center", gap: 5, marginLeft: "auto" }}>
        <div style={{ width: 50, height: 3, borderRadius: 2, background: "rgba(255,255,255,0.07)" }}>
          <div style={{ width: `${conf}%`, height: "100%", borderRadius: 2, background: CONF_COLOR(conf) }} />
        </div>
        <span style={{ fontSize: "0.64rem", color: "var(--t2)", minWidth: 26 }}>{conf}%</span>
      </div>
    </div>
  );
}

function DegradationChainView({ chain }) {
  if (!chain || chain.length === 0) return null;
  return (
    <div style={{ marginBottom: 20 }}>
      <div className="lux-eyebrow" style={{ marginBottom: 10 }}>DEGRADATION CHAIN — which fired first</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {chain.map((d, i) => (
          <div key={i} style={{ display: "flex", gap: 10, alignItems: "flex-start", padding: "10px 14px", borderRadius: 10, background: d.is_origin ? "rgba(248,113,113,0.07)" : "rgba(255,255,255,0.03)", border: `1px solid ${d.is_origin ? "rgba(248,113,113,0.3)" : "var(--border)"}` }}>
            <div style={{ flexShrink: 0, marginTop: 2 }}>
              <div style={{ width: 20, height: 20, borderRadius: "50%", background: d.is_origin ? "var(--red)" : "var(--border-strong)", display: "flex", alignItems: "center", justifyContent: "center", fontSize: "0.65rem", color: "#fff", fontWeight: 700 }}>
                {i + 1}
              </div>
            </div>
            <div style={{ flex: 1 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <strong style={{ fontSize: "0.85rem", color: "var(--t1)" }}>{d.service_name}</strong>
                {d.is_origin && <span style={{ fontSize: "0.62rem", padding: "1px 7px", borderRadius: 5, background: "rgba(239,68,68,0.2)", color: "var(--red)", fontWeight: 700 }}>ORIGIN — degraded first</span>}
                {d.severity && <span style={{ fontSize: "0.62rem", color: "var(--t2)" }}>{d.severity}</span>}
              </div>
              {d.description && <div style={{ fontSize: "0.75rem", color: "var(--t3)", marginTop: 3 }}>{d.description}</div>}
              {d.timestamp && (
                <div style={{ fontSize: "0.67rem", color: "var(--t4)", marginTop: 2 }}>
                  {new Date(d.timestamp).toLocaleTimeString()}
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function EvidenceNodesView({ nodes: evNodes }) {
  if (!evNodes || evNodes.length === 0) return null;
  return (
    <div style={{ marginBottom: 20 }}>
      <div className="lux-eyebrow" style={{ marginBottom: 10 }}>EVIDENCE — confidence per signal</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
        {evNodes.map((ev, i) => (
          <div key={i} style={{ display: "flex", gap: 10, alignItems: "center", padding: "8px 12px", borderRadius: 8, background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)" }}>
            <div style={{ flex: 1 }}>
              <div style={{ fontSize: "0.82rem", color: "var(--t1)", fontWeight: 600 }}>{ev.label}</div>
              {ev.detail && <div style={{ fontSize: "0.72rem", color: "var(--t3)", marginTop: 2 }}>{ev.detail}</div>}
              {ev.supports_conclusion && <div style={{ fontSize: "0.68rem", color: "var(--t4)", marginTop: 2, fontStyle: "italic" }}>{ev.supports_conclusion}</div>}
            </div>
            <div style={{ flexShrink: 0, textAlign: "right" }}>
              <div style={{ fontSize: "1rem", fontWeight: 800, color: CONF_COLOR(ev.confidence) }}>{ev.confidence}%</div>
              <div style={{ fontSize: "0.62rem", color: "var(--t3)" }}>confidence</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}

function ImpactListView({ title, items }) {
  if (!items || items.length === 0) return null;
  return (
    <div style={{ marginBottom: 20 }}>
      <div className="lux-eyebrow" style={{ marginBottom: 8 }}>{title}</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        {items.map((imp, i) => (
          <div key={i} style={{ display: "flex", alignItems: "center", gap: 8, padding: "7px 12px", borderRadius: 8, background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)" }}>
            <span style={{ flex: 1, fontSize: "0.82rem", color: "var(--t1)" }}>{imp.service_name}</span>
            {imp.is_customer_facing && <span style={{ fontSize: "0.58rem", padding: "1px 5px", borderRadius: 4, background: "rgba(239,68,68,0.15)", color: "var(--red)" }}>customer-facing</span>}
            {imp.tier && <span style={{ fontSize: "0.58rem", padding: "1px 5px", borderRadius: 4, background: "rgba(255,255,255,0.04)", color: TIER_COLOR(imp.tier) }}>{imp.tier}</span>}
            <span style={{ fontSize: "0.68rem", color: "var(--t3)" }}>depth {imp.depth}</span>
            <span style={{ fontSize: "0.76rem", fontWeight: 700, color: CONF_COLOR(imp.confidence), minWidth: 32, textAlign: "right" }}>{imp.confidence}%</span>
          </div>
        ))}
      </div>
    </div>
  );
}

function RCAPanel({ incidentId }) {
  const [rca, setRca] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!incidentId) return;
    setLoading(true);
    setError("");
    try {
      const d = await getCausalRCA(incidentId);
      setRca(d);
    } catch (err) {
      setError(err.message || "Failed to load RCA");
    } finally {
      setLoading(false);
    }
  }, [incidentId]);

  useEffect(() => { load(); }, [load]);

  if (!incidentId) return null;
  if (loading) return <div className="lux-muted" style={{ padding: "1rem" }}>Computing causal RCA…</div>;
  if (error) return <div style={{ color: "var(--red)", padding: "0.5rem" }}>{error}</div>;
  if (!rca) return null;

  return (
    <div style={{ marginTop: 24 }}>
      {/* Overall confidence banner */}
      <div style={{ display: "flex", alignItems: "center", gap: 12, padding: "12px 16px", borderRadius: 12, background: "rgba(255,255,255,0.04)", border: "1px solid var(--border)", marginBottom: 20 }}>
        <div>
          <div style={{ fontSize: "2rem", fontWeight: 900, color: CONF_COLOR(rca.overall_confidence) }}>{rca.overall_confidence}%</div>
          <div style={{ fontSize: "0.68rem", color: "var(--t3)" }}>Overall RCA Confidence</div>
        </div>
        <div style={{ flex: 1 }}>
          {rca.narrative?.what_happened && <div style={{ fontSize: "0.8rem", color: "var(--t2)", marginBottom: 4 }}>{rca.narrative.what_happened}</div>}
          {rca.narrative?.confidence_summary && <div style={{ fontSize: "0.72rem", color: "var(--t3)" }}>{rca.narrative.confidence_summary}</div>}
        </div>
      </div>

      {/* Root cause */}
      {rca.root_cause && (
        <div style={{ marginBottom: 20 }}>
          <div className="lux-eyebrow" style={{ marginBottom: 8 }}>ROOT CAUSE — what changed</div>
          <div style={{ padding: "12px 16px", borderRadius: 10, background: rca.root_cause.type !== "unknown" ? "rgba(167,139,250,0.07)" : "rgba(255,255,255,0.03)", border: `1px solid ${rca.root_cause.type !== "unknown" ? "rgba(167,139,250,0.3)" : "var(--border)"}` }}>
            <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 6 }}>
              <strong style={{ color: "var(--cat-violet)", fontSize: "0.88rem" }}>{rca.root_cause.type.replace(/_/g, " ")}</strong>
              {rca.root_cause.service && <code style={{ fontSize: "0.78rem", color: "var(--cyan)", background: "rgba(5,13,29,0.5)", padding: "1px 6px", borderRadius: 4 }}>{rca.root_cause.service}</code>}
              {rca.root_cause.version && <span style={{ fontSize: "0.72rem", color: "var(--t3)" }}>v{rca.root_cause.version}</span>}
              <span style={{ fontSize: "0.76rem", fontWeight: 700, color: CONF_COLOR(rca.root_cause.confidence), marginLeft: "auto" }}>{rca.root_cause.confidence}% confident</span>
            </div>
            {rca.root_cause.description && <div style={{ fontSize: "0.78rem", color: "var(--t3)" }}>{rca.root_cause.description}</div>}
            {rca.narrative?.what_changed && rca.narrative.what_changed !== "No deployment or configuration change was detected in the relevant window." && (
              <div style={{ fontSize: "0.73rem", color: "var(--t3)", marginTop: 6, borderTop: "1px solid rgba(255,255,255,0.06)", paddingTop: 6 }}>{rca.narrative.what_changed}</div>
            )}
          </div>
        </div>
      )}

      {/* Degradation chain */}
      <DegradationChainView chain={rca.degradation_chain} />

      {/* Evidence nodes */}
      <EvidenceNodesView nodes={rca.evidence_nodes} />

      {/* Impact */}
      <ImpactListView title="DOWNSTREAM IMPACT" items={rca.downstream_impact} />
      <ImpactListView title="UPSTREAM CALLERS IMPACTED" items={rca.upstream_impact} />

      {/* Propagation narrative */}
      {rca.narrative?.how_it_propagated && (
        <div style={{ marginBottom: 12, padding: "10px 14px", borderRadius: 8, background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)", fontSize: "0.78rem", color: "var(--t3)" }}>
          {rca.narrative.how_it_propagated}
        </div>
      )}
      {rca.narrative?.who_is_impacted && (
        <div style={{ padding: "8px 14px", borderRadius: 8, background: "rgba(255,255,255,0.025)", border: "1px solid var(--border)", fontSize: "0.75rem", color: "var(--t3)" }}>
          {rca.narrative.who_is_impacted}
        </div>
      )}
    </div>
  );
}

function BlastRadiusView({ serviceId }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!serviceId) return;
    setLoading(true);
    setError("");
    try {
      const d = await getBlastRadius(serviceId);
      setData(d);
    } catch (err) {
      setError(err.message || "Failed to load blast radius");
    } finally {
      setLoading(false);
    }
  }, [serviceId]);

  useEffect(() => { load(); }, [load]);

  if (!serviceId) return <div className="lux-muted">Select a service node to compute its blast radius.</div>;
  if (loading) return <div className="lux-muted">Computing blast radius…</div>;
  if (error) return <div style={{ color: "var(--red)" }}>{error}</div>;
  if (!data) return null;

  return (
    <div>
      <div style={{ display: "flex", gap: 16, marginBottom: 20, flexWrap: "wrap" }}>
        {[
          { label: "Total Impacted", value: data.total_impacted, color: "var(--amber)" },
          { label: "Customer-Facing", value: data.customer_facing_impacted, color: "var(--red)" },
          { label: "Critical Tier", value: data.critical_tier_impacted, color: "var(--red)" },
        ].map(kpi => (
          <div key={kpi.label} style={{ flex: 1, minWidth: 100, padding: "12px 16px", borderRadius: 10, background: "rgba(255,255,255,0.04)", border: "1px solid var(--border)" }}>
            <div style={{ fontSize: "1.6rem", fontWeight: 800, color: kpi.color }}>{kpi.value}</div>
            <div style={{ fontSize: "0.68rem", color: "var(--t3)" }}>{kpi.label}</div>
          </div>
        ))}
      </div>
      <div className="lux-eyebrow" style={{ marginBottom: 8 }}>IMPACTED NODES</div>
      <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
        {(data.hops || []).map((hop, i) => (
          <div key={i} style={{ display: "flex", alignItems: "center", gap: 8, padding: "8px 12px", borderRadius: 8, background: "rgba(255,255,255,0.03)", border: "1px solid var(--border)" }}>
            <span style={{ fontSize: "0.7rem", color: "var(--t4)", minWidth: 20 }}>D{hop.depth}</span>
            <span style={{ flex: 1, fontSize: "0.82rem", color: "var(--t1)" }}>{hop.node?.display_name || hop.node?.name || hop.node_id}</span>
            {hop.node?.is_customer_facing && <span style={{ fontSize: "0.58rem", padding: "1px 5px", borderRadius: 4, background: "rgba(239,68,68,0.15)", color: "var(--red)" }}>customer-facing</span>}
            {hop.node?.tier && <span style={{ fontSize: "0.6rem", color: TIER_COLOR(hop.node.tier) }}>{hop.node.tier}</span>}
            <span style={{ fontSize: "0.7rem", color: "var(--t3)" }}>{hop.edge_relation}</span>
            <span style={{ fontSize: "0.76rem", fontWeight: 700, color: CONF_COLOR(hop.confidence), minWidth: 32, textAlign: "right" }}>{hop.confidence}%</span>
          </div>
        ))}
        {(!data.hops || data.hops.length === 0) && (
          <div className="lux-muted">No downstream impact nodes found.</div>
        )}
      </div>
    </div>
  );
}

// ─── Visual graph canvas ────────────────────────────────────────────────────
// Deterministic layered layout: one column per node-type, edges drawn as
// bezier curves. Hover highlights the local neighborhood; click selects a node
// (drives the blast-radius panel). Stays cheap — no physics sim.

function truncLabel(s, n = 18) {
  s = String(s || "");
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}

function nodeHealthColor(n) {
  if (n.alert_count > 0) return "var(--red)";
  const s = n.health?.status;
  if (s === "healthy") return "var(--green)";
  if (s && s !== "unknown") return "var(--amber)";
  return null;
}

function TopologyCanvas({ nodes, edges, selectedId, onSelect }) {
  const [hoverId, setHoverId] = useState(null);
  const [zoom, setZoom] = useState(1);
  const [tip, setTip] = useState(null);

  const COL_ORDER = ["external", "team", "alert_origin", "service", "change", "evidence", "cache", "queue", "database", "infra", "deployment", "impacted_service", "dependency"];
  const NODE_W = 156, NODE_H = 46, COL_GAP = 210, ROW_GAP = 78, MX = 30, MY = 34;

  const layout = useMemo(() => {
    const byType = {};
    for (const n of nodes) { const t = n.node_type || "service"; (byType[t] = byType[t] || []).push(n); }
    const cols = [...COL_ORDER.filter((t) => byType[t]), ...Object.keys(byType).filter((t) => !COL_ORDER.includes(t))];
    const maxRows = Math.max(1, ...cols.map((t) => byType[t].length));
    const pos = new Map();
    cols.forEach((t, ci) => {
      const group = byType[t];
      const startY = MY + ((maxRows - group.length) * ROW_GAP) / 2;
      group.forEach((n, ri) => {
        const x = MX + ci * COL_GAP;
        const y = startY + ri * ROW_GAP;
        const p = { x, y, cx: x + NODE_W / 2, cy: y + NODE_H / 2, node: n };
        pos.set(n.id, p);
        if (n.name) pos.set("name:" + n.name, p);
      });
    });
    const width = MX * 2 + (cols.length - 1) * COL_GAP + NODE_W;
    const height = MY * 2 + maxRows * ROW_GAP;
    return { pos, width: Math.max(width, 520), height: Math.max(height, 320) };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodes]);

  const look = (k) => (k != null ? layout.pos.get(k) || layout.pos.get("name:" + k) : null);

  const rEdges = useMemo(() => edges.map((e, i) => {
    const from = look(e.from_node_id) || look(e.from_node_name) || look(e.from);
    const to = look(e.to_node_id) || look(e.to_node_name) || look(e.to);
    return from && to && from !== to ? { e, from, to, key: i } : null;
  }).filter(Boolean),
  // eslint-disable-next-line react-hooks/exhaustive-deps
  [edges, layout]);

  const focusId = hoverId || selectedId;
  const connected = useMemo(() => {
    if (!focusId) return null;
    const s = new Set([focusId]);
    for (const { from, to } of rEdges) {
      if (from.node.id === focusId) s.add(to.node.id);
      if (to.node.id === focusId) s.add(from.node.id);
    }
    return s;
  }, [focusId, rEdges]);

  const W = layout.width, H = layout.height;

  return (
    <div style={{ position: "relative" }}>
      <div style={{ position: "absolute", right: 10, top: 10, zIndex: 2, display: "flex", gap: 4, background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 8, padding: 3 }}>
        <button className="icon-btn" style={{ width: 26, height: 26 }} onClick={() => setZoom((z) => Math.max(0.5, +(z - 0.15).toFixed(2)))} title="Zoom out">−</button>
        <button className="icon-btn" style={{ width: 26, height: 26 }} onClick={() => setZoom(1)} title="Reset zoom">·</button>
        <button className="icon-btn" style={{ width: 26, height: 26 }} onClick={() => setZoom((z) => Math.min(2, +(z + 0.15).toFixed(2)))} title="Zoom in">+</button>
      </div>

      <div style={{ overflow: "auto", maxHeight: 600, borderRadius: "var(--r5)", border: "1px solid var(--border)", background: "radial-gradient(circle at 30% 15%, rgba(0,102,255,0.05), transparent 55%), var(--surface-1)" }}>
        <svg width={W * zoom} height={H * zoom} viewBox={`0 0 ${W} ${H}`} style={{ display: "block" }}>
          <defs>
            <marker id="topo-arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
              <path d="M0,0 L8,4 L0,8 Z" fill="var(--t3)" />
            </marker>
          </defs>

          {rEdges.map(({ e, from, to, key }) => {
            const conf = Math.min(100, Math.max(0, e.confidence || 0));
            const dim = connected && !(connected.has(from.node.id) && connected.has(to.node.id));
            const hot = focusId && (from.node.id === focusId || to.node.id === focusId);
            const dx = Math.max(40, Math.abs(to.cx - from.cx) * 0.4);
            const d = `M ${from.cx} ${from.cy} C ${from.cx + dx} ${from.cy}, ${to.cx - dx} ${to.cy}, ${to.cx} ${to.cy}`;
            return (
              <path key={key} d={d} fill="none"
                stroke={hot ? "var(--blue-lt)" : CONF_COLOR(conf)}
                strokeWidth={hot ? 2 : 1.2}
                strokeOpacity={dim ? 0.07 : hot ? 0.9 : 0.4}
                markerEnd="url(#topo-arrow)" />
            );
          })}

          {[...layout.pos.entries()].filter(([k]) => !k.startsWith("name:")).map(([id, p]) => {
            const n = p.node;
            const cfg = NODE_CFG[n.node_type] || NODE_CFG.dependency;
            const hc = nodeHealthColor(n);
            const isSel = selectedId === n.id;
            const dim = connected && !connected.has(n.id);
            return (
              <g key={id} transform={`translate(${p.x},${p.y})`}
                style={{ cursor: "pointer", opacity: dim ? 0.22 : 1, transition: "opacity .15s" }}
                onClick={() => onSelect(n)}
                onMouseEnter={(ev) => { setHoverId(n.id); setTip({ n, x: ev.clientX, y: ev.clientY }); }}
                onMouseLeave={() => { setHoverId(null); setTip(null); }}>
                {n.alert_count > 0 && (
                  <rect x={-2} y={-2} width={NODE_W + 4} height={NODE_H + 4} rx={11} fill="none" stroke="var(--red)" strokeOpacity={0.45} strokeWidth={3} />
                )}
                <rect width={NODE_W} height={NODE_H} rx={9} fill={cfg.bg}
                  stroke={isSel ? "var(--blue)" : hc || cfg.border} strokeWidth={isSel ? 2 : 1.4} />
                <circle cx={14} cy={NODE_H / 2} r={4} fill={hc || cfg.color} />
                <text x={26} y={NODE_H / 2 - 3} fill="var(--t1)" fontSize="11.5" fontWeight="700" dominantBaseline="middle">{truncLabel(n.display_name || n.name || n.id)}</text>
                <text x={26} y={NODE_H / 2 + 11} fill={cfg.color} fontSize="8" fontWeight="700" letterSpacing="0.08em" dominantBaseline="middle">{cfg.label.toUpperCase()}{n.is_customer_facing ? " · CF" : ""}</text>
                {n.alert_count > 0 && (
                  <>
                    <circle cx={NODE_W - 12} cy={13} r={8} fill="var(--red)" />
                    <text x={NODE_W - 12} y={13} fill="#fff" fontSize="9" fontWeight="800" textAnchor="middle" dominantBaseline="central">{n.alert_count}</text>
                  </>
                )}
              </g>
            );
          })}
        </svg>
      </div>

      {tip && (
        <div style={{ position: "fixed", left: Math.min(tip.x + 14, window.innerWidth - 240), top: tip.y + 14, zIndex: 50, pointerEvents: "none", width: 220, background: "var(--surface-3)", border: "1px solid var(--border-strong)", borderRadius: 8, padding: "9px 11px", boxShadow: "var(--shadow-md)" }}>
          <div style={{ fontSize: 12, fontWeight: 700, color: "var(--t1)", marginBottom: 3 }}>{tip.n.display_name || tip.n.name || tip.n.id}</div>
          <div style={{ fontSize: 11, color: "var(--t3)" }}>{(NODE_CFG[tip.n.node_type] || NODE_CFG.dependency).label}{tip.n.tier ? " · " + tip.n.tier : ""}</div>
          <div style={{ display: "flex", gap: 8, marginTop: 5, flexWrap: "wrap" }}>
            {tip.n.health?.status && tip.n.health.status !== "unknown" && <span style={{ fontSize: 10, color: tip.n.health.status === "healthy" ? "var(--green)" : "var(--red)" }}>{tip.n.health.status}</span>}
            {tip.n.alert_count > 0 && <span style={{ fontSize: 10, color: "var(--red)" }}>{tip.n.alert_count} alerts</span>}
            {tip.n.owner_team && <span style={{ fontSize: 10, color: "var(--t2)" }}>{tip.n.owner_team}</span>}
            {tip.n.is_customer_facing && <span style={{ fontSize: 10, color: "var(--amber)" }}>customer-facing</span>}
          </div>
          <div style={{ fontSize: 10, color: "var(--t4)", marginTop: 5 }}>Click for blast radius →</div>
        </div>
      )}
    </div>
  );
}

function LiveGraphView() {
  const [graph, setGraph] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [selectedNode, setSelectedNode] = useState(null);
  const [discovering, setDiscovering] = useState(false);
  const [discoverMsg, setDiscoverMsg] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const d = await getLiveTopologyGraph();
      setGraph(d);
    } catch (err) {
      setError(err.message || "Failed to load live topology");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleDiscover = async () => {
    setDiscovering(true);
    setDiscoverMsg("");
    try {
      const result = await discoverTopology();
      setDiscoverMsg(`Auto-discovered ${result.nodes_discovered} nodes from incident history.`);
      await load();
    } catch (err) {
      setDiscoverMsg("Discovery failed: " + (err.message || "unknown error"));
    } finally {
      setDiscovering(false);
    }
  };

  if (loading) return <div className="lux-muted">Loading live topology graph…</div>;
  if (error) return <div style={{ color: "var(--red)" }}>{error}</div>;

  const nodes = graph?.nodes || [];
  const edges = graph?.edges || [];

  return (
    <div>
      <div style={{ display: "flex", gap: 12, marginBottom: 18, flexWrap: "wrap" }}>
        {[
          { label: "Nodes", value: nodes.length },
          { label: "Edges", value: edges.length },
          { label: "Layers", value: (graph?.layers || []).length },
        ].map(kpi => (
          <div key={kpi.label} style={{ padding: "10px 18px", borderRadius: 10, background: "rgba(255,255,255,0.04)", border: "1px solid var(--border)" }}>
            <div style={{ fontSize: "1.4rem", fontWeight: 800, color: "var(--cyan)" }}>{kpi.value}</div>
            <div style={{ fontSize: "0.65rem", color: "var(--t3)" }}>{kpi.label}</div>
          </div>
        ))}
        <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: 8 }}>
          {discoverMsg && <span style={{ fontSize: "0.72rem", color: "var(--green)" }}>{discoverMsg}</span>}
          <button className="lux-secondary-btn" onClick={handleDiscover} disabled={discovering}>
            {discovering ? "Discovering…" : "Auto-Discover"}
          </button>
          <button className="lux-secondary-btn" onClick={load}>Refresh</button>
        </div>
      </div>

      {nodes.length === 0 ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>🔭</div>
          <p>No topology nodes yet. Click <strong>Auto-Discover</strong> to mine your incident history and build the graph automatically, or register nodes via the API.</p>
        </div>
      ) : (
        <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <TopologyCanvas
              nodes={nodes}
              edges={edges}
              selectedId={selectedNode?.id || null}
              onSelect={(n) => setSelectedNode(selectedNode?.id === n.id ? null : n)}
            />
          </div>
          {selectedNode && (
            <div style={{ width: 330, flexShrink: 0, background: "var(--surface-1)", border: "1px solid var(--border)", borderRadius: "var(--r5)", padding: 16, maxHeight: 600, overflowY: "auto" }}>
              <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 8, marginBottom: 12 }}>
                <div style={{ minWidth: 0 }}>
                  <div className="lux-eyebrow">Blast Radius</div>
                  <h4 style={{ margin: "0.25rem 0 0", color: "var(--t1)", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{selectedNode.display_name || selectedNode.name}</h4>
                </div>
                <button className="icon-btn" style={{ flexShrink: 0 }} onClick={() => setSelectedNode(null)} title="Close">✕</button>
              </div>
              <BlastRadiusView serviceId={selectedNode.name} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Main panel ───────────────────────────────────────────────────────────────

const TABS = ["Incident Graph", "Causal RCA", "Live Topology"];

export default function TopologyGraphPanel({ incidentId }) {
  const [tab, setTab] = useState(incidentId ? "Incident Graph" : "Live Topology");
  const [graph, setGraph] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const loadIncidentGraph = useCallback(async () => {
    if (!incidentId) return;
    setLoading(true);
    setError("");
    setGraph(null);
    try {
      const data = await getTopologyGraph(incidentId);
      setGraph(data);
    } catch (err) {
      setError(err.message || "Failed to load topology graph");
    } finally {
      setLoading(false);
    }
  }, [incidentId]);

  useEffect(() => {
    if (tab === "Incident Graph") loadIncidentGraph();
  }, [tab, loadIncidentGraph]);

  // Switch to incident graph tab when incidentId arrives
  useEffect(() => {
    if (incidentId) setTab("Incident Graph");
  }, [incidentId]);

  return (
    <Page>
      <PageHeader
        title="Topology graph"
        meta="Service, infra, deployment and ownership layers · blast radius · causal RCA with per-evidence confidence"
      />

      <nav className="tabs topology-tabs" role="tablist">
        {TABS.map((t) => (
          <button
            key={t}
            type="button"
            role="tab"
            aria-selected={tab === t}
            className={`tab-item${tab === t ? " active" : ""}`}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </nav>

      <ul className="graph-legend">
        {Object.entries(NODE_CFG)
          .filter(([t]) => !["dependency", "impacted_service", "owner"].includes(t))
          .map(([type, cfg]) => (
            <li key={type} className="graph-legend-item">
              <span className="graph-legend-dot" style={{ background: cfg.color }} aria-hidden="true" />
              {cfg.label}
            </li>
          ))}
      </ul>

      {/* Tab content */}
      {tab === "Incident Graph" && (
        <div>
          {!incidentId && <div className="lux-muted">No incident selected.</div>}
          {incidentId && loading && <div className="lux-muted">Building topology graph…</div>}
          {incidentId && error && <div style={{ color: "var(--red)" }}>{error}</div>}
          {incidentId && !loading && !error && graph && (
            <>
              <div style={{ marginBottom: 6, display: "flex", gap: 8, alignItems: "center" }}>
                <span className="lux-muted" style={{ fontSize: "0.72rem" }}>
                  {(graph.nodes || []).length} nodes · {(graph.edges || []).length} edges
                  {graph.root_node_id ? ` · Root: ${graph.root_node_id}` : ""}
                </span>
                {graph.built_at && <span style={{ fontSize: "0.67rem", color: "var(--t4)", marginLeft: "auto" }}>Built {new Date(graph.built_at).toLocaleTimeString()}</span>}
              </div>

              {/* Nodes by type */}
              {(() => {
                const byType = {};
                for (const n of graph.nodes || []) {
                  const t = n.node_type || "service";
                  if (!byType[t]) byType[t] = [];
                  byType[t].push(n);
                }
                const typeOrder = ["service", "alert_origin", "change", "evidence", "impacted_service", "dependency", "infra", "database", "queue", "cache"];
                const types = [...typeOrder.filter(t => byType[t]), ...Object.keys(byType).filter(t => !typeOrder.includes(t))];
                return types.map(type => {
                  const group = byType[type];
                  if (!group) return null;
                  const cfg = NODE_CFG[type] || NODE_CFG.dependency;
                  return (
                    <div key={type} style={{ marginBottom: 16 }}>
                      <div style={{ fontSize: "0.67rem", color: cfg.color, fontWeight: 700, textTransform: "uppercase", letterSpacing: "0.1em", marginBottom: 6 }}>
                        {cfg.label} ({group.length})
                      </div>
                      <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                        {group.map(node => <NodeCard key={node.id} node={node} />)}
                      </div>
                    </div>
                  );
                });
              })()}

              {(graph.edges || []).length > 0 && (
                <div style={{ marginTop: 10 }}>
                  <div className="lux-eyebrow" style={{ marginBottom: 8 }}>RELATIONSHIPS ({graph.edges.length})</div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                    {graph.edges.map((e, i) => <EdgeRow key={`${e.from}-${e.to}-${i}`} edge={e} />)}
                  </div>
                </div>
              )}
            </>
          )}
        </div>
      )}

      {tab === "Causal RCA" && (
        <div>
          {!incidentId && <div className="lux-muted">Select an incident to generate its causal RCA.</div>}
          {incidentId && <RCAPanel incidentId={incidentId} />}
        </div>
      )}

      {tab === "Live Topology" && <LiveGraphView />}
    </Page>
  );
}
