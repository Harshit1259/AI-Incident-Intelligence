import { useState, useEffect, useCallback } from "react";
import {
  listSchemaMappings,
  getSchemaMappingStats,
  createSchemaMapping,
  updateSchemaMapping,
  deleteSchemaMapping,
} from "../api/schema.js";

const SIGNAL_TYPES = ["alert", "log", "metric", "trace", "change"];
const SEVERITIES = ["critical", "high", "medium", "low"];

const emptyForm = () => ({
  source_type: "",
  name: "",
  enabled: true,
  field_map: '{"alert_level": "severity", "app_name": "service"}',
  severity_map: '{"fire": "critical", "warn": "high", "info": "low"}',
  default_signal_type: "alert",
  default_severity: "medium",
  title_template: "",
  sample_payload: "",
});

export default function SchemaRegistryPanel() {
  const [tab, setTab] = useState("mappings");
  const [mappings, setMappings] = useState([]);
  const [stats, setStats] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [editTarget, setEditTarget] = useState(null);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [form, setForm] = useState(emptyForm());
  const [formError, setFormError] = useState("");
  const [formSaving, setFormSaving] = useState(false);

  const loadMappings = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const data = await listSchemaMappings();
      setMappings(data || []);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  const loadStats = useCallback(async () => {
    try {
      const data = await getSchemaMappingStats();
      setStats(data || []);
    } catch {
      // stats are supplementary — silent fail
    }
  }, []);

  useEffect(() => {
    loadMappings();
    loadStats();
  }, [loadMappings, loadStats]);

  function openCreate() {
    setEditTarget(null);
    setForm(emptyForm());
    setFormError("");
    setShowForm(true);
  }

  function openEdit(m) {
    setEditTarget(m);
    setForm({
      source_type: m.source_type,
      name: m.name,
      enabled: m.enabled,
      field_map: JSON.stringify(m.field_map || {}, null, 2),
      severity_map: JSON.stringify(m.severity_map || {}, null, 2),
      default_signal_type: m.default_signal_type || "alert",
      default_severity: m.default_severity || "medium",
      title_template: m.title_template || "",
      sample_payload: m.sample_payload || "",
    });
    setFormError("");
    setShowForm(true);
  }

  async function handleSave() {
    setFormSaving(true);
    setFormError("");
    try {
      const parsed = parseForm(form);
      if (editTarget) {
        await updateSchemaMapping(editTarget.id, parsed);
      } else {
        await createSchemaMapping(parsed);
      }
      setShowForm(false);
      await loadMappings();
      await loadStats();
    } catch (e) {
      setFormError(e.message);
    } finally {
      setFormSaving(false);
    }
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      await deleteSchemaMapping(deleteTarget.id);
      setDeleteTarget(null);
      await loadMappings();
      await loadStats();
    } catch (e) {
      setError(e.message);
    }
  }

  function parseForm(f) {
    let fieldMap = {};
    let severityMap = {};
    try { fieldMap = JSON.parse(f.field_map || "{}"); } catch { throw new Error("field_map is not valid JSON"); }
    try { severityMap = JSON.parse(f.severity_map || "{}"); } catch { throw new Error("severity_map is not valid JSON"); }
    return {
      source_type: f.source_type.trim(),
      name: f.name.trim(),
      enabled: f.enabled,
      field_map: fieldMap,
      severity_map: severityMap,
      default_signal_type: f.default_signal_type,
      default_severity: f.default_severity,
      title_template: f.title_template.trim(),
      sample_payload: f.sample_payload.trim(),
    };
  }

  function statsFor(sourceType) {
    return stats.find((s) => s.source_type === sourceType);
  }

  const ingestUrl = (sourceType) =>
    `/api/v1/ingest/custom/${sourceType}`;

  return (
    <div style={{ padding: "24px", fontFamily: "sans-serif", color: "#e2e8f0" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "20px" }}>
        <div>
          <h2 style={{ margin: 0, fontSize: "22px", color: "#f1f5f9" }}>Schema Registry</h2>
          <p style={{ margin: "4px 0 0", fontSize: "13px", color: "#94a3b8" }}>
            Map custom source payloads to the canonical event model. Plug in any tool — no code required.
          </p>
        </div>
        <button onClick={openCreate} style={btnStyle("blue")}>+ New Mapping</button>
      </div>

      {/* Tabs */}
      <div style={{ display: "flex", gap: "8px", marginBottom: "20px", borderBottom: "1px solid #334155" }}>
        {[["mappings", "Mappings"], ["stats", "Usage Stats"]].map(([key, label]) => (
          <button key={key} onClick={() => setTab(key)} style={tabStyle(tab === key)}>
            {label}
          </button>
        ))}
      </div>

      {error && <div style={alertStyle("red")}>{error}</div>}

      {tab === "mappings" && (
        <MappingsTab
          mappings={mappings}
          loading={loading}
          statsFor={statsFor}
          ingestUrl={ingestUrl}
          onEdit={openEdit}
          onDelete={setDeleteTarget}
        />
      )}

      {tab === "stats" && <StatsTab stats={stats} />}

      {/* Form modal */}
      {showForm && (
        <Modal onClose={() => setShowForm(false)}>
          <h3 style={{ margin: "0 0 16px", color: "#f1f5f9" }}>
            {editTarget ? "Edit Schema Mapping" : "New Schema Mapping"}
          </h3>
          <MappingForm form={form} onChange={setForm} />
          {formError && <div style={{ ...alertStyle("red"), marginTop: "12px" }}>{formError}</div>}
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
            <button onClick={() => setShowForm(false)} style={btnStyle("gray")}>Cancel</button>
            <button onClick={handleSave} disabled={formSaving} style={btnStyle("blue")}>
              {formSaving ? "Saving…" : "Save Mapping"}
            </button>
          </div>
        </Modal>
      )}

      {/* Delete confirm modal */}
      {deleteTarget && (
        <Modal onClose={() => setDeleteTarget(null)}>
          <h3 style={{ margin: "0 0 12px", color: "#f87171" }}>Delete Schema Mapping</h3>
          <p style={{ color: "#cbd5e1" }}>
            Delete <strong>{deleteTarget.source_type}</strong>? Any custom source sending to{" "}
            <code>/api/v1/ingest/custom/{deleteTarget.source_type}</code> will receive a 400 error.
          </p>
          <div style={{ display: "flex", gap: "8px", justifyContent: "flex-end", marginTop: "20px" }}>
            <button onClick={() => setDeleteTarget(null)} style={btnStyle("gray")}>Cancel</button>
            <button onClick={confirmDelete} style={btnStyle("red")}>Delete</button>
          </div>
        </Modal>
      )}
    </div>
  );
}

