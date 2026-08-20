import { useState, useEffect } from "react";
import {
  getCurrentCommander, getCommanderHistory, assignCommander,
  listTemplates, createTemplate, updateTemplate, deleteTemplate, renderTemplate,
  getTickets, createTicket, syncTicket,
  generateExecSummary,
  getStatusComms, publishStatusComm,
} from "../api/workflow.js";

const TABS = [
  { key: "commander",    label: "👤 Commander" },
  { key: "templates",    label: "📄 Templates" },
  { key: "tickets",      label: "🎫 Tickets" },
  { key: "exec-summary", label: "📊 Exec Summary" },
  { key: "status",       label: "📢 Status Comms" },
];

const CHANNELS = ["slack", "teams", "email", "status-page"];
const STAGES = ["investigating", "identified", "monitoring", "resolved"];
const stageColor = { investigating: "#f97316", identified: "#eab308", monitoring: "#3b82f6", resolved: "#22c55e" };

export default function TeamWorkflowPanel({ incidentId }) {
  const [tab, setTab] = useState("commander");

  return (
    <div style={panelStyle}>
      <div style={{ marginBottom: "18px" }}>
        <h2 style={{ margin: 0, fontSize: "22px", color: "#f1f5f9" }}>Team Workflow</h2>
        {incidentId && (
          <p style={{ margin: "4px 0 0", fontSize: "13px", color: "#64748b" }}>
            Incident: <code style={{ color: "#7dd3fc" }}>{incidentId}</code>
          </p>
        )}
      </div>

      <div style={{ display: "flex", gap: "4px", marginBottom: "20px", flexWrap: "wrap" }}>
        {TABS.map((t) => (
          <button key={t.key} onClick={() => setTab(t.key)} style={tabBtn(tab === t.key)}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === "commander"    && <CommanderTab incidentId={incidentId} />}
      {tab === "templates"    && <TemplatesTab incidentId={incidentId} />}
      {tab === "tickets"      && <TicketsTab incidentId={incidentId} />}
      {tab === "exec-summary" && <ExecSummaryTab incidentId={incidentId} />}
      {tab === "status"       && <StatusCommsTab incidentId={incidentId} />}
    </div>
  );
}

// ── Commander Tab ─────────────────────────────────────────────────────────────

function CommanderTab({ incidentId }) {
  const [current, setCurrent] = useState(null);
  const [history, setHistory] = useState([]);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ user_name: "", user_id: "", user_email: "", notes: "" });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!incidentId) return;
    async function load() {
      try {
        const [c, h] = await Promise.all([
          getCurrentCommander(incidentId).catch(() => null),
          getCommanderHistory(incidentId).catch(() => []),
        ]);
        setCurrent(c?.user_name ? c : null);
        setHistory(h || []);
      } catch (e) { setError(e.message); }
    }
    load();
  }, [incidentId]);

  async function handleAssign() {
    if (!form.user_name) { setError("user_name is required"); return; }
    setSaving(true);
    try {
      await assignCommander(incidentId, form);
      setShowForm(false);
      setError("");
      const [c, h] = await Promise.all([
        getCurrentCommander(incidentId).catch(() => null),
        getCommanderHistory(incidentId).catch(() => []),
      ]);
      setCurrent(c?.user_name ? c : null);
      setHistory(h || []);
    } catch (e) { setError(e.message); }
    finally { setSaving(false); }
  }

  if (!incidentId) return <Notice>Select an incident to manage its commander.</Notice>;

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={card}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <div>
            <div style={{ fontSize: "13px", color: "#94a3b8", marginBottom: "4px" }}>Current Commander</div>
            {current
              ? <div style={{ fontSize: "18px", fontWeight: 700, color: "#f1f5f9" }}>
                  {current.user_name}
                  {current.user_email && <span style={{ fontSize: "13px", color: "#64748b", marginLeft: "8px" }}>{current.user_email}</span>}
                </div>
              : <div style={{ fontSize: "15px", color: "#64748b" }}>No commander assigned</div>
            }
            {current && (
              <div style={{ fontSize: "12px", color: "#64748b", marginTop: "4px" }}>
                Assigned by {current.assigned_by} · {fmtTime(current.assigned_at)}
                {current.notes && <span> · {current.notes}</span>}
              </div>
            )}
          </div>
          <button onClick={() => setShowForm(true)} style={btn("blue")}>
            {current ? "Reassign" : "Assign Commander"}
          </button>
        </div>
      </div>

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "#f1f5f9" }}>Assign Incident Commander</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <Field label="Name *">
              <input style={input} value={form.user_name}
                onChange={(e) => setForm({ ...form, user_name: e.target.value })} placeholder="Jane Doe" />
            </Field>
            <Field label="User ID (Slack/Teams)">
              <input style={input} value={form.user_id}
                onChange={(e) => setForm({ ...form, user_id: e.target.value })} placeholder="@jane" />
            </Field>
            <Field label="Email">
              <input style={input} value={form.user_email}
                onChange={(e) => setForm({ ...form, user_email: e.target.value })} placeholder="jane@company.com" />
            </Field>
            <Field label="Notes">
              <input style={input} value={form.notes}
                onChange={(e) => setForm({ ...form, notes: e.target.value })} placeholder="Optional context" />
            </Field>
          </div>
          <ModalActions onCancel={() => setShowForm(false)} onSave={handleAssign} saving={saving} saveLabel="Assign" />
        </Modal>
      )}

      {history.length > 0 && (
        <div style={{ marginTop: "20px" }}>
          <h4 style={{ color: "#94a3b8", fontSize: "12px", fontWeight: 600, marginBottom: "8px", textTransform: "uppercase" }}>
            Assignment History
          </h4>
          {history.map((h) => (
            <div key={h.id} style={{ ...card, padding: "10px 16px", marginBottom: "6px" }}>
              <div style={{ display: "flex", justifyContent: "space-between", fontSize: "13px" }}>
                <span style={{ color: "#e2e8f0" }}>{h.user_name}</span>
                <span style={{ color: "#64748b" }}>
                  {fmtTime(h.assigned_at)}{h.relieved_at ? ` → ${fmtTime(h.relieved_at)}` : " (active)"}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ── Templates Tab ─────────────────────────────────────────────────────────────

function TemplatesTab({ incidentId }) {
  const [templates, setTemplates] = useState([]);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [form, setForm] = useState({ name: "", channel: "slack", subject: "", body: "", is_default: false });
  const [renderResult, setRenderResult] = useState(null);
  const [renderIncident, setRenderIncident] = useState(incidentId || "");
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    let active = true;
    async function load() {
      try {
        const data = await listTemplates();
        if (active) setTemplates(data || []);
      } catch (e) { if (active) setError(e.message); }
    }
    load();
    return () => { active = false; };
  }, [refreshKey]);

  function openCreate() {
    setEditTarget(null);
    setForm({ name: "", channel: "slack", subject: "", body: "", is_default: false });
    setShowForm(true);
  }

  function openEdit(t) {
    setEditTarget(t);
    setForm({ name: t.name, channel: t.channel, subject: t.subject || "", body: t.body, is_default: t.is_default });
    setShowForm(true);
  }

  async function save() {
    try {
      if (editTarget) { await updateTemplate(editTarget.id, { ...form, tenant_id: editTarget.tenant_id }); }
      else { await createTemplate(form); }
      setShowForm(false);
      reload();
    } catch (e) { setError(e.message); }
  }

  async function confirmDelete() {
    try {
      await deleteTemplate(deleteTarget.id);
      setDeleteTarget(null);
      reload();
    } catch (e) { setError(e.message); }
  }

  async function handleRender(tmpl) {
    const rid = renderIncident || incidentId;
    if (!rid) { setError("Enter an incident ID to preview this template"); return; }
    try {
      const result = await renderTemplate(tmpl.id, rid);
      setRenderResult(result);
    } catch (e) { setError(e.message); }
  }

  const vars = ["{{incident_id}}", "{{service}}", "{{severity}}", "{{status}}", "{{title}}",
    "{{commander}}", "{{started_at}}", "{{duration_mins}}", "{{event_count}}", "{{impacted_services}}"];

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "14px", alignItems: "flex-start" }}>
        <div style={{ fontSize: "12px", color: "#64748b", maxWidth: "60%" }}>
          Available variables: {vars.join(" ")}
        </div>
        <button onClick={openCreate} style={btn("blue")}>+ New Template</button>
      </div>

      <div style={{ display: "flex", gap: "8px", marginBottom: "16px", alignItems: "center" }}>
        <span style={{ fontSize: "12px", color: "#94a3b8" }}>Preview with incident:</span>
        <input style={{ ...input, width: "220px" }} value={renderIncident}
          onChange={(e) => setRenderIncident(e.target.value)} placeholder="INC123…" />
      </div>

      {templates.length === 0
        ? <Notice>No templates yet. Create one to send consistent stakeholder communications.</Notice>
        : templates.map((t) => (
          <div key={t.id} style={{ ...card, marginBottom: "10px" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div>
                <span style={{ fontWeight: 600, color: "#f1f5f9" }}>{t.name}</span>
                {t.is_default && <span style={{ ...badge("blue"), marginLeft: "8px" }}>default</span>}
                <span style={{ ...badge("gray"), marginLeft: "6px" }}>{t.channel}</span>
                {t.subject && <div style={{ fontSize: "12px", color: "#64748b", marginTop: "2px" }}>Subject: {t.subject}</div>}
              </div>
              <div style={{ display: "flex", gap: "6px" }}>
                <button onClick={() => handleRender(t)} style={btnSm("gray")}>Preview</button>
                <button onClick={() => openEdit(t)} style={btnSm("blue")}>Edit</button>
                <button onClick={() => setDeleteTarget(t)} style={btnSm("red")}>Delete</button>
              </div>
            </div>
            <pre style={{ marginTop: "8px", fontSize: "12px", color: "#94a3b8", whiteSpace: "pre-wrap", wordBreak: "break-word", background: "#0f172a", padding: "8px", borderRadius: "6px" }}>
              {t.body.slice(0, 200)}{t.body.length > 200 ? "…" : ""}
            </pre>
          </div>
        ))
      }

      {renderResult && (
        <Modal onClose={() => setRenderResult(null)}>
          <h3 style={{ margin: "0 0 12px", color: "#f1f5f9" }}>Rendered: {renderResult.channel}</h3>
          {renderResult.subject && <div style={{ color: "#94a3b8", fontSize: "13px", marginBottom: "8px" }}>Subject: {renderResult.subject}</div>}
          <pre style={{ background: "#0f172a", padding: "16px", borderRadius: "8px", color: "#e2e8f0", fontSize: "13px", whiteSpace: "pre-wrap", wordBreak: "break-word" }}>
            {renderResult.body}
          </pre>
          <div style={{ textAlign: "right", marginTop: "14px" }}>
            <button onClick={() => setRenderResult(null)} style={btn("gray")}>Close</button>
          </div>
        </Modal>
      )}

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "#f1f5f9" }}>{editTarget ? "Edit Template" : "New Template"}</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <Field label="Name *"><input style={input} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
            <Field label="Channel">
              <select style={input} value={form.channel} onChange={(e) => setForm({ ...form, channel: e.target.value })}>
                {CHANNELS.map((c) => <option key={c} value={c}>{c}</option>)}
              </select>
            </Field>
            <Field label="Subject"><input style={input} value={form.subject} onChange={(e) => setForm({ ...form, subject: e.target.value })} /></Field>
            <Field label="Body *">
              <textarea style={{ ...input, height: "120px", fontFamily: "monospace", fontSize: "12px" }}
                value={form.body} onChange={(e) => setForm({ ...form, body: e.target.value })}
                placeholder="Service {{service}} is experiencing a {{severity}} severity incident." />
            </Field>
            <label style={{ display: "flex", gap: "8px", alignItems: "center", fontSize: "13px", color: "#e2e8f0" }}>
              <input type="checkbox" checked={form.is_default} onChange={(e) => setForm({ ...form, is_default: e.target.checked })} />
              Mark as default template
            </label>
          </div>
          <ModalActions onCancel={() => setShowForm(false)} onSave={save} saveLabel="Save" />
        </Modal>
      )}

      {deleteTarget && (
        <Modal onClose={() => setDeleteTarget(null)}>
          <h3 style={{ color: "#f87171", margin: "0 0 12px" }}>Delete Template?</h3>
          <p style={{ color: "#cbd5e1" }}>Delete <strong>{deleteTarget.name}</strong>?</p>
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end" }}>
            <button onClick={() => setDeleteTarget(null)} style={btn("gray")}>Cancel</button>
            <button onClick={confirmDelete} style={btn("red")}>Delete</button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ── Tickets Tab ───────────────────────────────────────────────────────────────

function TicketsTab({ incidentId }) {
  const [tickets, setTickets] = useState([]);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ provider: "jira", priority: "", assignee: "", description: "" });
  const [syncing, setSyncing] = useState({});
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    if (!incidentId) return;
    let active = true;
    async function load() {
      try {
        const data = await getTickets(incidentId);
        if (active) setTickets(data || []);
      } catch { if (active) setTickets([]); }
    }
    load();
    return () => { active = false; };
  }, [incidentId, refreshKey]);

  async function handleCreate() {
    try {
      await createTicket(incidentId, form);
      setShowForm(false);
      reload();
    } catch (e) { setError(e.message); }
  }

  async function handleSync(provider) {
    setSyncing((prev) => ({ ...prev, [provider]: true }));
    try { await syncTicket(incidentId, provider); reload(); }
    catch (e) { setError(e.message); }
    finally { setSyncing((prev) => ({ ...prev, [provider]: false })); }
  }

  if (!incidentId) return <Notice>Select an incident to manage tickets.</Notice>;

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: "14px" }}>
        <button onClick={() => setShowForm(true)} style={btn("blue")}>+ Create Ticket</button>
      </div>

      {tickets.length === 0
        ? <Notice>No tickets linked. Create a JIRA or ServiceNow ticket above.</Notice>
        : tickets.map((t) => (
          <div key={t.id} style={{ ...card, marginBottom: "10px" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <div>
                <span style={badge(t.provider === "jira" ? "blue" : "purple")}>{t.provider}</span>
                <a href={t.ticket_url} target="_blank" rel="noopener noreferrer"
                  style={{ marginLeft: "10px", color: "#7dd3fc", fontWeight: 600, textDecoration: "none" }}>
                  {t.ticket_key}
                </a>
                <span style={{ marginLeft: "8px", ...badge(ticketStatusColor(t.ticket_status)) }}>{t.ticket_status || "open"}</span>
              </div>
              <button onClick={() => handleSync(t.provider)} disabled={syncing[t.provider]} style={btnSm("gray")}>
                {syncing[t.provider] ? "Syncing…" : "Sync Status"}
              </button>
            </div>
            <div style={{ fontSize: "12px", color: "#64748b", marginTop: "4px" }}>
              Created by {t.created_by || "system"} · {fmtTime(t.created_at)}
            </div>
          </div>
        ))
      }

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "#f1f5f9" }}>Create Ticket</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <Field label="Provider">
              <select style={input} value={form.provider} onChange={(e) => setForm({ ...form, provider: e.target.value })}>
                <option value="jira">JIRA</option>
                <option value="servicenow">ServiceNow</option>
              </select>
            </Field>
            <Field label="Priority">
              <select style={input} value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })}>
                <option value="">Auto (from severity)</option>
                <option value="Highest">Highest / P1</option>
                <option value="High">High / P2</option>
                <option value="Medium">Medium / P3</option>
                <option value="Low">Low / P4</option>
              </select>
            </Field>
            <Field label="Assignee"><input style={input} value={form.assignee} onChange={(e) => setForm({ ...form, assignee: e.target.value })} placeholder="username or email" /></Field>
            <Field label="Additional Description">
              <textarea style={{ ...input, height: "80px" }} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </Field>
          </div>
          <ModalActions onCancel={() => setShowForm(false)} onSave={handleCreate} saveLabel="Create Ticket" />
        </Modal>
      )}
    </div>
  );
}

