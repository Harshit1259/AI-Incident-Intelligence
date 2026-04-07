import { useEffect, useState } from "react";
import { getIncidentActivity } from "../api/incidents";

function formatTime(value) {
  if (!value) {
    return "-";
  }

  return new Date(value).toLocaleString();
}

function ActivityTimeline({ incidentId }) {
  const [items, setItems] = useState([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!incidentId) {
      setItems([]);
      setError("");
      return;
    }

    let isActive = true;

    async function loadActivity() {
      setLoading(true);
      setError("");

      try {
        const payload = await getIncidentActivity(incidentId);

        if (!isActive) {
          return;
        }

        setItems(Array.isArray(payload) ? payload : []);
      } catch (activityError) {
        if (!isActive) {
          return;
        }

        setItems([]);
        setError(activityError.message || "Failed to load activity timeline.");
      } finally {
        if (isActive) {
          setLoading(false);
        }
      }
    }

    loadActivity();

    return () => {
      isActive = false;
    };
  }, [incidentId]);

  return (
    <div className="detail-section">
      <h3>Unified Activity Timeline</h3>

      {loading ? <p>Loading activity...</p> : null}
      {error ? <p>{error}</p> : null}
      {!loading && !error && items.length === 0 ? <p>No activity available yet.</p> : null}

      <div className="activity-timeline-list">
        {items.map((item, index) => (
          <div key={`${item.type}-${item.timestamp}-${index}`} className="activity-timeline-item">
            <div className="activity-timeline-top">
              <span className="signal-chip">{item.type}</span>
              <span className="signal-chip">{formatTime(item.timestamp)}</span>
            </div>

            <h4>{item.title || "Activity"}</h4>
            <p>{item.description || "-"}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

export default ActivityTimeline;
