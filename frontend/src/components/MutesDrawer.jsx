/**
 * MutesDrawer — right-side panel listing alert mutes and the alerts they
 * stopped. Opened from the dashboard's "Muted alerts" card.
 *
 * Muted-alert entries are read-only: they never became incidents, so there is
 * nothing to open or close. Admins can end a mute early from here.
 */
import { useCallback, useEffect, useRef, useState } from "react";
import { BellOff, BellRing, RefreshCw, X } from "lucide-react";

import { getMuteLog, listMutes, unmute } from "../api/mutes.js";

const PAGE = 50;

function relativeTime(iso) {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const s = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (s < 60) return "just now";
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  if (s < 172800) return `${Math.round(s / 3600)}h ago`;
  return `${Math.round(s / 86400)}d ago`;
}

function endsIn(iso) {
  const ms = Date.parse(iso) - Date.now();
  if (Number.isNaN(ms) || ms <= 0) return "ended";
  const h = Math.round(ms / 3600000);
  if (h < 1) return "ends in under an hour";
  if (h < 48) return `ends in ${h}h`;
  return `ends in ${Math.round(h / 24)} days`;
}

function listLabel(list, allText) {
  if (!list?.length) return "—";
  if (list[0] === "all") return allText;
  return list.length <= 2 ? list.join(", ") : `${list.slice(0, 2).join(", ")} +${list.length - 2}`;
}

/* One-line summary of what a mute matches, e.g. "HighCPU on web01, web02". */
function describeMute(m) {
  return `${listLabel(m.alert_names, "All alert names")} on ${listLabel(m.devices, "all devices")}`;
}

function rangeLabel(m) {
  const lo = m.value_min, hi = m.value_max;
  if (lo == null && hi == null) return "";
  if (lo != null && hi != null) return `value ${lo}–${hi}`;
  return lo != null ? `value ≥ ${lo}` : `value ≤ ${hi}`;
}

function MuteRow({ mute, isAdmin, selected, onSelect, onUnmute, busy }) {
  const active = mute.status === "active";
  const scope = [mute.service && `service ${mute.service}`, mute.environment && `env ${mute.environment}`, rangeLabel(mute)]
    .filter(Boolean)
    .join(" · ");

  return (
    <li className={`mute-item${selected ? " is-selected" : ""}${active ? "" : " is-ended"}`}>
      <button type="button" className="mute-item-main" onClick={() => onSelect(mute.id)} aria-pressed={selected}>
        <span className="mute-item-title">{describeMute(mute)}</span>
        {scope && <span className="mute-item-scope">{scope}</span>}
        <span className="mute-item-reason">“{mute.reason}”</span>
        <span className="mute-item-meta">
          {mute.created_by || "admin"} · {active ? endsIn(mute.ends_at) : mute.status}
          {" · "}
          <strong>{mute.match_count}</strong> muted
          {mute.incident_id && ` · from ${mute.incident_id}`}
        </span>
      </button>
      {active && isAdmin && (
        <button type="button" className="btn btn-ghost btn-xs" onClick={() => onUnmute(mute)} disabled={busy}>
          <BellRing size={11} /> Unmute
        </button>
      )}
    </li>
  );
}