// ── Executive Summary Tab ─────────────────────────────────────────────────────

function ExecSummaryTab({ incidentId }) {
  const [summary, setSummary] = useState(null);
  const [generating, setGenerating] = useState(false);
  const [error, setError] = useState("");
  const [view, setView] = useState("structured");

  async function generate() {
    if (!incidentId) { setError("Select an incident first"); return; }
    setGenerating(true);
    try { setSummary(await generateExecSummary(incidentId)); setError(""); }
    catch (e) { setError(e.message); }
    finally { setGenerating(false); }
  }

  if (!incidentId) return <Notice>Select an incident to generate an executive summary.</Notice>;

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", gap: "10px", marginBottom: "16px", alignItems: "center" }}>
        <button onClick={generate} disabled={generating} style={btn("blue")}>
          {generating ? "⏳ Generating…" : "✨ Generate Executive Summary"}
        </button>
        {summary && (
          <div style={{ display: "flex", gap: "6px" }}>
            <button onClick={() => setView("structured")} style={btnSm(view === "structured" ? "blue" : "gray")}>Structured</button>
            <button onClick={() => setView("markdown")} style={btnSm(view === "markdown" ? "blue" : "gray")}>Markdown</button>
          </div>
        )}
      </div>

      {summary && view === "structured" && (
        <div style={{ display: "grid", gap: "14px" }}>
          <SummarySection title="🏢 Business Impact" text={summary.business_impact} />
          <SummarySection title="📅 Timeline" text={summary.timeline_narrative} />
          <SummarySection title="🔍 Root Cause" text={summary.root_cause} />
          <SummarySection title="✅ Resolution" text={summary.resolution} />
          {summary.lessons_learned?.length > 0 && (
            <div style={card}>
              <div style={sectionTitle}>💡 Lessons Learned</div>
              <ul style={{ margin: "8px 0 0 16px", color: "#e2e8f0", fontSize: "13px" }}>
                {summary.lessons_learned.map((l, i) => <li key={i}>{l}</li>)}
              </ul>
            </div>
          )}
          {summary.next_steps?.length > 0 && (
            <div style={card}>
              <div style={sectionTitle}>🚀 Next Steps</div>
              <ul style={{ margin: "8px 0 0 16px", color: "#e2e8f0", fontSize: "13px" }}>
                {summary.next_steps.map((s, i) => <li key={i}>{s}</li>)}
              </ul>
            </div>
          )}
          <div style={{ fontSize: "11px", color: "#475569", textAlign: "right" }}>
            Generated {fmtTime(summary.generated_at)}
          </div>
        </div>
      )}

      {summary && view === "markdown" && (
        <pre style={{ background: "#0f172a", padding: "16px", borderRadius: "8px", color: "#e2e8f0", fontSize: "13px", whiteSpace: "pre-wrap", wordBreak: "break-word", lineHeight: 1.6 }}>
          {summary.raw_markdown}
        </pre>
      )}
    </div>
  );
}

