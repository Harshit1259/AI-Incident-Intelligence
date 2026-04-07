import { useEffect, useMemo, useState } from "react";
import { getEvents } from "../api/events";
import { getIncidentDetail, getIncidents, updateIncidentStatus } from "../api/incidents";
import DemoPanel from "../components/DemoPanel";
import FilterBar from "../components/FilterBar";
import IncidentDetailPanel from "../components/IncidentDetailPanel";
import IncidentList from "../components/IncidentList";
import PaginationControls from "../components/PaginationControls";
import { EmptyState, ErrorState, LoadingState } from "../components/StateBlocks";
import { useDebouncedValue } from "../hooks/useDebouncedValue";

const DEFAULT_FILTERS = {
  status: "",
  severity: "",
  service: "",
  search: "",
  from: "",
  to: "",
  page: 1,
  page_size: 20,
  sort_by: "last_event_time",
  sort_order: "desc",
};

const LIVE_REFRESH_INTERVAL_MS = 10000;

function getFiltersFromURL() {
  const searchParams = new URLSearchParams(window.location.search);

  return {
    ...DEFAULT_FILTERS,
    status: searchParams.get("status") || "",
    severity: searchParams.get("severity") || "",
    service: searchParams.get("service") || "",
    search: searchParams.get("search") || "",
    from: searchParams.get("from") || "",
    to: searchParams.get("to") || "",
    page: Number(searchParams.get("page") || DEFAULT_FILTERS.page),
    page_size: Number(searchParams.get("page_size") || DEFAULT_FILTERS.page_size),
    sort_by: searchParams.get("sort_by") || DEFAULT_FILTERS.sort_by,
    sort_order: searchParams.get("sort_order") || DEFAULT_FILTERS.sort_order,
  };
}

function formatDateForAPI(value, endOfDay = false) {
  if (!value) {
    return "";
  }

  return `${value}T${endOfDay ? "23:59:59" : "00:00:00"}Z`;
}

function buildTopMetrics(items) {
  const openCount = items.filter((incident) => incident.status === "open").length;
  const acknowledgedCount = items.filter((incident) => incident.status === "acknowledged").length;
  const criticalCount = items.filter((incident) => incident.severity === "critical").length;
  const changeLinkedCount = items.filter((incident) => incident.has_what_changed).length;

  return [
    { label: "Open incidents", value: openCount },
    { label: "Acknowledged", value: acknowledgedCount },
    { label: "Critical", value: criticalCount },
    { label: "Change-linked", value: changeLinkedCount },
  ];
}

