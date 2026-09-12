/**
 * MuteDialog — create an alert mute from one alert or from a whole incident.
 *
 * The form is pre-filled by /mutes/suggest so the matching rules (which label
 * is the device, which is the alert name) live only on the server. Devices are
 * never pre-selected: the admin picks specific devices or "All devices".
 * A live preview shows how many alerts from the last 7 days would have been
 * muted before anything is saved.
 */
import { useEffect, useMemo, useRef, useState } from "react";
import { BellOff, Plus, X } from "lucide-react";

import { createMute, previewMute, suggestMute } from "../api/mutes.js";

const ALL = "all";
const MUTE_DAYS = 7;

function fmtDate(d) {
  return d.toLocaleString([], { weekday: "short", day: "numeric", month: "short", hour: "2-digit", minute: "2-digit" });
}

/* A checkbox list with an "All" switch and a free-text "add" box. */
function ChoiceList({ id, label, hint, allLabel, options, selected, onChange, addPlaceholder }) {
  const [draft, setDraft] = useState("");
  const allOn = selected.includes(ALL);

  function toggle(value) {
    const next = selected.filter((v) => v !== ALL);
    onChange(next.includes(value) ? next.filter((v) => v !== value) : [...next, value]);
  }

  function add() {
    const v = draft.trim();
    if (!v) return;
    if (v.toLowerCase() === ALL) {
      onChange([ALL]);
    } else if (!selected.some((s) => s.toLowerCase() === v.toLowerCase())) {
      onChange([...selected.filter((s) => s !== ALL), v]);
    }
    setDraft("");
  }

  const custom = selected.filter((v) => v !== ALL && !options.includes(v));

  return (
    <fieldset className="mute-field">
      <legend className="form-label">{label}</legend>
      {hint && <p className="form-hint">{hint}</p>}
      <div className="mute-options">
        <label className="mute-option is-all">
          <input
            type="checkbox"
            id={`${id}-all`}
            checked={allOn}
            onChange={() => onChange(allOn ? [] : [ALL])}
          />
          {allLabel}
        </label>
        {[...options, ...custom].map((opt) => (
          <label key={opt} className={`mute-option${allOn ? " is-disabled" : ""}`}>
            <input
              type="checkbox"
              checked={!allOn && selected.includes(opt)}
              disabled={allOn}
              onChange={() => toggle(opt)}
            />
            {opt}
          </label>
        ))}
      </div>
      {!allOn && (
        <div className="mute-add">
          <input
            id={`${id}-add`}
            className="form-input"
            value={draft}
            placeholder={addPlaceholder}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                add();
              }
            }}
          />
          <button type="button" className="btn btn-ghost btn-sm" onClick={add} disabled={!draft.trim()}>
            <Plus size={12} /> Add
          </button>
        </div>
      )}
    </fieldset>
  );
}

