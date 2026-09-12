/**
 * Shared UI primitives.
 *
 * Every page in this app was re-implementing the same six components inline —
 * a page header, a KPI tile, a panel with a titled header, a list row, a
 * severity pill, and loading/empty/error states. Each copy drifted: different
 * paddings, different greys, four different reds. That drift is what made the
 * product read as fifteen separate apps.
 *
 * These are the canonical versions. They carry no colour literals of their own:
 * every value comes from the design-system tokens, so a token change moves the
 * whole product at once. Pages should compose these rather than style divs.
 */
import {
  AlertTriangle,
  CheckCircle2,
  ChevronRight,
  Inbox,
  RefreshCw,
  TrendingDown,
  TrendingUp,
} from "lucide-react";

import { SEVERITY_TONE, STATUS_TONE, toneClass } from "./tone.js";


/* ── Page header ─────────────────────────────────────────────────────────
   Title, optional eyebrow line above it, optional meta line below, and
   right-aligned actions. Every page starts with one.                       */
export function PageHeader({ eyebrow, title, meta, actions }) {
  return (
    <header className="page-header">
      <div className="page-header-text">
        {eyebrow && <div className="page-header-eyebrow">{eyebrow}</div>}
        <h1 className="page-header-title">{title}</h1>
        {meta && <div className="page-header-meta">{meta}</div>}
      </div>
      {actions && <div className="page-header-actions">{actions}</div>}
    </header>
  );
}

/* A live status pill for the eyebrow slot: pulsing dot plus label. */
export function StatusPulse({ tone = "success", label, detail }) {
  return (
    <span className={`status-pulse ${toneClass(tone)}`}>
      <span className="status-pulse-dot" aria-hidden="true" />
      <span className="status-pulse-label">{label}</span>
      {detail && <span className="status-pulse-detail">{detail}</span>}
    </span>
  );
}

/* ── Stat tile ───────────────────────────────────────────────────────────
   The KPI card. Clickable tiles get a real <button> so they are keyboard
   reachable — the hand-rolled versions used divs with onClick, which are not. */
export function StatTile({ label, value, sub, icon: Icon, tone = "neutral", trend, onClick, loading }) {
  const body = (
    <>
      <span className="stat-tile-bar" aria-hidden="true" />
      <div className="stat-tile-main">
        <div className="stat-tile-text">
          <div className="stat-tile-label">{label}</div>
          {loading ? (
            <div className="skeleton stat-tile-skeleton" />
          ) : (
            <div className="stat-tile-value">{value ?? "—"}</div>
          )}
          {sub && !loading && (
            <div className="stat-tile-sub">
              {trend === "up" && <TrendingUp size={10} className="trend-up" />}
              {trend === "down" && <TrendingDown size={10} className="trend-down" />}
              {sub}
            </div>
          )}
        </div>
        {Icon && (
          <span className="stat-tile-icon" aria-hidden="true">
            <Icon size={16} />
          </span>
        )}
      </div>
    </>
  );

  const className = `stat-tile ${toneClass(tone)}`;
  return onClick ? (
    <button type="button" className={`${className} is-clickable`} onClick={onClick}>
      {body}
    </button>
  ) : (
    <div className={className}>{body}</div>
  );
}

/* ── Panel ───────────────────────────────────────────────────────────────
   A card with an optional titled header and an optional trailing action.
   `flush` removes body padding for panels whose content is a full-bleed list. */
export function Panel({ title, sub, action, onAction, accent, flush, className = "", children }) {
  return (
    <section className={`panel-card${accent ? " is-accent" : ""} ${className}`}>
      {(title || action) && (
        <div className="panel-card-head">
          <div className="panel-card-headings">
            {title && <h2 className="panel-card-title">{title}</h2>}
            {sub && <p className="panel-card-sub">{sub}</p>}
          </div>
          {action && (
            <button type="button" className="link-action" onClick={onAction}>
              {action}
              <ChevronRight size={11} />
            </button>
          )}
        </div>
      )}
      <div className={flush ? "panel-card-body is-flush" : "panel-card-body"}>{children}</div>
    </section>
  );
}

/* ── Pills ───────────────────────────────────────────────────────────────*/
export function SeverityPill({ severity }) {
  const value = (severity || "low").toLowerCase();
  return (
    <span className={`pill ${toneClass(SEVERITY_TONE[value] || "neutral")}`}>
      <span className="pill-dot" aria-hidden="true" />
      {value}
    </span>
  );
}