function IncidentDashboardPage() {
  const [events, setEvents] = useState([]);
  const [allServiceOptions, setAllServiceOptions] = useState([]);
  const [incidentListResponse, setIncidentListResponse] = useState({
    items: [],
    page: 1,
    page_size: 20,
    total: 0,
    has_more: false,
  });
  const [filters, setFilters] = useState(() => getFiltersFromURL());
  const [selectedIncidentId, setSelectedIncidentId] = useState(null);
  const [selectedIncidentDetail, setSelectedIncidentDetail] = useState(null);
  const [listLoading, setListLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [liveRefreshEnabled, setLiveRefreshEnabled] = useState(true);
  const [listError, setListError] = useState("");
  const [detailError, setDetailError] = useState("");

  const debouncedSearch = useDebouncedValue(filters.search, 400);

  const queryFilters = useMemo(() => {
    return {
      ...filters,
      search: debouncedSearch,
      from: formatDateForAPI(filters.from, false),
      to: formatDateForAPI(filters.to, true),
    };
  }, [debouncedSearch, filters]);

  const serviceOptions = useMemo(() => {
    const serviceSet = new Set(allServiceOptions);

    incidentListResponse.items.forEach((incident) => {
      if (incident.service) {
        serviceSet.add(incident.service);
      }
    });

    events.forEach((event) => {
      if (event.service) {
        serviceSet.add(event.service);
      }
    });

    return Array.from(serviceSet).sort((leftValue, rightValue) =>
      leftValue.localeCompare(rightValue)
    );
  }, [allServiceOptions, events, incidentListResponse.items]);

  const topMetrics = useMemo(
    () => buildTopMetrics(incidentListResponse.items || []),
    [incidentListResponse.items]
  );

  useEffect(() => {
    const searchParams = new URLSearchParams();

    Object.entries(filters).forEach(([key, value]) => {
      if (value !== "" && value !== null && value !== undefined) {
        searchParams.set(key, String(value));
      }
    });

    const nextSearch = searchParams.toString();
    const nextURL = nextSearch
      ? `${window.location.pathname}?${nextSearch}`
      : window.location.pathname;

    window.history.replaceState({}, "", nextURL);
  }, [filters]);

  useEffect(() => {
    function handlePopState() {
      setFilters(getFiltersFromURL());
    }

    window.addEventListener("popstate", handlePopState);

    return () => {
      window.removeEventListener("popstate", handlePopState);
    };
  }, []);

  async function refreshCurrentData(options = {}) {
    const { silent = false, forceSelectLatest = false } = options;

    if (!silent) {
      setListLoading(true);
    }

    setListError("");

    try {
      const [eventsPayload, incidentsPayload] = await Promise.all([
        getEvents(),
        getIncidents(queryFilters),
      ]);

      const safeEvents = Array.isArray(eventsPayload) ? eventsPayload : [];
      const safeIncidentResponse = incidentsPayload || {
        items: [],
        page: 1,
        page_size: 20,
        total: 0,
        has_more: false,
      };

      setEvents(safeEvents);
      setIncidentListResponse(safeIncidentResponse);

      const refreshedServices = new Set();

      safeEvents.forEach((event) => {
        if (event.service) {
          refreshedServices.add(event.service);
        }
      });

      (safeIncidentResponse.items || []).forEach((incident) => {
        if (incident.service) {
          refreshedServices.add(incident.service);
        }
      });

      setAllServiceOptions((currentOptions) => {
        const merged = new Set(currentOptions);
        refreshedServices.forEach((service) => merged.add(service));
        return Array.from(merged).sort((leftValue, rightValue) =>
          leftValue.localeCompare(rightValue)
        );
      });

      const nextItems = Array.isArray(safeIncidentResponse.items)
        ? safeIncidentResponse.items
        : [];

      setSelectedIncidentId((currentIncidentId) => {
        if (nextItems.length === 0) {
          return null;
        }

        if (forceSelectLatest) {
          return nextItems[0].id;
        }

        if (
          currentIncidentId &&
          nextItems.some((incident) => incident.id === currentIncidentId)
        ) {
          return currentIncidentId;
        }

        return nextItems[0].id;
      });
    } catch (error) {
      setListError(error.message || "Failed to refresh data.");
    } finally {
      if (!silent) {
        setListLoading(false);
      }
    }
  }

  useEffect(() => {
    let isActive = true;

    async function loadDashboard() {
      setListLoading(true);
      setListError("");

      try {
        const [eventsPayload, incidentsPayload] = await Promise.all([
          getEvents(),
          getIncidents(queryFilters),
        ]);

        if (!isActive) {
          return;
        }

        const safeEvents = Array.isArray(eventsPayload) ? eventsPayload : [];
        const safeIncidentResponse = incidentsPayload || {
          items: [],
          page: 1,
          page_size: 20,
          total: 0,
          has_more: false,
        };
        const nextItems = Array.isArray(safeIncidentResponse.items)
          ? safeIncidentResponse.items
          : [];

        setEvents(safeEvents);
        setIncidentListResponse(safeIncidentResponse);

        const discoveredServices = new Set();

        safeEvents.forEach((event) => {
          if (event.service) {
            discoveredServices.add(event.service);
          }
        });

        nextItems.forEach((incident) => {
          if (incident.service) {
            discoveredServices.add(incident.service);
          }
        });

        setAllServiceOptions((currentOptions) => {
          const merged = new Set(currentOptions);
          discoveredServices.forEach((service) => merged.add(service));
          return Array.from(merged).sort((leftValue, rightValue) =>
            leftValue.localeCompare(rightValue)
          );
        });

        setSelectedIncidentId((currentIncidentId) => {
          if (nextItems.length === 0) {
            return null;
          }

          if (
            currentIncidentId &&
            nextItems.some((incident) => incident.id === currentIncidentId)
          ) {
            return currentIncidentId;
          }

          return nextItems[0].id;
        });
      } catch (error) {
        if (!isActive) {
          return;
        }

        setListError(error.message || "Failed to load incidents.");
      } finally {
        if (isActive) {
          setListLoading(false);
        }
      }
    }

    loadDashboard();

    return () => {
      isActive = false;
    };
  }, [queryFilters]);

  useEffect(() => {
    if (!liveRefreshEnabled) {
      return undefined;
    }

    const intervalId = window.setInterval(() => {
      refreshCurrentData({ silent: true });
    }, LIVE_REFRESH_INTERVAL_MS);

    return () => {
      window.clearInterval(intervalId);
    };
  }, [liveRefreshEnabled, queryFilters]);

  useEffect(() => {
    if (!selectedIncidentId) {
      setSelectedIncidentDetail(null);
      setDetailError("");
      return;
    }

    let isActive = true;

    async function loadIncidentDetail() {
      setDetailLoading(true);
      setDetailError("");

      try {
        const payload = await getIncidentDetail(selectedIncidentId);

        if (!isActive) {
          return;
        }

        setSelectedIncidentDetail(payload);
      } catch (error) {
        if (!isActive) {
          return;
        }

        setSelectedIncidentDetail(null);
        setDetailError(error.message || "Failed to load incident detail.");
      } finally {
        if (isActive) {
          setDetailLoading(false);
        }
      }
    }

    loadIncidentDetail();

    return () => {
      isActive = false;
    };
  }, [selectedIncidentId]);

  function handleFilterChange(key, value) {
    setFilters((currentFilters) => ({
      ...currentFilters,
      [key]: value,
      page: key === "page" || key === "page_size" ? currentFilters.page : 1,
    }));
  }

  function handleClearFilters() {
    setFilters(DEFAULT_FILTERS);
  }

  function handlePageChange(nextPage) {
    if (nextPage <= 0) {
      return;
    }

    setFilters((currentFilters) => ({
      ...currentFilters,
      page: nextPage,
    }));
  }

  async function handleIncidentAction(action) {
    if (!selectedIncidentId) {
      return;
    }

    try {
      setActionLoading(true);
      setDetailError("");

      await updateIncidentStatus(selectedIncidentId, action);
      await refreshCurrentData({ silent: true });

      const payload = await getIncidentDetail(selectedIncidentId);
      setSelectedIncidentDetail(payload);
    } catch (error) {
      setDetailError(error.message || "Failed to update incident.");
    } finally {
      setActionLoading(false);
    }
  }

  async function handleScenarioRun() {
    await refreshCurrentData({ forceSelectLatest: true });
  }

  return (
    <div className="page-shell">
      <section className="panel hero-shell">
        <div>
          <p className="panel-eyebrow">AI Incident Intelligence</p>
          <h1>Systems should explain themselves.</h1>
          <p className="hero-copy">
            Replace dashboard hunting with one-screen incident understanding: cause,
            confidence, recent change context, impact, timeline story, and next action.
          </p>
        </div>

        <div className="hero-actions">
          <button
            type="button"
            className="secondary-button"
            onClick={() => setLiveRefreshEnabled((currentValue) => !currentValue)}
          >
            Live refresh: {liveRefreshEnabled ? "On" : "Off"}
          </button>
        </div>
      </section>

      <section className="metric-grid">
        {topMetrics.map((metric) => (
          <div key={metric.label} className="panel metric-card">
            <p className="metric-label">{metric.label}</p>
            <strong className="metric-value">{metric.value}</strong>
          </div>
        ))}
      </section>

      <FilterBar
        filters={filters}
        onChange={handleFilterChange}
        onClear={handleClearFilters}
        serviceOptions={serviceOptions}
      />

      <div className="content-grid">
        <div className="left-column">
          <DemoPanel onScenarioRun={handleScenarioRun} />

          {listLoading ? (
            <LoadingState label="Loading incidents..." />
          ) : listError ? (
            <ErrorState message={listError} onRetry={() => refreshCurrentData()} />
          ) : incidentListResponse.items.length === 0 ? (
            <EmptyState
              title="No incidents found"
              subtitle="Try clearing filters or generating a demo scenario."
            />
          ) : (
            <>
              <IncidentList
                incidents={incidentListResponse.items}
                selectedIncidentId={selectedIncidentId}
                onSelect={setSelectedIncidentId}
              />

              <PaginationControls
                page={incidentListResponse.page}
                pageSize={incidentListResponse.page_size}
                total={incidentListResponse.total}
                hasMore={incidentListResponse.has_more}
                onPageChange={handlePageChange}
              />
            </>
          )}
        </div>

        <div className="right-column">
          {detailLoading ? (
            <LoadingState label="Loading incident detail..." />
          ) : detailError ? (
            <ErrorState
              message={detailError}
              onRetry={() => {
                if (selectedIncidentId) {
                  getIncidentDetail(selectedIncidentId)
                    .then((payload) => {
                      setSelectedIncidentDetail(payload);
                      setDetailError("");
                    })
                    .catch((error) => {
                      setDetailError(error.message || "Failed to load incident detail.");
                    });
                }
              }}
            />
          ) : (
            <IncidentDetailPanel
              detail={selectedIncidentDetail}
              actionLoading={actionLoading}
              onAction={handleIncidentAction}
            />
          )}
        </div>
      </div>
    </div>
  );
}

export default IncidentDashboardPage;
