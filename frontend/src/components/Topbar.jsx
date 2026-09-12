import { Bell, RefreshCw, Search, Settings } from "lucide-react";
import { useState } from "react";

const PAGE_META = {
  dashboard:    ["Dashboard",          "Live platform overview"],
  fleet:        ["Fleet Monitor",      "All monitored devices at a glance"],
  incidents:    ["Incidents",          "Active incident management"],
  alerts:       ["Alert Quality",      "Noise reduction & governance"],
  oncall:       ["On-Call",            "Schedules & escalations"],
  "ai-engine":  ["AI Engine",          "Provider, deployment mode & usage"],
  "ai-memory":  ["Domain Memory",      "AI learns your environment over time"],
  topology:     ["Topology Graph",     "Service dependency map"],
  agents:       ["Agents",             "Deployed NeuroOps monitoring agents"],
  logs:         ["Log Explorer",       "Structured log search & analysis"],
  slo:          ["SLO Dashboard",      "Service level objectives & burn rates"],
  digest:       ["Weekly Digest",      "Executive summary & incident trends"],
  integrations: ["Integrations",       "Data sources & third-party connectors"],
  onboarding:   ["Setup Guide",        "Onboard your first integration"],
  audit:        ["Audit Trail",        "Immutable action & change history"],
};

export default function Topbar({
  activeView,
  liveRefresh,
  onToggleLiveRefresh,
  notifCount = 0,
  notifCritical = 0,
  onOpenNotifications,
  notifOpen = false,
}) {
  const [search, setSearch] = useState("");
  const [name, sub] = PAGE_META[activeView] || [activeView, ""];

  return (
    <header className="topbar">
      <div className="topbar-title">
        <span className="topbar-page-name">{name}</span>
        {sub && <span className="topbar-breadcrumb">{sub}</span>}
      </div>

      <div className="topbar-search">
        <Search size={13} className="topbar-search-icon" />
        <input
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Search incidents, services, alerts…"
        />
      </div>

      <div className="topbar-actions">
        <button
          className={`icon-btn${liveRefresh ? " active" : ""}`}
          onClick={onToggleLiveRefresh}
          title={liveRefresh ? "Live refresh ON — click to pause" : "Live refresh OFF — click to enable"}
        >
          <RefreshCw
            size={14}
            style={{ animation: liveRefresh ? "spin 2s linear infinite" : "none" }}
          />
        </button>

        <button
          className={`icon-btn${notifOpen ? " active" : ""}`}
          onClick={onOpenNotifications}
          aria-haspopup="dialog"
          aria-expanded={notifOpen}
          title={
            notifCount === 0
              ? "Notifications — nothing needs attention"
              : `Notifications — ${notifCount} item${notifCount === 1 ? "" : "s"} need attention`
          }
        >
          <Bell size={14} />
          {notifCount > 0 && (
            <span className={`notif-badge${notifCritical > 0 ? " critical" : ""}`}>
              {notifCount > 9 ? "9+" : notifCount}
            </span>
          )}
        </button>

        <button className="icon-btn" title="Settings">
          <Settings size={14} />
        </button>
      </div>
    </header>
  );
}
