import { useEffect, useState, useCallback } from "react";

const API_BASE_URL = "http://localhost:8080/api/v1";

const SEV = {
  critical: { label: "CRITICAL", color: "#ff4d6a", bg: "rgba(255,77,106,0.09)", border: "rgba(255,77,106,0.28)", glow: "rgba(255,77,106,0.25)" },
  warning:  { label: "WARNING",  color: "#ffad33", bg: "rgba(255,173,51,0.09)",  border: "rgba(255,173,51,0.28)",  glow: "rgba(255,173,51,0.2)"  },
  info:     { label: "INFO",     color: "#33ccff", bg: "rgba(51,204,255,0.09)",   border: "rgba(51,204,255,0.28)",   glow: "rgba(51,204,255,0.2)"  },
  resolved: { label: "RESOLVED", color: "#00e5a0", bg: "rgba(0,229,160,0.09)",   border: "rgba(0,229,160,0.28)",   glow: "rgba(0,229,160,0.2)"  },
};

function getSev(title = "", status = "") {
  if (status === "resolved") return "resolved";
  const t = title.toLowerCase();
  if (t.includes("critical")) return "critical";
  if (t.includes("warning") || t.includes("warn")) return "warning";
  return "info";
}

function getEventSev(e) {
  if (e.severity) return e.severity;
  const m = (e.message || "").toLowerCase();
  if (m.includes("fail") || m.includes("error") || m.includes("critical")) return "critical";
  if (m.includes("timeout") || m.includes("slow") || m.includes("warn")) return "warning";
  return "info";
}

