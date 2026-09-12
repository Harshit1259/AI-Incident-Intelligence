import { useCallback, useEffect, useRef, useState } from "react";
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Inbox,
  PlugZap,
  RefreshCw,
  ShieldAlert,
  WifiOff,
  X,
} from "lucide-react";
import { dismissNotification, clearDismissals } from "../api/notifications.js";

/* Icon and label per notification kind. Falls back gracefully so a kind added
   on the backend before the frontend knows about it still renders. */
const KIND_META = {
  ingest_rejected: { icon: ShieldAlert, label: "Rejected alerts" },
  source_error:    { icon: PlugZap,     label: "Integration failing" },
  source_silent:   { icon: WifiOff,     label: "Integration silent" },
  agent_offline:   { icon: Bot,         label: "Agent offline" },
};

const DEFAULT_META = { icon: AlertTriangle, label: "Attention" };

/* Group order in the panel — most urgent class of problem first. */
const KIND_ORDER = ["source_error", "ingest_rejected", "agent_offline", "source_silent"];

function relativeTime(iso) {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return "";
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  return `${Math.round(hours / 24)}d ago`;
}

/* Pretty-print JSON evidence when possible; fall back to the raw string.
   Payloads are already truncated server-side to 2 KB. */
function formatEvidence(raw) {
  if (!raw) return "";
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

function NotificationRow({ item, onDismiss, onAction }) {
  const [expanded, setExpanded] = useState(false);
  const meta = KIND_META[item.kind] || DEFAULT_META;
  const Icon = meta.icon;
  const hasEvidence = Boolean(item.evidence);

  return (
    <li className={`notif-item notif-${item.severity}`}>
      <span className="notif-item-icon" aria-hidden="true">
        <Icon size={14} />
      </span>

      <div className="notif-item-body">
        <div className="notif-item-head">
          <span className="notif-item-title">{item.title}</span>
          <time className="notif-item-time" dateTime={item.last_seen}>
            {relativeTime(item.last_seen)}
          </time>
        </div>

        <p className="notif-item-detail">{item.detail}</p>

        <div className="notif-item-actions">
          {item.action_view && (
            <button
              type="button"
              className="notif-link"
              onClick={() => onAction(item.action_view)}
            >
              {item.action_label || "View"}
            </button>
          )}

          {hasEvidence && (
            <button
              type="button"
              className="notif-link notif-link-muted"
              onClick={() => setExpanded((v) => !v)}
              aria-expanded={expanded}
            >
              {expanded ? <ChevronDown size={11} /> : <ChevronRight size={11} />}
              {expanded ? "Hide payload" : "View payload"}
            </button>
          )}

          <button
            type="button"
            className="notif-link notif-link-muted notif-dismiss"
            onClick={() => onDismiss(item)}
          >
            Dismiss
          </button>
        </div>

        {expanded && hasEvidence && (
          <pre className="notif-evidence">{formatEvidence(item.evidence)}</pre>
        )}
      </div>
    </li>
  );
}

/**
 * Slide-over notification panel.
 *
 * Data is owned by AppShell (which also drives the bell badge) and passed down,
 * so the badge and the list are always the same numbers.
 */
export default function NotificationPanel({
  open,
  onClose,
  items,
  loading,
  error,
  degraded,
  onRefresh,
  onNavigate,
}) {
  const panelRef = useRef(null);
  const closeRef = useRef(null);

  /* Close on Escape. Bound only while open so the key stays free otherwise. */
  useEffect(() => {
    if (!open) return undefined;
    const onKey = (e) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  /* Move focus into the panel when it opens so keyboard users are not stranded
     behind it, and the Escape handler has somewhere sensible to return from. */
  useEffect(() => {
    if (open && closeRef.current) closeRef.current.focus();
  }, [open]);

  const handleDismiss = useCallback(
    (item) => {
      dismissNotification(item.id, item.last_seen);
      onRefresh();
    },
    [onRefresh],
  );

  const handleAction = useCallback(
    (view) => {
      onNavigate(view);
      onClose();
    },
    [onNavigate, onClose],
  );

  const handleClearDismissals = useCallback(() => {
    clearDismissals();
    onRefresh();
  }, [onRefresh]);

  if (!open) return null;

  /* Group by kind so related problems read together. */
  const grouped = KIND_ORDER.map((kind) => ({
    kind,
    label: (KIND_META[kind] || DEFAULT_META).label,
    rows: items.filter((n) => n.kind === kind),
  })).filter((g) => g.rows.length > 0);

  /* Any kind the frontend does not know about still gets rendered. */
  const knownKinds = new Set(KIND_ORDER);
  const unknown = items.filter((n) => !knownKinds.has(n.kind));
  if (unknown.length > 0) {
    grouped.push({ kind: "other", label: DEFAULT_META.label, rows: unknown });
  }

  return (
    <>
      <div className="notif-scrim" onClick={onClose} aria-hidden="true" />

      <aside
        ref={panelRef}
        className="notif-panel"
        role="dialog"
        aria-modal="true"
        aria-label="Notifications"
      >
        <header className="notif-panel-head">
          <div className="notif-panel-title">
            <span>Notifications</span>
            {items.length > 0 && <span className="notif-count-pill">{items.length}</span>}
          </div>

          <div className="notif-panel-tools">
            <button
              type="button"
              className="icon-btn"
              onClick={onRefresh}
              title="Refresh"
              disabled={loading}
            >
              <RefreshCw size={13} style={{ animation: loading ? "spin 1s linear infinite" : "none" }} />
            </button>
            <button
              ref={closeRef}
              type="button"
              className="icon-btn"
              onClick={onClose}
              title="Close"
            >
              <X size={14} />
            </button>
          </div>
        </header>

        {degraded && (
          <div className="notif-banner">
            <AlertTriangle size={12} />
            <span>Some checks could not run — this list may be incomplete.</span>
          </div>
        )}

        <div className="notif-panel-body">
          {error ? (
            <div className="notif-state notif-state-error">
              <AlertTriangle size={20} />
              <p>{error}</p>
              <button type="button" className="notif-link" onClick={onRefresh}>
                Try again
              </button>
            </div>
          ) : loading && items.length === 0 ? (
            <div className="notif-state">
              <Inbox size={20} />
              <p>Loading…</p>
            </div>
          ) : items.length === 0 ? (
            <div className="notif-state notif-state-ok">
              <CheckCircle2 size={22} />
              <p className="notif-state-title">Everything is flowing</p>
              <p>No rejected alerts, failing integrations, or offline agents.</p>
              <button type="button" className="notif-link notif-link-muted" onClick={handleClearDismissals}>
                Show dismissed
              </button>
            </div>
          ) : (
            grouped.map((group) => (
              <section key={group.kind} className="notif-group">
                <h4 className="notif-group-label">
                  {group.label}
                  <span className="notif-group-count">{group.rows.length}</span>
                </h4>
                <ul className="notif-list">
                  {group.rows.map((item) => (
                    <NotificationRow
                      key={item.id}
                      item={item}
                      onDismiss={handleDismiss}
                      onAction={handleAction}
                    />
                  ))}
                </ul>
              </section>
            ))
          )}
        </div>

        {items.length > 0 && (
          <footer className="notif-panel-foot">
            <button type="button" className="notif-link notif-link-muted" onClick={handleClearDismissals}>
              Show dismissed
            </button>
          </footer>
        )}
      </aside>
    </>
  );
}
