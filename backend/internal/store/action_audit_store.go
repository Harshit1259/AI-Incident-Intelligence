package store

import "database/sql"

type ActionAudit struct {
	ID         int    `json:"id"`
	ActionID   string `json:"action_id"`
	IncidentID string `json:"incident_id"`
	Approved   bool   `json:"approved"`
	Status     string `json:"status"`
	Message    string `json:"message"`
	ExecutedAt string `json:"executed_at"`
}

type ActionAuditStore struct {
	db *sql.DB
}

func NewActionAuditStore(db *sql.DB) *ActionAuditStore {
	return &ActionAuditStore{db: db}
}

func (actionAuditStore *ActionAuditStore) AddAudit(audit ActionAudit) error {
	_, err := actionAuditStore.db.Exec(
		`INSERT INTO action_audit (
			action_id,
			incident_id,
			approved,
			status,
			message,
			executed_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		audit.ActionID,
		audit.IncidentID,
		audit.Approved,
		audit.Status,
		audit.Message,
		audit.ExecutedAt,
	)

	return err
}

func (actionAuditStore *ActionAuditStore) GetAuditsByIncident(incidentID string) []ActionAudit {
	rows, err := actionAuditStore.db.Query(
		`SELECT
			id,
			action_id,
			incident_id,
			approved,
			status,
			message,
			executed_at
		 FROM action_audit
		 WHERE incident_id = $1
		 ORDER BY executed_at DESC`,
		incidentID,
	)
	if err != nil {
		return []ActionAudit{}
	}
	defer rows.Close()

	result := make([]ActionAudit, 0)

	for rows.Next() {
		var audit ActionAudit
		if err := rows.Scan(
			&audit.ID,
			&audit.ActionID,
			&audit.IncidentID,
			&audit.Approved,
			&audit.Status,
			&audit.Message,
			&audit.ExecutedAt,
		); err != nil {
			return result
		}

		result = append(result, audit)
	}

	return result
}