function timeAgo(d) {
  if (!d) return "just now";
  const s = Math.floor((Date.now() - new Date(d).getTime()) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

function Badge({ sev }) {
  const s = SEV[sev] || SEV.info;
  return (
    <span style={{
      display: "inline-flex", alignItems: "center", gap: 4,
      padding: "2px 8px", borderRadius: 4,
      background: s.bg, border: `1px solid ${s.border}`,
      color: s.color, fontSize: 9, fontWeight: 700,
      letterSpacing: "0.11em", fontFamily: "var(--mono)",
      whiteSpace: "nowrap",
    }}>
      <span style={{
        width: 4, height: 4, borderRadius: "50%",
        background: s.color, boxShadow: `0 0 5px ${s.color}`,
        animation: sev === "critical" ? "blink 1.1s ease-in-out infinite" : "none",
        flexShrink: 0,
      }} />
      {s.label}
    </span>
  );
}

function Chip({ label, value }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
      <span style={{ fontSize: 9, color: "#374151", fontFamily: "var(--mono)", letterSpacing: "0.1em" }}>{label}</span>
      <span style={{ fontSize: 12, color: "#94a3b8", fontFamily: "var(--mono)", fontWeight: 500 }}>{value}</span>
    </div>
  );
}

function NavIcon({ icon, id, active, onClick, title }) {
  return (
    <button title={title} onClick={() => onClick(id)} style={{
      width: 40, height: 40, borderRadius: 9,
      display: "flex", alignItems: "center", justifyContent: "center",
      background: active ? "rgba(0,229,204,0.1)" : "transparent",
      border: active ? "1px solid rgba(0,229,204,0.22)" : "1px solid transparent",
      color: active ? "#00e5cc" : "#374151",
      fontSize: 17, cursor: "pointer",
      transition: "all 0.15s ease",
      marginBottom: 6,
    }}>
      {icon}
    </button>
  );
}

function StatCard({ label, value, sub, color, icon, delay }) {
  return (
    <div style={{
      padding: "16px 18px",
      background: "var(--panel)",
      border: "1px solid var(--border)",
      borderRadius: 11,
      position: "relative",
      overflow: "hidden",
      animation: `slideIn 0.35s ease ${delay}s both`,
    }}>
      <div style={{
        position: "absolute", top: 0, left: 0, right: 0,
        height: 2,
        background: `linear-gradient(90deg, ${color}, transparent)`,
      }} />
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
        <div>
          <div style={{
            fontSize: 9, color: "#4b5563",
            fontFamily: "var(--mono)", letterSpacing: "0.11em",
            textTransform: "uppercase", marginBottom: 8,
          }}>{label}</div>
          <div style={{
            fontSize: 34, fontWeight: 800,
            color: value === 0 || value === "—" ? "#1f2937" : "#f1f5f9",
            lineHeight: 1, fontFamily: "var(--sans)", letterSpacing: "-0.02em",
          }}>{value}</div>
          <div style={{ fontSize: 10, color: "#374151", marginTop: 5, fontFamily: "var(--mono)" }}>{sub}</div>
        </div>
        <span style={{ fontSize: 20, color, opacity: 0.55 }}>{icon}</span>
      </div>
    </div>
  );
}

export default function App() {
  const [events, setEvents] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [selected, setSelected] = useState(null);
  const [nav, setNav] = useState("dashboard");
  const [time, setTime] = useState(new Date());

  const fetchData = useCallback(async () => {
    const [e, i] = await Promise.all([
      fetch(`${API_BASE_URL}/events`).then(r => r.json()).catch(() => []),
      fetch(`${API_BASE_URL}/incidents`).then(r => r.json()).catch(() => []),
    ]);
    const ev = Array.isArray(e) ? e : [];
    const inc = Array.isArray(i) ? i : [];
    setEvents(ev);
    setIncidents(inc);
    if (!selected && inc.length) setSelected(inc[0]);
  }, []);

  useEffect(() => {
    fetchData();
    const d = setInterval(fetchData, 30000);
    const t = setInterval(() => setTime(new Date()), 1000);
    return () => { clearInterval(d); clearInterval(t); };
  }, [fetchData]);

  const critical = incidents.filter(i => getSev(i.title) === "critical");
  const affectedServices = [...new Set([
    ...incidents.map(i => i.service),
    ...events.map(e => e.service),
  ].filter(Boolean))];
  const active = selected || incidents[0];

  return (
    <>
      <style>{`
        @import url('https://fonts.googleapis.com/css2?family=Syne:wght@400;500;600;700;800&family=JetBrains+Mono:wght@300;400;500;600&display=swap');
        *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
        :root {
          --sans: 'Syne', sans-serif;
          --mono: 'JetBrains Mono', monospace;
          --bg: #060709;
          --panel: rgba(12,13,20,0.92);
          --border: rgba(255,255,255,0.07);
          --teal: #00e5cc;
        }
        body { background: var(--bg); color: #dde4f0; font-family: var(--sans); -webkit-font-smoothing: antialiased; }
        ::-webkit-scrollbar { width: 3px; }
        ::-webkit-scrollbar-track { background: transparent; }
        ::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.08); border-radius: 2px; }

        @keyframes blink    { 0%,100%{opacity:1} 50%{opacity:0.2} }
        @keyframes slideIn  { from{opacity:0;transform:translateY(10px)} to{opacity:1;transform:translateY(0)} }
        @keyframes ringPop  { 0%{transform:scale(1);opacity:0.6} 100%{transform:scale(2.8);opacity:0} }
        @keyframes sweep    { 0%{transform:scaleX(0);transform-origin:left} 100%{transform:scaleX(1);transform-origin:left} }
        @keyframes scanMove { 0%{top:0} 100%{top:100%} }

        .grid-bg {
          position: fixed; inset: 0; pointer-events: none; z-index: 0;
          background-image:
            linear-gradient(rgba(0,229,204,0.028) 1px, transparent 1px),
            linear-gradient(90deg, rgba(0,229,204,0.028) 1px, transparent 1px);
          background-size: 56px 56px;
        }
        .glow-tl {
          position: fixed; top: -260px; left: -160px; width: 640px; height: 640px;
          border-radius: 50%; pointer-events: none; z-index: 0;
          background: radial-gradient(circle, rgba(0,229,204,0.045) 0%, transparent 65%);
        }
        .glow-br {
          position: fixed; bottom: -220px; right: -120px; width: 520px; height: 520px;
          border-radius: 50%; pointer-events: none; z-index: 0;
          background: radial-gradient(circle, rgba(255,77,106,0.04) 0%, transparent 65%);
        }
        .nav-btn:hover { background: rgba(255,255,255,0.05) !important; color: #94a3b8 !important; }
        .pill-btn:hover { background: rgba(0,229,204,0.14) !important; border-color: rgba(0,229,204,0.4) !important; transform: translateY(-1px); }
        .pill-btn:active { transform: translateY(0); }
        .row:hover { background: rgba(255,255,255,0.025) !important; }
        .ev-card:hover { border-color: rgba(255,255,255,0.12) !important; }
      `}</style>

      <div className="grid-bg" />
      <div className="glow-tl" />
      <div className="glow-br" />

      <div style={{ display: "flex", minHeight: "100vh", position: "relative", zIndex: 1 }}>

        {/* ── SIDEBAR ─────────────────────────────────────────────────── */}
        <aside style={{
          width: 62, background: "rgba(8,9,14,0.97)",
          borderRight: "1px solid var(--border)",
          display: "flex", flexDirection: "column", alignItems: "center",
          padding: "18px 0", position: "fixed", top: 0, left: 0, bottom: 0,
          zIndex: 200, backdropFilter: "blur(16px)",
        }}>
          {/* Logo */}
          <div style={{
            width: 36, height: 36, borderRadius: 10, marginBottom: 28,
            background: "linear-gradient(135deg, #00e5cc 0%, #3b82f6 100%)",
            display: "flex", alignItems: "center", justifyContent: "center",
            boxShadow: "0 0 22px rgba(0,229,204,0.35), 0 0 60px rgba(0,229,204,0.1)",
            fontSize: 15, cursor: "default", userSelect: "none",
          }}>⬡</div>

          {[
            { id: "dashboard", icon: "▦", title: "Dashboard" },
            { id: "incidents", icon: "◉", title: "Incidents" },
            { id: "events",    icon: "≋", title: "Events"    },
            { id: "topology",  icon: "◈", title: "Topology"  },
          ].map(n => (
            <NavIcon key={n.id} {...n} active={nav === n.id} onClick={setNav} />
          ))}

          <div style={{ marginTop: "auto", display: "flex", flexDirection: "column", alignItems: "center", gap: 8 }}>
            <div style={{
              width: 5, height: 5, borderRadius: "50%",
              background: critical.length > 0 ? "#ff4d6a" : "#00e5a0",
              boxShadow: `0 0 8px ${critical.length > 0 ? "#ff4d6a" : "#00e5a0"}`,
              animation: "blink 2.5s ease-in-out infinite",
            }} />
            <span style={{
              fontSize: 7, fontFamily: "var(--mono)", color: "#1f2937",
              letterSpacing: "0.05em", writingMode: "vertical-rl",
            }}>SYS</span>
          </div>
        </aside>

        {/* ── MAIN ───────────────────────────────────────────────────── */}
        <main style={{ flex: 1, marginLeft: 62, display: "flex", flexDirection: "column", minHeight: "100vh" }}>

          {/* ── TOPBAR ── */}
          <header style={{
            height: 52, background: "rgba(8,9,14,0.94)",
            borderBottom: "1px solid var(--border)",
            display: "flex", alignItems: "center", padding: "0 22px",
            position: "sticky", top: 0, zIndex: 100,
            backdropFilter: "blur(16px)", gap: 14,
          }}>
            <span style={{ fontSize: 10, fontFamily: "var(--mono)", color: "#374151", letterSpacing: "0.1em" }}>
              AIOPS
            </span>
            <span style={{ color: "#1f2937", fontSize: 14 }}>›</span>
            <span style={{ fontSize: 12, fontFamily: "var(--mono)", color: "#6b7280" }}>
              Incident Intelligence
            </span>

            <div style={{ flex: 1 }} />

            {critical.length > 0 && (
              <div style={{
                display: "flex", alignItems: "center", gap: 6,
                padding: "4px 10px", borderRadius: 6,
                background: "rgba(255,77,106,0.08)",
                border: "1px solid rgba(255,77,106,0.28)",
              }}>
                <div style={{
                  width: 6, height: 6, borderRadius: "50%",
                  background: "#ff4d6a", boxShadow: "0 0 8px #ff4d6a",
                  animation: "blink 0.9s ease-in-out infinite",
                }} />
                <span style={{ fontSize: 10, fontFamily: "var(--mono)", color: "#ff4d6a", fontWeight: 600 }}>
                  {critical.length} CRITICAL ACTIVE
                </span>
              </div>
            )}

            <span style={{ fontSize: 10, fontFamily: "var(--mono)", color: "#1f2937" }}>
              {time.toLocaleTimeString("en-US", { hour12: false })}
            </span>

            <button className="pill-btn" onClick={fetchData} style={{
              padding: "5px 13px", borderRadius: 6, cursor: "pointer",
              background: "rgba(0,229,204,0.07)",
              border: "1px solid rgba(0,229,204,0.2)",
              color: "#00e5cc", fontSize: 10,
              fontFamily: "var(--mono)", fontWeight: 500,
              letterSpacing: "0.06em", transition: "all 0.18s ease",
            }}>
              ↻ REFRESH
            </button>
          </header>

          {/* ── BODY ── */}
          <div style={{ flex: 1, padding: "22px 24px", display: "flex", flexDirection: "column", gap: 18 }}>

            {/* Page heading */}
            <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between" }}>
              <div>
                <h1 style={{
                  fontSize: 24, fontWeight: 800, color: "#f1f5f9",
                  letterSpacing: "-0.025em", lineHeight: 1.1,
                  fontFamily: "var(--sans)",
                }}>Incident Intelligence</h1>
                <p style={{ fontSize: 11, color: "#374151", marginTop: 4, fontFamily: "var(--mono)" }}>
                  Real-time correlation of noisy events → actionable incidents
                </p>
              </div>

              <div style={{
                display: "flex", alignItems: "center", gap: 10,
                padding: "8px 14px", borderRadius: 9,
                background: "var(--panel)", border: "1px solid var(--border)",
              }}>
                <div style={{ textAlign: "right" }}>
                  <div style={{ fontSize: 9, color: "#374151", fontFamily: "var(--mono)", letterSpacing: "0.1em", marginBottom: 2 }}>SYSTEM STATUS</div>
                  <div style={{
                    fontSize: 11, fontFamily: "var(--mono)", fontWeight: 700,
                    color: critical.length > 0 ? "#ff4d6a" : "#00e5a0",
                  }}>{critical.length > 0 ? "DEGRADED" : "OPERATIONAL"}</div>
                </div>
                <div style={{
                  width: 34, height: 34, borderRadius: "50%",
                  border: `2px solid ${critical.length > 0 ? "#ff4d6a" : "#00e5a0"}`,
                  display: "flex", alignItems: "center", justifyContent: "center",
                  background: critical.length > 0 ? "rgba(255,77,106,0.1)" : "rgba(0,229,160,0.1)",
                  boxShadow: `0 0 14px ${critical.length > 0 ? "rgba(255,77,106,0.35)" : "rgba(0,229,160,0.35)"}`,
                  fontSize: 14, animation: critical.length > 0 ? "blink 2s ease-in-out infinite" : "none",
                }}>
                  {critical.length > 0 ? "!" : "✓"}
                </div>
              </div>
            </div>

            {/* ── STAT CARDS ── */}
            <div style={{ display: "grid", gridTemplateColumns: "repeat(4, 1fr)", gap: 12 }}>
              <StatCard label="Total Incidents"   value={incidents.length}          sub={`${critical.length} critical`}        color="#ff4d6a" icon="◉" delay={0}    />
              <StatCard label="Active Events"     value={events.length}             sub="streaming in"                          color="#ffad33" icon="⚡" delay={0.06} />
              <StatCard label="Affected Services" value={affectedServices.length || "—"} sub="in impact radius"                color="#a78bfa" icon="⬡" delay={0.12} />
              <StatCard label="Correlation Rate"  value={incidents.length && events.length ? `${Math.round((incidents.length / events.length) * 100)}%` : "—"} sub="events → incidents" color="#00e5cc" icon="∿" delay={0.18} />
            </div>

            {/* ── TWO-COLUMN ── */}
            <div style={{ display: "grid", gridTemplateColumns: "1fr 330px", gap: 16, flex: 1 }}>

              {/* LEFT */}
              <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>

                {/* ── ACTIVE INCIDENT HERO ── */}
                {active ? (
                  <div style={{
                    padding: "20px 22px",
                    background: "var(--panel)",
                    border: `1px solid ${SEV[getSev(active.title)].border}`,
                    borderRadius: 13, position: "relative", overflow: "hidden",
                    boxShadow: `0 0 40px ${SEV[getSev(active.title)].glow}`,
                    animation: "slideIn 0.4s ease both",
                  }}>
                    {/* Sweeping top line */}
                    <div style={{
                      position: "absolute", top: 0, left: 0, right: 0, height: 2,
                      background: `linear-gradient(90deg, transparent, ${SEV[getSev(active.title)].color}, transparent)`,
                      animation: "sweep 2.4s ease-in-out infinite alternate",
                    }} />
                    {/* Scan line */}
                    <div style={{
                      position: "absolute", left: 0, right: 0, height: "1px",
                      background: `linear-gradient(90deg, transparent, ${SEV[getSev(active.title)].color}22, transparent)`,
                      animation: "scanMove 4s linear infinite",
                      pointerEvents: "none",
                    }} />

                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", marginBottom: 14 }}>
                      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                        <div style={{ position: "relative", width: 12, height: 12, flexShrink: 0 }}>
                          <div style={{
                            position: "absolute", inset: 0, borderRadius: "50%",
                            background: SEV[getSev(active.title)].color,
                            animation: "ringPop 1.6s ease-out infinite", opacity: 0.5,
                          }} />
                          <div style={{ position: "absolute", inset: "3px", borderRadius: "50%", background: SEV[getSev(active.title)].color }} />
                        </div>
                        <span style={{
                          fontSize: 9, fontFamily: "var(--mono)", fontWeight: 700,
                          color: SEV[getSev(active.title)].color, letterSpacing: "0.14em",
                        }}>ACTIVE INCIDENT</span>
                      </div>
                      <Badge sev={getSev(active.title)} />
                    </div>

                    <h2 style={{
                      fontSize: 19, fontWeight: 700, color: "#f1f5f9",
                      fontFamily: "var(--sans)", letterSpacing: "-0.01em",
                      marginBottom: 12, lineHeight: 1.3,
                    }}>
                      {(active.title || "Unnamed Incident")
                        .replace(/^\[CRITICAL\]\s*/i, "")
                        .replace(/^\[WARNING\]\s*/i, "")
                        .replace(/^\[INFO\]\s*/i, "")}
                    </h2>

                    <div style={{ display: "flex", gap: 20, flexWrap: "wrap", marginBottom: 16 }}>
                      <Chip label="SERVICE"  value={active.service || "—"} />
                      <Chip label="EVENTS"   value={events.filter(e => e.service === active.service).length || "—"} />
                      <Chip label="DETECTED" value={active.created_at ? timeAgo(active.created_at) : "just now"} />
                      <Chip label="ID"       value={active.id ? `#${active.id}` : "—"} />
                    </div>

                    {/* Correlated events */}
                    {events.length > 0 && (
                      <>
                        <div style={{
                          fontSize: 9, color: "#374151", fontFamily: "var(--mono)",
                          letterSpacing: "0.12em", marginBottom: 8,
                          paddingTop: 14, borderTop: "1px solid rgba(255,255,255,0.05)",
                        }}>CORRELATED EVENTS ({events.filter(e => e.service === active.service).length})</div>
                        <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
                          {events.filter(e => e.service === active.service).slice(0, 4).map((e, i) => (
                            <div key={i} style={{
                              display: "flex", alignItems: "center", gap: 8,
                              padding: "7px 10px",
                              background: "rgba(255,255,255,0.02)",
                              border: "1px solid rgba(255,255,255,0.04)",
                              borderRadius: 7,
                            }}>
                              <div style={{
                                width: 4, height: 4, borderRadius: "50%", flexShrink: 0,
                                background: SEV[getEventSev(e)].color,
                                boxShadow: `0 0 5px ${SEV[getEventSev(e)].color}`,
                              }} />
                              <span style={{ fontSize: 11, color: "#94a3b8", fontFamily: "var(--mono)", flex: 1, lineHeight: 1.4 }}>
                                {e.message || e.name || "event"}
                              </span>
                              <Badge sev={getEventSev(e)} />
                            </div>
                          ))}
                        </div>
                      </>
                    )}
                  </div>
                ) : (
                  <div style={{
                    padding: "32px", background: "var(--panel)",
                    border: "1px solid var(--border)", borderRadius: 13,
                    textAlign: "center", color: "#1f2937",
                    fontFamily: "var(--mono)", fontSize: 12,
                  }}>
                    ✓ No active incidents
                  </div>
                )}

                {/* ── INCIDENTS TABLE ── */}
                <div style={{
                  background: "var(--panel)", border: "1px solid var(--border)",
                  borderRadius: 13, overflow: "hidden", flex: 1,
                }}>
                  <div style={{
                    padding: "13px 18px", borderBottom: "1px solid var(--border)",
                    display: "flex", alignItems: "center", justifyContent: "space-between",
                  }}>
                    <span style={{ fontSize: 10, color: "#6b7280", fontFamily: "var(--mono)", letterSpacing: "0.1em" }}>
                      ALL INCIDENTS
                    </span>
                    <span style={{ fontSize: 10, fontFamily: "var(--mono)", color: "#374151" }}>
                      {incidents.length} total
                    </span>
                  </div>

                  {/* Header row */}
                  <div style={{
                    display: "grid", gridTemplateColumns: "1fr 110px 90px 80px",
                    padding: "7px 18px", background: "rgba(255,255,255,0.015)",
                    borderBottom: "1px solid rgba(255,255,255,0.04)",
                  }}>
                    {["TITLE", "SERVICE", "SEVERITY", "WHEN"].map(h => (
                      <span key={h} style={{ fontSize: 8, color: "#1f2937", fontFamily: "var(--mono)", letterSpacing: "0.12em" }}>{h}</span>
                    ))}
                  </div>

                  {incidents.length === 0 ? (
                    <div style={{ padding: "40px 0", textAlign: "center", color: "#1f2937", fontFamily: "var(--mono)", fontSize: 11 }}>
                      No incidents found
                    </div>
                  ) : incidents.map((inc, idx) => {
                    const sev = getSev(inc.title, inc.status);
                    const isSelected = selected?.id === inc.id;
                    return (
                      <div key={inc.id || idx} className="row"
                        onClick={() => setSelected(inc)}
                        style={{
                          display: "grid", gridTemplateColumns: "1fr 110px 90px 80px",
                          padding: "11px 18px",
                          borderBottom: "1px solid rgba(255,255,255,0.03)",
                          cursor: "pointer", transition: "background 0.12s",
                          background: isSelected ? "rgba(0,229,204,0.03)" : "transparent",
                          borderLeft: isSelected ? "2px solid #00e5cc" : "2px solid transparent",
                        }}>
                        <div style={{ paddingRight: 12 }}>
                          <div style={{
                            fontSize: 12, color: "#e2e8f0", fontWeight: 600,
                            fontFamily: "var(--sans)", lineHeight: 1.35, marginBottom: 2,
                          }}>
                            {(inc.title || "—").replace(/^\[.*?\]\s*/, "")}
                          </div>
                        </div>
                        <div style={{ fontSize: 10, color: "#6b7280", fontFamily: "var(--mono)", alignSelf: "center" }}>
                          {inc.service || "—"}
                        </div>
                        <div style={{ alignSelf: "center" }}>
                          <Badge sev={sev} />
                        </div>
                        <div style={{ fontSize: 10, color: "#374151", fontFamily: "var(--mono)", alignSelf: "center" }}>
                          {inc.created_at ? timeAgo(inc.created_at) : "just now"}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* RIGHT — EVENT STREAM */}
              <div style={{
                background: "var(--panel)", border: "1px solid var(--border)",
                borderRadius: 13, overflow: "hidden",
                display: "flex", flexDirection: "column",
              }}>
                <div style={{
                  padding: "13px 16px", borderBottom: "1px solid var(--border)",
                  display: "flex", alignItems: "center", justifyContent: "space-between",
                }}>
                  <span style={{ fontSize: 10, color: "#6b7280", fontFamily: "var(--mono)", letterSpacing: "0.1em" }}>
                    EVENT STREAM
                  </span>
                  <div style={{ display: "flex", alignItems: "center", gap: 5 }}>
                    <div style={{
                      width: 5, height: 5, borderRadius: "50%",
                      background: "#00e5a0", boxShadow: "0 0 6px #00e5a0",
                      animation: "blink 2s ease-in-out infinite",
                    }} />
                    <span style={{ fontSize: 9, color: "#374151", fontFamily: "var(--mono)", letterSpacing: "0.1em" }}>LIVE</span>
                  </div>
                </div>

                {/* Mini service filter chips */}
                {affectedServices.length > 0 && (
                  <div style={{
                    padding: "8px 12px", borderBottom: "1px solid rgba(255,255,255,0.04)",
                    display: "flex", gap: 4, flexWrap: "wrap",
                  }}>
                    {affectedServices.map(svc => (
                      <span key={svc} style={{
                        fontSize: 9, fontFamily: "var(--mono)",
                        color: "#4b5563", padding: "2px 7px",
                        border: "1px solid rgba(255,255,255,0.06)",
                        borderRadius: 4,
                      }}>{svc}</span>
                    ))}
                  </div>
                )}

                <div style={{ flex: 1, overflowY: "auto", padding: "10px 10px" }}>
                  {events.length === 0 ? (
                    <div style={{ padding: "40px 0", textAlign: "center", color: "#1f2937", fontFamily: "var(--mono)", fontSize: 11 }}>
                      No events detected
                    </div>
                  ) : (
                    <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                      {events.map((ev, idx) => {
                        const sev = getEventSev(ev);
                        const s = SEV[sev] || SEV.info;
                        return (
                          <div key={ev.id || idx} className="ev-card" style={{
                            padding: "10px 12px",
                            background: s.bg,
                            border: `1px solid ${s.border}`,
                            borderRadius: 9,
                            animation: `slideIn 0.28s ease ${idx * 0.04}s both`,
                            transition: "border-color 0.15s",
                          }}>
                            <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 5 }}>
                              <div style={{
                                width: 5, height: 5, borderRadius: "50%", flexShrink: 0,
                                background: s.color, boxShadow: `0 0 6px ${s.color}`,
                                animation: sev === "critical" ? "blink 1.1s infinite" : "none",
                              }} />
                              <span style={{ fontSize: 9, color: s.color, fontFamily: "var(--mono)", fontWeight: 700, letterSpacing: "0.1em" }}>
                                {sev.toUpperCase()}
                              </span>
                              <span style={{ fontSize: 9, color: "#1f2937", fontFamily: "var(--mono)", marginLeft: "auto" }}>
                                {ev.timestamp ? timeAgo(ev.timestamp) : ev.created_at ? timeAgo(ev.created_at) : "now"}
                              </span>
                            </div>
                            <div style={{
                              fontSize: 11, color: "#dde4f0", fontFamily: "var(--mono)",
                              lineHeight: 1.45, marginBottom: 5,
                            }}>
                              {ev.message || ev.name || "—"}
                            </div>
                            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                              <span style={{ fontSize: 9, color: "#4b5563", fontFamily: "var(--mono)" }}>
                                {ev.service}
                              </span>
                              {ev.id && (
                                <span style={{ fontSize: 8, color: "#1f2937", fontFamily: "var(--mono)" }}>
                                  #{ev.id}
                                </span>
                              )}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* ── STATUS FOOTER ── */}
          <footer style={{
            height: 27, background: "rgba(8,9,14,0.96)",
            borderTop: "1px solid rgba(255,255,255,0.04)",
            display: "flex", alignItems: "center", padding: "0 20px", gap: 18,
          }}>
            {[
              { label: "API",    status: "CONNECTED", ok: true  },
              { label: "ENGINE", status: "RUNNING",   ok: true  },
              { label: "POLL",   status: "30s",       ok: true  },
            ].map((item, i) => (
              <div key={i} style={{ display: "flex", alignItems: "center", gap: 5 }}>
                <div style={{ width: 4, height: 4, borderRadius: "50%", background: item.ok ? "#00e5a0" : "#ff4d6a" }} />
                <span style={{ fontSize: 8, fontFamily: "var(--mono)", color: "#374151", letterSpacing: "0.1em" }}>{item.label}</span>
                <span style={{ fontSize: 8, fontFamily: "var(--mono)", color: item.ok ? "#00e5a0" : "#ff4d6a" }}>{item.status}</span>
              </div>
            ))}
            <span style={{ marginLeft: "auto", fontSize: 8, fontFamily: "var(--mono)", color: "#1f2937" }}>
              © AIOPS PLATFORM · v1.0.0
            </span>
          </footer>
        </main>
      </div>
    </>
  );
}