function MappingsTab({ mappings, loading, statsFor, ingestUrl, onEdit, onDelete }) {
  if (loading) return <p style={{ color: "#94a3b8" }}>Loading…</p>;
  if (mappings.length === 0)
    return (
      <div style={{ textAlign: "center", padding: "48px 0", color: "#64748b" }}>
        No schema mappings yet. Click <strong>+ New Mapping</strong> to add one.
      </div>
    );

  return (
    <div style={{ display: "grid", gap: "12px" }}>
      {mappings.map((m) => {
        const st = statsFor(m.source_type);
        return (
          <div key={m.id} style={cardStyle}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start" }}>
              <div>
                <div style={{ display: "flex", gap: "8px", alignItems: "center" }}>
                  <span style={{ fontWeight: 600, fontSize: "15px", color: "#f1f5f9" }}>{m.name}</span>
                  <span style={badge(m.enabled ? "green" : "gray")}>{m.enabled ? "enabled" : "disabled"}</span>
                </div>
                <div style={{ fontSize: "12px", color: "#64748b", marginTop: "2px" }}>
                  source_type: <code style={{ color: "#7dd3fc" }}>{m.source_type}</code>
                </div>
              </div>
              <div style={{ display: "flex", gap: "8px" }}>
                <button onClick={() => onEdit(m)} style={btnSm("blue")}>Edit</button>
                <button onClick={() => onDelete(m)} style={btnSm("red")}>Delete</button>
              </div>
            </div>

            <div style={{ marginTop: "10px", display: "flex", gap: "24px", fontSize: "13px", flexWrap: "wrap" }}>
              <div>
                <span style={{ color: "#94a3b8" }}>Default signal: </span>
                <span style={{ color: "#e2e8f0" }}>{m.default_signal_type || "—"}</span>
              </div>
              <div>
                <span style={{ color: "#94a3b8" }}>Default severity: </span>
                <span style={{ color: "#e2e8f0" }}>{m.default_severity || "—"}</span>
              </div>
              {st && (
                <div>
                  <span style={{ color: "#94a3b8" }}>Events ingested: </span>
                  <span style={{ color: "#e2e8f0" }}>{st.events_ingested.toLocaleString()}</span>
                </div>
              )}
            </div>

            <div style={{ marginTop: "10px", fontSize: "12px", color: "#64748b" }}>
              POST to:{" "}
              <code style={{ color: "#a5b4fc", background: "#1e293b", padding: "2px 6px", borderRadius: "4px" }}>
                {ingestUrl(m.source_type)}
              </code>
              <span style={{ marginLeft: "8px" }}>with <code>X-Source-Token</code> header</span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function StatsTab({ stats }) {
  if (stats.length === 0)
    return <p style={{ color: "#64748b" }}>No stats available yet. Start ingesting events to see usage.</p>;

  return (
    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }}>
      <thead>
        <tr style={{ color: "#94a3b8", borderBottom: "1px solid #334155" }}>
          {["Source Type", "Name", "Enabled", "Events Ingested", "Last Ingested"].map((h) => (
            <th key={h} style={{ textAlign: "left", padding: "8px 12px", fontWeight: 500 }}>{h}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {stats.map((s) => (
          <tr key={s.source_type} style={{ borderBottom: "1px solid #1e293b" }}>
            <td style={{ padding: "8px 12px", color: "#7dd3fc" }}>{s.source_type}</td>
            <td style={{ padding: "8px 12px" }}>{s.name}</td>
            <td style={{ padding: "8px 12px" }}>
              <span style={badge(s.enabled ? "green" : "gray")}>{s.enabled ? "yes" : "no"}</span>
            </td>
            <td style={{ padding: "8px 12px" }}>{s.events_ingested.toLocaleString()}</td>
            <td style={{ padding: "8px 12px", color: "#64748b" }}>
              {s.last_ingested_at ? new Date(s.last_ingested_at).toLocaleString() : "—"}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

function MappingForm({ form, onChange }) {
  const set = (k) => (e) => onChange({ ...form, [k]: e.target.type === "checkbox" ? e.target.checked : e.target.value });

  return (
    <div style={{ display: "grid", gap: "14px" }}>
      <Field label="Source Type (slug)" required>
        <input style={inputStyle} value={form.source_type} onChange={set("source_type")}
          placeholder="my-custom-tool" disabled={false} />
        <div style={hintStyle}>Unique identifier — used in the ingest URL and ingest_schema field</div>
      </Field>

      <Field label="Display Name" required>
        <input style={inputStyle} value={form.name} onChange={set("name")} placeholder="My Custom Tool" />
      </Field>

      <Field label="">
        <label style={{ display: "flex", alignItems: "center", gap: "8px", cursor: "pointer" }}>
          <input type="checkbox" checked={form.enabled} onChange={set("enabled")} />
          <span style={{ color: "#e2e8f0" }}>Enabled</span>
        </label>
      </Field>

      <Field label="Field Map (JSON)" required>
        <textarea style={{ ...inputStyle, height: "90px", fontFamily: "monospace", fontSize: "12px" }}
          value={form.field_map} onChange={set("field_map")} />
        <div style={hintStyle}>
          Map source JSON keys → canonical fields: service, resource, environment, severity, signal_type, title, message, external_id, timestamp
        </div>
      </Field>

      <Field label="Severity Map (JSON)">
        <textarea style={{ ...inputStyle, height: "70px", fontFamily: "monospace", fontSize: "12px" }}
          value={form.severity_map} onChange={set("severity_map")} />
        <div style={hintStyle}>Maps source severity values → critical | high | medium | low (case-insensitive keys)</div>
      </Field>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "12px" }}>
        <Field label="Default Signal Type">
          <select style={inputStyle} value={form.default_signal_type} onChange={set("default_signal_type")}>
            {SIGNAL_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </Field>
        <Field label="Default Severity">
          <select style={inputStyle} value={form.default_severity} onChange={set("default_severity")}>
            {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </Field>
      </div>

      <Field label="Title Template">
        <input style={inputStyle} value={form.title_template} onChange={set("title_template")}
          placeholder="$alertname on $service" />
        <div style={hintStyle}>$field_name tokens are replaced with resolved values. Leave blank to use the title field directly.</div>
      </Field>

      <Field label="Sample Payload (optional, for documentation)">
        <textarea style={{ ...inputStyle, height: "80px", fontFamily: "monospace", fontSize: "12px" }}
          value={form.sample_payload} onChange={set("sample_payload")}
          placeholder='{"alert_level": "fire", "app_name": "checkout", "message": "CPU > 90%"}' />
      </Field>
    </div>
  );
}

function Field({ label, required, children }) {
  return (
    <div>
      {label && (
        <label style={{ fontSize: "12px", fontWeight: 600, color: "#94a3b8", display: "block", marginBottom: "5px" }}>
          {label}{required && <span style={{ color: "#f87171" }}> *</span>}
        </label>
      )}
      {children}
    </div>
  );
}

function Modal({ onClose: _onClose, children }) {
  return (
    <div style={{
      position: "fixed", inset: 0, background: "rgba(0,0,0,0.7)",
      display: "flex", alignItems: "center", justifyContent: "center", zIndex: 1000,
    }}>
      <div style={{
        background: "#1e293b", borderRadius: "12px", padding: "28px",
        width: "640px", maxWidth: "95vw", maxHeight: "90vh", overflowY: "auto",
        border: "1px solid #334155",
      }}>
        {children}
      </div>
    </div>
  );
}

// ── Styles ────────────────────────────────────────────────────────────────────

const cardStyle = {
  background: "#1e293b",
  border: "1px solid #334155",
  borderRadius: "10px",
  padding: "16px 20px",
};

const inputStyle = {
  width: "100%",
  background: "#0f172a",
  border: "1px solid #334155",
  borderRadius: "6px",
  color: "#e2e8f0",
  padding: "8px 10px",
  fontSize: "13px",
  boxSizing: "border-box",
};

const hintStyle = { fontSize: "11px", color: "#64748b", marginTop: "4px" };

function tabStyle(active) {
  return {
    background: "none",
    border: "none",
    borderBottom: active ? "2px solid #3b82f6" : "2px solid transparent",
    color: active ? "#93c5fd" : "#64748b",
    padding: "8px 16px",
    cursor: "pointer",
    fontSize: "14px",
    marginBottom: "-1px",
  };
}

function btnStyle(color) {
  const colors = {
    blue: { bg: "#2563eb", hover: "#1d4ed8" },
    red: { bg: "#dc2626", hover: "#b91c1c" },
    gray: { bg: "#374151", hover: "#4b5563" },
  };
  const c = colors[color] || colors.gray;
  return {
    background: c.bg,
    color: "#fff",
    border: "none",
    borderRadius: "6px",
    padding: "8px 16px",
    fontSize: "13px",
    cursor: "pointer",
    fontWeight: 500,
  };
}

function btnSm(color) {
  return { ...btnStyle(color), padding: "4px 10px", fontSize: "12px" };
}

function badge(color) {
  const colors = {
    green: { bg: "#052e16", text: "#4ade80", border: "#166534" },
    gray: { bg: "#1e293b", text: "#94a3b8", border: "#334155" },
    red: { bg: "#450a0a", text: "#f87171", border: "#7f1d1d" },
  };
  const c = colors[color] || colors.gray;
  return {
    background: c.bg,
    color: c.text,
    border: `1px solid ${c.border}`,
    borderRadius: "12px",
    padding: "1px 8px",
    fontSize: "11px",
    fontWeight: 600,
  };
}

function alertStyle(color) {
  return {
    background: color === "red" ? "#450a0a" : "#0f2a1a",
    color: color === "red" ? "#fca5a5" : "#4ade80",
    border: `1px solid ${color === "red" ? "#7f1d1d" : "#166534"}`,
    borderRadius: "6px",
    padding: "10px 14px",
    fontSize: "13px",
  };
}
