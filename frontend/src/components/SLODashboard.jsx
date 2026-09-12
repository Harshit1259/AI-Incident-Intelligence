/**
 * SLO Dashboard — service level objectives, error budget burn, time to breach.
 *
 * Reordered so the fleet-wide answer ("how many SLOs are in trouble?") is
 * readable before any individual card. Each SLO card leads with the number
 * that decides action — error budget remaining — rather than burying it under
 * four equal-weight metrics as the previous version did.
 */
import { useState, useEffect, useCallback } from "react";
import { AlertTriangle, CheckCircle2, Gauge, Plus, RefreshCw, Target, Trash2, X } from "lucide-react";

import { getSLOs, createSLO, deleteSLO } from "../api/phase3.js";
import {
  EmptyState, ErrorState, Grid, Page, PageHeader, Panel, SkeletonRows, StatTile,
} from "./ui/Primitives.jsx";

/* Budget thresholds. Below 10% remaining an SLO is effectively spent, which is
   a different conversation from "burning fast" — hence two bands, not one. */
function budgetTone(remaining) {
  if (remaining > 25) return "tone-success";
  if (remaining > 10) return "tone-warning";
  return "tone-danger";
}

const STATUS_TONE = {
  healthy: "tone-success",
  warning: "tone-warning",
  critical: "tone-danger",
  breached: "tone-danger",
};

const EMPTY_FORM = {
  service: "",
  name: "",
  description: "",
  target_percent: 99.9,
  window_days: 30,
  metric_type: "availability",
};

function SLOCard({ slo, onDelete }) {
  const def = slo.definition || {};
  const remaining = Math.max(0, Math.min(100, slo.error_budget_remaining || 0));
  const tone = STATUS_TONE[slo.status] || "tone-neutral";

  return (
    <article className={`slo-card ${tone}`}>
      <header className="slo-card-head">
        <div className="slo-card-headings">
          <h3 className="slo-card-name">{def.name || "Unnamed SLO"}</h3>
          <p className="slo-card-scope">
            {def.service} · {def.metric_type} · {def.window_days}d window
          </p>
        </div>
        <span className={`pill is-plain ${tone}`}>{slo.status || "unknown"}</span>
      </header>

      {/* Error budget leads — it is the number that decides whether to act. */}
      <div className="slo-budget">
        <div className="slo-budget-head">
          <span className="slo-budget-label">Error budget remaining</span>
          <span className={`slo-budget-value ${budgetTone(remaining)}`}>{remaining.toFixed(1)}%</span>
        </div>
        <div className={`slo-budget-track ${budgetTone(remaining)}`}>
          <div className="slo-budget-fill" style={{ width: `${remaining}%` }} />
        </div>
      </div>

      <dl className="slo-metrics">
        <div className="slo-metric">
          <dt>Current</dt>
          <dd>{(slo.current_percent || 0).toFixed(3)}%</dd>
        </div>
        <div className="slo-metric">
          <dt>Target</dt>
          <dd>{(def.target_percent || 99.9).toFixed(1)}%</dd>
        </div>
        <div className="slo-metric">
          <dt>Burn rate</dt>
          <dd>{(slo.burn_rate || 0).toFixed(2)}×</dd>
        </div>
        <div className="slo-metric">
          <dt>Time to breach</dt>
          <dd>{slo.time_to_breach_hours < 0 ? "∞" : `${(slo.time_to_breach_hours || 0).toFixed(0)}h`}</dd>
        </div>
      </dl>

      {def.description && <p className="slo-card-desc">{def.description}</p>}

      <footer className="slo-card-foot">
        <button type="button" className="btn btn-ghost btn-xs" onClick={() => onDelete(def.id)}>
          <Trash2 size={11} /> Delete
        </button>
      </footer>
    </article>
  );
}

