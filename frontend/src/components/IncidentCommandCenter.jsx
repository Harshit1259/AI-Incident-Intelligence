import React, { useCallback, useEffect, useMemo, useState } from "react";
import { ensureAuthenticated, clearStoredToken, login, register } from "../api/auth";
import { getProgress as getOnboardingProgress } from "../api/onboarding.js";
import PostMortemPanel from "./PostMortemPanel.jsx";
import IntegrationsHub from "./IntegrationsHub.jsx";
import StatusPageView from "./StatusPageView.jsx";
import SLODashboard from "./SLODashboard.jsx";
import OnCallPanel from "./OnCallPanel.jsx";
import AnomalyPanel from "./AnomalyPanel.jsx";
import EngineeringHealthPanel from "./EngineeringHealthPanel.jsx";
import ROIDashboardPanel from "./ROIDashboardPanel.jsx";
import WeeklyDigestPanel from "./WeeklyDigestPanel.jsx";
import OnboardingWizard from "./OnboardingWizard.jsx";
import BillingPanel from "./BillingPanel.jsx";
import LandingPage from "./LandingPage.jsx";
import { BusinessImpactPanel, AlertFeedbackPanel, AlertFeedbackButtons, AutoResolvePanel, RunbookPanel, DependencyPanel, CompliancePanel } from "./GapFeatures.jsx";
import AgentPanel from "./AgentPanel.jsx";
import LogExplorer from "./LogExplorer.jsx";
import TopologyGraphPanel from "./TopologyGraphPanel.jsx";
import IncidentMemoryPanel from "./IncidentMemoryPanel.jsx";
import RiskExposureDashboard from "./RiskExposureDashboard.jsx";
import AIStatusPanel from "./AIStatusPanel.jsx";
import AlertQualityPanel from "./AlertQualityPanel.jsx";
import AutomationPolicyEngine from "./AutomationPolicyEngine.jsx";
import SchemaRegistryPanel from "./SchemaRegistryPanel.jsx";
import TeamWorkflowPanel from "./TeamWorkflowPanel.jsx";

const API_BASE = "/api/v1";

