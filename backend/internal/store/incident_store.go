package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type IncidentStore struct {
	db *sql.DB
}

func NewIncidentStore(db *sql.DB) *IncidentStore {
	return &IncidentStore{db: db}
}

func (incidentStore *IncidentStore) AddIncident(incident models.Incident) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reasoningJSON, err := json.Marshal(incident.Reasoning)
	if err != nil {
		return err
	}

	impactedServicesJSON, err := json.Marshal(incident.ImpactedServices)
	if err != nil {
		return err
	}

	mergedIDsJSON, err := json.Marshal(incident.MergedIncidentIDs)
	if err != nil {
		return err
	}
	if string(mergedIDsJSON) == "null" {
		mergedIDsJSON = []byte("[]")
	}

	tenantID := incident.TenantID
	if tenantID == "" {
		tenantID = "default"
	}

	tx, err := incidentStore.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("incident_store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx, 
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
			last_seen_at,
			fingerprint,
			tenant_id,
			parent_incident_id,
			merged_incident_ids,
			is_merged,
			priority_score
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32, $33
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
		incident.Fingerprint,
		tenantID,
		incident.ParentIncidentID,
		string(mergedIDsJSON),
		incident.IsMerged,
		incident.PriorityScore,
	)
	if err != nil {
		return fmt.Errorf("incident_store: insert incident %s: %w", incident.ID, err)
	}

	for _, eventID := range incident.EventIDs {
		if _, err = tx.ExecContext(ctx, 
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`,
			incident.ID,
			eventID,
		); err != nil {
			return fmt.Errorf("incident_store: link event %s to incident %s: %w", eventID, incident.ID, err)
		}
	}

	return tx.Commit()
}

func (incidentStore *IncidentStore) GetIncidents() ([]models.Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := incidentStore.db.QueryContext(ctx, 
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
			what_changed_timestamp,
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			last_seen_at,
			COALESCE(fingerprint, ''),
			COALESCE(tenant_id, 'default'),
			COALESCE(parent_incident_id, ''),
			COALESCE(merged_incident_ids, '[]'),
			COALESCE(is_merged, false),
			COALESCE(priority_score, 0)
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
		var mergedIDsJSON string
		var wct, lsa sql.NullTime

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
			&wct,
			&impactedServicesJSON,
			&incident.ImpactCount,
			&incident.SeenBefore,
			&incident.RecurringCount,
			&incident.SimilarIncidentID,
			&lsa,
			&incident.Fingerprint,
			&incident.TenantID,
			&incident.ParentIncidentID,
			&mergedIDsJSON,
			&incident.IsMerged,
			&incident.PriorityScore,
		)
		if err != nil {
			return nil, err
		}
		if wct.Valid {
			incident.WhatChangedTimestamp = &wct.Time
		}
		if lsa.Valid {
			incident.LastSeenAt = &lsa.Time
		}

		if err := json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning); err != nil {
			incident.Reasoning = []string{}
		}

		if err := json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices); err != nil {
			incident.ImpactedServices = []string{}
		}

		if err := json.Unmarshal([]byte(mergedIDsJSON), &incident.MergedIncidentIDs); err != nil {
			incident.MergedIncidentIDs = []string{}
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := incidentStore.db.QueryRowContext(ctx, 
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
			what_changed_timestamp,
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			last_seen_at,
			COALESCE(fingerprint, ''),
			COALESCE(tenant_id, 'default'),
			COALESCE(parent_incident_id, ''),
			COALESCE(merged_incident_ids, '[]'),
			COALESCE(is_merged, false),
			COALESCE(priority_score, 0)
		 FROM incidents
		 WHERE id = $1`,
		incidentID,
	)

	var incident models.Incident
	var reasoningJSON string
	var impactedServicesJSON string
	var mergedIDsJSON string
	var wct, lsa sql.NullTime

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
		&wct,
		&impactedServicesJSON,
		&incident.ImpactCount,
		&incident.SeenBefore,
		&incident.RecurringCount,
		&incident.SimilarIncidentID,
		&lsa,
		&incident.Fingerprint,
		&incident.TenantID,
		&incident.ParentIncidentID,
		&mergedIDsJSON,
		&incident.IsMerged,
		&incident.PriorityScore,
	)
	if err != nil {
		return models.Incident{}, false
	}
	if wct.Valid {
		incident.WhatChangedTimestamp = &wct.Time
	}
	if lsa.Valid {
		incident.LastSeenAt = &lsa.Time
	}

	if err := json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning); err != nil {
		incident.Reasoning = []string{}
	}

	if err := json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices); err != nil {
		incident.ImpactedServices = []string{}
	}

	if err := json.Unmarshal([]byte(mergedIDsJSON), &incident.MergedIncidentIDs); err != nil {
		incident.MergedIncidentIDs = []string{}
	}

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err != nil {
		return models.Incident{}, false
	}
	incident.EventIDs = eventIDs

	return incident, true
}