export default function MuteDialog({ token, eventId = "", incidentId = "", onClose, onCreated }) {
  const [suggestion, setSuggestion] = useState(null);
  const [loadError, setLoadError] = useState("");
  const [alertNames, setAlertNames] = useState([]);
  const [devices, setDevices] = useState([]);
  const [service, setService] = useState("");
  const [environment, setEnvironment] = useState("");
  const [valueMin, setValueMin] = useState("");
  const [valueMax, setValueMax] = useState("");
  const [reason, setReason] = useState("");
  const [preview, setPreview] = useState(null);
  const [previewError, setPreviewError] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const dialogRef = useRef(null);
  const endsAt = useMemo(() => new Date(Date.now() + MUTE_DAYS * 24 * 3600 * 1000), []);

  /* Pre-fill from the alert or incident. */
  useEffect(() => {
    let cancelled = false;
    suggestMute(token, { eventId, incidentId })
      .then((s) => {
        if (cancelled) return;
        setSuggestion(s);
        setAlertNames(s.alert_names || []);
        setService(s.service || "");
        setEnvironment(s.environment || "");
      })
      .catch((e) => !cancelled && setLoadError(e.message));
    return () => {
      cancelled = true;
    };
  }, [token, eventId, incidentId]);

  /* Escape closes; focus moves into the dialog on open. */
  useEffect(() => {
    const onKey = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    dialogRef.current?.focus();
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const body = useMemo(() => {
    const num = (v) => (v === "" || v == null ? null : Number(v));
    return {
      alert_names: alertNames,
      devices,
      service: service.trim(),
      environment: environment.trim(),
      value_min: num(valueMin),
      value_max: num(valueMax),
      reason: reason.trim(),
      incident_id: incidentId,
    };
  }, [alertNames, devices, service, environment, valueMin, valueMax, reason, incidentId]);

  const rangeInvalid =
    body.value_min != null && body.value_max != null && body.value_min > body.value_max;
  const matchersReady = alertNames.length > 0 && devices.length > 0 && !rangeInvalid;
  const supported = Boolean(suggestion?.supported);

  /* Everything except the reason — the preview does not change while typing it. */
  const matcherKey = JSON.stringify({ ...body, reason: undefined });

  /* Live preview, debounced so typing a range does not flood the API. */
  useEffect(() => {
    if (!supported || !matchersReady) return undefined;
    let cancelled = false;
    const t = setTimeout(() => {
      previewMute(token, JSON.parse(matcherKey))
        .then((p) => {
          if (cancelled) return;
          setPreview(p);
          setPreviewError("");
        })
        .catch((e) => {
          if (cancelled) return;
          setPreview(null);
          setPreviewError(e.message);
        });
    }, 400);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [token, supported, matchersReady, matcherKey]);

  async function save(e) {
    e.preventDefault();
    setSaving(true);
    setSaveError("");
    try {
      const mute = await createMute(token, body);
      onCreated?.(mute);
      onClose();
    } catch (err) {
      setSaveError(err.message);
    } finally {
      setSaving(false);
    }
  }

  const deviceOptions = useMemo(() => {
    const seen = new Set();
    return [...(suggestion?.devices || []), ...(suggestion?.known_devices || [])].filter((d) => {
      const k = d.toLowerCase();
      if (seen.has(k)) return false;
      seen.add(k);
      return true;
    });
  }, [suggestion]);

  const topDevices = preview
    ? Object.entries(preview.by_device || {}).sort((a, b) => b[1] - a[1]).slice(0, 4)
    : [];

  return (
    <>
      <div className="mute-scrim" onClick={onClose} aria-hidden="true" />
      <div
        ref={dialogRef}
        className="mute-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="mute-dialog-title"
        tabIndex={-1}
      >
        <header className="mute-dialog-head">
          <span className="mute-dialog-icon" aria-hidden="true"><BellOff size={15} /></span>
          <div className="mute-dialog-headings">
            <h2 id="mute-dialog-title" className="mute-dialog-title">
              {incidentId ? "Mute alerts like this incident's" : "Mute this alert"}
            </h2>
            <p className="mute-dialog-sub">
              Matching alerts are logged but won't create or update incidents until {fmtDate(endsAt)}.
            </p>
          </div>
          <button type="button" className="icon-btn" onClick={onClose} title="Close">
            <X size={14} />
          </button>
        </header>

        {loadError ? (
          <div className="mute-dialog-body">
            <p className="form-error">{loadError}</p>
          </div>
        ) : !suggestion ? (
          <div className="mute-dialog-body">
            <p className="form-hint">Loading alert details…</p>
          </div>
        ) : !suggestion.supported ? (
          <div className="mute-dialog-body">
            <p className="mute-notice">
              Muting works for Prometheus, Grafana, Zabbix and OpenTelemetry metric alerts. This{" "}
              {incidentId ? "incident's alerts come" : "alert comes"} from a source muting doesn't cover yet.
            </p>
          </div>
        ) : (
          <form className="mute-dialog-body" onSubmit={save}>
            <ChoiceList
              id="mute-alert-names"
              label="Alert name / KPI"
              allLabel="All alert names"
              options={suggestion.alert_names || []}
              selected={alertNames}
              onChange={setAlertNames}
              addPlaceholder="Add an alert name, e.g. HighCPU"
            />

            <ChoiceList
              id="mute-devices"
              label="Devices"
              hint="Pick the devices to mute, or All devices."
              allLabel="All devices"
              options={deviceOptions}
              selected={devices}
              onChange={setDevices}
              addPlaceholder="Add a device, e.g. web01"
            />

            <div className="mute-row">
              <div className="form-group">
                <label className="form-label" htmlFor="mute-service">Service</label>
                <input id="mute-service" className="form-input" value={service}
                  placeholder="Any service" onChange={(e) => setService(e.target.value)} />
              </div>
              <div className="form-group">
                <label className="form-label" htmlFor="mute-environment">Environment</label>
                <input id="mute-environment" className="form-input" value={environment}
                  placeholder="Any environment" onChange={(e) => setEnvironment(e.target.value)} />
              </div>
            </div>

            <fieldset className="mute-field">
              <legend className="form-label">Value range <span className="mute-optional">optional</span></legend>
              <p className="form-hint">
                Mute only while the value is inside this range. Values outside it, and alerts with no value, still come through.
              </p>
              <div className="mute-row">
                <div className="form-group">
                  <label className="form-label" htmlFor="mute-value-min">From</label>
                  <input id="mute-value-min" className="form-input" type="number" step="any" inputMode="decimal"
                    value={valueMin} placeholder="e.g. 80" onChange={(e) => setValueMin(e.target.value)} />
                </div>
                <div className="form-group">
                  <label className="form-label" htmlFor="mute-value-max">To</label>
                  <input id="mute-value-max" className="form-input" type="number" step="any" inputMode="decimal"
                    value={valueMax} placeholder="e.g. 90" onChange={(e) => setValueMax(e.target.value)} />
                </div>
              </div>
              {rangeInvalid && <p className="form-error">"From" must be less than or equal to "To".</p>}
            </fieldset>

            <div className="form-group">
              <label className="form-label" htmlFor="mute-reason">Reason</label>
              <textarea id="mute-reason" className="form-textarea mute-reason" value={reason} maxLength={500}
                placeholder="Why is this safe to mute? e.g. Known batch job, fix planned for Friday"
                onChange={(e) => setReason(e.target.value)} />
            </div>

            <div className={`mute-preview${preview && preview.matched > 0 ? " has-matches" : ""}`} aria-live="polite">
              {!matchersReady ? (
                <span>Choose at least one alert name and one device to see what this would mute.</span>
              ) : previewError ? (
                <span className="form-error">{previewError}</span>
              ) : !preview ? (
                <span>Checking the last 7 days…</span>
              ) : (
                <>
                  <span>
                    Would have muted <strong>{preview.matched}{preview.truncated ? "+" : ""}</strong>{" "}
                    alert{preview.matched === 1 ? "" : "s"} in the last {preview.window_days} days
                  </span>
                  {topDevices.length > 0 && (
                    <span className="mute-preview-devices">
                      {topDevices.map(([d, n]) => `${d || "no device"} ×${n}`).join(" · ")}
                    </span>
                  )}
                </>
              )}
            </div>

            {saveError && <p className="form-error">{saveError}</p>}

            <footer className="mute-dialog-foot">
              <span className="mute-ends">Ends {fmtDate(endsAt)} · 7 days</span>
              <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>Cancel</button>
              <button type="submit" className="btn btn-primary btn-sm"
                disabled={saving || !matchersReady || !reason.trim()}>
                {saving ? <span className="spinner spinner-xs" /> : <BellOff size={12} />}
                Mute for 7 days
              </button>
            </footer>
          </form>
        )}
      </div>
    </>
  );
}
