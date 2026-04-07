package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

const incidentTimeLayout = "2006-01-02T15:04:05Z07:00"

type IncidentStore struct {
	db *sql.DB
}

func NewIncidentStore(db *sql.DB) *IncidentStore {
	return &IncidentStore{db: db}
}

func (incidentStore *IncidentStore) AddIncident(incident models.Incident) error {
	reasoningJSON, err := json.Marshal(incident.Reasoning)
	if err != nil {
		return err
	}

	impactedServicesJSON, err := json.Marshal(incident.ImpactedServices)
	if err != nil {
		return err
	}

	_, err = incidentStore.db.Exec(
		`INSERT INTO incidents (
			id,
			service,
			severity,
			status,
			first_event_time,
			last_event_time,
			title,
			correlation_pattern,
			correlation_score,
			correlation_reason,
			confidence,
			risk_score,
			event_count,
			root_cause_summary,
			root_cause_type,
			reasoning_json,
			what_changed_type,
			what_changed_service,
			what_changed_version,
			what_changed_description,
			what_changed_timestamp,
			impacted_services_json,
			impact_count,
			seen_before,
			recurring_count,
			similar_incident_id,
			last_seen_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27
		)`,
		incident.ID,
		incident.Service,
		incident.Severity,
		incident.Status,
		incident.FirstEventTime,
		incident.LastEventTime,
		incident.Title,
		incident.CorrelationPattern,
		incident.CorrelationScore,
		incident.CorrelationReason,
		incident.Confidence,
		incident.RiskScore,
		incident.EventCount,
		incident.RootCauseSummary,
		incident.RootCauseType,
		string(reasoningJSON),
		incident.WhatChangedType,
		incident.WhatChangedService,
		incident.WhatChangedVersion,
		incident.WhatChangedDescription,
		incident.WhatChangedTimestamp,
		string(impactedServicesJSON),
		incident.ImpactCount,
		incident.SeenBefore,
		incident.RecurringCount,
		incident.SimilarIncidentID,
		incident.LastSeenAt,
	)
	if err != nil {
		return err
	}

	for _, eventID := range incident.EventIDs {
		_, err = incidentStore.db.Exec(
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`,
			incident.ID,
			eventID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (incidentStore *IncidentStore) GetIncidents() ([]models.Incident, error) {
	rows, err := incidentStore.db.Query(
		`SELECT
			id,
			service,
			severity,
			status,
			first_event_time,
			last_event_time,
			title,
			COALESCE(correlation_pattern, ''),
			COALESCE(correlation_score, 0),
			COALESCE(correlation_reason, ''),
			COALESCE(confidence, 0),
			COALESCE(risk_score, 0),
			COALESCE(event_count, 0),
			COALESCE(root_cause_summary, ''),
			COALESCE(root_cause_type, ''),
			COALESCE(reasoning_json, '[]'),
			COALESCE(what_changed_type, ''),
			COALESCE(what_changed_service, ''),
			COALESCE(what_changed_version, ''),
			COALESCE(what_changed_description, ''),
			COALESCE(what_changed_timestamp, ''),
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			COALESCE(last_seen_at, '')
		 FROM incidents`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	incidents := make([]models.Incident, 0)

	for rows.Next() {
		var incident models.Incident
		var reasoningJSON string
		var impactedServicesJSON string

		err := rows.Scan(
			&incident.ID,
			&incident.Service,
			&incident.Severity,
			&incident.Status,
			&incident.FirstEventTime,
			&incident.LastEventTime,
			&incident.Title,
			&incident.CorrelationPattern,
			&incident.CorrelationScore,
			&incident.CorrelationReason,
			&incident.Confidence,
			&incident.RiskScore,
			&incident.EventCount,
			&incident.RootCauseSummary,
			&incident.RootCauseType,
			&reasoningJSON,
			&incident.WhatChangedType,
			&incident.WhatChangedService,
			&incident.WhatChangedVersion,
			&incident.WhatChangedDescription,
			&incident.WhatChangedTimestamp,
			&impactedServicesJSON,
			&incident.ImpactCount,
			&incident.SeenBefore,
			&incident.RecurringCount,
			&incident.SimilarIncidentID,
			&incident.LastSeenAt,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning); err != nil {
			incident.Reasoning = []string{}
		}

		if err := json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices); err != nil {
			incident.ImpactedServices = []string{}
		}

		eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
		if err != nil {
			return nil, err
		}
		incident.EventIDs = eventIDs

		incidents = append(incidents, incident)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return incidents, nil
}

func (incidentStore *IncidentStore) GetIncidentByID(incidentID string) (models.Incident, bool) {
	row := incidentStore.db.QueryRow(
		`SELECT
			id,
			service,
			severity,
			status,
			first_event_time,
			last_event_time,
			title,
			COALESCE(correlation_pattern, ''),
			COALESCE(correlation_score, 0),
			COALESCE(correlation_reason, ''),
			COALESCE(confidence, 0),
			COALESCE(risk_score, 0),
			COALESCE(event_count, 0),
			COALESCE(root_cause_summary, ''),
			COALESCE(root_cause_type, ''),
			COALESCE(reasoning_json, '[]'),
			COALESCE(what_changed_type, ''),
			COALESCE(what_changed_service, ''),
			COALESCE(what_changed_version, ''),
			COALESCE(what_changed_description, ''),
			COALESCE(what_changed_timestamp, ''),
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			COALESCE(last_seen_at, '')
		 FROM incidents
		 WHERE id = $1`,
		incidentID,
	)

	var incident models.Incident
	var reasoningJSON string
	var impactedServicesJSON string

	err := row.Scan(
		&incident.ID,
		&incident.Service,
		&incident.Severity,
		&incident.Status,
		&incident.FirstEventTime,
		&incident.LastEventTime,
		&incident.Title,
		&incident.CorrelationPattern,
		&incident.CorrelationScore,
		&incident.CorrelationReason,
		&incident.Confidence,
		&incident.RiskScore,
		&incident.EventCount,
		&incident.RootCauseSummary,
		&incident.RootCauseType,
		&reasoningJSON,
		&incident.WhatChangedType,
		&incident.WhatChangedService,
		&incident.WhatChangedVersion,
		&incident.WhatChangedDescription,
		&incident.WhatChangedTimestamp,
		&impactedServicesJSON,
		&incident.ImpactCount,
		&incident.SeenBefore,
		&incident.RecurringCount,
		&incident.SimilarIncidentID,
		&incident.LastSeenAt,
	)
	if err != nil {
		return models.Incident{}, false
	}

	if err := json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning); err != nil {
		incident.Reasoning = []string{}
	}

	if err := json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices); err != nil {
		incident.ImpactedServices = []string{}
	}

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err != nil {
		return models.Incident{}, false
	}
	incident.EventIDs = eventIDs

	return incident, true
}

func (incidentStore *IncidentStore) GetEventIDsByIncidentID(incidentID string) ([]string, error) {
	rows, err := incidentStore.db.Query(
		`SELECT event_id FROM incident_events WHERE incident_id = $1`,
		incidentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	eventIDs := make([]string, 0)

	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return nil, err
		}
		eventIDs = append(eventIDs, eventID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return eventIDs, nil
}

func (incidentStore *IncidentStore) UpdateIncident(updatedIncident models.Incident) error {
	reasoningJSON, err := json.Marshal(updatedIncident.Reasoning)
	if err != nil {
		return err
	}

	impactedServicesJSON, err := json.Marshal(updatedIncident.ImpactedServices)
	if err != nil {
		return err
	}

	_, err = incidentStore.db.Exec(
		`UPDATE incidents
		 SET service = $2,
		     severity = $3,
		     status = $4,
		     first_event_time = $5,
		     last_event_time = $6,
		     title = $7,
		     correlation_pattern = $8,
		     correlation_score = $9,
		     correlation_reason = $10,
		     confidence = $11,
		     risk_score = $12,
		     event_count = $13,
		     root_cause_summary = $14,
		     root_cause_type = $15,
		     reasoning_json = $16,
		     what_changed_type = $17,
		     what_changed_service = $18,
		     what_changed_version = $19,
		     what_changed_description = $20,
		     what_changed_timestamp = $21,
		     impacted_services_json = $22,
		     impact_count = $23,
		     seen_before = $24,
		     recurring_count = $25,
		     similar_incident_id = $26,
		     last_seen_at = $27
		 WHERE id = $1`,
		updatedIncident.ID,
		updatedIncident.Service,
		updatedIncident.Severity,
		updatedIncident.Status,
		updatedIncident.FirstEventTime,
		updatedIncident.LastEventTime,
		updatedIncident.Title,
		updatedIncident.CorrelationPattern,
		updatedIncident.CorrelationScore,
		updatedIncident.CorrelationReason,
		updatedIncident.Confidence,
		updatedIncident.RiskScore,
		updatedIncident.EventCount,
		updatedIncident.RootCauseSummary,
		updatedIncident.RootCauseType,
		string(reasoningJSON),
		updatedIncident.WhatChangedType,
		updatedIncident.WhatChangedService,
		updatedIncident.WhatChangedVersion,
		updatedIncident.WhatChangedDescription,
		updatedIncident.WhatChangedTimestamp,
		string(impactedServicesJSON),
		updatedIncident.ImpactCount,
		updatedIncident.SeenBefore,
		updatedIncident.RecurringCount,
		updatedIncident.SimilarIncidentID,
		updatedIncident.LastSeenAt,
	)
	if err != nil {
		return err
	}

	_, err = incidentStore.db.Exec(
		`DELETE FROM incident_events WHERE incident_id = $1`,
		updatedIncident.ID,
	)
	if err != nil {
		return err
	}

	for _, eventID := range updatedIncident.EventIDs {
		_, err = incidentStore.db.Exec(
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`,
			updatedIncident.ID,
			eventID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (incidentStore *IncidentStore) UpdateIncidentStatus(incidentID string, status string) (models.Incident, error) {
	result, err := incidentStore.db.Exec(
		`UPDATE incidents SET status = $2 WHERE id = $1`,
		incidentID,
		status,
	)
	if err != nil {
		return models.Incident{}, err
	}

	affectedRows, err := result.RowsAffected()
	if err != nil {
		return models.Incident{}, err
	}

	if affectedRows == 0 {
		return models.Incident{}, sql.ErrNoRows
	}

	incident, found := incidentStore.GetIncidentByID(incidentID)
	if !found {
		return models.Incident{}, sql.ErrNoRows
	}

	return incident, nil
}

func (incidentStore *IncidentStore) FindRecentSimilarIncident(service string, pattern string, incidentTime time.Time) (*models.Incident, error) {
	windowStart := incidentTime.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	windowEnd := incidentTime.Add(-1 * time.Minute).Format(time.RFC3339)

	row := incidentStore.db.QueryRow(
		`SELECT
			id,
			service,
			severity,
			status,
			first_event_time,
			last_event_time,
			title,
			COALESCE(correlation_pattern, ''),
			COALESCE(correlation_score, 0),
			COALESCE(correlation_reason, ''),
			COALESCE(confidence, 0),
			COALESCE(risk_score, 0),
			COALESCE(event_count, 0),
			COALESCE(root_cause_summary, ''),
			COALESCE(root_cause_type, ''),
			COALESCE(reasoning_json, '[]'),
			COALESCE(what_changed_type, ''),
			COALESCE(what_changed_service, ''),
			COALESCE(what_changed_version, ''),
			COALESCE(what_changed_description, ''),
			COALESCE(what_changed_timestamp, ''),
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			COALESCE(last_seen_at, '')
		 FROM incidents
		 WHERE LOWER(service) = LOWER($1)
		   AND LOWER(correlation_pattern) = LOWER($2)
		   AND last_event_time >= $3
		   AND last_event_time <= $4
		 ORDER BY last_event_time DESC
		 LIMIT 1`,
		service,
		pattern,
		windowStart,
		windowEnd,
	)

	var incident models.Incident
	var reasoningJSON string
	var impactedServicesJSON string

	err := row.Scan(
		&incident.ID,
		&incident.Service,
		&incident.Severity,
		&incident.Status,
		&incident.FirstEventTime,
		&incident.LastEventTime,
		&incident.Title,
		&incident.CorrelationPattern,
		&incident.CorrelationScore,
		&incident.CorrelationReason,
		&incident.Confidence,
		&incident.RiskScore,
		&incident.EventCount,
		&incident.RootCauseSummary,
		&incident.RootCauseType,
		&reasoningJSON,
		&incident.WhatChangedType,
		&incident.WhatChangedService,
		&incident.WhatChangedVersion,
		&incident.WhatChangedDescription,
		&incident.WhatChangedTimestamp,
		&impactedServicesJSON,
		&incident.ImpactCount,
		&incident.SeenBefore,
		&incident.RecurringCount,
		&incident.SimilarIncidentID,
		&incident.LastSeenAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if err := json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning); err != nil {
		incident.Reasoning = []string{}
	}

	if err := json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices); err != nil {
		incident.ImpactedServices = []string{}
	}

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err != nil {
		return nil, err
	}
	incident.EventIDs = eventIDs

	return &incident, nil
}

func (incidentStore *IncidentStore) ListIncidents(filter models.IncidentListFilter) (models.IncidentListResponse, error) {
	whereClause, args := buildIncidentListWhereClause(filter)

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM incidents i
		%s`, whereClause)

	var total int
	if err := incidentStore.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return models.IncidentListResponse{}, err
	}

	orderBy := buildIncidentOrderByClause(filter.SortBy, filter.SortOrder)
	limit := filter.PageSize
	offset := (filter.Page - 1) * filter.PageSize

	listArgs := append([]interface{}{}, args...)
	listArgs = append(listArgs, limit, offset)

	listQuery := fmt.Sprintf(`
		SELECT
			i.id,
			i.service,
			i.severity,
			i.status,
			i.first_event_time,
			i.last_event_time,
			i.title,
			COUNT(ie.event_id) AS event_count,
			COALESCE(i.confidence, 0),
			COALESCE(i.risk_score, 0),
			COALESCE(i.impact_count, 0),
			COALESCE(i.root_cause_summary, ''),
			COALESCE(i.what_changed_type, ''),
			COALESCE(i.seen_before, false),
			COALESCE(i.recurring_count, 0),
			COALESCE(i.similar_incident_id, '')
		FROM incidents i
		LEFT JOIN incident_events ie ON ie.incident_id = i.id
		%s
		GROUP BY i.id
		%s
		LIMIT $%d OFFSET $%d`,
		whereClause,
		orderBy,
		len(args)+1,
		len(args)+2,
	)

	rows, err := incidentStore.db.Query(listQuery, listArgs...)
	if err != nil {
		return models.IncidentListResponse{}, err
	}
	defer rows.Close()

	items := make([]models.IncidentListItem, 0)
	for rows.Next() {
		var item models.IncidentListItem
		if err := rows.Scan(
			&item.ID,
			&item.Service,
			&item.Severity,
			&item.Status,
			&item.FirstEventTime,
			&item.LastEventTime,
			&item.Title,
			&item.EventCount,
			&item.Confidence,
			&item.RiskScore,
			&item.ImpactCount,
			&item.RootCauseSummary,
			&item.WhatChangedType,
			&item.SeenBefore,
			&item.RecurringCount,
			&item.SimilarIncidentID,
		); err != nil {
			return models.IncidentListResponse{}, err
		}

		item.HasWhatChanged = strings.TrimSpace(item.WhatChangedType) != ""
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return models.IncidentListResponse{}, err
	}

	return models.IncidentListResponse{
		Items:    items,
		Page:     filter.Page,
		PageSize: filter.PageSize,
		Total:    total,
		HasMore:  offset+len(items) < total,
	}, nil
}

func buildIncidentListWhereClause(filter models.IncidentListFilter) (string, []interface{}) {
	clauses := make([]string, 0)
	args := make([]interface{}, 0)

	addClause := func(template string, value interface{}) {
		placeholder := fmt.Sprintf(template, len(args)+1)
		clauses = append(clauses, placeholder)
		args = append(args, value)
	}

	if filter.Status != "" {
		addClause("i.status = $%d", filter.Status)
	}

	if filter.Severity != "" {
		addClause("i.severity = $%d", filter.Severity)
	}

	if filter.Service != "" {
		addClause("LOWER(i.service) LIKE $%d", "%"+strings.ToLower(filter.Service)+"%")
	}

	if filter.Search != "" {
		searchValue := "%" + strings.ToLower(filter.Search) + "%"
		clauses = append(
			clauses,
			fmt.Sprintf("(LOWER(i.title) LIKE $%d OR LOWER(i.service) LIKE $%d OR LOWER(i.root_cause_summary) LIKE $%d)", len(args)+1, len(args)+2, len(args)+3),
		)
		args = append(args, searchValue, searchValue, searchValue)
	}

	if filter.From != nil {
		addClause("i.last_event_time >= $%d", filter.From.Format(incidentTimeLayout))
	}

	if filter.To != nil {
		addClause("i.last_event_time <= $%d", filter.To.Format(incidentTimeLayout))
	}

	if len(clauses) == 0 {
		return "", args
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

func buildIncidentOrderByClause(sortBy string, sortOrder string) string {
	validSortFields := map[string]string{
		"last_event_time":  "i.last_event_time",
		"first_event_time": "i.first_event_time",
		"severity":         "i.severity",
		"status":           "i.status",
		"service":          "i.service",
		"title":            "i.title",
		"risk_score":       "i.risk_score",
		"confidence":       "i.confidence",
	}

	orderColumn, found := validSortFields[sortBy]
	if !found {
		orderColumn = "i.last_event_time"
	}

	normalizedSortOrder := strings.ToUpper(sortOrder)
	if normalizedSortOrder != "ASC" && normalizedSortOrder != "DESC" {
		normalizedSortOrder = "DESC"
	}

	return fmt.Sprintf("ORDER BY %s %s", orderColumn, normalizedSortOrder)
}

func IsNotFoundError(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