func (incidentStore *IncidentStore) GetEventIDsByIncidentID(incidentID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := incidentStore.db.QueryContext(ctx, 
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reasoningJSON, err := json.Marshal(updatedIncident.Reasoning)
	if err != nil {
		return err
	}

	impactedServicesJSON, err := json.Marshal(updatedIncident.ImpactedServices)
	if err != nil {
		return err
	}

	mergedIDsJSON, err := json.Marshal(updatedIncident.MergedIncidentIDs)
	if err != nil {
		return err
	}
	if string(mergedIDsJSON) == "null" {
		mergedIDsJSON = []byte("[]")
	}

	tx, err := incidentStore.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("incident_store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.ExecContext(ctx, 
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
		     last_seen_at = $27,
		     fingerprint = $28,
		     parent_incident_id = $29,
		     merged_incident_ids = $30,
		     is_merged = $31,
		     priority_score = $32
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
		updatedIncident.Fingerprint,
		updatedIncident.ParentIncidentID,
		string(mergedIDsJSON),
		updatedIncident.IsMerged,
		updatedIncident.PriorityScore,
	)
	if err != nil {
		return fmt.Errorf("incident_store: update incident %s: %w", updatedIncident.ID, err)
	}

	if _, err = tx.ExecContext(ctx, 
		`DELETE FROM incident_events WHERE incident_id = $1`,
		updatedIncident.ID,
	); err != nil {
		return fmt.Errorf("incident_store: delete events for incident %s: %w", updatedIncident.ID, err)
	}

	for _, eventID := range updatedIncident.EventIDs {
		if _, err = tx.ExecContext(ctx, 
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2)`,
			updatedIncident.ID,
			eventID,
		); err != nil {
			return fmt.Errorf("incident_store: link event %s to incident %s: %w", eventID, updatedIncident.ID, err)
		}
	}

	return tx.Commit()
}

func (incidentStore *IncidentStore) UpdateIncidentStatus(incidentID string, status string) (models.Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := incidentStore.db.ExecContext(ctx, 
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	windowStart := incidentTime.Add(-7 * 24 * time.Hour)
	windowEnd := incidentTime.Add(-1 * time.Minute)

	row := incidentStore.db.QueryRowContext(ctx, 
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
			what_changed_timestamp,
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			last_seen_at,
			COALESCE(fingerprint, ''),
			COALESCE(tenant_id, 'default'),
			COALESCE(parent_incident_id, ''),
			COALESCE(merged_incident_ids, '[]'),
			COALESCE(is_merged, false),
			COALESCE(priority_score, 0)
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
	var mergedIDsJSON string
	var wct, lsa sql.NullTime

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
		&wct,
		&impactedServicesJSON,
		&incident.ImpactCount,
		&incident.SeenBefore,
		&incident.RecurringCount,
		&incident.SimilarIncidentID,
		&lsa,
		&incident.Fingerprint,
		&incident.TenantID,
		&incident.ParentIncidentID,
		&mergedIDsJSON,
		&incident.IsMerged,
		&incident.PriorityScore,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if wct.Valid {
		incident.WhatChangedTimestamp = &wct.Time
	}
	if lsa.Valid {
		incident.LastSeenAt = &lsa.Time
	}

	if err := json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning); err != nil {
		incident.Reasoning = []string{}
	}

	if err := json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices); err != nil {
		incident.ImpactedServices = []string{}
	}

	if err := json.Unmarshal([]byte(mergedIDsJSON), &incident.MergedIncidentIDs); err != nil {
		incident.MergedIncidentIDs = []string{}
	}

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err != nil {
		return nil, err
	}
	incident.EventIDs = eventIDs

	return &incident, nil
}

func (incidentStore *IncidentStore) ListIncidents(filter models.IncidentListFilter) (models.IncidentListResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	whereClause, args := buildIncidentListWhereClause(filter)

	countQuery := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM incidents i
		%s`, whereClause)

	var total int
	if err := incidentStore.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
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
			COALESCE(i.similar_incident_id, ''),
			COALESCE(i.priority_score, 0)
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

	rows, err := incidentStore.db.QueryContext(ctx, listQuery, listArgs...)
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
			&item.PriorityScore,
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

	// Tenant scoping is always applied — it is the primary isolation boundary.
	tenantID := filter.TenantID
	if tenantID == "" {
		tenantID = "default"
	}
	addClause("COALESCE(i.tenant_id, 'default') = $%d", tenantID)

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
		addClause("i.last_event_time >= $%d", *filter.From)
	}

	if filter.To != nil {
		addClause("i.last_event_time <= $%d", *filter.To)
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
		"priority_score":   "i.priority_score",
	}

	orderColumn, found := validSortFields[sortBy]
	if !found {
		orderColumn = "i.priority_score DESC, i.last_event_time"
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

// FindOpenIncidentForService returns the most-recently-active open incident
// for the given service whose last_event_time falls within the correlation
// window [windowStart, now). Returns nil when no match is found.
func (incidentStore *IncidentStore) FindOpenIncidentForService(service string, windowStart time.Time) *models.Incident {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := incidentStore.db.QueryRowContext(ctx, 
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
			what_changed_timestamp,
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			last_seen_at,
			COALESCE(fingerprint, ''),
			COALESCE(tenant_id, 'default'),
			COALESCE(parent_incident_id, ''),
			COALESCE(merged_incident_ids, '[]'),
			COALESCE(is_merged, false),
			COALESCE(priority_score, 0)
		 FROM incidents
		 WHERE LOWER(service) = LOWER($1)
		   AND status IN ('open', 'acknowledged')
		   AND last_event_time >= $2
		 ORDER BY last_event_time DESC
		 LIMIT 1`,
		service,
		windowStart,
	)

	var incident models.Incident
	var reasoningJSON, impactedServicesJSON, mergedIDsJSON string
	var wct, lsa sql.NullTime

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
		&wct,
		&impactedServicesJSON,
		&incident.ImpactCount,
		&incident.SeenBefore,
		&incident.RecurringCount,
		&incident.SimilarIncidentID,
		&lsa,
		&incident.Fingerprint,
		&incident.TenantID,
		&incident.ParentIncidentID,
		&mergedIDsJSON,
		&incident.IsMerged,
		&incident.PriorityScore,
	)
	if err != nil {
		return nil
	}
	if wct.Valid {
		incident.WhatChangedTimestamp = &wct.Time
	}
	if lsa.Valid {
		incident.LastSeenAt = &lsa.Time
	}

	_ = json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning)
	_ = json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices)
	_ = json.Unmarshal([]byte(mergedIDsJSON), &incident.MergedIncidentIDs)

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err == nil {
		incident.EventIDs = eventIDs
	}

	return &incident
}

// FindOpenIncidentByFingerprint returns ANY open/acknowledged incident with the
// same fingerprint, regardless of time window. This ensures recurring anomalies
// (e.g., disk alerts every 30 min) merge into one incident instead of creating duplicates.
func (incidentStore *IncidentStore) FindOpenIncidentByFingerprint(fingerprint string) *models.Incident {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if fingerprint == "" {
		return nil
	}
	row := incidentStore.db.QueryRowContext(ctx, 
		`SELECT id FROM incidents
		 WHERE fingerprint = $1
		   AND status IN ('open', 'acknowledged')
		 ORDER BY last_event_time DESC
		 LIMIT 1`,
		fingerprint,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		return nil
	}
	inc, found := incidentStore.GetIncidentByID(id)
	if !found {
		return nil
	}
	return &inc
}

// FindOpenIncidentForServices returns the most recent open incident for any
// of the given services within the correlation window.
func (incidentStore *IncidentStore) FindOpenIncidentForServices(services []string, windowStart time.Time) *models.Incident {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(services) == 0 {
		return nil
	}
	// Build placeholders: $1, $2, ... $N and last param for windowStart
	placeholders := make([]string, len(services))
	args := make([]interface{}, len(services)+1)
	for i, svc := range services {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = strings.ToLower(svc)
	}
	args[len(services)] = windowStart

	query := fmt.Sprintf(`SELECT
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
			what_changed_timestamp,
			COALESCE(impacted_services_json, '[]'),
			COALESCE(impact_count, 0),
			COALESCE(seen_before, false),
			COALESCE(recurring_count, 0),
			COALESCE(similar_incident_id, ''),
			last_seen_at,
			COALESCE(fingerprint, ''),
			COALESCE(tenant_id, 'default'),
			COALESCE(parent_incident_id, ''),
			COALESCE(merged_incident_ids, '[]'),
			COALESCE(is_merged, false),
			COALESCE(priority_score, 0)
		 FROM incidents
		 WHERE LOWER(service) IN (%s)
		   AND status IN ('open', 'acknowledged')
		   AND last_event_time >= $%d
		 ORDER BY last_event_time DESC
		 LIMIT 1`, strings.Join(placeholders, ","), len(services)+1)

	row := incidentStore.db.QueryRowContext(ctx, query, args...)

	var incident models.Incident
	var reasoningJSON, impactedServicesJSON, mergedIDsJSON string
	var wct, lsa sql.NullTime

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
		&wct,
		&impactedServicesJSON,
		&incident.ImpactCount,
		&incident.SeenBefore,
		&incident.RecurringCount,
		&incident.SimilarIncidentID,
		&lsa,
		&incident.Fingerprint,
		&incident.TenantID,
		&incident.ParentIncidentID,
		&mergedIDsJSON,
		&incident.IsMerged,
		&incident.PriorityScore,
	)
	if err != nil {
		return nil
	}
	if wct.Valid {
		incident.WhatChangedTimestamp = &wct.Time
	}
	if lsa.Valid {
		incident.LastSeenAt = &lsa.Time
	}

	_ = json.Unmarshal([]byte(reasoningJSON), &incident.Reasoning)
	_ = json.Unmarshal([]byte(impactedServicesJSON), &incident.ImpactedServices)
	_ = json.Unmarshal([]byte(mergedIDsJSON), &incident.MergedIncidentIDs)

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(incident.ID)
	if err == nil {
		incident.EventIDs = eventIDs
	}

	return &incident
}

// MoveEventsToIncident reassigns all events from one incident to another.
func (incidentStore *IncidentStore) MoveEventsToIncident(fromID, toID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eventIDs, err := incidentStore.GetEventIDsByIncidentID(fromID)
	if err != nil {
		return err
	}
	if len(eventIDs) == 0 {
		return nil
	}

	tx, err := incidentStore.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("incident_store: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	for _, eid := range eventIDs {
		if _, err = tx.ExecContext(ctx, 
			`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			toID, eid,
		); err != nil {
			return fmt.Errorf("incident_store: move event %s to incident %s: %w", eid, toID, err)
		}
	}

	return tx.Commit()
}

// AddEventToIncident links an event to an existing incident.
func (incidentStore *IncidentStore) AddEventToIncident(incidentID, eventID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := incidentStore.db.ExecContext(ctx, 
		`INSERT INTO incident_events (incident_id, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		incidentID, eventID,
	)
	return err
}

// GetDiscoveredServices returns all unique services from incidents for a tenant.
func (incidentStore *IncidentStore) GetDiscoveredServices(tenantID string) ([]models.DiscoveredService, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := incidentStore.db.QueryContext(ctx, 
		`SELECT DISTINCT service,
		        COUNT(*) as incident_count,
		        MAX(severity) as max_severity,
		        MAX(last_event_time) as last_incident
		 FROM incidents
		 WHERE tenant_id = $1
		 GROUP BY service
		 ORDER BY incident_count DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []models.DiscoveredService
	for rows.Next() {
		var ds models.DiscoveredService
		if err := rows.Scan(&ds.Service, &ds.IncidentCount, &ds.MaxSeverity, &ds.LastIncident); err != nil {
			return nil, err
		}
		ds.SuggestedTier = suggestTier(ds.Service)
		services = append(services, ds)
	}
	return services, rows.Err()
}

// CountSimilarByService returns the number of other incidents for the same
// service, giving a real "how many times has this happened" count that is
// independent of the seen_before flag stored at creation time.
func (s *IncidentStore) CountSimilarByService(service, excludeID string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if strings.TrimSpace(service) == "" {
		return 0
	}
	var count int
	err := s.db.QueryRowContext(ctx, 
		`SELECT COUNT(*) FROM incidents WHERE service = $1 AND id != $2`,
		service, excludeID,
	).Scan(&count)
	if err != nil {
		return 0
	}
	return count
}

func suggestTier(service string) string {
	svc := strings.ToLower(service)
	switch {
	case strings.Contains(svc, "payment") || strings.Contains(svc, "checkout") || strings.Contains(svc, "order"):
		return "TIER_0"
	case strings.Contains(svc, "api") || strings.Contains(svc, "service") || strings.Contains(svc, "gateway"):
		return "TIER_1"
	case strings.Contains(svc, "worker") || strings.Contains(svc, "cron") || strings.Contains(svc, "batch"):
		return "TIER_3"
	default:
		return "TIER_2"
	}
}

// CountIncidentsInPeriod returns the total number of incidents created for a tenant
// since the given time — the "after correlation" figure for noise dashboards.
func (s *IncidentStore) CountIncidentsInPeriod(tenantID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM incidents
		WHERE tenant_id = $1 AND first_event_time >= $2
	`, tenantID, since).Scan(&count)
	return count, err
}

// CountConfirmedIncidents returns incidents that were acknowledged or resolved
// by a human — the "true incidents" figure (excludes incidents that stayed open
// without any human interaction, which may indicate auto-noise-resolved ones).
func (s *IncidentStore) CountConfirmedIncidents(tenantID string, since time.Time) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM incidents
		WHERE tenant_id = $1
		  AND first_event_time >= $2
		  AND status IN ('acknowledged', 'resolved', 'investigating')
	`, tenantID, since).Scan(&count)
	return count, err
}

// ListOpenIncidentsByTenant returns all open/acknowledged incidents for a tenant.
// Used by the risk exposure service for live aggregate dashboards.
func (s *IncidentStore) ListOpenIncidentsByTenant(tenantID string) ([]models.Incident, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, service, severity, status,
		       COALESCE(fingerprint,''), COALESCE(risk_score,0), COALESCE(impact_count,0),
		       first_event_time, last_event_time,
		       COALESCE(what_changed_type,''), COALESCE(impacted_services_json,'[]')
		FROM incidents
		WHERE tenant_id = $1 AND status IN ('open','acknowledged')
		ORDER BY first_event_time DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		var inc models.Incident
		var impactedJSON string
		if err := rows.Scan(
			&inc.ID, &inc.Service, &inc.Severity, &inc.Status,
			&inc.Fingerprint, &inc.RiskScore, &inc.ImpactCount,
			&inc.FirstEventTime, &inc.LastEventTime,
			&inc.WhatChangedType, &impactedJSON,
		); err != nil {
			return nil, err
		}
		if err2 := json.Unmarshal([]byte(impactedJSON), &inc.ImpactedServices); err2 != nil {
			inc.ImpactedServices = []string{}
		}
		incidents = append(incidents, inc)
	}
	return incidents, rows.Err()
}

// GetStatusCounts returns the count of incidents per status for a tenant.
func (incidentStore *IncidentStore) GetStatusCounts(tenantID string) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := incidentStore.db.QueryContext(ctx,
		`SELECT COALESCE(status, 'open'), COUNT(*)
		 FROM incidents
		 WHERE COALESCE(tenant_id, 'default') = $1
		 GROUP BY status`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{"open": 0, "acknowledged": 0, "resolved": 0}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}
