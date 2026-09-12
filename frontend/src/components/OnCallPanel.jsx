/**
 * On-call schedules — rotation management with follow-the-sun support.
 *
 * Each card answers "who is on call right now" before anything else, since
 * that is the only question asked during an incident. Rotation order and
 * overrides sit below as configuration.
 */
import { useState, useEffect, useCallback } from "react";
import { CalendarClock, Plus, RefreshCw, Trash2, UserCheck, Users, X } from "lucide-react";

import {
  getOnCallSchedules, createOnCallSchedule, deleteOnCallSchedule,
  getOnCallByID, addOnCallOverride,
} from "../api/phase3.js";
import {
  EmptyState, ErrorState, Page, PageHeader, Panel, SkeletonRows,
} from "./ui/Primitives.jsx";

const TIMEZONES = [
  "UTC", "America/New_York", "America/Chicago", "America/Los_Angeles",
  "Europe/London", "Europe/Berlin", "Asia/Tokyo", "Asia/Kolkata", "Australia/Sydney",
];

const EMPTY_OVERRIDE = { override_user: "", start_time: "", end_time: "", reason: "" };

function ScheduleCard({ schedule, onRefresh }) {
  const [detail, setDetail] = useState(null);
  const [showOverride, setShowOverride] = useState(false);
  const [overrideForm, setOverrideForm] = useState(EMPTY_OVERRIDE);
  const [overrideError, setOverrideError] = useState("");

  const loadDetail = useCallback(async () => {
    try {
      setDetail(await getOnCallByID(schedule.id));
    } catch {
      /* the card still renders from the schedule summary */
    }
  }, [schedule.id]);

  useEffect(() => {
    let alive = true;
    getOnCallByID(schedule.id)
      .then((d) => {
        if (alive) setDetail(d);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [schedule.id]);

  async function handleOverride(e) {
    e.preventDefault();
    setOverrideError("");
    try {
      await addOnCallOverride(schedule.id, overrideForm);
      setShowOverride(false);
      setOverrideForm(EMPTY_OVERRIDE);
      loadDetail();
    } catch (err) {
      setOverrideError(err.message || "Could not add override");
    }
  }

  async function handleDelete() {
    try {
      await deleteOnCallSchedule(schedule.id);
      onRefresh();
    } catch {
      /* the parent refresh reflects the true state */
    }
  }

  const current = detail?.current_user;
  const next = detail?.next_user;
  const override = detail?.override;
  const nextRot = detail?.next_rotation;
  const members = [...(schedule.members || [])].sort((a, b) => a.position - b.position);
  const set = (key) => (e) => setOverrideForm((f) => ({ ...f, [key]: e.target.value }));

  return (
    <article className="oncall-card">
      <header className="oncall-head">
        <div className="oncall-headings">
          <h3 className="oncall-team">{schedule.team_name}</h3>
          <p className="oncall-scope">
            {schedule.rotation_type} rotation · {schedule.timezone}
          </p>
        </div>
        <div className="oncall-head-actions">
          <button type="button" className="btn btn-ghost btn-xs" onClick={() => setShowOverride((v) => !v)}>
            {showOverride ? <X size={11} /> : <CalendarClock size={11} />}
            {showOverride ? "Cancel" : "Override"}
          </button>
          <button type="button" className="btn btn-ghost btn-xs" onClick={handleDelete}>
            <Trash2 size={11} /> Delete
          </button>
        </div>
      </header>

      {/* Who is on call right now — the only question that matters mid-incident. */}
      <div className="oncall-now">
        <span className={`oncall-now-avatar ${override ? "tone-warning" : "tone-success"}`}>
          <UserCheck size={16} />
        </span>
        <div className="oncall-now-text">
          <span className="oncall-now-name">
            {override ? override.override_user : current?.user_name || "—"}
          </span>
          <span className="oncall-now-label">
            {override ? `Override — ${override.reason || "manual"}` : "Currently on call"}
          </span>
        </div>
        <div className="oncall-next">
          <span className="oncall-next-label">Next</span>
          <span className="oncall-next-name">{next?.user_name || "—"}</span>
          <span className="oncall-next-date">
            {nextRot
              ? new Date(nextRot).toLocaleDateString(undefined, {
                  weekday: "short", month: "short", day: "numeric",
                })
              : "—"}
          </span>
        </div>
      </div>

      <div className="detail-section is-divided">
        <h4 className="detail-section-label">Rotation order</h4>
        {members.length === 0 ? (
          <p className="oncall-scope">No members configured.</p>
        ) : (
          <ol className="rotation-list">
            {members.map((m, i) => (
              <li key={m.id || i} className="rotation-item">
                <span className="rotation-pos">{m.position + 1}</span>
                <span className="rotation-name">{m.user_name}</span>
                {m.user_email && <span className="rotation-email">{m.user_email}</span>}
              </li>
            ))}
          </ol>
        )}
      </div>

      {showOverride && (
        <form className="oncall-override-form" onSubmit={handleOverride}>
          <div className="form-grid">
            <div className="form-group">
              <label className="form-label" htmlFor={`ov-user-${schedule.id}`}>Override user</label>
              <input id={`ov-user-${schedule.id}`} className="form-input" value={overrideForm.override_user} onChange={set("override_user")} required />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor={`ov-start-${schedule.id}`}>Start</label>
              <input id={`ov-start-${schedule.id}`} className="form-input" type="datetime-local" value={overrideForm.start_time} onChange={set("start_time")} required />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor={`ov-end-${schedule.id}`}>End</label>
              <input id={`ov-end-${schedule.id}`} className="form-input" type="datetime-local" value={overrideForm.end_time} onChange={set("end_time")} required />
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor={`ov-reason-${schedule.id}`}>Reason</label>
              <input id={`ov-reason-${schedule.id}`} className="form-input" value={overrideForm.reason} onChange={set("reason")} />
            </div>
          </div>
          {overrideError && <p className="form-error">{overrideError}</p>}
          <button className="btn btn-primary btn-sm" type="submit">Add override</button>
        </form>
      )}
    </article>
  );
}

const EMPTY_FORM = { team_name: "", timezone: "UTC", rotation_type: "weekly", membersRaw: "" };

export default function OnCallPanel() {
  const [schedules, setSchedules] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [createError, setCreateError] = useState("");
  const [form, setForm] = useState(EMPTY_FORM);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getOnCallSchedules();
      setSchedules(Array.isArray(data) ? data : []);
      setError("");
    } catch (e) {
      setError(e.message || "Could not load schedules");
      setSchedules([]);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleCreate(e) {
    e.preventDefault();
    setCreateError("");
    const members = form.membersRaw
      .split("\n")
      .filter(Boolean)
      .map((line) => {
        const [name, email] = line.split(",").map((s) => s.trim());
        return { user_name: name, user_email: email || "" };
      });
    try {
      await createOnCallSchedule({
        team_name: form.team_name,
        timezone: form.timezone,
        rotation_type: form.rotation_type,
        members,
      });
      setShowCreate(false);
      setForm(EMPTY_FORM);
      load();
    } catch (err) {
      setCreateError(err.message || "Could not create schedule");
    }
  }

  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));

  return (
    <Page>
      <PageHeader
        title="On-call schedules"
        meta="Rotation management with follow-the-sun support and manual overrides"
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={load} disabled={loading}>
              <RefreshCw size={13} className={loading ? "is-spinning" : ""} /> Refresh
            </button>
            <button type="button" className="btn btn-primary" onClick={() => setShowCreate((v) => !v)}>
              {showCreate ? <X size={13} /> : <Plus size={13} />}
              {showCreate ? "Cancel" : "New schedule"}
            </button>
          </>
        }
      />

      {showCreate && (
        <Panel title="Create a rotation" sub="members rotate in the order you list them">
          <form onSubmit={handleCreate}>
            <div className="form-grid">
              <div className="form-group">
                <label className="form-label" htmlFor="oc-team">Team name</label>
                <input id="oc-team" className="form-input" value={form.team_name} onChange={set("team_name")} required />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="oc-tz">Timezone</label>
                <select id="oc-tz" className="form-select" value={form.timezone} onChange={set("timezone")}>
                  {TIMEZONES.map((tz) => (
                    <option key={tz} value={tz}>{tz}</option>
                  ))}
                </select>
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="oc-rot">Rotation</label>
                <select id="oc-rot" className="form-select" value={form.rotation_type} onChange={set("rotation_type")}>
                  <option value="weekly">Weekly</option>
                  <option value="daily">Daily</option>
                </select>
              </div>
            </div>
            <div className="form-group">
              <label className="form-label" htmlFor="oc-members">Members</label>
              <textarea
                id="oc-members"
                className="form-textarea"
                rows={4}
                value={form.membersRaw}
                onChange={set("membersRaw")}
                placeholder={"Alice Smith, alice@company.com\nBob Jones, bob@company.com"}
                required
              />
              <p className="form-hint">One per line, as “Name, email”. Order sets the rotation.</p>
            </div>
            {createError && <p className="form-error">{createError}</p>}
            <button className="btn btn-primary" type="submit">Create schedule</button>
          </form>
        </Panel>
      )}

      {error ? (
        <ErrorState message={error} onRetry={load} />
      ) : loading ? (
        <SkeletonRows count={2} height={210} />
      ) : schedules.length === 0 ? (
        <EmptyState
          icon={Users}
          title="No on-call schedules"
          message="Set up a rotation so incidents reach whoever is actually on duty."
          action="Create your first rotation"
          onAction={() => setShowCreate(true)}
        />
      ) : (
        <div className="oncall-grid">
          {schedules.map((s) => (
            <ScheduleCard key={s.id} schedule={s} onRefresh={load} />
          ))}
        </div>
      )}
    </Page>
  );
}