export function StatusPill({ status }) {
  const value = (status || "open").toLowerCase();
  return <span className={`pill is-plain ${toneClass(STATUS_TONE[value] || "neutral")}`}>{value}</span>;
}

/* ── List row ────────────────────────────────────────────────────────────
   Leading slot (pill or icon), primary + secondary text, trailing meta.
   Renders as a button when clickable so rows are keyboard navigable.        */
export function ListRow({ lead, title, sub, trailing, onClick }) {
  const inner = (
    <>
      {lead && <span className="list-row-lead">{lead}</span>}
      <span className="list-row-text">
        <span className="list-row-title">{title}</span>
        {sub && <span className="list-row-sub">{sub}</span>}
      </span>
      {trailing && <span className="list-row-trailing">{trailing}</span>}
    </>
  );

  return onClick ? (
    <button type="button" className="list-row is-clickable" onClick={onClick}>
      {inner}
    </button>
  ) : (
    <div className="list-row">{inner}</div>
  );
}

/* A label/value line — the "Platform Internals" pattern, used on many pages. */
export function MetricRow({ icon: Icon, label, value, tone = "neutral" }) {
  return (
    <div className="metric-row">
      {Icon && (
        <span className={`metric-row-icon ${toneClass(tone)}`} aria-hidden="true">
          <Icon size={12} />
        </span>
      )}
      <span className="metric-row-label">{label}</span>
      <span className="metric-row-value">{value ?? "—"}</span>
    </div>
  );
}

/* ── Bar list ────────────────────────────────────────────────────────────
   Horizontal magnitude comparison. Bars are scaled to the largest value so
   small differences stay visible.                                           */
export function BarList({ items }) {
  const max = Math.max(...items.map((i) => i.value || 0), 1);
  return (
    <div className="bar-list">
      {items.map((item) => (
        <div key={item.label} className={`bar-row ${toneClass(item.tone)}`}>
          <span className="bar-row-label">{item.label}</span>
          <span className="bar-row-track">
            <span className="bar-row-fill" style={{ width: `${((item.value || 0) / max) * 100}%` }} />
          </span>
          <span className={`bar-row-value${item.value > 0 ? " is-active" : ""}`}>{item.value || 0}</span>
        </div>
      ))}
    </div>
  );
}

/* ── States ──────────────────────────────────────────────────────────────
   Several pages had no empty or error state at all — a failed fetch showed a
   blank rectangle, indistinguishable from "nothing to report".              */
export function EmptyState({ icon: Icon = Inbox, title, message, action, onAction, tone = "neutral" }) {
  return (
    <div className={`state-block ${toneClass(tone)}`}>
      {Icon && <Icon size={22} className="state-block-icon" />}
      <p className="state-block-title">{title}</p>
      {message && <p className="state-block-message">{message}</p>}
      {action && (
        <button type="button" className="btn btn-outline btn-sm" onClick={onAction}>
          {action}
        </button>
      )}
    </div>
  );
}

export function AllClearState({ title = "All clear", message }) {
  return <EmptyState icon={CheckCircle2} tone="success" title={title} message={message} />;
}

export function ErrorState({ message, onRetry }) {
  return (
    <div className="state-block tone-danger">
      <AlertTriangle size={22} className="state-block-icon" />
      <p className="state-block-title">Something went wrong</p>
      {message && <p className="state-block-message">{message}</p>}
      {onRetry && (
        <button type="button" className="btn btn-outline btn-sm" onClick={onRetry}>
          <RefreshCw size={11} /> Try again
        </button>
      )}
    </div>
  );
}

/* Skeleton rows sized to the content they stand in for, so the layout does
   not jump when real data lands. */
export function SkeletonRows({ count = 4, height = 44 }) {
  return (
    <div className="skeleton-stack">
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="skeleton" style={{ height }} />
      ))}
    </div>
  );
}

/* ── Layout helpers ──────────────────────────────────────────────────────
   Named grids replace the ad-hoc `gridTemplateColumns` strings that were
   repeated with slightly different gaps on every page.                      */
export function Grid({ cols = 3, children, className = "" }) {
  return <div className={`ui-grid cols-${cols} ${className}`}>{children}</div>;
}

export function Split({ children, className = "" }) {
  return <div className={`ui-split ${className}`}>{children}</div>;
}

export function Page({ children }) {
  return <div className="ui-page fade-in">{children}</div>;
}
