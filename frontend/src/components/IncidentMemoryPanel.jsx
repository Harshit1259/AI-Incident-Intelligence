import { useState, useEffect } from "react";
import {
  getMemoryContext,
  recordResolution,
  listRemediationPatterns,
  createRemediationPattern,
  updateRemediationPattern,
  deleteRemediationPattern,
  markRemediationOutcome,
  listDeploySignatures,
  createDeploySignature,
  updateDeploySignature,
  deleteDeploySignature,
  listRunbookPreferences,
  createRunbookPreference,
  updateRunbookPreference,
  deleteRunbookPreference,
} from "../api/domainMemory.js";

const TABS = [
  { key: "context",     label: "Memory Context", incidentOnly: true },
  { key: "remediation", label: "Remediations" },
  { key: "deploy",      label: "Deploy Signatures" },
  { key: "runbook",     label: "Runbook Preferences" },
];

const SEVERITIES = ["critical", "high", "medium", "low"];

export default function IncidentMemoryPanel({ incidentId }) {
  // When opened from the nav (no incident), Memory Context has nothing to show —
  // land on Remediations, the knowledge base that works standalone.
  const [tab, setTab] = useState(incidentId ? "context" : "remediation");

  return (
    <div style={panelStyle}>
      <div style={{ marginBottom: "18px" }}>
        <div className="lux-eyebrow" style={{ marginBottom: 4 }}>AI Intelligence</div>
        <h2 style={{ margin: 0, fontSize: "22px", fontWeight: 800, letterSpacing: "-0.02em", color: "var(--t1)" }}>Domain Memory</h2>
        <p style={{ margin: "5px 0 0", fontSize: "13px", color: "var(--t3)" }}>
          Operational memory the AI builds over time: past patterns, proven fixes, deploy risks, team preferences.
          {incidentId && (
            <> Incident: <code style={{ color: "var(--cyan)" }}>{incidentId}</code></>
          )}
        </p>
      </div>

      <div style={{ display: "flex", gap: "6px", marginBottom: "20px", flexWrap: "wrap" }}>
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            style={tabBtn(tab === t.key)}
            title={t.incidentOnly && !incidentId ? "Open from within an incident for live context" : undefined}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === "context"     && <ContextTab incidentId={incidentId} />}
      {tab === "remediation" && <RemediationTab />}
      {tab === "deploy"      && <DeployTab />}
      {tab === "runbook"     && <RunbookTab />}
    </div>
  );
}

// ── Memory Context Tab ────────────────────────────────────────────────────────

