import { useCallback, useEffect, useState } from "react";
import { ensureAuthenticated, clearStoredToken, storeToken } from "../api/auth.js";
import { getProgress as getOnboardingProgress } from "../api/onboarding.js";

import Sidebar from "./Sidebar.jsx";
import Topbar from "./Topbar.jsx";
import Dashboard from "./Dashboard.jsx";
import AuthScreen from "./AuthScreen.jsx";

import IntegrationsHub from "./IntegrationsHub.jsx";
import SLODashboard from "./SLODashboard.jsx";
import OnCallPanel from "./OnCallPanel.jsx";
import WeeklyDigestPanel from "./WeeklyDigestPanel.jsx";
import OnboardingWizard from "./OnboardingWizard.jsx";
import AgentPanel from "./AgentPanel.jsx";
import LogExplorer from "./LogExplorer.jsx";
import TopologyGraphPanel from "./TopologyGraphPanel.jsx";
import IncidentMemoryPanel from "./IncidentMemoryPanel.jsx";
import AIStatusPanel from "./AIStatusPanel.jsx";
import AlertQualityPanel from "./AlertQualityPanel.jsx";
import AuditPanel from "./AuditPanel.jsx";
import IncidentWorkspace from "./IncidentWorkspace.jsx";
import FleetView from "./FleetView.jsx";

const API_BASE = "/api/v1";

async function apiReq(path, token, options = {}) {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers || {}),
    },
    ...options,
  });
  if (res.status === 401) throw new Error("auth");
  if (!res.ok) {
    let msg = `Request failed (${res.status})`;
    try { const d = await res.json(); msg = d.error || d.message || msg; } catch { /**/ }
    throw new Error(msg);
  }
  const ct = res.headers.get("content-type") || "";
  return ct.includes("application/json") ? res.json() : res.text();
}

function extractIncidents(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload) return [];
  if (Array.isArray(payload.incidents)) return payload.incidents;
  if (Array.isArray(payload.items))     return payload.items;
  if (Array.isArray(payload.data))      return payload.data;
  if (payload.data?.incidents)          return payload.data.incidents;
  return [];
}

/* Page wrapper with fade animation */
function PageWrap({ children, maxWidth }) {
  return (
    <div className="page-inner fade-in" style={maxWidth ? { maxWidth } : {}}>
      {children}
    </div>
  );
}

export default function AppShell() {
  const [authReady,       setAuthReady]       = useState(false);
  const [token,           setToken]           = useState(null);
  const [userEmail,       setUserEmail]       = useState("");
  const [view,            setView]            = useState("dashboard");
  const [liveRefresh,     setLiveRefresh]     = useState(false);
  const [incidents,       setIncidents]       = useState([]);
  const [onboardingDone,  setOnboardingDone]  = useState(true);
  const [onboardingSteps, setOnboardingSteps] = useState(0);

  const loadIncidents = useCallback(async (tok) => {
    const t = tok || token;
    if (!t) return;
    try {
      const data = await apiReq("/incidents?page=1&page_size=50&sort_by=last_event_time&sort_order=desc", t);
      setIncidents(extractIncidents(data));
    } catch { /**/ }
  }, [token]);

  useEffect(() => {
    ensureAuthenticated().then(async (tok) => {
      if (tok) {
        setToken(tok);
        setAuthReady(true);
        loadIncidents(tok);
        try {
          const p = await getOnboardingProgress();
          if (p) {
            setOnboardingDone(p.aha_moment_reached || p.step === "complete");
            const steps = ["signup","connect_source","first_alert","first_incident","install_agent"];
            setOnboardingSteps((p.completed_steps || []).filter(s => steps.includes(s)).length);
          }
        } catch { /**/ }
      }
    });
  }, [loadIncidents]);

  useEffect(() => {
    if (!authReady || !liveRefresh || !token) return;
    const id = setInterval(() => loadIncidents(token), 10000);
    return () => clearInterval(id);
  }, [authReady, liveRefresh, token, loadIncidents]);

  function handleAuthenticated(tok, email, isNewUser = false) {
    storeToken(tok);
    setToken(tok);
    setUserEmail(email || "");
    setAuthReady(true);
    loadIncidents(tok);
    // New users go straight to the setup wizard so they're not dropped on an empty dashboard.
    if (isNewUser) {
      setOnboardingDone(false);
      setView("onboarding");
    }
  }

  function handleLogout() {
    clearStoredToken();
    setToken(null);
    setAuthReady(false);
    setIncidents([]);
  }

  if (!authReady) {
    return <AuthScreen onAuthenticated={handleAuthenticated} />;
  }

  const openCount = incidents.filter(i => i.status === "open" || !i.status).length;
  const badges = {
    openCount: openCount > 0 ? openCount : null,
    onboarding: !onboardingDone ? `${onboardingSteps}/5` : null,
  };

  function renderView() {
    switch (view) {
      case "dashboard":
        return <Dashboard token={token} onNavigate={setView} incidents={incidents} />;

      case "fleet":
        return <FleetView />;

      case "incidents":
        return <IncidentWorkspace token={token} incidents={incidents} onRefresh={() => loadIncidents(token)} />;

      case "ai-engine":
        return <PageWrap><AIStatusPanel /></PageWrap>;

      case "alerts":
        return <PageWrap><AlertQualityPanel token={token} /></PageWrap>;

      case "oncall":
        return <PageWrap><OnCallPanel /></PageWrap>;

      case "ai-memory":
        return <PageWrap><IncidentMemoryPanel /></PageWrap>;

      case "topology":
        return <PageWrap><TopologyGraphPanel /></PageWrap>;

      case "agents":
        return <PageWrap><AgentPanel /></PageWrap>;

      case "logs":
        return <PageWrap><LogExplorer /></PageWrap>;

      case "slo":
        return <PageWrap><SLODashboard /></PageWrap>;

      case "digest":
        return <PageWrap><WeeklyDigestPanel /></PageWrap>;

      case "integrations":
        return <PageWrap><IntegrationsHub /></PageWrap>;

      case "onboarding":
        return (
          <PageWrap>
            <OnboardingWizard onComplete={() => { setOnboardingDone(true); setView("dashboard"); }} />
          </PageWrap>
        );

      case "audit":
        return <AuditPanel />;

      default:
        return <Dashboard token={token} onNavigate={setView} incidents={incidents} />;
    }
  }

  return (
    <div className="app-shell">
      <Sidebar
        activeView={view}
        onNavigate={setView}
        badges={badges}
        userEmail={userEmail}
        onLogout={handleLogout}
      />
      <div className="main-content">
        <Topbar
          activeView={view}
          liveRefresh={liveRefresh}
          onToggleLiveRefresh={() => setLiveRefresh(v => !v)}
          hasNotif={openCount > 0}
        />
        <div className="page-content">
          {renderView()}
        </div>
      </div>
    </div>
  );
}