export default function SLODashboard() {
  const [slos, setSlos] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [createError, setCreateError] = useState("");
  const [form, setForm] = useState(EMPTY_FORM);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const data = await getSLOs();
      setSlos(Array.isArray(data) ? data : []);
      setError("");
    } catch (e) {
      // Previously swallowed into an empty array, so a failed request looked
      // identical to "no SLOs defined".
      setError(e.message || "Could not load SLOs");
      setSlos([]);
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
    try {
      await createSLO({
        ...form,
        target_percent: parseFloat(form.target_percent),
        window_days: parseInt(form.window_days, 10),
      });
      setShowForm(false);
      setForm(EMPTY_FORM);
      load();
    } catch (err) {
      setCreateError(err.message || "Could not create SLO");
    }
  }

  async function handleDelete(id) {
    try {
      await deleteSLO(id);
      load();
    } catch {
      /* the reload below surfaces the true state */
    }
  }

  const healthy = slos.filter((s) => s.status === "healthy").length;
  const warning = slos.filter((s) => s.status === "warning").length;
  const critical = slos.filter((s) => s.status === "critical" || s.status === "breached").length;

  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));

  return (
    <Page>
      <PageHeader
        title="Service level objectives"
        meta="Error budget burn rate and time-to-breach projections per service"
        actions={
          <>
            <button type="button" className="btn btn-ghost" onClick={load} disabled={loading}>
              <RefreshCw size={13} className={loading ? "is-spinning" : ""} /> Refresh
            </button>
            <button type="button" className="btn btn-primary" onClick={() => setShowForm((v) => !v)}>
              {showForm ? <X size={13} /> : <Plus size={13} />}
              {showForm ? "Cancel" : "Define SLO"}
            </button>
          </>
        }
      />

      <Grid cols={4}>
        <StatTile label="Total SLOs" icon={Target} tone="neutral" value={slos.length} sub="defined" loading={loading} />
        <StatTile label="Healthy" icon={CheckCircle2} tone="success" value={healthy} sub="within budget" loading={loading} />
        <StatTile label="Warning" icon={Gauge} tone={warning > 0 ? "warning" : "neutral"} value={warning} sub="burning fast" loading={loading} />
        <StatTile label="Critical" icon={AlertTriangle} tone={critical > 0 ? "danger" : "neutral"} value={critical} sub="at or past breach" loading={loading} />
      </Grid>

      {showForm && (
        <Panel title="Define a new SLO" sub="targets are evaluated against live incident data">
          <form onSubmit={handleCreate}>
            <div className="form-grid">
              <div className="form-group">
                <label className="form-label" htmlFor="slo-service">Service</label>
                <input id="slo-service" className="form-input" value={form.service} onChange={set("service")} required />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="slo-name">SLO name</label>
                <input id="slo-name" className="form-input" value={form.name} onChange={set("name")} required />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="slo-target">Target %</label>
                <input id="slo-target" className="form-input" type="number" step="0.01" value={form.target_percent} onChange={set("target_percent")} />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="slo-window">Window (days)</label>
                <input id="slo-window" className="form-input" type="number" value={form.window_days} onChange={set("window_days")} />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="slo-metric">Metric type</label>
                <select id="slo-metric" className="form-select" value={form.metric_type} onChange={set("metric_type")}>
                  <option value="availability">Availability</option>
                  <option value="latency">Latency</option>
                  <option value="error_rate">Error rate</option>
                </select>
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="slo-desc">Description</label>
                <input id="slo-desc" className="form-input" value={form.description} onChange={set("description")} />
              </div>
            </div>
            {createError && <p className="form-error">{createError}</p>}
            <button className="btn btn-primary" type="submit">Create SLO</button>
          </form>
        </Panel>
      )}

      {error ? (
        <ErrorState message={error} onRetry={load} />
      ) : loading ? (
        <SkeletonRows count={3} height={190} />
      ) : slos.length === 0 ? (
        <EmptyState
          icon={Target}
          title="No SLOs defined"
          message="Define an objective to start tracking error budget burn for a service."
          action="Define your first SLO"
          onAction={() => setShowForm(true)}
        />
      ) : (
        <div className="slo-grid">
          {slos.map((slo) => (
            <SLOCard key={slo.definition?.id} slo={slo} onDelete={handleDelete} />
          ))}
        </div>
      )}
    </Page>
  );
}