export default function MutesDrawer({ token, open, onClose, isAdmin, onChanged }) {
  const [mutes, setMutes] = useState(null);
  const [log, setLog] = useState([]);
  const [logMore, setLogMore] = useState(false);
  const [filter, setFilter] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const closeRef = useRef(null);

  const loadLog = useCallback(async (muteId, offset = 0) => {
    const res = await getMuteLog(token, { muteId, limit: PAGE, offset });
    const items = res.items || [];
    setLog((prev) => (offset === 0 ? items : [...prev, ...items]));
    setLogMore(items.length === PAGE);
  }, [token]);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [list] = await Promise.all([listMutes(token), loadLog(filter, 0)]);
      setMutes(list);
    } catch (e) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, [token, filter, loadLog]);

  useEffect(() => {
    if (open) load();
  }, [open, load]);

  useEffect(() => {
    if (!open) return undefined;
    const onKey = (e) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    closeRef.current?.focus();
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  async function handleUnmute(mute) {
    setBusyId(mute.id);
    try {
      await unmute(token, mute.id);
      await load();
      onChanged?.();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusyId("");
    }
  }

  if (!open) return null;

  const items = mutes?.items || [];
  const active = items.filter((m) => m.status === "active");
  const ended = items.filter((m) => m.status !== "active");
  const byId = Object.fromEntries(items.map((m) => [m.id, m]));
  const selectMute = (id) => setFilter((f) => (f === id ? "" : id));

  return (
    <>
      <div className="notif-scrim" onClick={onClose} aria-hidden="true" />
      <aside className="notif-panel mutes-drawer" role="dialog" aria-modal="true" aria-label="Muted alerts">
        <header className="notif-panel-head">
          <div className="notif-panel-title">
            <BellOff size={13} />
            <span>Muted alerts</span>
            {mutes && <span className="notif-count-pill">{mutes.muted_7d} in 7 days</span>}
          </div>
          <div className="notif-panel-tools">
            <button type="button" className="icon-btn" onClick={load} title="Refresh" disabled={loading}>
              <RefreshCw size={13} className={loading ? "is-spinning" : ""} />
            </button>
            <button ref={closeRef} type="button" className="icon-btn" onClick={onClose} title="Close">
              <X size={14} />
            </button>
          </div>
        </header>

        <div className="notif-panel-body">
          {error && <div className="notif-banner mute-error">{error}</div>}

          <section className="notif-group">
            <h4 className="notif-group-label">
              Active mutes <span className="notif-group-count">{active.length}</span>
            </h4>
            {mutes && active.length === 0 ? (
              <p className="mute-empty">Nothing is muted right now.</p>
            ) : (
              <ul className="mute-list">
                {active.map((m) => (
                  <MuteRow key={m.id} mute={m} isAdmin={isAdmin} selected={filter === m.id}
                    onSelect={selectMute} onUnmute={handleUnmute} busy={busyId === m.id} />
                ))}
              </ul>
            )}
          </section>

          {ended.length > 0 && (
            <details className="notif-group mute-ended">
              <summary className="notif-group-label">
                Ended in the last 7 days <span className="notif-group-count">{ended.length}</span>
              </summary>
              <ul className="mute-list">
                {ended.map((m) => (
                  <MuteRow key={m.id} mute={m} isAdmin={false} selected={filter === m.id}
                    onSelect={selectMute} onUnmute={handleUnmute} busy={false} />
                ))}
              </ul>
            </details>
          )}

          <section className="notif-group">
            <h4 className="notif-group-label mute-log-head">
              Muted alert log
              <select
                id="mute-log-filter"
                className="form-select form-select-sm mute-log-filter"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                aria-label="Show entries for"
              >
                <option value="">All mutes</option>
                {items.map((m) => (
                  <option key={m.id} value={m.id}>{describeMute(m)}</option>
                ))}
              </select>
            </h4>
            {log.length === 0 ? (
              <p className="mute-empty">
                {filter ? "This mute hasn't caught any alerts yet." : "No alerts have been muted yet."}
              </p>
            ) : (
              <ol className="mute-log">
                {log.map((entry) => (
                  <li key={entry.id} className="mute-log-row">
                    <div className="mute-log-main">
                      <span className="mute-log-alert">{entry.alert_name || "—"}</span>
                      <span className="mute-log-device">{entry.device || "no device"}</span>
                      {entry.value && <span className="mute-log-value">{entry.value}</span>}
                    </div>
                    <div className="mute-log-meta">
                      <time dateTime={entry.received_at} title={new Date(entry.received_at).toLocaleString()}>
                        {relativeTime(entry.received_at)}
                      </time>
                      {entry.service && <span> · {entry.service}</span>}
                      {!filter && byId[entry.mute_id] && <span> · {describeMute(byId[entry.mute_id])}</span>}
                    </div>
                  </li>
                ))}
              </ol>
            )}
            {logMore && (
              <div className="mute-log-more">
                <button type="button" className="btn btn-ghost btn-sm"
                  onClick={() => loadLog(filter, log.length).catch((e) => setError(e.message))}>
                  Load more
                </button>
              </div>
            )}
          </section>
        </div>
      </aside>
    </>
  );
}
