// GapFeatures.jsx — All gap-filling feature panels
// Business Impact, Alert Feedback, Auto-Resolve, Runbooks, Dependencies, WhatsApp, Compliance

import { useState, useEffect, useCallback } from "react";
import * as api from "../api/gaps.js";

/* ═══════════════════════════════════════════════════════
   1. Business Impact Panel (in incident detail)
   ═══════════════════════════════════════════════════════ */
export function BusinessImpactPanel({ incidentID }) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [generating, setGenerating] = useState(false);
  const [generateError, setGenerateError] = useState("");

  const load = useCallback(async () => {
    if (!incidentID) return;
    setLoading(true);
    try { const d = await api.getBusinessImpact(incidentID); setData(d); }
    catch { setData(null); }
    finally { setLoading(false); }
  }, [incidentID]);

  useEffect(() => { load(); }, [load]);

  async function handleGenerate() {
    setGenerating(true);
    setGenerateError("");
    try { const d = await api.generateBusinessImpact(incidentID); setData(d); }
    catch (e) { setGenerateError("Failed: " + e.message); }
    finally { setGenerating(false); }
  }

  const fmtUSD = (v) => {
    if (!v || v <= 0) return "$0";
    if (v >= 1000000) return `$${(v / 1000000).toFixed(1)}M`;
    if (v >= 1000) return `$${(v / 1000).toFixed(1)}K`;
    return `$${v.toFixed(0)}`;
  };

  const CONF_COLOR = { HIGH: "#10b981", MEDIUM: "#f59e0b", LOW: "#6b7280" };

  if (loading) return <div className="lux-muted" style={{ padding: "1rem" }}>Loading financial impact...</div>;

  const est = data?.estimate;
  const bd = data?.breakdown;
  const prof = data?.profile;

  return (
    <div className="gap-panel">
      <div className="gap-panel-header">
        <div>
          <div className="lux-eyebrow">FINANCIAL IMPACT ENGINE</div>
          <div className="slo-service">Real financial estimation — actual loss, counterfactual loss, avoided loss</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", alignItems: "flex-end", gap: "0.3rem" }}>
          <button className="lux-primary-btn small" onClick={handleGenerate} disabled={generating}>
            {generating ? "Calculating..." : est ? "Recalculate" : "Calculate Impact"}
          </button>
          {generateError && (
            <div style={{ fontSize: "0.8rem", color: "#fca5a5" }}>{generateError}</div>
          )}
        </div>
      </div>

      {!est && !generating && (
        <div className="gap-empty">Click "Calculate Impact" to compute financial impact for this incident.</div>
      )}
      {generating && <div className="gap-empty">Computing financial model...</div>}

      {est && !generating && (
        <>
          {/* Hero: 3 big numbers */}
          <div className="biz-hero-grid">
            <div className="biz-hero-card biz-actual">
              <div className="biz-hero-label">Actual Loss</div>
              <div className="biz-hero-val">{fmtUSD(est.actual_loss)}</div>
              <div className="biz-hero-sub">What this incident actually cost</div>
            </div>
            <div className="biz-hero-card biz-counter">
              <div className="biz-hero-label">Without Intervention</div>
              <div className="biz-hero-val">{fmtUSD(est.counterfactual_loss)}</div>
              <div className="biz-hero-sub">What it would have cost unresolved</div>
            </div>
            <div className="biz-hero-card biz-saved">
              <div className="biz-hero-label">Money Saved</div>
              <div className="biz-hero-val">{fmtUSD(est.avoided_loss)}</div>
              <div className="biz-hero-sub">Your team's response value</div>
            </div>
          </div>

          {/* Confidence + Method */}
          <div className="biz-meta-row">
            <span className="lux-mini-chip" style={{ color: CONF_COLOR[est.confidence_level] || "#6b7280" }}>
              {est.confidence_level} CONFIDENCE
            </span>
            <span className="slo-service">Method: {est.method_used?.replace(/_/g, " ")}</span>
            <span className="slo-service">Currency: {est.currency || "USD"}</span>
          </div>

          {/* Explanation */}
          {est.explanation && (
            <div className="biz-explanation">
              <div className="gap-card-label">Explanation</div>
              <div className="biz-explain-text">{est.explanation}</div>
            </div>
          )}

          {/* Breakdown */}
          {bd && (
            <div className="biz-breakdown">
              <div className="gap-card-label" style={{ marginBottom: "0.5rem" }}>CALCULATION BREAKDOWN</div>
              <div className="biz-bd-grid">
                <div className="biz-bd-col">
                  <div className="biz-bd-title">Actual Loss</div>
                  <div className="biz-bd-row"><span>Duration</span><span>{bd.actual_duration_minutes?.toFixed(0) || "0"} min</span></div>
                  <div className="biz-bd-row"><span>Revenue Loss</span><span>{fmtUSD(bd.actual_revenue_loss)}</span></div>
                  <div className="biz-bd-row"><span>SLA Penalty</span><span>{fmtUSD(bd.actual_sla_penalty)}</span></div>
                  <div className="biz-bd-row"><span>Productivity Loss</span><span>{fmtUSD(bd.actual_productivity_loss)}</span></div>
                  <div className="biz-bd-row"><span>Infra Cost</span><span>{fmtUSD(bd.actual_infra_loss)}</span></div>
                  <div className="biz-bd-row biz-bd-total"><span>Total</span><span>{fmtUSD(bd.total_actual_loss)}</span></div>
                </div>
                <div className="biz-bd-col">
                  <div className="biz-bd-title">Counterfactual Loss</div>
                  <div className="biz-bd-row"><span>Duration (baseline)</span><span>{bd.counterfactual_duration_minutes?.toFixed(0) || "0"} min</span></div>
                  <div className="biz-bd-row"><span>Phase 1 ({(bd.phase1_impact * 100)?.toFixed(0)}% impact)</span><span>{bd.phase1_minutes?.toFixed(0)} min</span></div>
                  <div className="biz-bd-row"><span>Phase 2 ({(bd.phase2_impact * 100)?.toFixed(0)}% impact)</span><span>{bd.phase2_minutes?.toFixed(0)} min</span></div>
                  <div className="biz-bd-row"><span>Revenue Loss</span><span>{fmtUSD(bd.counterfactual_revenue_loss)}</span></div>
                  <div className="biz-bd-row"><span>SLA Penalty</span><span>{fmtUSD(bd.counterfactual_sla_penalty)}</span></div>
                  <div className="biz-bd-row"><span>Productivity + Infra</span><span>{fmtUSD((bd.counterfactual_productivity_loss || 0) + (bd.counterfactual_infra_loss || 0))}</span></div>
                  <div className="biz-bd-row biz-bd-total"><span>Total</span><span>{fmtUSD(bd.total_counterfactual_loss)}</span></div>
                </div>
              </div>
            </div>
          )}

          {/* Risk Projections: if unresolved */}
          {(data?.projections || []).length > 0 && (
            <div className="biz-projections">
              <div className="gap-card-label">IF UNRESOLVED — ESCALATION RISK</div>
              <div className="biz-proj-grid">
                {data.projections.map((p, i) => (
                  <div key={i} className="biz-proj-card">
                    <div className="biz-proj-label">{p.label}</div>
                    <div className="biz-proj-val">{fmtUSD(p.estimated_loss)}</div>
                    <div className="biz-proj-sub">total exposure</div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Multipliers */}
          {bd && (
            <div className="biz-meta-row" style={{ marginTop: "0.5rem" }}>
              <span className="slo-service">Severity multiplier: {bd.severity_multiplier}x</span>
              <span className="slo-service">Tier multiplier: {bd.tier_multiplier}x</span>
            </div>
          )}

          {/* Profile info */}
          {prof && (
            <div className="biz-profile-info">
              <div className="gap-card-label">SERVICE PROFILE</div>
              <div className="biz-profile-grid">
                <span>Tier: {prof.tier}</span>
                <span>Revenue: {fmtUSD(prof.hourly_revenue)}/hr</span>
                <span>SLA: {fmtUSD(prof.sla_penalty_per_minute)}/min after {prof.sla_threshold_minutes}min</span>
                <span>Users: {prof.users_per_hour?.toFixed(0)}/hr</span>
                <span>Infra: {fmtUSD(prof.infra_cost_per_hour)}/hr</span>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════
   2. Alert Feedback Panel
   ═══════════════════════════════════════════════════════ */
export function AlertFeedbackPanel() {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    try { const d = await api.getAlertFeedbackStats(); setStats(d); }
    catch { setStats(null); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  return (
    <div className="gap-panel">
      <div className="gap-panel-header">
        <div>
          <div className="lux-eyebrow">ALERT FEEDBACK LOOP</div>
          <div className="slo-service">Mark alerts as useful or noise. Noisy alerts get auto-suppressed.</div>
        </div>
        <button className="lux-secondary-btn small" onClick={load}>Refresh</button>
      </div>
      {loading ? <div className="lux-muted">Loading...</div> : !stats ? (
        <div className="gap-empty">No feedback data yet. Mark alerts as useful/noise from the incident timeline.</div>
      ) : (
        <>
          <div className="p3-kpi-strip">
            <div className="p3-kpi"><div className="p3-kpi-val">{stats.total_feedback || 0}</div><div className="p3-kpi-label">Total Feedback</div></div>
            <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#10b981" }}>{stats.useful_count || 0}</div><div className="p3-kpi-label">Useful</div></div>
            <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#ef4444" }}>{stats.noise_count || 0}</div><div className="p3-kpi-label">Noise</div></div>
            <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#f59e0b" }}>{stats.suppressed_count || 0}</div><div className="p3-kpi-label">Auto-Suppressed</div></div>
          </div>
          {(stats.top_noisy || []).length > 0 && (
            <div style={{ marginTop: "1rem" }}>
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>TOP NOISY ALERTS (auto-suppressed after 5+ noise marks)</div>
              <div className="anomaly-list">
                {stats.top_noisy.map((n, i) => (
                  <div key={i} className="gap-noisy-row">
                    <span className="gap-fp">{n.fingerprint?.slice(0, 12)}...</span>
                    <span>{n.service || "—"}</span>
                    <span className="lux-mini-chip" style={{ color: "#ef4444" }}>{n.noise_count} noise</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════
   3. Alert Feedback Buttons (inline in timeline)
   ═══════════════════════════════════════════════════════ */
export function AlertFeedbackButtons({ eventID, incidentID }) {
  const [sent, setSent] = useState(null);

  async function send(feedback) {
    try {
      await api.submitAlertFeedback({ event_id: eventID, incident_id: incidentID, feedback });
      setSent(feedback);
    } catch { /* ignore — feedback is best-effort */ }
  }

  if (sent) return <span className="gap-fb-sent">{sent === "useful" ? "👍" : "🔇"} {sent}</span>;
  return (
    <span className="gap-fb-btns">
      <button className="gap-fb-btn useful" onClick={() => send("useful")} title="Useful alert">👍</button>
      <button className="gap-fb-btn noise" onClick={() => send("noise")} title="Noise — suppress similar">🔇</button>
    </span>
  );
}

/* ═══════════════════════════════════════════════════════
   4. Auto-Resolve Rules Panel
   ═══════════════════════════════════════════════════════ */
export function AutoResolvePanel() {
  const [rules, setRules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [createError, setCreateError] = useState("");
  const [form, setForm] = useState({ name: "", pattern: "", service: "", severity: "", action: "resolve", cooldown_minutes: 30 });

  const load = useCallback(async () => {
    try { const d = await api.getAutoResolveRules(); setRules(Array.isArray(d) ? d : []); }
    catch { setRules([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleCreate(e) {
    e.preventDefault();
    setCreateError("");
    try {
      await api.createAutoResolveRule({ ...form, cooldown_minutes: parseInt(form.cooldown_minutes) });
      setShowForm(false);
      setForm({ name: "", pattern: "", service: "", severity: "", action: "resolve", cooldown_minutes: 30 });
      load();
    } catch (err) { setCreateError("Failed: " + err.message); }
  }

  async function handleToggle(id, enabled) {
    try { await api.toggleAutoResolveRule(id, !enabled); load(); } catch { /* ignore — list refreshes on retry */ }
  }

  async function handleDelete(id) {
    try { await api.deleteAutoResolveRule(id); load(); } catch { /* ignore — list refreshes on retry */ }
  }

  return (
    <div className="gap-panel">
      <div className="gap-panel-header">
        <div>
          <div className="lux-eyebrow">AUTO-RESOLUTION ENGINE</div>
          <div className="slo-service">Define rules to auto-resolve known issues. No 2am pages for known problems.</div>
        </div>
        <button className="lux-primary-btn small" onClick={() => setShowForm(!showForm)}>
          {showForm ? "Cancel" : "+ New Rule"}
        </button>
      </div>

      {showForm && (
        <form className="p3-form" onSubmit={handleCreate}>
          <div className="p3-form-grid">
            <label><span className="pm-label">Rule Name</span><input className="pm-input" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} required /></label>
            <label><span className="pm-label">Pattern (substring match)</span><input className="pm-input" value={form.pattern} onChange={e => setForm({ ...form, pattern: e.target.value })} placeholder="e.g. connection timeout" /></label>
            <label><span className="pm-label">Service (empty=all)</span><input className="pm-input" value={form.service} onChange={e => setForm({ ...form, service: e.target.value })} /></label>
            <label><span className="pm-label">Severity (empty=all)</span>
              <select className="pm-input" value={form.severity} onChange={e => setForm({ ...form, severity: e.target.value })}>
                <option value="">Any</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
              </select>
            </label>
            <label><span className="pm-label">Action</span>
              <select className="pm-input" value={form.action} onChange={e => setForm({ ...form, action: e.target.value })}>
                <option value="resolve">Auto-Resolve</option><option value="ack">Auto-Acknowledge</option>
              </select>
            </label>
            <label><span className="pm-label">Cooldown (min)</span><input className="pm-input" type="number" value={form.cooldown_minutes} onChange={e => setForm({ ...form, cooldown_minutes: e.target.value })} /></label>
          </div>
          {createError && (
            <div style={{ marginTop: "0.4rem", fontSize: "0.82rem", color: "#fca5a5" }}>{createError}</div>
          )}
          <button className="lux-primary-btn" type="submit" style={{ marginTop: "0.75rem" }}>Create Rule</button>
        </form>
      )}

      {loading ? <div className="lux-muted">Loading...</div> : rules.length === 0 ? (
        <div className="gap-empty">No auto-resolve rules yet. Create one to stop being paged for known issues.</div>
      ) : (
        <div className="anomaly-list">
          {rules.map(r => (
            <div key={r.id} className="gap-rule-card">
              <div className="gap-rule-top">
                <div>
                  <div style={{ fontWeight: 600 }}>{r.name}</div>
                  <div className="slo-service">
                    {r.service || "any service"} · {r.severity || "any severity"} · pattern: "{r.pattern || "*"}" · {r.action}
                  </div>
                </div>
                <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
                  <span className="slo-service">{r.times_fired}x fired</span>
                  <button className={`lux-secondary-btn small ${r.enabled ? "" : "gap-disabled"}`}
                    onClick={() => handleToggle(r.id, r.enabled)}>
                    {r.enabled ? "Enabled" : "Disabled"}
                  </button>
                  <button className="lux-secondary-btn small" onClick={() => handleDelete(r.id)}>Delete</button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════
   5. Runbook Knowledge Base
   ═══════════════════════════════════════════════════════ */
export function RunbookPanel() {
  const [runbooks, setRunbooks] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [search, setSearch] = useState("");
  const [form, setForm] = useState({ service: "", title: "", content: "", tags: "", severity_match: "", pattern_match: "" });
  const [editID, setEditID] = useState(null);

  const load = useCallback(async () => {
    try { const d = await api.getRunbooks("", search); setRunbooks(Array.isArray(d) ? d : []); }
    catch { setRunbooks([]); }
    finally { setLoading(false); }
  }, [search]);

  useEffect(() => { load(); }, [load]);

  async function handleSave(e) {
    e.preventDefault();
    setSaveError("");
    try {
      if (editID) { await api.updateRunbook(editID, form); }
      else { await api.createRunbook(form); }
      setShowForm(false); setEditID(null);
      setForm({ service: "", title: "", content: "", tags: "", severity_match: "", pattern_match: "" });
      load();
    } catch (err) { setSaveError("Failed: " + err.message); }
  }

  function startEdit(rb) {
    setForm({ service: rb.service, title: rb.title, content: rb.content, tags: rb.tags, severity_match: rb.severity_match, pattern_match: rb.pattern_match });
    setEditID(rb.id);
    setShowForm(true);
  }

  async function handleDelete(id) {
    try { await api.deleteRunbook(id); load(); } catch { /* ignore — list refreshes on retry */ }
  }

  return (
    <div className="gap-panel">
      <div className="gap-panel-header">
        <div>
          <div className="lux-eyebrow">RUNBOOK KNOWLEDGE BASE</div>
          <div className="slo-service">Capture tribal knowledge. New engineers find answers instantly. AI links relevant runbooks to incidents.</div>
        </div>
        <button className="lux-primary-btn small" onClick={() => { setShowForm(!showForm); setEditID(null); }}>
          {showForm ? "Cancel" : "+ New Runbook"}
        </button>
      </div>

      <div style={{ display: "flex", gap: "0.5rem" }}>
        <input className="pm-input" placeholder="Search runbooks..." value={search}
          onChange={e => setSearch(e.target.value)} style={{ maxWidth: 300 }} />
      </div>

      {showForm && (
        <form className="p3-form" onSubmit={handleSave}>
          <div className="p3-form-grid">
            <label><span className="pm-label">Title</span><input className="pm-input" value={form.title} onChange={e => setForm({ ...form, title: e.target.value })} required /></label>
            <label><span className="pm-label">Service</span><input className="pm-input" value={form.service} onChange={e => setForm({ ...form, service: e.target.value })} /></label>
            <label><span className="pm-label">Tags (comma-sep)</span><input className="pm-input" value={form.tags} onChange={e => setForm({ ...form, tags: e.target.value })} placeholder="database, timeout, redis" /></label>
            <label><span className="pm-label">Severity Match</span>
              <select className="pm-input" value={form.severity_match} onChange={e => setForm({ ...form, severity_match: e.target.value })}>
                <option value="">Any</option><option value="low">Low</option><option value="medium">Medium</option><option value="high">High</option><option value="critical">Critical</option>
              </select>
            </label>
          </div>
          <label style={{ marginTop: "0.5rem", display: "block" }}><span className="pm-label">Content (Markdown)</span>
            <textarea className="pm-textarea" rows={8} value={form.content} onChange={e => setForm({ ...form, content: e.target.value })}
              placeholder={"## Steps\n1. Check database connectivity\n2. Verify connection pool settings\n3. ..."} required />
          </label>
          {saveError && (
            <div style={{ marginTop: "0.4rem", fontSize: "0.82rem", color: "#fca5a5" }}>{saveError}</div>
          )}
          <button className="lux-primary-btn" type="submit" style={{ marginTop: "0.75rem" }}>{editID ? "Update" : "Create"}</button>
        </form>
      )}

      {loading ? <div className="lux-muted">Loading...</div> : runbooks.length === 0 ? (
        <div className="gap-empty">No runbooks yet. Capture your team's tribal knowledge so it's never lost.</div>
      ) : (
        <div className="anomaly-list">
          {runbooks.map(rb => (
            <div key={rb.id} className="gap-runbook-card">
              <div className="gap-rule-top">
                <div>
                  <div style={{ fontWeight: 600 }}>{rb.title}</div>
                  <div className="slo-service">{rb.service || "all services"} · {rb.tags || "no tags"}</div>
                </div>
                <div style={{ display: "flex", gap: "0.5rem" }}>
                  <button className="lux-secondary-btn small" onClick={() => startEdit(rb)}>Edit</button>
                  <button className="lux-secondary-btn small" onClick={() => handleDelete(rb.id)}>Delete</button>
                </div>
              </div>
              <pre className="gap-runbook-content">{rb.content}</pre>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════
   6. Dependency Catalog + Vendor Attribution
   ═══════════════════════════════════════════════════════ */
export function DependencyPanel() {
  const [deps, setDeps] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [createError, setCreateError] = useState("");
  const [form, setForm] = useState({ service: "", dependency_name: "", dependency_type: "internal", vendor: "", health_url: "", status_page_url: "" });

  const load = useCallback(async () => {
    try { const d = await api.getDependencies(); setDeps(Array.isArray(d) ? d : []); }
    catch { setDeps([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleCreate(e) {
    e.preventDefault();
    setCreateError("");
    try { await api.addDependency(form); setShowForm(false); setForm({ service: "", dependency_name: "", dependency_type: "internal", vendor: "", health_url: "", status_page_url: "" }); load(); }
    catch (err) { setCreateError("Failed: " + err.message); }
  }

  async function handleDelete(id) { try { await api.deleteDependency(id); load(); } catch { /* ignore — list refreshes on retry */ } }

  const TYPE_COLOR = { internal: "#818cf8", vendor: "#f59e0b", third_party: "#ef4444" };

  return (
    <div className="gap-panel">
      <div className="gap-panel-header">
        <div>
          <div className="lux-eyebrow">DEPENDENCY CATALOG</div>
          <div className="slo-service">Track internal and vendor dependencies. Know instantly if it's your failure or theirs.</div>
        </div>
        <button className="lux-primary-btn small" onClick={() => setShowForm(!showForm)}>
          {showForm ? "Cancel" : "+ Add Dependency"}
        </button>
      </div>

      {showForm && (
        <form className="p3-form" onSubmit={handleCreate}>
          <div className="p3-form-grid">
            <label><span className="pm-label">Your Service</span><input className="pm-input" value={form.service} onChange={e => setForm({ ...form, service: e.target.value })} required /></label>
            <label><span className="pm-label">Dependency Name</span><input className="pm-input" value={form.dependency_name} onChange={e => setForm({ ...form, dependency_name: e.target.value })} required /></label>
            <label><span className="pm-label">Type</span>
              <select className="pm-input" value={form.dependency_type} onChange={e => setForm({ ...form, dependency_type: e.target.value })}>
                <option value="internal">Internal</option><option value="vendor">Vendor</option><option value="third_party">Third-Party</option>
              </select>
            </label>
            <label><span className="pm-label">Vendor Name</span><input className="pm-input" value={form.vendor} onChange={e => setForm({ ...form, vendor: e.target.value })} placeholder="e.g. AWS, Stripe" /></label>
            <label><span className="pm-label">Health Check URL</span><input className="pm-input" value={form.health_url} onChange={e => setForm({ ...form, health_url: e.target.value })} /></label>
            <label><span className="pm-label">Status Page URL</span><input className="pm-input" value={form.status_page_url} onChange={e => setForm({ ...form, status_page_url: e.target.value })} /></label>
          </div>
          {createError && (
            <div style={{ marginTop: "0.4rem", fontSize: "0.82rem", color: "#fca5a5" }}>{createError}</div>
          )}
          <button className="lux-primary-btn" type="submit" style={{ marginTop: "0.75rem" }}>Add</button>
        </form>
      )}

      {loading ? <div className="lux-muted">Loading...</div> : deps.length === 0 ? (
        <div className="gap-empty">No dependencies cataloged. Add your vendor and internal dependencies to enable automatic attribution.</div>
      ) : (
        <div className="anomaly-list">
          {deps.map(d => (
            <div key={d.id} className="gap-dep-card">
              <div className="gap-rule-top">
                <div style={{ display: "flex", alignItems: "center", gap: "0.75rem" }}>
                  <span className="lux-mini-chip" style={{ color: TYPE_COLOR[d.dependency_type] || "#6b7280" }}>
                    {(d.dependency_type || "unknown").toUpperCase()}
                  </span>
                  <div>
                    <div style={{ fontWeight: 600 }}>{d.dependency_name}</div>
                    <div className="slo-service">{d.service} → {d.dependency_name}{d.vendor ? ` (${d.vendor})` : ""}</div>
                  </div>
                </div>
                <button className="lux-secondary-btn small" onClick={() => handleDelete(d.id)}>Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

/* ═══════════════════════════════════════════════════════
   7. Compliance Report Panel
   ═══════════════════════════════════════════════════════ */
export function CompliancePanel() {
  const [report, setReport] = useState(null);
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(30);
  const [exportError, setExportError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try { const d = await api.getComplianceReport(days); setReport(d); }
    catch { setReport(null); }
    finally { setLoading(false); }
  }, [days]);

  useEffect(() => { load(); }, [load]);

  async function handleExport() {
    setExportError("");
    try {
      const blob = await api.exportComplianceCSV(days);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url; a.download = `compliance-report-${days}d.csv`; a.click();
      URL.revokeObjectURL(url);
    } catch (e) { setExportError("Export failed: " + e.message); }
  }

  return (
    <div className="gap-panel">
      <div className="gap-panel-header">
        <div>
          <div className="lux-eyebrow">COMPLIANCE & SLA REPORTING</div>
          <div className="slo-service">No more copy-paste from dashboards to Excel. Export-ready compliance reports.</div>
        </div>
        <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem", alignItems: "flex-end" }}>
          <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
            <select className="pm-input" value={days} onChange={e => setDays(parseInt(e.target.value))} style={{ width: "auto" }}>
              <option value={7}>7 days</option><option value={14}>14 days</option><option value={30}>30 days</option><option value={90}>90 days</option>
            </select>
            <button className="lux-secondary-btn small" onClick={load}>Refresh</button>
            <button className="lux-primary-btn small" onClick={handleExport}>Export CSV</button>
          </div>
          {exportError && (
            <div style={{ fontSize: "0.8rem", color: "#fca5a5" }}>{exportError}</div>
          )}
        </div>
      </div>

      {loading ? <div className="lux-muted">Loading...</div> : !report ? (
        <div className="gap-empty">No compliance data available yet.</div>
      ) : (
        <>
          <div className="p3-kpi-strip">
            <div className="p3-kpi"><div className="p3-kpi-val">{report.total_incidents}</div><div className="p3-kpi-label">Incidents</div></div>
            <div className="p3-kpi"><div className="p3-kpi-val">{report.mttr_seconds > 0 ? `${(report.mttr_seconds / 60).toFixed(0)}m` : "—"}</div><div className="p3-kpi-label">MTTR</div></div>
            <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: "#10b981" }}>{(report.uptime_percent || 0).toFixed(3)}%</div><div className="p3-kpi-label">Uptime</div></div>
            <div className="p3-kpi"><div className="p3-kpi-val" style={{ color: report.breached_slos > 0 ? "#ef4444" : "#10b981" }}>{report.breached_slos}</div><div className="p3-kpi-label">SLO Breaches</div></div>
          </div>

          {(report.slo_compliance || []).length > 0 && (
            <div style={{ marginTop: "1rem" }}>
              <div className="lux-eyebrow" style={{ marginBottom: "0.5rem" }}>SLO COMPLIANCE</div>
              <div className="eh-table">
                <div className="eh-table-head" style={{ gridTemplateColumns: "2fr 1.5fr 1fr 1fr 1fr 1fr" }}>
                  <span>SLO</span><span>Service</span><span>Target</span><span>Actual</span><span>Compliant</span><span>Budget Used</span>
                </div>
                {report.slo_compliance.map((line, i) => (
                  <div key={i} className="eh-table-row" style={{ gridTemplateColumns: "2fr 1.5fr 1fr 1fr 1fr 1fr" }}>
                    <span className="eh-name">{line.slo_name}</span>
                    <span>{line.service}</span>
                    <span>{line.target_percent?.toFixed(1)}%</span>
                    <span>{line.actual_percent?.toFixed(2)}%</span>
                    <span style={{ color: line.compliant ? "#10b981" : "#ef4444" }}>{line.compliant ? "YES" : "NO"}</span>
                    <span>{line.error_budget_used_percent?.toFixed(1)}%</span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