function SummarySection({ title, text }) {
  return (
    <div style={card}>
      <div style={sectionTitle}>{title}</div>
      <p style={{ margin: "6px 0 0", color: "#e2e8f0", fontSize: "13px", lineHeight: 1.6 }}>{text}</p>
    </div>
  );
}

// ── Status Communications Tab ─────────────────────────────────────────────────

function StatusCommsTab({ incidentId }) {
  const [comms, setComms] = useState([]);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({ stage: "investigating", title: "", body: "", affected_services: "", published_by: "" });
  const [publishing, setPublishing] = useState(false);
  const [refreshKey, setRefreshKey] = useState(0);
  const reload = () => setRefreshKey((k) => k + 1);

  useEffect(() => {
    if (!incidentId) return;
    let active = true;
    async function load() {
      try {
        const data = await getStatusComms(incidentId);
        if (active) setComms(data || []);
      } catch { if (active) setComms([]); }
    }
    load();
    return () => { active = false; };
  }, [incidentId, refreshKey]);

  async function publish() {
    setPublishing(true);
    try {
      const payload = {
        ...form,
        affected_services: form.affected_services.split(",").map((s) => s.trim()).filter(Boolean),
      };
      await publishStatusComm(incidentId, payload);
      setShowForm(false);
      setError("");
      reload();
    } catch (e) { setError(e.message); }
    finally { setPublishing(false); }
  }

  if (!incidentId) return <Notice>Select an incident to manage status communications.</Notice>;

  return (
    <div>
      {error && <div style={alertBox("red")}>{error}</div>}
      <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: "14px" }}>
        <button onClick={() => setShowForm(true)} style={btn("blue")}>📢 Publish Update</button>
      </div>

      {comms.length === 0
        ? <Notice>No status updates published yet.</Notice>
        : comms.map((c) => (
          <div key={c.id} style={{ ...card, borderLeft: `3px solid ${stageColor[c.stage] || "#475569"}`, marginBottom: "10px" }}>
            <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "6px" }}>
              <span style={{ fontWeight: 600, color: "#f1f5f9" }}>{c.title}</span>
              <span style={{ ...badge("gray"), borderColor: stageColor[c.stage], color: stageColor[c.stage] }}>{c.stage}</span>
            </div>
            <p style={{ margin: 0, fontSize: "13px", color: "#e2e8f0", lineHeight: 1.5 }}>{c.body}</p>
            {c.affected_services?.length > 0 && (
              <div style={{ marginTop: "6px", fontSize: "12px", color: "#64748b" }}>
                Affected: {c.affected_services.join(", ")}
              </div>
            )}
            <div style={{ marginTop: "6px", fontSize: "11px", color: "#475569" }}>
              {c.published_by && `by ${c.published_by} · `}{fmtTime(c.published_at)}
            </div>
          </div>
        ))
      }

      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "#f1f5f9" }}>Publish Status Update</h3>
          <div style={{ display: "grid", gap: "12px" }}>
            <Field label="Stage">
              <select style={input} value={form.stage} onChange={(e) => setForm({ ...form, stage: e.target.value })}>
                {STAGES.map((s) => <option key={s} value={s}>{s}</option>)}
              </select>
            </Field>
            <Field label="Title *"><input style={input} value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} placeholder="We are investigating reports of…" /></Field>
            <Field label="Body *">
              <textarea style={{ ...input, height: "100px" }} value={form.body} onChange={(e) => setForm({ ...form, body: e.target.value })} placeholder="Our team has identified a disruption affecting…" />
            </Field>
            <Field label="Affected Services (comma-separated)">
              <input style={input} value={form.affected_services} onChange={(e) => setForm({ ...form, affected_services: e.target.value })} placeholder="checkout, payments, auth" />
            </Field>
            <Field label="Published by">
              <input style={input} value={form.published_by} onChange={(e) => setForm({ ...form, published_by: e.target.value })} placeholder="optional — defaults to logged-in user" />
            </Field>
          </div>
          <ModalActions onCancel={() => setShowForm(false)} onSave={publish} saving={publishing} saveLabel="Publish" />
        </Modal>
      )}
    </div>
  );
}

