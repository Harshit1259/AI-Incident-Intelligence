import { useEffect, useState } from "react";
import "./App.css";

const API_BASE_URL = "http://localhost:8080/api/v1";

function formatTime(timestamp) {
  if (!timestamp) return "-";
  return new Date(timestamp).toLocaleString();
}

function App() {
  const [events, setEvents] = useState([]);
  const [incidents, setIncidents] = useState([]);
  const [selectedIncidentId, setSelectedIncidentId] = useState(null);
  const [selectedIncidentDetail, setSelectedIncidentDetail] = useState(null);

  const fetchDashboardData = async () => {
    const [eventsResponse, incidentsResponse] = await Promise.all([
      fetch(`${API_BASE_URL}/events`).then((response) => response.json()),
      fetch(`${API_BASE_URL}/incidents`).then((response) => response.json()),
    ]);

    const fetchedEvents = Array.isArray(eventsResponse) ? eventsResponse : [];
    const fetchedIncidents = Array.isArray(incidentsResponse) ? incidentsResponse : [];

    setEvents(fetchedEvents);
    setIncidents(fetchedIncidents);

    if (fetchedIncidents.length > 0) {
      setSelectedIncidentId((currentSelectedIncidentId) => {
        const exists = fetchedIncidents.some(
          (incident) => incident.id === currentSelectedIncidentId
        );
        return exists ? currentSelectedIncidentId : fetchedIncidents[0].id;
      });
    } else {
      setSelectedIncidentId(null);
      setSelectedIncidentDetail(null);
    }
  };

  const fetchIncidentDetail = async (incidentId) => {
    if (!incidentId) {
      setSelectedIncidentDetail(null);
      return;
    }

    try {
      const response = await fetch(`${API_BASE_URL}/incidents/${incidentId}`);
      if (!response.ok) {
        setSelectedIncidentDetail(null);
        return;
      }

      const detail = await response.json();
      setSelectedIncidentDetail(detail);
    } catch (error) {
      console.error("Failed to fetch incident detail:", error);
      setSelectedIncidentDetail(null);
    }
  };

  useEffect(() => {
    fetchDashboardData();
  }, []);

  useEffect(() => {
    fetchIncidentDetail(selectedIncidentId);
  }, [selectedIncidentId]);

  const selectedIncident = selectedIncidentDetail?.incident || null;
  const selectedIncidentEvents = selectedIncidentDetail?.events || [];
  const selectedIncidentInsight = selectedIncidentDetail?.insight || null;

  const criticalIncidentCount = incidents.filter(
    (incident) => incident.severity === "critical"
  ).length;

  const activeEventCount = selectedIncidentDetail?.summary?.event_count || 0;
  const affectedServicesCount = selectedIncidentDetail?.summary?.service ? 1 : 0;

  const correlationRate = events.length > 0
    ? Math.round((incidents.length / events.length) * 100)
    : 0;

  const refreshAll = async () => {
    await fetchDashboardData();
  };

  return (
    <div className="ui-root">
      <aside className="sidebar">
        <div className="sidebar-logo-wrap">
          <div className="sidebar-logo-mark">O</div>
          <div>
            <p className="sidebar-mini-label">AIOPS</p>
            <h2 className="sidebar-logo-text">Incident Intelligence</h2>
          </div>
        </div>

        <nav className="sidebar-nav">
          <button className="sidebar-nav-item active">Dashboard</button>
          <button className="sidebar-nav-item">Incidents</button>
          <button className="sidebar-nav-item">Events</button>
        </nav>
      </aside>

      <main className="main-shell">
        <header className="top-shell-bar">
          <div>
            <p className="eyebrow">AI INCIDENT INTELLIGENCE</p>
            <h1 className="page-title">Incident Intelligence</h1>
            <p className="page-subtitle">
              Real-time correlation of noisy events into actionable incidents
            </p>
          </div>

          <div className="top-bar-actions">
            <div className="critical-chip">
              {criticalIncidentCount} critical active
            </div>

            <button className="refresh-button" onClick={refreshAll}>
              Refresh
            </button>
          </div>
        </header>

        <section className="metrics-row">
          <div className="metric-card metric-red">
            <span className="metric-label">Total incidents</span>
            <h2>{incidents.length}</h2>
            <p>{criticalIncidentCount} critical</p>
          </div>

          <div className="metric-card metric-amber">
            <span className="metric-label">Active events</span>
            <h2>{activeEventCount}</h2>
            <p>for selected incident</p>
          </div>

          <div className="metric-card metric-purple">
            <span className="metric-label">Affected services</span>
            <h2>{affectedServicesCount}</h2>
            <p>in impact radius</p>
          </div>

          <div className="metric-card metric-cyan">
            <span className="metric-label">Correlation rate</span>
            <h2>{correlationRate}%</h2>
            <p>events → incidents</p>
          </div>
        </section>

        <section className="dashboard-grid">
          <div className="primary-column">
            <section className="hero-panel">
              <div className="panel-header-row">
                <span className="panel-kicker danger">Active Incident</span>
                {selectedIncident && (
                  <span className="severity-pill">
                    {selectedIncident.severity}
                  </span>
                )}
              </div>

              {selectedIncident ? (
                <>
                  <h2 className="hero-title">{selectedIncident.title}</h2>

                  <div className="hero-meta">
                    <div>
                      <span className="meta-label">Service</span>
                      <strong>{selectedIncident.service}</strong>
                    </div>

                    <div>
                      <span className="meta-label">Events</span>
                      <strong>{selectedIncidentDetail?.summary?.event_count || 0}</strong>
                    </div>

                    <div>
                      <span className="meta-label">Detected</span>
                      <strong>{formatTime(selectedIncident.last_event_time)}</strong>
                    </div>

                    <div>
                      <span className="meta-label">Incident ID</span>
                      <strong>{selectedIncident.id}</strong>
                    </div>
                  </div>

                  {selectedIncidentInsight && (
                    <div className="rca-panel">
                      <div className="timeline-header">
                        <span className="panel-kicker">Root Cause Analysis</span>
                        <span className="timeline-count">
                          {selectedIncidentInsight.confidence} confidence
                        </span>
                      </div>

                      <div className="rca-block">
                        <span className="meta-label">Incident Type</span>
                        <p className="rca-main-text">{selectedIncidentInsight.incident_type}</p>
                      </div>

                      <div className="rca-block">
                        <span className="meta-label">Likely Root Cause</span>
                        <p className="rca-main-text">{selectedIncidentInsight.likely_root_cause}</p>
                      </div>

                      <div className="rca-columns">
                        <div className="rca-column">
                          <span className="meta-label">Why This Is Likely</span>
                          <ul className="rca-list">
                            {selectedIncidentInsight.why_this_is_likely?.map((item, index) => (
                              <li key={index}>{item}</li>
                            ))}
                          </ul>
                        </div>

                        <div className="rca-column">
                          <span className="meta-label">Recommended Checks</span>
                          <ul className="rca-list">
                            {selectedIncidentInsight.recommended_checks?.map((item, index) => (
                              <li key={index}>{item}</li>
                            ))}
                          </ul>
                        </div>
                      </div>

                      <div className="rca-block">
                        <span className="meta-label">Suggested Action</span>
                        <p className="rca-main-text">{selectedIncidentInsight.suggested_action}</p>
                      </div>
                    </div>
                  )}

                  <div className="timeline-panel">
                    <div className="timeline-header">
                      <span className="panel-kicker">Incident Timeline</span>
                      <span className="timeline-count">
                        {selectedIncidentEvents.length} events
                      </span>
                    </div>

                    <div className="timeline-list">
                      {selectedIncidentEvents.length === 0 ? (
                        <p className="empty-copy">No correlated events found.</p>
                      ) : (
                        selectedIncidentEvents.map((event, index) => (
                          <div className="timeline-item" key={event.id}>
                            <div className="timeline-marker-wrap">
                              <div className="timeline-marker"></div>
                              {index !== selectedIncidentEvents.length - 1 && (
                                <div className="timeline-line"></div>
                              )}
                            </div>

                            <div className="timeline-content">
                              <div className="timeline-top-row">
                                <strong>{event.message}</strong>
                                <span className="timeline-badge">
                                  {event.severity}
                                </span>
                              </div>

                              <div className="timeline-meta">
                                <span>{event.service}</span>
                                <span>{formatTime(event.timestamp)}</span>
                                <span>{event.id}</span>
                              </div>
                            </div>
                          </div>
                        ))
                      )}
                    </div>
                  </div>
                </>
              ) : (
                <p className="empty-copy">No active incident selected.</p>
              )}
            </section>

            <section className="table-panel">
              <div className="table-header">
                <span className="panel-kicker">All Incidents</span>
                <span className="table-count">{incidents.length} total</span>
              </div>

              <div className="incident-table">
                {incidents.length === 0 ? (
                  <p className="empty-copy">No incidents available.</p>
                ) : (
                  incidents.map((incident) => (
                    <button
                      key={incident.id}
                      className={`incident-row ${
                        incident.id === selectedIncidentId ? "selected" : ""
                      }`}
                      onClick={() => setSelectedIncidentId(incident.id)}
                    >
                      <div className="incident-row-title">
                        {incident.title}
                      </div>
                      <div>{incident.service}</div>
                      <div>{incident.severity}</div>
                      <div>{incident.status}</div>
                      <div>{formatTime(incident.last_event_time)}</div>
                    </button>
                  ))
                )}
              </div>
            </section>
          </div>

          <aside className="stream-panel">
            <div className="stream-header">
              <span className="panel-kicker">Event Stream</span>
              <span className="stream-live">live</span>
            </div>

            <div className="stream-list">
              {events.length === 0 ? (
                <p className="empty-copy">No incoming events.</p>
              ) : (
                events
                  .slice()
                  .sort((leftEvent, rightEvent) => {
                    return new Date(rightEvent.timestamp).getTime() - new Date(leftEvent.timestamp).getTime();
                  })
                  .map((event) => (
                    <div className="stream-card" key={event.id}>
                      <div className="stream-severity">{event.severity}</div>
                      <strong>{event.message}</strong>
                      <p>{event.service}</p>
                      <span>{formatTime(event.timestamp)}</span>
                    </div>
                  ))
              )}
            </div>
          </aside>
        </section>
      </main>
    </div>
  );
}

export default App;
