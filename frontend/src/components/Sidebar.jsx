import {
  BarChart2, Bell, Bot,
  Brain, Database, FileText, Flame,
  LayoutDashboard, LogOut, Network,
  Package, Radio, Shield, Terminal,
  Zap, Server,
} from "lucide-react";

const NAV = [
  {
    group: "Operations",
    items: [
      { key: "dashboard",   label: "Dashboard",       icon: LayoutDashboard },
      { key: "fleet",       label: "Fleet Monitor",    icon: Server },
      { key: "incidents",   label: "Incidents",        icon: Flame,    badgeKey: "openCount" },
      { key: "alerts",      label: "Alert Quality",    icon: Bell },
      { key: "oncall",      label: "On-Call",          icon: Radio },
    ],
  },
  {
    group: "AI Intelligence",
    items: [
      { key: "ai-engine",   label: "AI Engine",        icon: Brain },
      { key: "ai-memory",   label: "Domain Memory",    icon: Database },
    ],
  },
  {
    group: "Observability",
    items: [
      { key: "topology",    label: "Topology Graph",   icon: Network },
      { key: "logs",        label: "Log Explorer",     icon: Terminal },
      { key: "agents",      label: "Agents",           icon: Bot },
    ],
  },
  {
    group: "Analytics",
    items: [
      { key: "slo",         label: "SLO Dashboard",    icon: BarChart2 },
      { key: "digest",      label: "Weekly Digest",    icon: FileText },
    ],
  },
  {
    group: "Platform",
    items: [
      { key: "integrations",label: "Integrations",     icon: Package },
      { key: "onboarding",  label: "Setup Guide",      icon: Zap,     badgeKey: "onboarding" },
    ],
  },
  {
    group: "Admin",
    items: [
      { key: "audit",       label: "Audit Trail",      icon: Shield },
    ],
  },
];

export default function Sidebar({ activeView, onNavigate, badges = {}, userEmail, userRole, onLogout }) {
  const initials = userEmail ? userEmail.slice(0, 2).toUpperCase() : "U";

  return (
    <aside className="sidebar">
      {/* Logo */}
      <div className="sidebar-logo">
        <div className="sidebar-logo-mark">
          <Zap size={15} color="#fff" />
        </div>
        <div className="sidebar-logo-text">
          <span className="sidebar-logo-name">NeuroOps</span>
          <span className="sidebar-logo-sub">AI Incident Platform</span>
        </div>
      </div>

      {/* Nav */}
      <nav className="sidebar-nav">
        {NAV.map((group) => (
          <div key={group.group} className="nav-group">
            <div className="nav-group-label">{group.group}</div>
            {group.items.map((item) => {
              const Icon = item.icon;
              const badgeVal = item.badgeKey ? badges[item.badgeKey] : null;
              return (
                <button
                  key={item.key}
                  className={`nav-item${activeView === item.key ? " active" : ""}`}
                  onClick={() => onNavigate(item.key)}
                >
                  <Icon size={14} className="nav-item-icon" />
                  <span style={{ flex: 1, overflow: "hidden", textOverflow: "ellipsis" }}>
                    {item.label}
                  </span>
                  {badgeVal && (
                    <span className={`nav-badge${item.badgeKey === "onboarding" ? " info" : ""}`}>
                      {badgeVal}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        ))}
      </nav>

      {/* Footer */}
      <div className="sidebar-footer">
        <div className="sidebar-user">
          <div className="user-avatar">{initials}</div>
          <div className="user-info">
            <div className="user-name">{userEmail || "User"}</div>
            {userRole && <div className="user-role">{userRole[0].toUpperCase() + userRole.slice(1)}</div>}
          </div>
          <button
            onClick={onLogout}
            title="Sign out"
            style={{
              width: 28, height: 28, flexShrink: 0,
              display: "flex", alignItems: "center", justifyContent: "center",
              background: "none", border: "none", borderRadius: "var(--r2)",
              color: "var(--t3)", cursor: "pointer", transition: "color 0.14s",
            }}
            onMouseEnter={e => (e.currentTarget.style.color = "var(--red)")}
            onMouseLeave={e => (e.currentTarget.style.color = "var(--t3)")}
          >
            <LogOut size={13} />
          </button>
        </div>
      </div>
    </aside>
  );
}