// request() — authenticated fetch wrapper.
// Always calls ensureAuthenticated() first — it returns instantly from cache
// when a valid token is stored, so there is zero overhead on subsequent calls.
async function request(path, options = {}) {
  // Always ensure we have a token before every call.
  // ensureAuthenticated() returns the cached token immediately if valid.
  const token = await ensureAuthenticated();

  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers || {}),
    },
    ...options,
  });

  // On 401: token is stale — clear it, re-auth, retry ONCE
  if (response.status === 401) {
    clearStoredToken();
    const freshToken = await ensureAuthenticated();
    if (freshToken) {
      const retry = await fetch(`${API_BASE}${path}`, {
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${freshToken}`,
          ...(options.headers || {}),
        },
        ...options,
      });
      return parseResponse(retry);
    }
    throw new Error("Authentication failed — please refresh the page");
  }

  return parseResponse(response);
}

async function parseResponse(response) {
  const contentType = response.headers.get("content-type") || "";
  const payload = contentType.includes("application/json")
    ? await response.json()
    : await response.text();

  if (!response.ok) {
    const message =
      (typeof payload === "object" && (payload.error || payload.message)) ||
      (typeof payload === "string" && payload) ||
      `Request failed (${response.status})`;
    throw new Error(message);
  }

  return payload;
}

function extractIncidents(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  if (Array.isArray(payload.incidents)) return payload.incidents;
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.data)) return payload.data;
  if (payload.data && Array.isArray(payload.data.incidents)) return payload.data.incidents;
  if (payload.data && Array.isArray(payload.data.items)) return payload.data.items;
  return [];
}

function extractDetail(payload) {
  if (!payload || typeof payload !== "object") return null;
  if (payload.incident || payload.summary || payload.events) return payload;
  if (payload.data && (payload.data.incident || payload.data.summary || payload.data.events)) {
    return payload.data;
  }
  return payload;
}

function extractActivity(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  if (Array.isArray(payload.activities)) return payload.activities;
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.data)) return payload.data;
  if (payload.data && Array.isArray(payload.data.activities)) return payload.data.activities;
  return [];
}

function extractActionAudit(payload) {
  if (Array.isArray(payload)) return payload;
  if (!payload || typeof payload !== "object") return [];
  if (Array.isArray(payload.items)) return payload.items;
  if (Array.isArray(payload.audit)) return payload.audit;
  if (Array.isArray(payload.data)) return payload.data;
  if (payload.data && Array.isArray(payload.data.items)) return payload.data.items;
  return [];
}

function severityClass(value) {
  return `severity-${String(value || "unknown").toLowerCase()}`;
}

function statusClass(value) {
  return `status-${String(value || "unknown").toLowerCase()}`;
}

function riskClass(value) {
  return `risk-${String(value || "low").toLowerCase()}`;
}

function formatTimestamp(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function clamp(value, min, max) {
  return Math.max(min, Math.min(max, value));
}

function titleCase(value) {
  if (!value) return "-";
  return String(value)
    .replace(/[_-]/g, " ")
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function deriveIntelligence(detail, incidents, selectedIncidentId) {
  const incident = detail?.incident || {};
  const summary = detail?.summary || {};
  const insight = detail?.insight || {};
  const impact = detail?.impact || {};
  const whatChanged = detail?.what_changed || {};
  const primaryAction = detail?.primary_action || null;
  const actions = detail?.actions || [];
  const evidence = detail?.evidence || [];
  const reasoning = incident.reasoning || [];
  const correlationScore = Number(summary.correlation_score || 0);
  const eventCount = Number(summary.event_count || 0);
  const impactCount = Number(impact.impact_count || summary.impact_count || 0);
  const recurringCount = Number(summary.recurring_count || summary.recurringCount || 0);
  const confidence = Number(summary.confidence || incident.confidence || 0);
  const rootCauseType = summary.root_cause_type || incident.root_cause_type || "generic";
  const service = incident.service || summary.service || "";
  const severity = incident.severity || summary.severity || "medium";

  const causeCandidates = [];
  const rootScore = clamp(
    42 +
      Math.round(correlationScore * 0.25) +
      Math.round(eventCount * 4) +
      Math.round(impactCount * 3) +
      (whatChanged.type ? 10 : 0),
    35,
    95
  );

  causeCandidates.push({
    name: titleCase(rootCauseType),
    confidence: rootScore,
    rationale:
      summary.root_cause_summary ||
      "Signals grouped around the strongest incident pattern.",
  });

  if (whatChanged.type) {
    causeCandidates.push({
      name: `${titleCase(whatChanged.type)} Change`,
      confidence: clamp(rootScore - 2, 28, 92),
      rationale:
        `A recent ${whatChanged.type} is linked to the same service or incident window.`,
    });
  }

  if (impact.downstream?.length || impact.affected_services?.length) {
    causeCandidates.push({
      name: "Dependency Degradation",
      confidence: clamp(28 + impactCount * 8 + Math.round(correlationScore * 0.12), 25, 88),
      rationale:
        "Impact spread suggests a dependency or downstream propagation path.",
    });
  }

  if (recurringCount > 0) {
    causeCandidates.push({
      name: "Recurring Failure Pattern",
      confidence: clamp(24 + recurringCount * 8 + Math.round(confidence * 0.15), 22, 84),
      rationale:
        "A similar incident was seen before, which increases the probability of a repeated failure mode.",
    });
  }

  const dedupedCandidates = [];
  const seenNames = new Set();
  causeCandidates
    .sort((left, right) => right.confidence - left.confidence)
    .forEach((candidate) => {
      if (!seenNames.has(candidate.name)) {
        seenNames.add(candidate.name);
        dedupedCandidates.push(candidate);
      }
    });

  const confidenceBreakdown = [
    {
      label: "Correlation Strength",
      value: clamp(correlationScore, 0, 100),
      note: "Pattern, timing, and service matching",
    },
    {
      label: "Signal Volume",
      value: clamp(18 + eventCount * 10, 0, 100),
      note: "More grouped signals increase confidence",
    },
    {
      label: "Impact Spread",
      value: clamp(15 + impactCount * 12, 0, 100),
      note: "Blast radius across services",
    },
    {
      label: "Change Link",
      value: whatChanged.type ? 78 : 24,
      note: whatChanged.type
        ? `${titleCase(whatChanged.type)} linked`
        : "No linked change found",
    },
    {
      label: "Recurrence",
      value: recurringCount > 0 ? clamp(40 + recurringCount * 10, 0, 100) : 18,
      note: recurringCount > 0 ? "Seen before" : "No recurrence context",
    },
  ];

  const derivedNextBestAction =
    primaryAction?.label ||
    actions[0]?.label ||
    insight.suggested_action ||
    detail?.recommended_next_step ||
    "Validate the suspected failure path";

  const derivedNextBestActionDescription =
    primaryAction?.description ||
    actions[0]?.description ||
    "Start with the highest-confidence action to reduce time-to-restore.";

  const fallbackAction =
    actions[1]?.label ||
    (whatChanged.type ? "Validate or rollback recent change" : "Inspect dependency health and recent logs");

  const fallbackActionDescription =
    actions[1]?.description ||
    (whatChanged.type
      ? "If the primary action does not improve the situation, inspect the recent change path."
      : "If the primary action fails, verify dependencies and event evidence.");

  const relatedIncidents = incidents
    .filter((item) => item.id !== selectedIncidentId)
    .filter((item) => {
      const sameService = (item.service || "") === service;
      const samePattern =
        (item.root_cause_type || item.rootCauseType || "") === rootCauseType;
      const openOrAcknowledged =
        item.status === "open" || item.status === "acknowledged";
      return sameService || (samePattern && openOrAcknowledged);
    })
    .slice(0, 4)
    .map((item) => ({
      id: item.id,
      title: item.title,
      service: item.service,
      severity: item.severity,
      status: item.status,
      confidence: item.confidence,
      risk: item.risk_score,
    }));

  const correlationSignature = [
    service || "unknown-service",
    rootCauseType || "generic",
    severity || "medium",
    eventCount,
    impactCount,
  ].join(" · ");

  const evidenceHighlights = [
    ...reasoning.slice(0, 3),
    ...evidence.slice(0, 3),
  ].filter(Boolean);

  return {
    causeCandidates: dedupedCandidates.slice(0, 4),
    confidenceBreakdown,
    nextBestAction: {
      label: derivedNextBestAction,
      description: derivedNextBestActionDescription,
    },
    fallbackAction: {
      label: fallbackAction,
      description: fallbackActionDescription,
    },
    relatedIncidents,
    correlationSignature,
    evidenceHighlights,
  };
}

function deriveLiveOpsStats(incidents, selectedDetail, lastRefreshAt) {
  const open = incidents.filter((item) => item.status === "open").length;
  const acknowledged = incidents.filter((item) => item.status === "acknowledged").length;
  const critical = incidents.filter((item) => item.severity === "critical").length;

  const latestIncidentTime = incidents
    .map((item) => item.last_event_time || item.lastEventTime || item.first_event_time)
    .filter(Boolean)
    .sort()
    .reverse()[0];

  const selectedSummary = selectedDetail?.summary || {};
  const selectedIncident = selectedDetail?.incident || {};
  const freshnessSource =
    selectedSummary.latest_event_time ||
    selectedIncident.last_event_time ||
    selectedIncident.lastEventTime ||
    latestIncidentTime;

  let freshnessLabel = "No signal";
  if (freshnessSource) {
    const now = Date.now();
    const then = new Date(freshnessSource).getTime();
    if (!Number.isNaN(then)) {
      const diffMinutes = Math.max(0, Math.round((now - then) / 60000));
      if (diffMinutes <= 1) freshnessLabel = "Live";
      else if (diffMinutes <= 5) freshnessLabel = `${diffMinutes} min ago`;
      else if (diffMinutes <= 60) freshnessLabel = `${diffMinutes} min ago`;
      else freshnessLabel = `${Math.round(diffMinutes / 60)} hr ago`;
    }
  }

  return {
    open,
    acknowledged,
    critical,
    total: incidents.length,
    latestIncidentTime,
    freshnessLabel,
    lastRefreshAt,
  };
}

export default function IncidentCommandCenter() {
  const [incidents, setIncidents] = useState([]);
  const [selectedIncidentId, setSelectedIncidentId] = useState("");
  const [detail, setDetail] = useState(null);
  const [explanation, setExplanation] = useState("");
  const [activity, setActivity] = useState([]);
  const [actionAudit, setActionAudit] = useState([]);
  const [copilotQuestion, setCopilotQuestion] = useState("Why is this happening?");
  const [copilotAnswer, setCopilotAnswer] = useState(null);
  const [loading, setLoading] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [error, setError] = useState("");
  const [liveRefresh, setLiveRefresh] = useState(true);
  const [activeTab, setActiveTab] = useState("overview");
  const [incidentOpen, setIncidentOpen] = useState(false);
  const [guidelineModal, setGuidelineModal] = useState(null); // action object to show in modal
  const [workspaceMode, setWorkspaceMode] = useState("live");
  const [lastRefreshAt, setLastRefreshAt] = useState("");
  // authReady gates all data fetching — nothing loads until we have a token
  const [authReady, setAuthReady] = useState(false);
  const [loginMode, setLoginMode] = useState("login"); // "login" | "register"
  const [loginEmail, setLoginEmail] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [loginLoading, setLoginLoading] = useState(false);
  const [onboardingComplete, setOnboardingComplete] = useState(true);
  const [onboardingStepsDone, setOnboardingStepsDone] = useState(0);

  const [filters, setFilters] = useState({
    search: "",
    status: "",
    severity: "",
    service: "",
    from: "",
    to: "",
  });

  const selectedIncident = useMemo(
    () => incidents.find((item) => item.id === selectedIncidentId) || null,
    [incidents, selectedIncidentId]
  );

  const services = useMemo(() => {
    const values = Array.from(new Set(incidents.map((item) => item.service).filter(Boolean)));
    return values.sort();
  }, [incidents]);

  const filteredIncidents = useMemo(() => {
    return incidents.filter((incident) => {
      const title = `${incident.title || ""} ${incident.service || ""}`.toLowerCase();
      const searchMatch =
        !filters.search || title.includes(filters.search.trim().toLowerCase());

      const statusMatch = !filters.status || incident.status === filters.status;
      const severityMatch = !filters.severity || incident.severity === filters.severity;
      const serviceMatch = !filters.service || incident.service === filters.service;

      const incidentTime = incident.last_event_time || incident.lastEventTime || incident.first_event_time;
      const time = incidentTime ? new Date(incidentTime).getTime() : null;
      const fromOk = !filters.from || (time && time >= new Date(filters.from).getTime());
      const toOk = !filters.to || (time && time <= new Date(filters.to).getTime() + 86400000);

      return searchMatch && statusMatch && severityMatch && serviceMatch && fromOk && toOk;
    });
  }, [incidents, filters]);

  const metrics = useMemo(() => {
    return {
      open: incidents.filter((item) => item.status === "open").length,
      acknowledged: incidents.filter((item) => item.status === "acknowledged").length,
      critical: incidents.filter((item) => item.severity === "critical").length,
      changeLinked: incidents.filter((item) => item.what_changed_type || item.whatChangedType).length,
    };
  }, [incidents]);

  const loadIncidents = useCallback(async (selectFirst = false) => {
    setLoading(true);
    setError("");

    try {
      const payload = await request(
        `/incidents?page=1&page_size=20&sort_by=last_event_time&sort_order=desc`
      );
      const items = extractIncidents(payload);
      setIncidents(items);
      setLastRefreshAt(new Date().toISOString());

      setSelectedIncidentId((currentId) => {
        const found = Boolean(currentId && items.some((item) => item.id === currentId));
        if (!found || selectFirst) {
          return found ? currentId : (items[0]?.id || "");
        }
        return currentId;
      });
    } catch (err) {
      setError(err.message || "Failed to load incidents");
    } finally {
      setLoading(false);
    }
  }, []);

  // Track whether we have ever loaded a detail, so refreshes never flash "loading"
  const hasDetailRef = React.useRef(false);

  const loadIncidentBundle = useCallback(async (incidentId) => {
    if (!incidentId) {
      setDetail(null);
      setExplanation("");
      setActivity([]);
      setActionAudit([]);
      hasDetailRef.current = false;
      return;
    }

    // Only show loading on the very first load — never on refresh
    if (!hasDetailRef.current) setDetailLoading(true);

    try {
      const [detailPayload, explainPayload, activityPayload, auditPayload] = await Promise.allSettled([
        request(`/incidents/${incidentId}`),
        request(`/incidents/explain/${incidentId}`),
        request(`/incidents/activity/${incidentId}`),
        request(`/actions/audit?incident_id=${incidentId}`),
      ]);

      if (detailPayload.status === "fulfilled") {
        setDetail(extractDetail(detailPayload.value));
        hasDetailRef.current = true;
      }

      if (explainPayload.status === "fulfilled") {
        const explainValue = explainPayload.value;
        setExplanation(
          explainValue?.explanation ||
            explainValue?.answer ||
            explainValue?.data?.explanation ||
            ""
        );
      }

      if (activityPayload.status === "fulfilled") {
        setActivity(extractActivity(activityPayload.value));
      }

      if (auditPayload.status === "fulfilled") {
        setActionAudit(extractActionAudit(auditPayload.value));
      }
    } finally {
      setDetailLoading(false);
    }
  }, []);

  // ── 1. Bootstrap: auth FIRST, then load data ──────────────────────────────
  useEffect(() => {
    ensureAuthenticated().then(async (token) => {
      if (token) {
        setAuthReady(true);
        loadIncidents(true);
        // Check onboarding state for returning users
        try {
          const p = await getOnboardingProgress();
          if (p) {
            const done = p.aha_moment_reached || p.step === "complete";
            setOnboardingComplete(done);
            const steps = ["signup","connect_source","first_alert","first_incident","install_agent"];
            setOnboardingStepsDone((p.completed_steps || []).filter(s => steps.includes(s)).length);
          }
        } catch { /* onboarding check is non-critical */ }
      }
      // else: no token → login form is shown (authReady stays false)
    });
  }, [loadIncidents]);

  // ── 2a. Update incidentOpen status when selection or incident list changes ──
  useEffect(() => {
    if (!selectedIncidentId) return;
    const sel = incidents.find((i) => i.id === selectedIncidentId);
    if (sel) setIncidentOpen(sel.status === "acknowledged");
  }, [incidents, selectedIncidentId]);

  // ── 2b. Load detail bundle when auth is ready and selection changes ────────
  useEffect(() => {
    if (!authReady || !selectedIncidentId) return;
    hasDetailRef.current = false;
    loadIncidentBundle(selectedIncidentId);
  }, [authReady, selectedIncidentId, loadIncidentBundle]);

  // ── 3. Polling interval — only starts after auth is ready ─────────────────
  useEffect(() => {
    if (!authReady || !liveRefresh) return;

    const interval = setInterval(() => {
      loadIncidents(false);
      if (selectedIncidentId) loadIncidentBundle(selectedIncidentId);
    }, 10000);

    return () => clearInterval(interval);
  }, [authReady, liveRefresh, selectedIncidentId, loadIncidents, loadIncidentBundle]);

  async function updateStatus(action) {
    if (!selectedIncidentId) return;

    try {
      await request(`/incidents/${selectedIncidentId}/${action}`, {
        method: "POST",
        body: JSON.stringify({
          changed_by: "operator",
          note: `${action} requested from UI`,
        }),
      });
      await loadIncidents(false);
      await loadIncidentBundle(selectedIncidentId);
    } catch (err) {
      setError(err.message || `Failed to ${action} incident`);
    }
  }

  async function askCopilot(question) {
    if (!selectedIncidentId || !question.trim()) return;

    try {
      const payload = await request(`/incidents/copilot/${selectedIncidentId}`, {
        method: "POST",
        body: JSON.stringify({ question }),
      });
      setCopilotAnswer(payload);
    } catch (err) {
      setCopilotAnswer({
        answer: err.message || "Failed to fetch",
        suggested_followups: [],
      });
    }
  }


  async function handleAuthSubmit(e) {
    e.preventDefault();
    setLoginError("");
    setLoginLoading(true);
    try {
      let token = null;
      if (loginMode === "login") {
        token = await login(loginEmail, loginPassword);
        if (!token) {
          setLoginError("Invalid email or password.");
          setLoginLoading(false);
          return;
        }
      } else {
        // Derive a unique tenant slug from the email local part so every signup
        // gets its own isolated tenant (e.g. john@acme.com → john-a3bx9f).
        const localPart = loginEmail.split("@")[0].toLowerCase().replace(/[^a-z0-9]/g, "").slice(0, 20);
        const suffix = Math.random().toString(36).slice(2, 8);
        const tenantId = localPart ? `${localPart}-${suffix}` : `tenant-${suffix}`;
        token = await register(loginEmail, loginPassword, "admin", tenantId);
        if (!token) {
          setLoginError("Registration failed — email may already exist. Try logging in.");
          setLoginLoading(false);
          return;
        }
        // New user → start the setup guide
        setOnboardingComplete(false);
        setOnboardingStepsDone(0);
        setAuthReady(true);
        loadIncidents(true);
        setWorkspaceMode("onboarding");
        return;
      }
      setAuthReady(true);
      loadIncidents(true);
    } catch (err) {
      setLoginError(err.message || "Request failed — is the backend running on port 8080?");
    } finally {
      setLoginLoading(false);
    }
  }

  const incident = detail?.incident || selectedIncident || {};
  const summary = detail?.summary || {};
  const insight = detail?.insight || {};
  const impact = detail?.impact || {};
  const graph = detail?.graph || { nodes: [], edges: [] };
  const timeline = detail?.events || [];
  const actions = detail?.actions || [];
  const statusAudit = detail?.status_audit || [];

  const intelligence = useMemo(
    () => deriveIntelligence(detail, incidents, selectedIncidentId),
    [detail, incidents, selectedIncidentId]
  );

  const liveOpsStats = useMemo(
    () => deriveLiveOpsStats(incidents, detail, lastRefreshAt),
    [incidents, detail, lastRefreshAt]
  );

  // ── Login / Register screen — shown before any data loads ─────────────────
  // MUST be after all hooks — React requires the same number of hooks every render
  if (!authReady) {
    return (
      <div style={{
        minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center",
        fontFamily: "Inter, system-ui, sans-serif",
        background: "radial-gradient(circle at top left, #112142 0%, #081225 58%, #050b16 100%)",
      }}>
        <div style={{
          width: "100%", maxWidth: 400, padding: "2.5rem 2rem",
          background: "linear-gradient(180deg, rgba(8,20,43,0.98) 0%, rgba(6,16,35,0.96) 100%)",
          border: "1px solid rgba(90,123,186,0.28)", borderRadius: 24,
          boxShadow: "0 18px 44px rgba(0,0,0,0.4)",
        }}>
          <div style={{ textAlign: "center", marginBottom: "2rem" }}>
            <div style={{
              width: 48, height: 48, borderRadius: 14, margin: "0 auto 1rem",
              background: "linear-gradient(180deg,#3aa7ff 0%,#1f56ff 100%)",
              display: "flex", alignItems: "center", justifyContent: "center",
              fontSize: 22, fontWeight: 800, color: "#fff",
              boxShadow: "0 10px 30px rgba(31,86,255,0.35)",
            }}>AI</div>
            <h2 style={{ margin: "0 0 0.25rem", fontSize: "1.3rem", fontWeight: 800, color: "#edf4ff" }}>
              AIOps Platform
            </h2>
            <p style={{ margin: 0, fontSize: "0.82rem", color: "#9eb5da" }}>
              {loginMode === "login" ? "Sign in to your account" : "Create a new account"}
            </p>
          </div>

          <form onSubmit={handleAuthSubmit} style={{ display: "flex", flexDirection: "column", gap: "1rem" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem" }}>
              <label style={{ fontSize: "0.75rem", fontWeight: 600, color: "#9eb5da", textTransform: "uppercase", letterSpacing: "0.06em" }}>Email</label>
              <input
                type="email" required autoFocus
                value={loginEmail} onChange={e => setLoginEmail(e.target.value)}
                placeholder="you@example.com"
                style={{
                  padding: "0.7rem 0.9rem", borderRadius: 12, fontSize: "0.9rem",
                  border: "1px solid rgba(100,132,190,0.28)", background: "rgba(9,18,36,0.96)",
                  color: "#edf4ff", outline: "none",
                }}
              />
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem" }}>
              <label style={{ fontSize: "0.75rem", fontWeight: 600, color: "#9eb5da", textTransform: "uppercase", letterSpacing: "0.06em" }}>Password</label>
              <input
                type="password" required minLength={6}
                value={loginPassword} onChange={e => setLoginPassword(e.target.value)}
                placeholder="••••••••"
                style={{
                  padding: "0.7rem 0.9rem", borderRadius: 12, fontSize: "0.9rem",
                  border: "1px solid rgba(100,132,190,0.28)", background: "rgba(9,18,36,0.96)",
                  color: "#edf4ff", outline: "none",
                }}
              />
            </div>

            {loginError && (
              <div style={{
                padding: "0.6rem 0.9rem", borderRadius: 10, fontSize: "0.82rem",
                background: "rgba(127,29,29,0.32)", border: "1px solid rgba(248,113,113,0.32)", color: "#fecaca",
              }}>{loginError}</div>
            )}

            <button type="submit" disabled={loginLoading} style={{
              marginTop: "0.5rem", padding: "0.75rem", borderRadius: 12, fontSize: "0.95rem", fontWeight: 700,
              background: "linear-gradient(180deg,#35a7ff 0%,#2563eb 100%)", color: "#fff",
              border: "none", cursor: loginLoading ? "default" : "pointer",
              boxShadow: "0 10px 28px rgba(37,99,235,0.26)", opacity: loginLoading ? 0.7 : 1,
            }}>
              {loginLoading ? "Please wait…" : loginMode === "login" ? "Sign In" : "Create Account"}
            </button>
          </form>

          <p style={{ textAlign: "center", marginTop: "1.5rem", fontSize: "0.82rem", color: "#9eb5da" }}>
            {loginMode === "login" ? "No account yet?" : "Already have an account?"}{" "}
            <button
              onClick={() => { setLoginMode(loginMode === "login" ? "register" : "login"); setLoginError(""); }}
              style={{ background: "none", border: "none", color: "#55c6ff", cursor: "pointer", fontSize: "0.82rem", textDecoration: "underline" }}
            >
              {loginMode === "login" ? "Register" : "Sign In"}
            </button>
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="lux-shell">
      {error ? <div className="lux-global-error">{error}</div> : null}

      <div className="lux-topbar">
        <div className="lux-brand">
          <div className="lux-brand-mark">AI</div>
          <div>
            <div className="lux-eyebrow">AI INCIDENT INTELLIGENCE</div>
            <div className="lux-brand-title">Incident Command Center</div>
          </div>
        </div>

        <div className="lux-top-actions">
          <div className="lux-mode-switch">
            <button
              className={`lux-mode-btn ${workspaceMode === "live" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("live")}
            >
              Incidents
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "agents" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("agents")}
            >
              Agents
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "logs" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("logs")}
            >
              Log Explorer
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "integrations" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("integrations")}
            >
              Integrations
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "status" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("status")}
            >
              Status Page
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "intelligence" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("intelligence")}
            >
              Intelligence
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "billing" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("billing")}
            >
              Billing
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "risk" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("risk")}
            >
              Risk
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "aiconfig" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("aiconfig")}
            >
              AI Config
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "alert-quality" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("alert-quality")}
            >
              Alert Quality
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "policy-engine" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("policy-engine")}
            >
              Policy Engine
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "schema-registry" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("schema-registry")}
            >
              Schema Registry
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "team-workflow" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("team-workflow")}
            >
              Team Workflow
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "ai-memory" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("ai-memory")}
            >
              AI Memory
            </button>
            <button
              className={`lux-mode-btn ${workspaceMode === "onboarding" ? "active" : ""}`}
              onClick={() => setWorkspaceMode("onboarding")}
              style={{ position: "relative" }}
            >
              Setup Guide
              {!onboardingComplete && (
                <span style={{
                  display: "inline-flex", alignItems: "center", justifyContent: "center",
                  marginLeft: 6, minWidth: 18, height: 18, borderRadius: 9,
                  background: "#818cf8", color: "#fff", fontSize: "0.65rem", fontWeight: 700,
                  padding: "0 4px",
                }}>
                  {onboardingStepsDone}/5
                </span>
              )}
            </button>
          </div>
          <button className="lux-secondary-btn" onClick={() => setLiveRefresh((value) => !value)}>
            Live refresh: {liveRefresh ? "On" : "Off"}
          </button>
        </div>
      </div>

      <section className="lux-hero">
        <div className="lux-hero-copy">
          <div className="lux-eyebrow">INCIDENT COMMAND CENTER</div>
          <h1>AI-Powered Incident Intelligence</h1>
          <p>
            Real-time correlation, AI root cause analysis, and automated response — from NeuroOps agents on your infrastructure to resolution in one screen.
          </p>
        </div>

        <div className="lux-kpi-strip">
          <MetricCard label="Open" value={metrics.open} />
          <MetricCard label="Acknowledged" value={metrics.acknowledged} />
          <MetricCard label="Critical" value={metrics.critical} />
          <MetricCard label="Change-linked" value={metrics.changeLinked} />
        </div>
      </section>

      <section className="lux-filter-bar">
        <div className="lux-filter-title">
          <div className="lux-eyebrow">INCIDENT QUERY LAYER</div>
          <h2>Filter incidents</h2>
        </div>

        <div className="lux-filter-grid">
          <label>
            <span>Search</span>
            <input
              value={filters.search}
              onChange={(e) =>
                setFilters((current) => ({ ...current, search: e.target.value }))
              }
              placeholder="Title or service"
            />
          </label>

          <label>
            <span>Status</span>
            <select
              value={filters.status}
              onChange={(e) =>
                setFilters((current) => ({ ...current, status: e.target.value }))
              }
            >
              <option value="">All statuses</option>
              <option value="open">open</option>
              <option value="acknowledged">acknowledged</option>
              <option value="resolved">resolved</option>
            </select>
          </label>

          <label>
            <span>Severity</span>
            <select
              value={filters.severity}
              onChange={(e) =>
                setFilters((current) => ({ ...current, severity: e.target.value }))
              }
            >
              <option value="">All severities</option>
              <option value="critical">critical</option>
              <option value="high">high</option>
              <option value="medium">medium</option>
              <option value="low">low</option>
            </select>
          </label>

          <label>
            <span>Service</span>
            <select
              value={filters.service}
              onChange={(e) =>
                setFilters((current) => ({ ...current, service: e.target.value }))
              }
            >
              <option value="">All services</option>
              {services.map((service) => (
                <option key={service} value={service}>
                  {service}
                </option>
              ))}
            </select>
          </label>

          <label>
            <span>From</span>
            <input
              type="date"
              value={filters.from}
              onChange={(e) =>
                setFilters((current) => ({ ...current, from: e.target.value }))
              }
            />
          </label>

          <label>
            <span>To</span>
            <input
              type="date"
              value={filters.to}
              onChange={(e) =>
                setFilters((current) => ({ ...current, to: e.target.value }))
              }
            />
          </label>
        </div>

        <div className="lux-filter-actions">
          <button
            className="lux-secondary-btn"
            onClick={() =>
              setFilters({
                search: "",
                status: "",
                severity: "",
                service: "",
                from: "",
                to: "",
              })
            }
          >
            Clear filters
          </button>
        </div>
      </section>

      {/* ── Full-page panels for Integrations and Status ── */}
      {workspaceMode === "integrations" && (
        <div className="lux-fullpage-panel">
          <IntegrationsHub />
        </div>
      )}
      {workspaceMode === "status" && (
        <div className="lux-fullpage-panel">
          <StatusPageView tenant="default" />
        </div>
      )}

      {workspaceMode === "intelligence" && (
        <IntelligenceHub />
      )}

      {workspaceMode === "agents" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1100 }}>
          <AgentPanel />
        </div>
      )}

      {workspaceMode === "logs" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1400 }}>
          <LogExplorer />
        </div>
      )}

      {workspaceMode === "billing" && (
        <div className="lux-fullpage-panel">
          <BillingPanel />
        </div>
      )}

      {workspaceMode === "risk" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1200 }}>
          <RiskExposureDashboard />
        </div>
      )}

      {workspaceMode === "aiconfig" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 900 }}>
          <AIStatusPanel />
        </div>
      )}

      {workspaceMode === "alert-quality" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 900 }}>
          <AlertQualityPanel />
        </div>
      )}

      {workspaceMode === "policy-engine" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1200 }}>
          <AutomationPolicyEngine />
        </div>
      )}

      {workspaceMode === "schema-registry" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1100 }}>
          <SchemaRegistryPanel />
        </div>
      )}

      {workspaceMode === "team-workflow" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1100 }}>
          <TeamWorkflowPanel incidentId={selectedIncidentId} />
        </div>
      )}

      {workspaceMode === "ai-memory" && (
        <div className="lux-fullpage-panel" style={{ maxWidth: 1100 }}>
          <IncidentMemoryPanel incidentId={selectedIncidentId} />
        </div>
      )}

      {workspaceMode === "onboarding" && (
        <div className="lux-fullpage-panel">
          <OnboardingWizard
            onComplete={() => {
              setOnboardingComplete(true);
              setWorkspaceMode("live");
            }}
          />
        </div>
      )}

      {workspaceMode === "live" && (
      <div className="lux-layout">
        <aside className="lux-left-rail">
          <section className="lux-card lux-sticky-list">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">LIVE HEALTH</div>
                <h3>Source health snapshot</h3>
              </div>
            </div>

            <div className="lux-source-health-grid">
              <LiveHealthCard label="Total incidents" value={liveOpsStats.total} note="Current dataset" />
              <LiveHealthCard label="Open" value={liveOpsStats.open} note="Needs response" />
              <LiveHealthCard label="Critical" value={liveOpsStats.critical} note="Highest urgency" />
              <LiveHealthCard label="Freshness" value={liveOpsStats.freshnessLabel} note="Latest signal age" />
            </div>

            <div className="lux-health-footer">
              <div><strong>Last refresh:</strong> {formatTimestamp(liveOpsStats.lastRefreshAt)}</div>
              <div><strong>Latest incident:</strong> {formatTimestamp(liveOpsStats.latestIncidentTime)}</div>
            </div>
          </section>

          <section className="lux-card lux-sticky-list">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">INCIDENTS</div>
                <h3>Current incidents</h3>
              </div>
              <span className="lux-small-note">{filteredIncidents.length} visible</span>
            </div>

            {loading ? (
              <div className="lux-muted">Loading incidents...</div>
            ) : filteredIncidents.length === 0 ? (
              <div className="lux-muted">No incidents found.</div>
            ) : (
              <div className="lux-incident-list">
                {filteredIncidents.map((item) => {
                  const selected = item.id === selectedIncidentId;
                  return (
                    <button
                      key={item.id}
                      className={`lux-incident-item ${selected ? "selected" : ""}`}
                      onClick={() => setSelectedIncidentId(item.id)}
                    >
                      <div className="lux-incident-top">
                        <span className={`lux-pill ${severityClass(item.severity)}`}>
                          {item.severity}
                        </span>
                        <span className={`lux-pill ${statusClass(item.status)}`}>
                          {item.status}
                        </span>
                      </div>

                      <div className="lux-incident-title" title={item.title}>{item.title}</div>
                      <div className="lux-incident-service">{item.service}</div>
                      <div className="lux-incident-cause">
                        {item.root_cause_summary || item.rootCauseSummary}
                      </div>

                      <div className="lux-incident-meta">
                        <span className="lux-mini-chip">Confidence {item.confidence}</span>
                        <span className="lux-mini-chip">Risk {item.risk_score}</span>
                        <span className="lux-mini-chip">Impact {item.impact_count}</span>
                      </div>

                      {(item.recurring_count || item.recurringCount) > 0 ? (
                        <div className="lux-recurring-line">
                          Seen before · {item.recurring_count || item.recurringCount}
                        </div>
                      ) : null}
                    </button>
                  );
                })}
              </div>
            )}
          </section>
        </aside>

        <main className="lux-main">
          <section className="lux-card lux-detail-hero">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">INCIDENT DETAIL</div>
                <h2>{incident.title || "Select an incident"}</h2>
                <div className="lux-detail-subtitle">
                  {incident.service || "-"} · {incident.status || "-"} · {incident.severity || "-"}
                </div>
              </div>

              <div className="lux-detail-actions">
                {incident.severity ? (
                  <span className={`lux-pill ${severityClass(incident.severity)}`}>
                    {incident.severity}
                  </span>
                ) : null}

                {incident.status === "resolved" ? (
                  <span className="lux-pill" style={{ background: "rgba(16,185,129,0.15)", color: "#10b981" }}>Resolved</span>
                ) : !incidentOpen ? (
                  <button className="lux-primary-btn" onClick={() => {
                    setIncidentOpen(true);
                    if (incident.status === "open") updateStatus("ack");
                  }}>
                    Open Investigation
                  </button>
                ) : (
                  <span className="lux-pill" style={{ background: "rgba(245,158,11,0.15)", color: "#f59e0b" }}>
                    Investigating
                  </span>
                )}
              </div>
            </div>

            <div className="lux-detail-hero-grid">
              <div className="lux-cause-panel">
                <div className="lux-eyebrow">CAUSE</div>
                <h3>{summary.root_cause_summary || "-"}</h3>
                <p>{detail?.narrative || "-"}</p>

                <div className="lux-detail-meta-wrap">
                  {(summary.recurring_count || summary.recurringCount) > 0 ? (
                    <span className="lux-mini-chip">
                      Recurring · {summary.recurring_count || summary.recurringCount}
                    </span>
                  ) : null}
                  <span className="lux-mini-chip">
                    Last seen {formatTimestamp(summary.last_seen_at || summary.lastSeenAt)}
                  </span>
                  {(summary.similar_incident_id || summary.similarIncidentID) ? (
                    <span className="lux-mini-chip">
                      Similar {summary.similar_incident_id || summary.similarIncidentID}
                    </span>
                  ) : null}
                  <span className="lux-mini-chip">
                    Signature {intelligence.correlationSignature}
                  </span>
                </div>
              </div>

              <div className="lux-kpi-grid">
                <DetailKpi label="Confidence" value={summary.confidence ?? incident.confidence ?? "-"} />
                <DetailKpi label="Risk Score" value={summary.risk_score ?? incident.risk_score ?? "-"} />
                <DetailKpi label="Impact" value={`${impact.impact_count ?? summary.impact_count ?? 0} services`} />
                <DetailKpi label="Pattern" value={summary.root_cause_type || "-"} />
              </div>
            </div>
          </section>

          {/* Resolved banner */}
          {incident.status === "resolved" && (
            <div className="lux-resolved-banner">
              <span style={{ fontSize: "1.2rem" }}>✅</span>
              <div>
                <strong>Incident Resolved</strong>
                <div className="slo-service">This incident has been resolved. View the post-mortem or business impact for details.</div>
              </div>
              <button className="lux-secondary-btn small" onClick={() => { setIncidentOpen(true); }}>View Details</button>
            </div>
          )}

          {/* Tabs — only visible when investigation is open OR incident is resolved and user clicks View Details */}
          {(incidentOpen || incident.status === "resolved") && (
          <section className="lux-tabs-card">
            <div className="lux-tab-bar">
              {[
                { key: "overview",  label: "Overview" },
                { key: "activity",  label: "Activity" },
                { key: "actions",   label: "Actions" },
                { key: "topology",  label: "Topology Graph" },
                { key: "memory",    label: "Memory & Playbook" },
                { key: "audit",     label: "Audit" },
                { key: "postmortem",label: "Post-Mortem" },
                { key: "bizimpact", label: "Business Impact" },
              ].map((tab) => (
                <button
                  key={tab.key}
                  className={`lux-tab ${activeTab === tab.key ? "active" : ""}`}
                  onClick={() => setActiveTab(tab.key)}
                >
                  {tab.label}
                </button>
              ))}
            </div>

            {detailLoading ? (
              <div className="lux-muted">Loading incident detail...</div>
            ) : (
              <>
                {activeTab === "overview" ? (
                  <div className="lux-overview-grid">
                    <div className="lux-two-grid">
                      <section className="lux-card">
                        <div className="lux-section-head">
                          <div>
                            <div className="lux-eyebrow">AI LAYER</div>
                            <h3>AI Explanation</h3>
                          </div>
                          <span className="lux-pill">Plain English</span>
                        </div>
                        <div className="lux-long-text">{explanation || "Failed to fetch"}</div>
                      </section>

                      <section className="lux-card">
                        <div className="lux-section-head">
                          <div>
                            <div className="lux-eyebrow">RCA</div>
                            <h3>Ranked Root Causes</h3>
                          </div>
                        </div>
                        <div className="lux-cause-candidate-list">
                          {intelligence.causeCandidates.map((candidate) => (
                            <div key={candidate.name} className="lux-cause-candidate-card">
                              <div className="lux-cause-candidate-top">
                                <strong>{candidate.name}</strong>
                                <span className="lux-mini-chip">{candidate.confidence}% confidence</span>
                              </div>
                              <div className="lux-action-subtext">{candidate.rationale}</div>
                            </div>
                          ))}
                        </div>
                      </section>
                    </div>

                    {/* Resolution Steps */}
                    {(detail?.resolution_steps || []).length > 0 && (
                      <section className="lux-card">
                        <div className="lux-section-head">
                          <div>
                            <div className="lux-eyebrow">HOW TO RESOLVE</div>
                            <h3>Resolution Steps</h3>
                          </div>
                        </div>
                        <ol className="resolution-steps-list">
                          {detail.resolution_steps.map((step, i) => (
                            <li key={i}>{step}</li>
                          ))}
                        </ol>
                      </section>
                    )}

                    {/* Occurrence Times */}
                    {(detail?.occurrence_times || []).length > 0 && (
                      <section className="lux-card">
                        <div className="lux-section-head">
                          <div>
                            <div className="lux-eyebrow">OCCURRENCES</div>
                            <h3>When this incident occurred ({(detail.occurrence_times || []).length} times)</h3>
                          </div>
                        </div>
                        <div className="occurrence-times-list">
                          {detail.occurrence_times.slice(0, 20).map((t, i) => (
                            <span key={i} className="lux-mini-chip">{t}</span>
                          ))}
                          {detail.occurrence_times.length > 20 && (
                            <span className="lux-muted">...and {detail.occurrence_times.length - 20} more</span>
                          )}
                        </div>
                      </section>
                    )}

                    <section className="lux-card">
                      <div className="lux-section-head">
                        <div>
                          <div className="lux-eyebrow">COPILOT</div>
                          <h3>Incident Copilot</h3>
                        </div>
                      </div>

                      <div className="lux-copilot-quick">
                        {[
                          "Why is this happening?",
                          "What should I do first?",
                          "What changed?",
                          "Has this happened before?",
                        ].map((question) => (
                          <button
                            key={question}
                            className="lux-secondary-btn small"
                            onClick={() => {
                              setCopilotQuestion(question);
                              askCopilot(question);
                            }}
                          >
                            {question}
                          </button>
                        ))}
                      </div>

                      <div className="lux-copilot-bar">
                        <input
                          value={copilotQuestion}
                          onChange={(e) => setCopilotQuestion(e.target.value)}
                          placeholder="Ask about this incident..."
                        />
                        <button className="lux-primary-btn" onClick={() => askCopilot(copilotQuestion)}>
                          Ask
                        </button>
                      </div>

                      {copilotAnswer ? (
                        <div className="lux-copilot-answer">
                          <div className="lux-long-text">{copilotAnswer.answer || "-"}</div>
                          {Array.isArray(copilotAnswer.suggested_followups) && copilotAnswer.suggested_followups.length > 0 ? (
                            <div className="lux-copilot-followups">
                              {copilotAnswer.suggested_followups.map((item) => (
                                <button
                                  key={item}
                                  className="lux-secondary-btn small"
                                  onClick={() => {
                                    setCopilotQuestion(item);
                                    askCopilot(item);
                                  }}
                                >
                                  {item}
                                </button>
                              ))}
                            </div>
                          ) : null}
                        </div>
                      ) : null}
                    </section>

                    <div className="lux-two-grid">
                      <section className="lux-card">
                        <div className="lux-eyebrow">ACTION PLANNER</div>
                        <div className="lux-week2-action-stack">
                          <div className="lux-week2-action-card">
                            <div className="lux-week2-action-label">Next Best Action</div>
                            <h3>{intelligence.nextBestAction.label}</h3>
                            <p>{intelligence.nextBestAction.description}</p>
                          </div>
                          <div className="lux-week2-action-card">
                            <div className="lux-week2-action-label">Fallback Action</div>
                            <h3>{intelligence.fallbackAction.label}</h3>
                            <p>{intelligence.fallbackAction.description}</p>
                          </div>
                        </div>
                      </section>

                      <section className="lux-card">
                        <div className="lux-eyebrow">CONFIDENCE MODEL</div>
                        <div className="lux-confidence-breakdown-list">
                          {intelligence.confidenceBreakdown.map((item) => (
                            <div key={item.label} className="lux-confidence-item">
                              <div className="lux-confidence-head">
                                <span>{item.label}</span>
                                <strong>{item.value}%</strong>
                              </div>
                              <div className="lux-confidence-bar">
                                <div className="lux-confidence-fill" style={{ width: `${item.value}%` }} />
                              </div>
                              <div className="lux-action-subtext">{item.note}</div>
                            </div>
                          ))}
                        </div>
                      </section>
                    </div>

                    <div className="lux-three-grid">
                      <InfoCard
                        eyebrow="DECISION SUMMARY"
                        title=""
                        body={
                          <div className="lux-dense-block">
                            <div className="lux-chip-row">
                              <span className="lux-mini-chip">Pattern {summary.root_cause_type || "-"}</span>
                              <span className="lux-mini-chip">Events {summary.event_count || 0}</span>
                              <span className="lux-mini-chip">Score {summary.correlation_score || 0}</span>
                            </div>
                            <div><strong>Latest event</strong><br />{formatTimestamp(summary.latest_event_time)}</div>
                            <div><strong>Reasoning</strong><br />{(detail?.incident?.reasoning || []).join(", ") || "-"}</div>
                            <div><strong>Signature</strong><br />{intelligence.correlationSignature}</div>
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow="WHAT CHANGED"
                        title=""
                        body={
                          <div className="lux-dense-block">
                            {detail?.what_changed?.type ? (
                              <>
                                <div><strong>Type</strong><br />{detail.what_changed.type}</div>
                                <div><strong>Service</strong><br />{detail.what_changed.service || "-"}</div>
                                <div><strong>Version</strong><br />{detail.what_changed.version || "-"}</div>
                                <div><strong>Description</strong><br />{detail.what_changed.description || "-"}</div>
                                <div><strong>Timestamp</strong><br />{formatTimestamp(detail.what_changed.timestamp)}</div>
                              </>
                            ) : (
                              <div>No recent deployment, config, or infra change is currently linked.</div>
                            )}
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow="OPERATOR GUIDANCE"
                        title=""
                        body={
                          <div className="lux-dense-block">
                            <span className="lux-pill guidance-pill">
                              {String(insight.confidence || "medium").toUpperCase()} CONFIDENCE
                            </span>
                            <div>{insight.suggested_action || detail?.recommended_next_step || "-"}</div>
                            <div><strong>Evidence highlights</strong></div>
                            <ul className="lux-bullet-list compact">
                              {intelligence.evidenceHighlights.length > 0 ? (
                                intelligence.evidenceHighlights.map((item, index) => (
                                  <li key={index}>{item}</li>
                                ))
                              ) : (
                                <li>No evidence highlights available.</li>
                              )}
                            </ul>
                          </div>
                        }
                      />
                    </div>

                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Why This Is Likely"
                        body={
                          Array.isArray(insight.why_this_is_likely) && insight.why_this_is_likely.length > 0 ? (
                            <ul className="lux-bullet-list">
                              {insight.why_this_is_likely.map((item, index) => (
                                <li key={index}>{item}</li>
                              ))}
                            </ul>
                          ) : (
                            <div>-</div>
                          )
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Recommended Checks"
                        body={
                          Array.isArray(insight.recommended_checks) && insight.recommended_checks.length > 0 ? (
                            <ul className="lux-bullet-list">
                              {insight.recommended_checks.map((item, index) => (
                                <li key={index}>{item}</li>
                              ))}
                            </ul>
                          ) : (
                            <div>-</div>
                          )
                        }
                      />
                    </div>

                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Service Graph"
                        body={
                          <div className="lux-graph-wrap">
                            <div className="lux-graph-col">
                              <h4>Nodes</h4>
                              <div className="lux-graph-stack">
                                {(graph.nodes || []).map((node) => (
                                  <div className="lux-graph-node" key={node.id}>
                                    <div className="lux-graph-node-title">{node.label || node.id}</div>
                                    <div className="lux-graph-node-meta">
                                      {node.node_type || "-"} {node.severity ? `· ${node.severity}` : ""}
                                    </div>
                                  </div>
                                ))}
                              </div>
                            </div>

                            <div className="lux-graph-col">
                              <h4>Relationships</h4>
                              <div className="lux-graph-stack">
                                {(graph.edges || []).map((edge, index) => (
                                  <div className="lux-graph-edge" key={`${edge.from}-${edge.to}-${index}`}>
                                    <strong>{edge.from}</strong>
                                    <span>→</span>
                                    <strong>{edge.to}</strong>
                                    <small>{edge.relation}</small>
                                  </div>
                                ))}
                              </div>
                            </div>
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Related Incidents"
                        body={
                          intelligence.relatedIncidents.length > 0 ? (
                            <div className="lux-related-incident-list">
                              {intelligence.relatedIncidents.map((item) => (
                                <div key={item.id} className="lux-related-incident-card">
                                  <div className="lux-chip-row">
                                    <span className={`lux-pill ${severityClass(item.severity)}`}>{item.severity}</span>
                                    <span className={`lux-pill ${statusClass(item.status)}`}>{item.status}</span>
                                  </div>
                                  <strong>{item.title}</strong>
                                  <div className="lux-action-subtext">{item.service}</div>
                                  <div className="lux-chip-row">
                                    <span className="lux-mini-chip">Confidence {item.confidence}</span>
                                    <span className="lux-mini-chip">Risk {item.risk}</span>
                                  </div>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <div>No related incidents found in the current dataset.</div>
                          )
                        }
                      />
                    </div>
                  </div>
                ) : null}

                {activeTab === "activity" ? (
                  <div className="lux-overview-grid">
                    <section className="lux-card">
                      <div className="lux-section-head">
                        <h3>Unified Activity Timeline</h3>
                      </div>
                      {activity.length === 0 ? (
                        <div className="lux-muted">Failed to fetch</div>
                      ) : (
                        <div className="lux-activity-list">
                          {activity.map((item, index) => (
                            <div key={item.id || index} className="lux-activity-item">
                              <div className="lux-activity-title">
                                {item.title || item.label || item.type || "Activity"}
                              </div>
                              <div className="lux-activity-body">
                                {item.message || item.description || "-"}
                              </div>
                              <div className="lux-activity-time">
                                {formatTimestamp(item.timestamp || item.created_at)}
                              </div>
                            </div>
                          ))}
                        </div>
                      )}
                    </section>

                    <section className="lux-card">
                      <div className="lux-section-head">
                        <h3>Timeline Story</h3>
                      </div>
                      {timeline.length === 0 ? (
                        <div className="lux-muted">No timeline available.</div>
                      ) : (
                        <div className="lux-story-list">
                          {timeline.map((item, index) => (
                            <div key={`${item.event?.id || index}-${index}`} className="lux-story-item">
                              <div className="lux-chip-row">
                                <span className="lux-mini-chip">{item.signal_type || "-"}</span>
                                <span className="lux-mini-chip">{formatTimestamp(item.event?.timestamp)}</span>
                                <span className="lux-mini-chip">{item.stage_type || "-"}</span>
                              </div>
                              <strong>{item.story_label || "-"}</strong>
                              <div>{item.event?.message || "-"}</div>
                            </div>
                          ))}
                        </div>
                      )}
                    </section>
                  </div>
                ) : null}

                {activeTab === "actions" ? (
                  <div className="lux-overview-grid">
                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Impact Analysis"
                        body={
                          <div className="lux-dense-block">
                            <div><strong>Primary Service:</strong> {impact.primary_service || "-"}</div>
                            <div><strong>Impact Level:</strong> {String(impact.impact_level || "-").toUpperCase()}</div>
                            <div><strong>Downstream Services:</strong></div>
                            <ul className="lux-bullet-list compact">
                              {(impact.downstream || []).map((item) => <li key={item}>{item}</li>)}
                            </ul>
                            <div><strong>Affected Services:</strong></div>
                            <ul className="lux-bullet-list compact">
                              {(impact.affected_services || []).map((item) => <li key={item}>{item}</li>)}
                            </ul>
                          </div>
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Recommended Actions"
                        body={
                          actions.length === 0 ? (
                            <div>-</div>
                          ) : (
                            <div className="lux-action-stack">
                              {actions.map((action, idx) => {
                                const isVerify = action.type === "verification";
                                const isRemediation = action.type === "remediation";
                                const borderColor = isVerify
                                  ? "rgba(16,185,129,0.5)"
                                  : isRemediation
                                  ? "rgba(251,146,60,0.4)"
                                  : "rgba(95,128,189,0.24)";
                                return (
                                  <div key={action.id} className="lux-action-card" style={{ borderColor, position: "relative" }}>
                                    <div className="lux-action-top">
                                      <strong style={{ display: "flex", alignItems: "center", gap: 6 }}>
                                        {isVerify && <span style={{ color: "#10b981" }}>✓</span>}
                                        <span style={{ color: isVerify ? "#86efac" : "inherit" }}>
                                          {isVerify ? `Step ${idx + 1} (Final): ` : `Step ${idx + 1}: `}
                                          {action.label}
                                        </span>
                                      </strong>
                                      <span className={`lux-risk-badge ${riskClass(action.risk_level)}`}>
                                        {String(action.risk_level || "low").toUpperCase()} RISK
                                      </span>
                                    </div>
                                    <pre style={{
                                      margin: "8px 0 6px", padding: "10px 12px", borderRadius: 10,
                                      background: "rgba(5,13,29,0.9)", color: isVerify ? "#86efac" : "#8fd3ff",
                                      fontSize: "0.78rem", lineHeight: 1.65, whiteSpace: "pre-wrap",
                                      wordBreak: "break-word", border: `1px solid ${borderColor}`,
                                      fontFamily: "monospace",
                                    }}>{action.description}</pre>
                                    <div className="lux-action-subtext">
                                      {isVerify ? "🔍 Verification · " : ""}
                                      {action.type} · {action.requires_approval ? "⚠ Approval required" : "No approval needed"}
                                    </div>
                                    {isVerify ? (
                                      <button
                                        className="lux-primary-btn small"
                                        style={{ marginTop: 8, background: "linear-gradient(180deg,#10b981 0%,#059669 100%)", border: "none" }}
                                        onClick={() => updateStatus("resolve")}
                                      >
                                        Mark as Resolved
                                      </button>
                                    ) : (
                                      <button className="lux-guideline-btn" onClick={() => setGuidelineModal(action)}>
                                        View Resolution Guide
                                      </button>
                                    )}
                                  </div>
                                );
                              })}
                            </div>
                          )
                        }
                      />
                    </div>
                  </div>
                ) : null}

                {activeTab === "audit" ? (
                  <div className="lux-overview-grid">
                    <div className="lux-two-grid">
                      <InfoCard
                        eyebrow=""
                        title="Status Audit"
                        body={
                          statusAudit.length === 0 ? (
                            <div>-</div>
                          ) : (
                            <div className="lux-audit-stack">
                              {statusAudit.map((item) => (
                                <div key={item.id} className="lux-audit-card">
                                  <div className="lux-chip-row">
                                    <span className="lux-mini-chip">
                                      {item.previous_status} → {item.new_status}
                                    </span>
                                    <span className="lux-mini-chip">{formatTimestamp(item.changed_at)}</span>
                                  </div>
                                  <div>{item.note || "-"}</div>
                                  <div className="lux-action-subtext">Changed by {item.changed_by || "-"}</div>
                                </div>
                              ))}
                            </div>
                          )
                        }
                      />

                      <InfoCard
                        eyebrow=""
                        title="Action Audit"
                        body={
                          actionAudit.length === 0 ? (
                            <div>-</div>
                          ) : (
                            <div className="lux-audit-stack">
                              {actionAudit.map((item, index) => (
                                <div key={item.id || index} className="lux-audit-card">
                                  <div className="lux-chip-row">
                                    <span className="lux-mini-chip">{item.action_id || item.action || "-"}</span>
                                    <span className="lux-mini-chip">{String(item.status || "-").toUpperCase()}</span>
                                  </div>
                                  <div>{item.message || item.note || "-"}</div>
                                  <div className="lux-action-subtext">
                                    {item.approved_at
                                      ? `Approved ${formatTimestamp(item.approved_at)}`
                                      : formatTimestamp(item.created_at)}
                                  </div>
                                </div>
                              ))}
                            </div>
                          )
                        }
                      />
                    </div>
                  </div>
                ) : null}

                {activeTab === "postmortem" ? (
                  <PostMortemPanel incidentID={incident.id} />
                ) : null}

                {activeTab === "bizimpact" ? (
                  <BusinessImpactPanel incidentID={incident.id} />
                ) : null}

                {activeTab === "topology" ? (
                  <TopologyGraphPanel incidentId={incident.id} />
                ) : null}

                {activeTab === "memory" ? (
                  <IncidentMemoryPanel incidentId={incident.id} />
                ) : null}
              </>
            )}

            {/* Action bar at bottom of tabs — resolve/acknowledge */}
            {incident.status !== "resolved" && (
              <div className="lux-incident-action-bar">
                {incident.status === "open" && (
                  <button className="lux-secondary-btn" onClick={() => updateStatus("ack")}>
                    Mark as Acknowledged
                  </button>
                )}
                <button className="lux-primary-btn" onClick={() => updateStatus("resolve")}
                  style={{ background: "#10b981", borderColor: "#10b981" }}>
                  Mark as Resolved
                </button>
              </div>
            )}
          </section>
          )} {/* end incidentOpen conditional */}
        </main>

        <aside className="lux-right-rail">
          <section className="lux-card lux-sticky-rail">
            <div className="lux-section-head">
              <div>
                <div className="lux-eyebrow">OPERATOR RAIL</div>
                <h3>Live execution context</h3>
              </div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Selected incident</div>
              <div className="lux-rail-title">{incident.title || "-"}</div>
              <div className="lux-rail-subtext">{incident.service || "-"} · {incident.status || "-"} · {incident.severity || "-"}</div>
            </div>

            <div className="lux-rail-grid">
              <RailStat label="Confidence" value={summary.confidence ?? incident.confidence ?? "-"} />
              <RailStat label="Risk" value={summary.risk_score ?? incident.risk_score ?? "-"} />
              <RailStat label="Impact" value={impact.impact_count ?? summary.impact_count ?? "-"} />
              <RailStat label="Events" value={summary.event_count ?? 0} />
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Next best action</div>
              <div className="lux-rail-title">{intelligence.nextBestAction.label}</div>
              <div className="lux-rail-subtext">{intelligence.nextBestAction.description}</div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Fallback action</div>
              <div className="lux-rail-subtext">{intelligence.fallbackAction.label}</div>
              <div className="lux-rail-subtext">{intelligence.fallbackAction.description}</div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Quick actions</div>
              <div className="lux-rail-actions">
                {!incidentOpen && incident.status !== "resolved" && (
                  <button className="lux-primary-btn" onClick={() => setIncidentOpen(true)}>Open Investigation</button>
                )}
                {incident.status === "resolved" && (
                  <button className="lux-secondary-btn" onClick={() => updateStatus("reopen")}>Reopen</button>
                )}
              </div>
            </div>

            <div className="lux-rail-block">
              <div className="lux-rail-label">Operator guidance</div>
              <div className="lux-guidance-card">
                <span className="lux-pill guidance-pill">
                  {String(insight.confidence || "medium").toUpperCase()} CONFIDENCE
                </span>
                <p>{insight.suggested_action || detail?.recommended_next_step || "-"}</p>
              </div>
            </div>
          </section>
        </aside>
      </div>
      )} {/* end live layout */}

      {/* ── Guideline Modal ── */}
      {guidelineModal && (
        <div className="guide-overlay" onClick={() => setGuidelineModal(null)}>
          <div className="guide-modal" onClick={e => e.stopPropagation()}>
            <div className="guide-modal-header">
              <div>
                <div className="lux-eyebrow">RESOLUTION GUIDE</div>
                <h2 style={{ margin: "0.25rem 0 0" }}>{guidelineModal.label}</h2>
              </div>
              <button className="guide-close" onClick={() => setGuidelineModal(null)}>✕</button>
            </div>

            <div className="guide-meta">
              <span className={`lux-risk-badge ${riskClass(guidelineModal.risk_level)}`}>
                {String(guidelineModal.risk_level || "low").toUpperCase()} RISK
              </span>
              <span className="slo-service">Type: {guidelineModal.type}</span>
              {guidelineModal.requires_approval && <span className="lux-mini-chip" style={{ color: "#f59e0b" }}>Requires approval</span>}
            </div>

            <div className="guide-section">
              <div className="guide-section-title">What this does</div>
              <p className="guide-desc">{guidelineModal.description}</p>
            </div>

            <div className="guide-section">
              <div className="guide-section-title">Step-by-step instructions</div>
              <div className="guide-steps">
                {buildGuidelineSteps(guidelineModal).map((step, i) => (
                  <div key={i} className="guide-step">
                    <div className="guide-step-num">{i + 1}</div>
                    <div className="guide-step-body">
                      <div className="guide-step-title">{step.title}</div>
                      {step.command && (
                        <div className="guide-step-cmd">
                          <code>$ {step.command}</code>
                          <button className="guide-copy-btn" onClick={() => { navigator.clipboard.writeText(step.command).catch(() => {}); }}>Copy</button>
                        </div>
                      )}
                      {step.note && <div className="guide-step-note">{step.note}</div>}
                    </div>
                  </div>
                ))}
              </div>
            </div>

            <div className="guide-section">
              <div className="guide-section-title">After completing</div>
              <p className="guide-desc">
                Once you have followed these steps and verified the issue is resolved, go back to the incident and click <strong>"Mark as Resolved"</strong>.
              </p>
            </div>

            <div className="guide-footer">
              <button className="lux-secondary-btn" onClick={() => setGuidelineModal(null)}>Close</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function IntelligenceHub() {
  const [subTab, setSubTab] = useState("slos");
  const tabs = [
    { key: "slos",        label: "SLO Tracking" },
    { key: "oncall",      label: "On-Call" },
    { key: "anomalies",   label: "Anomaly Detection" },
    { key: "health",      label: "Engineering Health" },
    { key: "roi",         label: "ROI Dashboard" },
    { key: "digest",      label: "Weekly Digest" },
    { key: "feedback",    label: "Alert Feedback" },
    { key: "autoresolve", label: "Auto-Resolve" },
    { key: "runbooks",    label: "Runbooks" },
    { key: "dependencies",label: "Dependencies" },
    { key: "compliance",  label: "Compliance" },
    { key: "risk",        label: "Risk Dashboard" },
    { key: "aistatus",    label: "AI Status" },
  ];
  return (
    <div className="lux-fullpage-panel" style={{ maxWidth: 1100 }}>
      <div className="p3-nav">
        {tabs.map(t => (
          <button key={t.key} className={`lux-mode-btn ${subTab === t.key ? "active" : ""}`}
            onClick={() => setSubTab(t.key)}>{t.label}</button>
        ))}
      </div>
      {subTab === "slos" && <SLODashboard />}
      {subTab === "oncall" && <OnCallPanel />}
      {subTab === "anomalies" && <AnomalyPanel />}
      {subTab === "health" && <EngineeringHealthPanel />}
      {subTab === "roi" && <ROIDashboardPanel />}
      {subTab === "digest" && <WeeklyDigestPanel />}
      {subTab === "feedback" && <AlertFeedbackPanel />}
      {subTab === "autoresolve" && <AutoResolvePanel />}
      {subTab === "runbooks" && <RunbookPanel />}
      {subTab === "dependencies" && <DependencyPanel />}
      {subTab === "compliance" && <CompliancePanel />}
      {subTab === "risk" && <RiskExposureDashboard />}
      {subTab === "aistatus" && <AIStatusPanel />}
    </div>
  );
}

function MetricCard({ label, value }) {
  return (
    <div className="lux-metric-card">
      <div className="lux-metric-label">{label}</div>
      <div className="lux-metric-value">{value}</div>
    </div>
  );
}

// Builds step-by-step terminal instructions from an action
function buildGuidelineSteps(action) {
  const label = (action.label || "").toLowerCase();
  const desc = (action.description || "").toLowerCase();
  const text = label + " " + desc;
  const steps = [];

  // Extract any commands from the description
  const cmdPatterns = [
    { match: "df -h", title: "Check disk usage", command: "df -h", note: "Look for partitions above 85% usage" },
    { match: "du -sh", title: "Find large directories", command: "du -sh /* 2>/dev/null | sort -hr | head -20", note: "Identifies which directories are consuming the most space" },
    { match: "free -", title: "Check memory usage", command: "free -h", note: "Look at 'available' column — if very low, memory pressure exists" },
    { match: "top ", title: "Check running processes", command: "top -b -n 1 -o %CPU | head -25", note: "Look for processes consuming excessive CPU" },
    { match: "ps aux", title: "List all processes", command: "ps aux --sort=-%mem | head -20", note: "Sorted by memory usage — look for memory-hungry processes" },
    { match: "systemctl status", title: "Check service status", command: "systemctl status <service-name>", note: "Replace <service-name> with the affected service. Look for 'Active: failed' or error messages" },
    { match: "docker ps", title: "Check Docker containers", command: "docker ps -a --format 'table {{.Names}}\\t{{.Status}}\\t{{.Ports}}'", note: "Look for containers in 'Exited' or 'Restarting' status" },
    { match: "docker system", title: "Check Docker disk usage", command: "docker system df", note: "If space is high, run: docker system prune -a --volumes" },
    { match: "kubectl get", title: "Check Kubernetes resources", command: "kubectl get pods -A --field-selector status.phase!=Running", note: "Shows pods that are not running — check their events with kubectl describe pod <name>" },
    { match: "kubectl logs", title: "Check pod logs", command: "kubectl logs <pod-name> --tail=50", note: "Replace <pod-name> — look for error messages in the last 50 lines" },
    { match: "kubectl describe", title: "Describe Kubernetes resource", command: "kubectl describe pod <pod-name>", note: "Look at 'Events' section at the bottom for scheduling/pull/crash errors" },
    { match: "netstat ", title: "Check network connections", command: "ss -tlnp", note: "Shows all listening ports — verify your service is listening on the expected port" },
    { match: "find /var/log", title: "Clean old log files", command: "find /var/log -name '*.gz' -mtime +30 -delete", note: "Removes compressed logs older than 30 days. Run with sudo if needed" },
    { match: "journalctl", title: "Check system journal", command: "journalctl -u <service-name> --since '30 min ago' --no-pager", note: "Shows recent logs for a systemd service" },
    { match: "telnet", title: "Test port connectivity", command: "nc -zv <host> <port>", note: "Tests if the target host:port is reachable. Replace <host> and <port>" },
    { match: "connection pool", title: "Check database connections", command: "psql -c \"SELECT count(*) as total, state FROM pg_stat_activity GROUP BY state;\"", note: "Shows active/idle connections. If 'active' is near max_connections, the pool is exhausted" },
    { match: "connection refused", title: "Verify service is running", command: "systemctl status <service-name> && ss -tlnp | grep <port>", note: "Check if the target service is running and listening on the expected port" },
    { match: "certificate", title: "Check certificate expiry", command: "openssl s_client -connect <host>:443 -servername <host> 2>/dev/null | openssl x509 -noout -dates", note: "Shows certificate validity dates" },
    { match: "permission denied", title: "Check file permissions", command: "ls -la <file-or-directory>", note: "Verify the process user has read/write/execute access" },
    { match: "out of memory", title: "Check memory and OOM events", command: "dmesg | grep -i 'oom\\|killed process' | tail -10", note: "Shows if the kernel OOM killer terminated any process" },
  ];

  // Step 1: Always start with SSH/terminal
  steps.push({
    title: "Open a terminal on the affected host",
    command: null,
    note: "SSH into the machine where the incident is occurring, or open a local terminal if the agent is on this machine",
  });

  // Step 2+: Match commands from the action text
  let matched = false;
  for (const p of cmdPatterns) {
    if (text.includes(p.match)) {
      steps.push({ title: p.title, command: p.command, note: p.note });
      matched = true;
    }
  }

  // If no specific command matched, try to extract from the description directly
  if (!matched) {
    // Look for text that looks like a command (starts with common command names)
    const descWords = (action.description || "").split(/\n|;/).map(s => s.trim()).filter(Boolean);
    for (const line of descWords) {
      const lower = line.toLowerCase();
      if (/^(check|inspect|validate|verify|review|restart|kill|scale)/.test(lower)) {
        steps.push({ title: line, command: null, note: "Follow this step based on your environment setup" });
      }
    }
  }

  // If still nothing, add generic investigation steps
  if (steps.length <= 1) {
    steps.push({ title: "Check service logs", command: "journalctl -u <service> --since '1 hour ago' | tail -50", note: "Look for error messages, stack traces, or repeated warnings" });
    steps.push({ title: "Check system resources", command: "top -b -n 1 | head -10 && free -h && df -h", note: "Quick overview of CPU, memory, and disk" });
  }

  // Final step: always verify
  steps.push({
    title: "Verify the issue is resolved",
    command: null,
    note: "Check that the service is responding normally, error rates have dropped, and no new alerts are firing. Then return to the incident and click 'Mark as Resolved'.",
  });

  return steps;
}

function DetailKpi({ label, value }) {
  return (
    <div className="lux-detail-kpi">
      <div className="lux-metric-label">{label}</div>
      <div className="lux-detail-kpi-value">{value}</div>
    </div>
  );
}

function InfoCard({ eyebrow, title, body }) {
  return (
    <div className="lux-card">
      {eyebrow ? <div className="lux-eyebrow">{eyebrow}</div> : null}
      {title ? <h3>{title}</h3> : null}
      <div>{body}</div>
    </div>
  );
}

function RailStat({ label, value }) {
  return (
    <div className="lux-rail-stat">
      <div className="lux-rail-label">{label}</div>
      <div className="lux-rail-stat-value">{value}</div>
    </div>
  );
}

function LiveHealthCard({ label, value, note }) {
  return (
    <div className="lux-live-health-card">
      <div className="lux-rail-label">{label}</div>
      <div className="lux-live-health-value">{value}</div>
      <div className="lux-action-subtext">{note}</div>
    </div>
  );
}
