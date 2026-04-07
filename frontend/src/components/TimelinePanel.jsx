import React from "react";

export default function TimelinePanel({ events }) {
  const timeline = events || [];

  return (
    <div className="panel">
      <div className="panel-header">
        <h3>Timeline</h3>
      </div>

      {timeline.length === 0 ? (
        <div className="empty-state">No timeline events available.</div>
      ) : (
        <div className="timeline-list">
          {timeline.map((item, index) => (
            <div className="timeline-item" key={`${item.event?.id || index}-${index}`}>
              <div className="timeline-marker" />
              <div className="timeline-content">
                <div className="timeline-top">
                  <span className="timeline-label">{item.story_label || item.signal_type || "event"}</span>
                  <span className="timeline-time">{item.event?.timestamp || "-"}</span>
                </div>
                <div className="timeline-message">{item.event?.message || "-"}</div>
                <div className="timeline-meta">
                  <span>Stage: {item.stage_type || "-"}</span>
                  <span>Signal: {item.signal_type || "-"}</span>
                  <span>{item.gap_from_previous || ""}</span>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