// ── Reusable primitives ───────────────────────────────────────────────────────

function Modal({ onClose: _onClose, children }) {
  return (
    <div style={{ position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)", display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000 }}>
      <div style={{ background: "#1e293b", borderRadius: "12px", padding: "28px", width: "580px", maxWidth: "95vw", maxHeight: "90vh", overflowY: "auto", border: "1px solid #334155" }}>
        {children}
      </div>
    </div>
  );
}

function ModalActions({ onCancel, onSave, saving, saveLabel = "Save" }) {
  return (
    <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
      <button onClick={onCancel} style={btn("gray")}>Cancel</button>
      <button onClick={onSave} disabled={saving} style={btn("blue")}>{saving ? "Saving…" : saveLabel}</button>
    </div>
  );
}

function Field({ label, children }) {
  return (
    <div>
      {label && <label style={{ fontSize: "12px", fontWeight: 600, color: "#94a3b8", display: "block", marginBottom: "5px" }}>{label}</label>}
      {children}
    </div>
  );
}

function Notice({ children }) {
  return <div style={{ color: "#64748b", textAlign: "center", padding: "40px 20px", fontSize: "14px" }}>{children}</div>;
}

// ── Utils ─────────────────────────────────────────────────────────────────────

function fmtTime(ts) {
  if (!ts) return "—";
  try { return new Date(ts).toLocaleString(); } catch { return ts; }
}

function ticketStatusColor(status) {
  if (!status || status === "open" || status === "new") return "blue";
  if (status === "in_progress") return "yellow";
  if (status === "resolved" || status === "closed") return "green";
  return "gray";
}

// ── Styles ────────────────────────────────────────────────────────────────────

const panelStyle = { padding: "24px", fontFamily: "sans-serif", color: "#e2e8f0" };
const card = { background: "#1e293b", border: "1px solid #334155", borderRadius: "10px", padding: "16px 20px" };
const input = { width: "100%", background: "#0f172a", border: "1px solid #334155", borderRadius: "6px", color: "#e2e8f0", padding: "8px 10px", fontSize: "13px", boxSizing: "border-box" };
const sectionTitle = { fontSize: "12px", fontWeight: 700, color: "#94a3b8", textTransform: "uppercase", letterSpacing: "0.05em" };

function tabBtn(active) {
  return { background: active ? "#1e3a5f" : "none", border: active ? "1px solid #2563eb" : "1px solid #334155", color: active ? "#93c5fd" : "#64748b", padding: "6px 14px", borderRadius: "6px", cursor: "pointer", fontSize: "13px", fontWeight: active ? 600 : 400 };
}

function btn(color) {
  const bg = { blue: "#2563eb", red: "#dc2626", gray: "#374151" };
  return { background: bg[color] || bg.gray, color: "#fff", border: "none", borderRadius: "6px", padding: "8px 16px", fontSize: "13px", cursor: "pointer", fontWeight: 500 };
}

function btnSm(color) { return { ...btn(color), padding: "4px 10px", fontSize: "12px" }; }

function badge(color) {
  const theme = {
    blue:   { bg: "#0c1d3a", text: "#93c5fd", border: "#1d4ed8" },
    green:  { bg: "#052e16", text: "#4ade80", border: "#166534" },
    gray:   { bg: "#1e293b", text: "#94a3b8", border: "#334155" },
    red:    { bg: "#450a0a", text: "#f87171", border: "#7f1d1d" },
    yellow: { bg: "#1c1500", text: "#fbbf24", border: "#b45309" },
    purple: { bg: "#2e1065", text: "#c084fc", border: "#7e22ce" },
  };
  const t = theme[color] || theme.gray;
  return { background: t.bg, color: t.text, border: `1px solid ${t.border}`, borderRadius: "12px", padding: "1px 8px", fontSize: "11px", fontWeight: 600, display: "inline-block" };
}

function alertBox(color) {
  return { ...badge(color), borderRadius: "6px", padding: "10px 14px", fontSize: "13px", marginBottom: "14px", display: "block" };
}