function ContextTab({ incidentId }) {
  const [ctx, setCtx] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [showRecordForm, setShowRecordForm] = useState(false);
  const [recordForm, setRecordForm] = useState({
    resolution_note: "",
    error_signature: "",
    root_cause_category: "",
    remediation_summary: "",
    remediation_steps: "",
    was_successful: true,
    deploy_signature_name: "",
    deploy_indicators: "",
    impacted_services: "",
  });
  const [recording, setRecording] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    if (!incidentId) return;
    let active = true;
    setLoading(true);
    setError("");
    async function load() {
      try {
        const data = await getMemoryContext(incidentId);
        if (active) { setCtx(data); setLoading(false); }
      } catch (e) { if (active) { setError(e.message); setLoading(false); } }
    }
    load();
    return () => { active = false; };
  }, [incidentId, refreshKey]);

  async function handleRecord() {
    setRecording(true);
    try {
      const payload = {
        ...recordForm,
        remediation_steps: recordForm.remediation_steps.split("\n").map((s) => s.trim()).filter(Boolean),
        deploy_indicators: recordForm.deploy_indicators.split(",").map((s) => s.trim()).filter(Boolean),
        impacted_services: recordForm.impacted_services.split(",").map((s) => s.trim()).filter(Boolean),
      };
      await recordResolution(incidentId, payload);
      setShowRecordForm(false);
      reload();
    } catch (e) { setError(e.message); }
    finally { setRecording(false); }
  }

  if (!incidentId) return <Notice>Memory Context is incident-specific — open it from within an incident to see its synthesized history. The Remediations, Deploy Signatures, and Runbook Preferences tabs work standalone.</Notice>;
  if (loading) return <Notice>Loading AI memory context…</Notice>;

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}

      <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: "14px" }}>
        <button onClick={() => setShowRecordForm(true)} style={btn("blue")}>
          📝 Record Resolution Learning
        </button>
      </div>

      {ctx ? (
        <div style={{ display: "grid", gap: "16px" }}>

          {/* LLM Narrative */}
          {ctx.llm_narrative && (
            <div style={{ ...card, borderLeft: "3px solid var(--cyan)", background: "var(--cyan-dim)" }}>
              <div style={{ ...sectionTitle, color: "var(--cyan)" }}>AI Synthesis</div>
              <p style={{ margin: "8px 0 0", color: "var(--t1)", fontSize: "14px", lineHeight: 1.7 }}>
                {ctx.llm_narrative}
              </p>
            </div>
          )}

          {/* Recurrence + playbook */}
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
            <div style={card}>
              <div style={sectionTitle}>Recurrence</div>
              <div style={{ fontSize: "32px", fontWeight: 800, color: ctx.recurrence_count > 3 ? "var(--red)" : ctx.recurrence_count > 0 ? "var(--amber)" : "var(--green)", marginTop: "6px" }}>
                {ctx.recurrence_count}×
              </div>
              <div style={{ fontSize: "12px", color: "var(--t3)" }}>times this pattern appeared</div>
            </div>
            <div style={card}>
              <div style={sectionTitle}>Suggested Playbook</div>
              {ctx.suggested_playbook?.length > 0
                ? <ol style={{ margin: "8px 0 0 16px", color: "var(--t1)", fontSize: "13px", lineHeight: 1.8 }}>
                    {ctx.suggested_playbook.map((step, i) => <li key={i}>{step}</li>)}
                  </ol>
                : <div style={{ color: "var(--t4)", fontSize: "13px", marginTop: "8px" }}>No playbook yet</div>
              }
            </div>
          </div>

          {/* Past incidents */}
          {ctx.similar_past_incidents?.length > 0 && (
            <div style={card}>
              <div style={sectionTitle}>Past Incidents ({ctx.similar_past_incidents.length})</div>
              <div style={{ marginTop: "8px", display: "grid", gap: "8px" }}>
                {ctx.similar_past_incidents.slice(0, 5).map((r) => (
                  <div key={r.incident_id} style={{ background: "var(--surface-2)", borderRadius: "6px", padding: "10px 14px" }}>
                    <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "4px" }}>
                      <code style={{ color: "var(--cyan)", fontSize: "12px" }}>{r.incident_id}</code>
                      <span style={{ color: "var(--t3)", fontSize: "12px" }}>
                        TTR: {r.ttr_seconds}s · {r.resolved_at?.slice(0, 10)}
                      </span>
                    </div>
                    {r.resolution_note && (
                      <div style={{ color: "var(--t2)", fontSize: "12px" }}>{r.resolution_note}</div>
                    )}
                    {r.actions_taken?.length > 0 && (
                      <div style={{ color: "var(--t3)", fontSize: "11px", marginTop: "4px" }}>
                        Actions: {r.actions_taken.join(" → ")}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Remediation suggestions */}
          {ctx.remediation_suggestions?.length > 0 && (
            <div style={card}>
              <div style={sectionTitle}>Proven Remediations</div>
              <div style={{ marginTop: "8px", display: "grid", gap: "8px" }}>
                {ctx.remediation_suggestions.map((r) => (
                  <div key={r.id} style={{ background: "var(--surface-2)", borderRadius: "6px", padding: "10px 14px" }}>
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
                      <div>
                        <span style={{ fontWeight: 600, color: "var(--t1)", fontSize: "13px" }}>{r.remediation_summary}</span>
                        {r.root_cause_category && (
                          <span style={{ ...badge("purple"), marginLeft: "8px" }}>{r.root_cause_category}</span>
                        )}
                      </div>
                      <span style={{ ...badge(successColor(r.success_rate)), flexShrink: 0 }}>
                        {Math.round((r.success_rate || 0) * 100)}% success
                      </span>
                    </div>
                    {r.remediation_steps?.length > 0 && (
                      <ol style={{ margin: "6px 0 0 16px", color: "var(--t2)", fontSize: "12px", lineHeight: 1.7 }}>
                        {r.remediation_steps.map((s, i) => <li key={i}>{s}</li>)}
                      </ol>
                    )}
                    <div style={{ fontSize: "11px", color: "var(--t4)", marginTop: "6px" }}>
                      {r.success_count} successes · {r.failure_count} failures · last used {r.last_used_at?.slice(0, 10)}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Deploy risks */}
          {ctx.deploy_risks?.length > 0 && (
            <div style={card}>
              <div style={sectionTitle}>Known Deploy Risk Signals</div>
              <div style={{ marginTop: "8px", display: "grid", gap: "8px" }}>
                {ctx.deploy_risks.map((d) => (
                  <div key={d.id} style={{ background: "var(--surface-2)", borderRadius: "6px", padding: "10px 14px", borderLeft: "3px solid var(--orange)" }}>
                    <div style={{ display: "flex", justifyContent: "space-between" }}>
                      <span style={{ fontWeight: 600, color: "var(--t1)", fontSize: "13px" }}>{d.signature_name}</span>
                      <div>
                        <span style={{ ...badge("orange"), marginRight: "6px" }}>{d.typical_severity}</span>
                        <span style={{ fontSize: "11px", color: "var(--t3)" }}>{d.occurrence_count}× seen</span>
                      </div>
                    </div>
                    {d.description && (
                      <div style={{ color: "var(--t2)", fontSize: "12px", marginTop: "4px" }}>{d.description}</div>
                    )}
                    {d.indicators?.length > 0 && (
                      <div style={{ marginTop: "4px", display: "flex", flexWrap: "wrap", gap: "4px" }}>
                        {d.indicators.map((ind, i) => <span key={i} style={badge("gray")}>{ind}</span>)}
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Team preferences */}
          {ctx.runbook_preferences?.length > 0 && (
            <div style={card}>
              <div style={sectionTitle}>Team Runbook Preferences</div>
              <div style={{ marginTop: "8px", display: "grid", gap: "6px" }}>
                {ctx.runbook_preferences.map((p) => (
                  <div key={p.id} style={{ display: "flex", gap: "10px", alignItems: "flex-start", background: "var(--surface-2)", padding: "8px 12px", borderRadius: "6px" }}>
                    <span style={badge("blue")}>{p.team_name || "global"}</span>
                    <div style={{ flex: 1 }}>
                      <div style={{ fontSize: "13px", color: "var(--t1)" }}>
                        <strong>{p.preference_key}</strong>: {p.preference_value}
                      </div>
                      {p.context && <div style={{ fontSize: "11px", color: "var(--t3)", marginTop: "2px" }}>{p.context}</div>}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {!ctx.llm_narrative && ctx.recurrence_count === 0 && (
            <Notice>No prior memory for this service yet. Record a resolution to start building intelligence.</Notice>
          )}
        </div>
      ) : (
        <Notice>No memory context available for this incident.</Notice>
      )}

      {showRecordForm && (
        <Modal onClose={() => setShowRecordForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "var(--t1)" }}>Record Resolution Learning</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <Field label="Resolution Note">
              <textarea style={{ ...inp, height: "70px" }} value={recordForm.resolution_note}
                onChange={(e) => setRecordForm({ ...recordForm, resolution_note: e.target.value })}
                placeholder="What actually fixed this incident?" />
            </Field>
            <Field label="Remediation Steps (one per line)">
              <textarea style={{ ...inp, height: "90px", fontFamily: "monospace", fontSize: "12px" }}
                value={recordForm.remediation_steps}
                onChange={(e) => setRecordForm({ ...recordForm, remediation_steps: e.target.value })}
                placeholder={"1. Restart the payment service\n2. Verify DB connection pool\n3. Roll back config change"} />
            </Field>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
              <Field label="Error Signature">
                <input style={inp} value={recordForm.error_signature}
                  onChange={(e) => setRecordForm({ ...recordForm, error_signature: e.target.value })}
                  placeholder="OOMKilled, db_deadlock, …" />
              </Field>
              <Field label="Root Cause Category">
                <input style={inp} value={recordForm.root_cause_category}
                  onChange={(e) => setRecordForm({ ...recordForm, root_cause_category: e.target.value })}
                  placeholder="memory_leak, config_error, …" />
              </Field>
            </div>
            <Field label="Remediation Summary">
              <input style={inp} value={recordForm.remediation_summary}
                onChange={(e) => setRecordForm({ ...recordForm, remediation_summary: e.target.value })}
                placeholder="Restart pod + scale replicas" />
            </Field>
            <label style={{ display: "flex", gap: "8px", alignItems: "center", fontSize: "13px", color: "var(--t1)" }}>
              <input type="checkbox" checked={recordForm.was_successful}
                onChange={(e) => setRecordForm({ ...recordForm, was_successful: e.target.checked })} />
              Remediation was successful
            </label>
            <Field label="Deploy Signature (if deploy-related)">
              <input style={inp} value={recordForm.deploy_signature_name}
                onChange={(e) => setRecordForm({ ...recordForm, deploy_signature_name: e.target.value })}
                placeholder="bad-nginx-config-deploy" />
            </Field>
            <Field label="Deploy Indicators (comma-separated)">
              <input style={inp} value={recordForm.deploy_indicators}
                onChange={(e) => setRecordForm({ ...recordForm, deploy_indicators: e.target.value })}
                placeholder="nginx_reload, config_change, rollback_needed" />
            </Field>
            <Field label="Impacted Services (comma-separated)">
              <input style={inp} value={recordForm.impacted_services}
                onChange={(e) => setRecordForm({ ...recordForm, impacted_services: e.target.value })}
                placeholder="checkout, payments" />
            </Field>
          </div>
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
            <button onClick={() => setShowRecordForm(false)} style={btn("gray")}>Cancel</button>
            <button onClick={handleRecord} disabled={recording} style={btn("blue")}>
              {recording ? "Recording…" : "Record Learning"}
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ── Remediation Patterns Tab ──────────────────────────────────────────────────

function RemediationTab() {
  const [patterns, setPatterns] = useState([]);
  const [error, setError] = useState("");
  const [filter, setFilter] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [form, setForm] = useState({ service: "", error_signature: "", root_cause_category: "", remediation_summary: "", remediation_steps: "", success_count: 1, failure_count: 0 });
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const data = await listRemediationPatterns();
        if (active) setPatterns(data || []);
      } catch (e) { if (active) setError(e.message); }
    }
    load();
    return () => { active = false; };
  }, [refreshKey]);

  function openCreate() {
    setEditTarget(null);
    setForm({ service: "", error_signature: "", root_cause_category: "", remediation_summary: "", remediation_steps: "", success_count: 1, failure_count: 0 });
    setShowForm(true);
  }

  function openEdit(p) {
    setEditTarget(p);
    setForm({ ...p, remediation_steps: (p.remediation_steps || []).join("\n") });
    setShowForm(true);
  }

  async function save() {
    const payload = {
      ...form,
      success_count: Number(form.success_count),
      failure_count: Number(form.failure_count),
      remediation_steps: String(form.remediation_steps).split("\n").map((s) => s.trim()).filter(Boolean),
    };
    try {
      if (editTarget) await updateRemediationPattern(editTarget.id, payload);
      else await createRemediationPattern(payload);
      setShowForm(false);
      reload();
    } catch (e) { setError(e.message); }
  }

  async function remove(id) {
    try { await deleteRemediationPattern(id); reload(); }
    catch (e) { setError(e.message); }
  }

  async function markOutcome(id, success) {
    try { await markRemediationOutcome(id, success); reload(); }
    catch (e) { setError(e.message); }
  }

  const visible = filter
    ? patterns.filter((p) =>
        p.service?.toLowerCase().includes(filter.toLowerCase()) ||
        p.error_signature?.toLowerCase().includes(filter.toLowerCase()))
    : patterns;

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", gap: "10px", marginBottom: "14px", alignItems: "center" }}>
        <input style={{ ...inp, flex: 1 }} value={filter} onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter by service or error signature…" />
        <button onClick={openCreate} style={btn("blue")}>+ New Pattern</button>
      </div>

      {visible.length === 0
        ? <Notice>No remediation patterns yet. Record resolutions to build knowledge.</Notice>
        : visible.map((p) => (
          <div key={p.id} style={{ ...card, marginBottom: "10px" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div style={{ flex: 1 }}>
                <div style={{ display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap", marginBottom: "4px" }}>
                  <span style={{ fontWeight: 600, color: "var(--t1)" }}>{p.remediation_summary}</span>
                  {p.service && <span style={badge("blue")}>{p.service}</span>}
                  {p.root_cause_category && <span style={badge("purple")}>{p.root_cause_category}</span>}
                  <span style={badge(successColor(p.success_rate))}>{Math.round((p.success_rate || 0) * 100)}%</span>
                </div>
                {p.error_signature && (
                  <div style={{ fontSize: "12px", color: "var(--t3)" }}>
                    Signature: <code style={{ color: "var(--t2)" }}>{p.error_signature}</code>
                  </div>
                )}
                {p.remediation_steps?.length > 0 && (
                  <ol style={{ margin: "6px 0 0 16px", color: "var(--t2)", fontSize: "12px", lineHeight: 1.7 }}>
                    {p.remediation_steps.map((s, i) => <li key={i}>{s}</li>)}
                  </ol>
                )}
                <div style={{ fontSize: "11px", color: "var(--t4)", marginTop: "6px" }}>
                  ✅ {p.success_count} · ❌ {p.failure_count} · last {p.last_used_at?.slice(0, 10)}
                </div>
              </div>
              <div style={{ display: "flex", gap: "4px", flexShrink: 0, marginLeft: "12px" }}>
                <button onClick={() => markOutcome(p.id, true)} title="Mark success" style={btnSm("green")}>✅</button>
                <button onClick={() => markOutcome(p.id, false)} title="Mark failure" style={btnSm("red")}>❌</button>
                <button onClick={() => openEdit(p)} style={btnSm("blue")}>Edit</button>
                <button onClick={() => remove(p.id)} style={btnSm("red")}>Del</button>
              </div>
            </div>
          </div>
        ))
      }

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "var(--t1)" }}>{editTarget ? "Edit Pattern" : "New Remediation Pattern"}</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
              <Field label="Service"><input style={inp} value={form.service} onChange={(e) => setForm({ ...form, service: e.target.value })} /></Field>
              <Field label="Error Signature"><input style={inp} value={form.error_signature} onChange={(e) => setForm({ ...form, error_signature: e.target.value })} placeholder="OOMKilled" /></Field>
            </div>
            <Field label="Root Cause Category"><input style={inp} value={form.root_cause_category} onChange={(e) => setForm({ ...form, root_cause_category: e.target.value })} placeholder="memory_leak" /></Field>
            <Field label="Remediation Summary *"><input style={inp} value={form.remediation_summary} onChange={(e) => setForm({ ...form, remediation_summary: e.target.value })} /></Field>
            <Field label="Steps (one per line)">
              <textarea style={{ ...inp, height: "100px", fontFamily: "monospace", fontSize: "12px" }}
                value={form.remediation_steps} onChange={(e) => setForm({ ...form, remediation_steps: e.target.value })} />
            </Field>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
              <Field label="Success Count"><input type="number" style={inp} value={form.success_count} onChange={(e) => setForm({ ...form, success_count: e.target.value })} /></Field>
              <Field label="Failure Count"><input type="number" style={inp} value={form.failure_count} onChange={(e) => setForm({ ...form, failure_count: e.target.value })} /></Field>
            </div>
          </div>
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
            <button onClick={() => setShowForm(false)} style={btn("gray")}>Cancel</button>
            <button onClick={save} style={btn("blue")}>Save</button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ── Deploy Signatures Tab ─────────────────────────────────────────────────────

function DeployTab() {
  const [sigs, setSigs] = useState([]);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [form, setForm] = useState({ service: "", signature_name: "", description: "", indicators: "", impacted_services: "", typical_severity: "medium" });
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const data = await listDeploySignatures();
        if (active) setSigs(data || []);
      } catch (e) { if (active) setError(e.message); }
    }
    load();
    return () => { active = false; };
  }, [refreshKey]);

  function openCreate() {
    setEditTarget(null);
    setForm({ service: "", signature_name: "", description: "", indicators: "", impacted_services: "", typical_severity: "medium" });
    setShowForm(true);
  }

  function openEdit(d) {
    setEditTarget(d);
    setForm({ ...d, indicators: (d.indicators || []).join(", "), impacted_services: (d.impacted_services || []).join(", ") });
    setShowForm(true);
  }

  async function save() {
    const payload = {
      ...form,
      indicators: form.indicators.split(",").map((s) => s.trim()).filter(Boolean),
      impacted_services: form.impacted_services.split(",").map((s) => s.trim()).filter(Boolean),
    };
    try {
      if (editTarget) await updateDeploySignature(editTarget.id, payload);
      else await createDeploySignature(payload);
      setShowForm(false);
      reload();
    } catch (e) { setError(e.message); }
  }

  async function remove(id) {
    try { await deleteDeploySignature(id); reload(); }
    catch (e) { setError(e.message); }
  }

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "14px" }}>
        <p style={{ margin: 0, fontSize: "13px", color: "var(--t3)" }}>
          Known deploy patterns that historically triggered incidents — surfaced when a new incident matches.
        </p>
        <button onClick={openCreate} style={btn("blue")}>+ Add Signature</button>
      </div>

      {sigs.length === 0
        ? <Notice>No deploy signatures yet. Add patterns from past incident post-mortems.</Notice>
        : sigs.map((d) => (
          <div key={d.id} style={{ ...card, marginBottom: "10px", borderLeft: "3px solid var(--orange)" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div>
                <div style={{ display: "flex", gap: "8px", alignItems: "center", marginBottom: "4px" }}>
                  <span style={{ fontWeight: 600, color: "var(--t1)" }}>{d.signature_name}</span>
                  {d.service && <span style={badge("blue")}>{d.service}</span>}
                  <span style={badge(severityColor(d.typical_severity))}>{d.typical_severity}</span>
                  <span style={{ fontSize: "12px", color: "var(--t3)" }}>{d.occurrence_count}×</span>
                </div>
                {d.description && <div style={{ fontSize: "13px", color: "var(--t1)", marginBottom: "6px" }}>{d.description}</div>}
                {d.indicators?.length > 0 && (
                  <div style={{ display: "flex", flexWrap: "wrap", gap: "4px", marginBottom: "4px" }}>
                    <span style={{ fontSize: "11px", color: "var(--t3)", alignSelf: "center" }}>Indicators:</span>
                    {d.indicators.map((ind, i) => <span key={i} style={badge("gray")}>{ind}</span>)}
                  </div>
                )}
                {d.impacted_services?.length > 0 && (
                  <div style={{ fontSize: "12px", color: "var(--t3)" }}>
                    Impacts: {d.impacted_services.join(", ")}
                  </div>
                )}
              </div>
              <div style={{ display: "flex", gap: "4px", flexShrink: 0, marginLeft: "12px" }}>
                <button onClick={() => openEdit(d)} style={btnSm("blue")}>Edit</button>
                <button onClick={() => remove(d.id)} style={btnSm("red")}>Del</button>
              </div>
            </div>
          </div>
        ))
      }

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "var(--t1)" }}>{editTarget ? "Edit Signature" : "New Deploy Signature"}</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
              <Field label="Service"><input style={inp} value={form.service} onChange={(e) => setForm({ ...form, service: e.target.value })} /></Field>
              <Field label="Typical Severity">
                <select style={inp} value={form.typical_severity} onChange={(e) => setForm({ ...form, typical_severity: e.target.value })}>
                  {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
                </select>
              </Field>
            </div>
            <Field label="Signature Name *">
              <input style={inp} value={form.signature_name} onChange={(e) => setForm({ ...form, signature_name: e.target.value })} placeholder="bad-nginx-config-deploy" />
            </Field>
            <Field label="Description">
              <textarea style={{ ...inp, height: "70px" }} value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
                placeholder="Deploy of nginx config with upstream timeout = 0 causes 502 cascade." />
            </Field>
            <Field label="Indicators (comma-separated)">
              <input style={inp} value={form.indicators} onChange={(e) => setForm({ ...form, indicators: e.target.value })} placeholder="nginx_reload, upstream_timeout, 502_spike" />
            </Field>
            <Field label="Impacted Services (comma-separated)">
              <input style={inp} value={form.impacted_services} onChange={(e) => setForm({ ...form, impacted_services: e.target.value })} placeholder="checkout, api-gateway" />
            </Field>
          </div>
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
            <button onClick={() => setShowForm(false)} style={btn("gray")}>Cancel</button>
            <button onClick={save} style={btn("blue")}>Save</button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ── Runbook Preferences Tab ───────────────────────────────────────────────────

function RunbookTab() {
  const [prefs, setPrefs] = useState([]);
  const [error, setError] = useState("");
  const [filterTeam, setFilterTeam] = useState("");
  const [filterService, setFilterService] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [form, setForm] = useState({ team_name: "", service: "", preference_key: "", preference_value: "", context: "" });
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const data = await listRunbookPreferences();
        if (active) setPrefs(data || []);
      } catch (e) { if (active) setError(e.message); }
    }
    load();
    return () => { active = false; };
  }, [refreshKey]);

  function openCreate() {
    setEditTarget(null);
    setForm({ team_name: "", service: "", preference_key: "", preference_value: "", context: "" });
    setShowForm(true);
  }

  function openEdit(p) { setEditTarget(p); setForm({ ...p }); setShowForm(true); }

  async function save() {
    try {
      if (editTarget) await updateRunbookPreference(editTarget.id, form);
      else await createRunbookPreference(form);
      setShowForm(false);
      reload();
    } catch (e) { setError(e.message); }
  }

  async function remove(id) {
    try { await deleteRunbookPreference(id); reload(); }
    catch (e) { setError(e.message); }
  }

  const visible = prefs.filter((p) =>
    (!filterTeam || p.team_name?.toLowerCase().includes(filterTeam.toLowerCase())) &&
    (!filterService || p.service?.toLowerCase().includes(filterService.toLowerCase()))
  );

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", gap: "8px", marginBottom: "14px", alignItems: "center" }}>
        <input style={{ ...inp, flex: 1 }} value={filterTeam} onChange={(e) => setFilterTeam(e.target.value)} placeholder="Filter by team…" />
        <input style={{ ...inp, flex: 1 }} value={filterService} onChange={(e) => setFilterService(e.target.value)} placeholder="Filter by service…" />
        <button onClick={openCreate} style={btn("blue")}>+ Add Preference</button>
      </div>

      {visible.length === 0
        ? <Notice>No runbook preferences yet. Add team-specific preferences to guide incident response.</Notice>
        : visible.map((p) => (
          <div key={p.id} style={{ ...card, marginBottom: "8px" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div style={{ flex: 1 }}>
                <div style={{ display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap", marginBottom: "4px" }}>
                  {p.team_name && <span style={badge("blue")}>{p.team_name}</span>}
                  {p.service && <span style={badge("gray")}>{p.service}</span>}
                  <span style={{ fontWeight: 600, color: "var(--t1)", fontSize: "13px" }}>{p.preference_key}</span>
                </div>
                <div style={{ fontSize: "13px", color: "var(--t2)" }}>{p.preference_value}</div>
                {p.context && <div style={{ fontSize: "12px", color: "var(--t4)", marginTop: "4px" }}>{p.context}</div>}
              </div>
              <div style={{ display: "flex", gap: "4px", flexShrink: 0, marginLeft: "12px" }}>
                <button onClick={() => openEdit(p)} style={btnSm("blue")}>Edit</button>
                <button onClick={() => remove(p.id)} style={btnSm("red")}>Del</button>
              </div>
            </div>
          </div>
        ))
      }

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "var(--t1)" }}>{editTarget ? "Edit Preference" : "New Runbook Preference"}</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
              <Field label="Team Name">
                <input style={inp} value={form.team_name} onChange={(e) => setForm({ ...form, team_name: e.target.value })} placeholder="payments-team" />
              </Field>
              <Field label="Service">
                <input style={inp} value={form.service} onChange={(e) => setForm({ ...form, service: e.target.value })} placeholder="checkout" />
              </Field>
            </div>
            <Field label="Preference Key *">
              <input style={inp} value={form.preference_key}
                onChange={(e) => setForm({ ...form, preference_key: e.target.value })}
                placeholder="first_action, escalation_path, communication_channel, preferred_runbook…" />
            </Field>
            <Field label="Preference Value *">
              <input style={inp} value={form.preference_value}
                onChange={(e) => setForm({ ...form, preference_value: e.target.value })}
                placeholder="always rollback first, #payments-oncall, …" />
            </Field>
            <Field label="Context (why)">
              <textarea style={{ ...inp, height: "70px" }} value={form.context}
                onChange={(e) => setForm({ ...form, context: e.target.value })}
                placeholder="Payment team learned from Q3 outage that rolling back first saves 40% TTR." />
            </Field>
          </div>
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
            <button onClick={() => setShowForm(false)} style={btn("gray")}>Cancel</button>
            <button onClick={save} style={btn("blue")}>Save</button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ── Shared primitives ─────────────────────────────────────────────────────────

function Modal({ onClose: _onClose, children }) {
  return (
    <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000 }}>
      <div style={{ background: "var(--surface-1)", borderRadius: "var(--r5)", padding: "28px", width: "600px", maxWidth: "95vw", maxHeight: "90vh", overflowY: "auto", border: "1px solid var(--border-strong)", boxShadow: "var(--shadow-lg)" }}>
        {children}
      </div>
    </div>
  );
}

function Field({ label, children }) {
  return (
    <div>
      {label && <label style={{ fontSize: "12px", fontWeight: 600, color: "var(--t2)", display: "block", marginBottom: "5px" }}>{label}</label>}
      {children}
    </div>
  );
}

function Notice({ children }) {
  return <div style={{ color: "var(--t3)", textAlign: "center", padding: "40px 20px", fontSize: "14px" }}>{children}</div>;
}

// ── Style helpers ─────────────────────────────────────────────────────────────

function successColor(rate) {
  if (!rate) return "gray";
  if (rate >= 0.8) return "green";
  if (rate >= 0.5) return "yellow";
  return "red";
}

function severityColor(sev) {
  const m = { critical: "red", high: "orange", medium: "yellow", low: "green" };
  return m[sev] || "gray";
}

const panelStyle = { padding: "24px 28px", color: "var(--t1)" };
const card = { background: "var(--surface-1)", border: "1px solid var(--border)", borderRadius: "var(--r5)", padding: "16px 20px" };
const inp = { width: "100%", background: "var(--surface-2)", border: "1px solid var(--border-strong)", borderRadius: "var(--r3)", color: "var(--t1)", padding: "9px 12px", fontSize: "13px", boxSizing: "border-box", outline: "none" };
const sectionTitle = { fontSize: "11px", fontWeight: 700, color: "var(--t3)", textTransform: "uppercase", letterSpacing: "0.08em", marginBottom: "4px" };

function tabBtn(active) {
  return { background: active ? "var(--blue-dim)" : "transparent", border: active ? "1px solid rgba(0,102,255,0.35)" : "1px solid var(--border)", color: active ? "var(--blue-lt)" : "var(--t3)", padding: "7px 14px", borderRadius: "var(--r3)", cursor: "pointer", fontSize: "13px", fontWeight: 600, transition: "all 0.14s" };
}

function btn(color) {
  const map = {
    blue:  { bg: "var(--blue)",       fg: "#fff",         bd: "var(--blue)" },
    red:   { bg: "var(--red-dim)",    fg: "var(--red)",   bd: "rgba(255,59,59,0.3)" },
    gray:  { bg: "var(--surface-2)",  fg: "var(--t1)",    bd: "var(--border)" },
    green: { bg: "var(--green-dim)",  fg: "var(--green)", bd: "rgba(0,208,132,0.3)" },
  };
  const c = map[color] || map.gray;
  return { background: c.bg, color: c.fg, border: `1px solid ${c.bd}`, borderRadius: "var(--r3)", padding: "8px 16px", fontSize: "13px", cursor: "pointer", fontWeight: 600, transition: "all 0.14s" };
}

function btnSm(color) { return { ...btn(color), padding: "5px 11px", fontSize: "12px" }; }

function badge(color) {
  const theme = {
    blue:   { bg: "var(--blue-dim)",        text: "var(--blue-lt)", border: "rgba(0,102,255,0.25)" },
    green:  { bg: "var(--green-dim)",       text: "var(--green)",   border: "rgba(0,208,132,0.25)" },
    gray:   { bg: "rgba(255,255,255,0.06)", text: "var(--t2)",      border: "var(--border)" },
    red:    { bg: "var(--red-dim)",         text: "var(--red)",     border: "rgba(255,59,59,0.25)" },
    yellow: { bg: "var(--amber-dim)",       text: "var(--amber)",   border: "rgba(255,188,0,0.25)" },
    purple: { bg: "var(--cyan-dim)",        text: "var(--cyan)",    border: "rgba(0,200,232,0.25)" },
    orange: { bg: "var(--orange-dim)",      text: "var(--orange)",  border: "rgba(255,106,0,0.25)" },
  };
  const t = theme[color] || theme.gray;
  return { background: t.bg, color: t.text, border: `1px solid ${t.border}`, borderRadius: "var(--pill)", padding: "2px 8px", fontSize: "11px", fontWeight: 700, display: "inline-block" };
}

function alertBox(color) {
  return { ...badge(color), borderRadius: "var(--r3)", padding: "10px 14px", fontSize: "13px", marginBottom: "14px", display: "block" };
}
