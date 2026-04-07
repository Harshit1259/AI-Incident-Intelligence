CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents (status);
CREATE INDEX IF NOT EXISTS idx_incidents_severity ON incidents (severity);
CREATE INDEX IF NOT EXISTS idx_incidents_service ON incidents (service);
CREATE INDEX IF NOT EXISTS idx_incidents_last_event_time ON incidents (last_event_time DESC);

CREATE INDEX IF NOT EXISTS idx_incident_events_incident_id 
ON incident_events (incident_id);

