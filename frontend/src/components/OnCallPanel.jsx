// OnCallPanel.jsx — Phase 3, Week 7
// On-call scheduling with rotation management and follow-the-sun support.

import { useState, useEffect, useCallback } from "react";
import { getOnCallSchedules, createOnCallSchedule, deleteOnCallSchedule, getOnCallByID, addOnCallOverride } from "../api/phase3.js";

function ScheduleCard({ schedule, onRefresh }) {
  const [detail, setDetail] = useState(null);
  const [showOverride, setShowOverride] = useState(false);
  const [overrideForm, setOverrideForm] = useState({ override_user: "", start_time: "", end_time: "", reason: "" });
  const [overrideError, setOverrideError] = useState("");

  async function loadDetail() {
    try { const d = await getOnCallByID(schedule.id); setDetail(d); } catch { /* ignore — schedule card shows without detail */ }
  }

  useEffect(() => {
    let alive = true;
    getOnCallByID(schedule.id)
      .then((d) => { if (alive) setDetail(d); })
      .catch(() => {});
    return () => { alive = false; };
  }, [schedule.id]);

  async function handleOverride(e) {
    e.preventDefault();
    setOverrideError("");
    try {
      await addOnCallOverride(schedule.id, overrideForm);
      setShowOverride(false);
      setOverrideForm({ override_user: "", start_time: "", end_time: "", reason: "" });
      loadDetail();
    } catch (err) { setOverrideError("Failed: " + err.message); }
  }

  async function handleDelete() {
    try { await deleteOnCallSchedule(schedule.id); onRefresh(); } catch { /* ignore — list refreshes on next poll */ }
  }

  const current = detail?.current_user;
  const next = detail?.next_user;
  const override = detail?.override;
  const nextRot = detail?.next_rotation;

  return (
    <div className="oncall-card">
      <div className="oncall-card-header">
        <div>
          <div className="slo-name">{schedule.team_name}</div>
          <div className="slo-service">{schedule.rotation_type} rotation · {schedule.timezone}</div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button className="lux-secondary-btn small" onClick={() => setShowOverride(!showOverride)}>Override</button>
          <button className="lux-secondary-btn small" onClick={handleDelete}>Delete</button>
        </div>
      </div>

      {/* Current on-call */}
      <div className="oncall-status">
        <div className="oncall-now">
          <div className="oncall-indicator" />
          <div>
            <div className="oncall-who">{override ? override.override_user : current?.user_name || "—"}</div>
            <div className="slo-service">
              {override ? `Override (${override.reason || "manual"})` : "Currently on-call"}
            </div>
          </div>
        </div>
        <div className="oncall-next">
          <div className="slo-metric-label">Next</div>
          <div>{next?.user_name || "—"}</div>
          <div className="slo-service">
            {nextRot ? new Date(nextRot).toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" }) : "—"}
          </div>
        </div>
      </div>

      {/* Members */}
      <div className="oncall-members">
        <div className="slo-metric-label">Rotation order</div>
        <div className="oncall-member-list">
          {(schedule.members || []).sort((a, b) => a.position - b.position).map((m, i) => (
            <div key={m.id || i} className="oncall-member">
              <span className="oncall-pos">#{m.position + 1}</span>
              <span>{m.user_name}</span>
              {m.user_email && <span className="slo-service">{m.user_email}</span>}
            </div>
          ))}
        </div>
      </div>

      {/* Override form */}
      {showOverride && (
        <form className="p3-form" onSubmit={handleOverride} style={{ marginTop: "0.75rem" }}>
          <div className="p3-form-grid">
            <label>
              <span className="pm-label">Override User</span>
              <input className="pm-input" value={overrideForm.override_user}
                onChange={e => setOverrideForm({ ...overrideForm, override_user: e.target.value })} required />
            </label>
            <label>
              <span className="pm-label">Start Time</span>
              <input className="pm-input" type="datetime-local" value={overrideForm.start_time}
                onChange={e => setOverrideForm({ ...overrideForm, start_time: e.target.value })} required />
            </label>
            <label>
              <span className="pm-label">End Time</span>
              <input className="pm-input" type="datetime-local" value={overrideForm.end_time}
                onChange={e => setOverrideForm({ ...overrideForm, end_time: e.target.value })} required />
            </label>
            <label>
              <span className="pm-label">Reason</span>
              <input className="pm-input" value={overrideForm.reason}
                onChange={e => setOverrideForm({ ...overrideForm, reason: e.target.value })} />
            </label>
          </div>
          {overrideError && (
            <div style={{ marginTop: "0.4rem", fontSize: "0.82rem", color: "#fca5a5" }}>{overrideError}</div>
          )}
          <button className="lux-primary-btn small" type="submit" style={{ marginTop: "0.5rem" }}>Add Override</button>
        </form>
      )}
    </div>
  );
}

export default function OnCallPanel() {
  const [schedules, setSchedules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);
  const [createError, setCreateError] = useState("");
  const [form, setForm] = useState({ team_name: "", timezone: "UTC", rotation_type: "weekly", membersRaw: "" });

  const load = useCallback(async () => {
    try {
      const data = await getOnCallSchedules();
      setSchedules(Array.isArray(data) ? data : []);
    } catch { setSchedules([]); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  async function handleCreate(e) {
    e.preventDefault();
    setCreateError("");
    const members = form.membersRaw.split("\n").filter(Boolean).map((line) => {
      const [name, email] = line.split(",").map(s => s.trim());
      return { user_name: name, user_email: email || "" };
    });
    try {
      await createOnCallSchedule({ team_name: form.team_name, timezone: form.timezone, rotation_type: form.rotation_type, members });
      setShowCreate(false);
      setForm({ team_name: "", timezone: "UTC", rotation_type: "weekly", membersRaw: "" });
      load();
    } catch (err) { setCreateError("Failed: " + err.message); }
  }

  return (
    <div className="p3-panel">
      <div className="p3-header">
        <div>
          <div className="lux-eyebrow">ON-CALL MANAGEMENT</div>
          <h2 style={{ margin: "0.25rem 0" }}>On-Call Schedules</h2>
          <div className="lux-muted" style={{ fontSize: "0.8rem" }}>
            Built-in rotation management. Follow-the-sun support. Replace PagerDuty for SMBs.
          </div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <button className="lux-secondary-btn" onClick={load}>Refresh</button>
          <button className="lux-primary-btn" onClick={() => setShowCreate(!showCreate)}>
            {showCreate ? "Cancel" : "+ New Schedule"}
          </button>
        </div>
      </div>

      {showCreate && (
        <form className="p3-form" onSubmit={handleCreate}>
          <div className="p3-form-grid">
            <label>
              <span className="pm-label">Team Name</span>
              <input className="pm-input" value={form.team_name} onChange={e => setForm({ ...form, team_name: e.target.value })} required />
            </label>
            <label>
              <span className="pm-label">Timezone</span>
              <select className="pm-input" value={form.timezone} onChange={e => setForm({ ...form, timezone: e.target.value })}>
                {["UTC", "America/New_York", "America/Chicago", "America/Los_Angeles", "Europe/London", "Europe/Berlin", "Asia/Tokyo", "Asia/Kolkata", "Australia/Sydney"].map(tz => (
                  <option key={tz} value={tz}>{tz}</option>
                ))}
              </select>
            </label>
            <label>
              <span className="pm-label">Rotation</span>
              <select className="pm-input" value={form.rotation_type} onChange={e => setForm({ ...form, rotation_type: e.target.value })}>
                <option value="weekly">Weekly</option>
                <option value="daily">Daily</option>
              </select>
            </label>
          </div>
          <label style={{ marginTop: "0.5rem", display: "block" }}>
            <span className="pm-label">Members <span className="pm-hint">One per line: Name, email</span></span>
            <textarea className="pm-textarea" rows={4} value={form.membersRaw}
              onChange={e => setForm({ ...form, membersRaw: e.target.value })}
              placeholder={"Alice Smith, alice@company.com\nBob Jones, bob@company.com\nCarol Lee, carol@company.com"} required />
          </label>
          {createError && (
            <div style={{ marginTop: "0.4rem", fontSize: "0.82rem", color: "#fca5a5" }}>{createError}</div>
          )}
          <button className="lux-primary-btn" type="submit" style={{ marginTop: "0.75rem" }}>Create Schedule</button>
        </form>
      )}

      {loading ? (
        <div className="lux-muted" style={{ padding: "2rem 0" }}>Loading schedules...</div>
      ) : schedules.length === 0 ? (
        <div className="p3-empty">
          <div style={{ fontSize: "2rem" }}>📅</div>
          <p>No on-call schedules. Click "+ New Schedule" to set up your first rotation.</p>
        </div>
      ) : (
        <div className="oncall-grid">
          {schedules.map(s => <ScheduleCard key={s.id} schedule={s} onRefresh={load} />)}
        </div>
      )}
    </div>
  );
}
