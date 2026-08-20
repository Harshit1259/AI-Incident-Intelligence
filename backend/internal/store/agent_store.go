package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// AgentStore provides database access for NeuroOps agents and their metrics.
type AgentStore struct {
	db *sql.DB
}

// NewAgentStore creates a new AgentStore backed by the given database connection.
func NewAgentStore(db *sql.DB) *AgentStore {
	return &AgentStore{db: db}
}

// Register inserts a new agent or updates an existing one (upsert on id).
// Does NOT touch the secret_enc column — use RegisterWithSecret for initial enrollment.
func (s *AgentStore) Register(a models.Agent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	metadataJSON := "{}"
	if a.Metadata != nil {
		b, err := json.Marshal(a.Metadata)
		if err == nil {
			metadataJSON = string(b)
		}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, tenant_id, name, host_ip, os_type, version, status, last_seen_at, registered_at, metadata_json)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			last_seen_at  = EXCLUDED.last_seen_at,
			version       = EXCLUDED.version,
			host_ip       = EXCLUDED.host_ip,
			os_type       = EXCLUDED.os_type,
			name          = EXCLUDED.name,
			metadata_json = EXCLUDED.metadata_json
	`, a.ID, a.TenantID, a.Name, a.HostIP, a.OSType, a.Version, a.Status,
		a.LastSeenAt, a.RegisteredAt, metadataJSON)
	return err
}

// RegisterWithSecret is the enrollment path: inserts or re-enrolls an agent, replacing
// its AES-encrypted signing secret and recording which enrollment token was used.
func (s *AgentStore) RegisterWithSecret(a models.Agent, secretEnc, enrolledVia string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	metadataJSON := "{}"
	if a.Metadata != nil {
		b, err := json.Marshal(a.Metadata)
		if err == nil {
			metadataJSON = string(b)
		}
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents
			(id, tenant_id, name, host_ip, os_type, version, status,
			 last_seen_at, registered_at, metadata_json, secret_enc, enrolled_via, last_rotated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
		ON CONFLICT (id) DO UPDATE SET
			last_seen_at    = EXCLUDED.last_seen_at,
			version         = EXCLUDED.version,
			host_ip         = EXCLUDED.host_ip,
			os_type         = EXCLUDED.os_type,
			name            = EXCLUDED.name,
			metadata_json   = EXCLUDED.metadata_json,
			secret_enc      = EXCLUDED.secret_enc,
			enrolled_via    = EXCLUDED.enrolled_via,
			last_rotated_at = NOW()
	`, a.ID, a.TenantID, a.Name, a.HostIP, a.OSType, a.Version, a.Status,
		a.LastSeenAt, a.RegisteredAt, metadataJSON, secretEnc, enrolledVia)
	return err
}

// GetSecretEnc returns the AES-256-GCM encrypted signing secret for an agent.
// Returns ("", nil) when the agent exists but was never enrolled with a secret.
func (s *AgentStore) GetSecretEnc(agentID string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var secretEnc string
	err := s.db.QueryRowContext(ctx, `SELECT secret_enc FROM agents WHERE id = $1`, agentID).Scan(&secretEnc)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return secretEnc, nil
}

// RotateSecret replaces an agent's encrypted signing secret.
func (s *AgentStore) RotateSecret(agentID, secretEnc string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.db.ExecContext(ctx, `
		UPDATE agents SET secret_enc = $2, last_rotated_at = NOW() WHERE id = $1
	`, agentID, secretEnc)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateLastSeen sets the agent's last_seen_at to now.
// Does NOT change status — user-set status (stopped) must be respected.
func (s *AgentStore) UpdateLastSeen(agentID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE agents SET last_seen_at = NOW()
		WHERE id = $1
	`, agentID)
	return err
}

// GetAgents returns agents for a tenant, ordered by last_seen_at DESC.
// limit <= 0 means use the default (50). Internal callers that need a broad
// scan should pass a large explicit limit (e.g. 500).
func (s *AgentStore) GetAgents(tenantID string, limit, offset int) ([]models.Agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, host_ip, os_type, version, status, last_seen_at, registered_at, metadata_json
		FROM agents
		WHERE tenant_id = $1
		ORDER BY last_seen_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

// GetAgentByID returns a single agent by its ID.
func (s *AgentStore) GetAgentByID(id string) (*models.Agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, host_ip, os_type, version, status, last_seen_at, registered_at, metadata_json
		FROM agents
		WHERE id = $1
	`, id)

	var a models.Agent
	var metaJSON string
	err := row.Scan(&a.ID, &a.TenantID, &a.Name, &a.HostIP, &a.OSType,
		&a.Version, &a.Status, &a.LastSeenAt, &a.RegisteredAt, &metaJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	return &a, nil
}

// GetAgentForTenant returns the agent only if it belongs to tenantID.
// Returns (nil, nil) when the agent does not exist or belongs to a different tenant.
func (s *AgentStore) GetAgentForTenant(id, tenantID string) (*models.Agent, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, host_ip, os_type, version, status, last_seen_at, registered_at, metadata_json
		FROM agents
		WHERE id = $1 AND tenant_id = $2
	`, id, tenantID)

	var a models.Agent
	var metaJSON string
	err := row.Scan(&a.ID, &a.TenantID, &a.Name, &a.HostIP, &a.OSType,
		&a.Version, &a.Status, &a.LastSeenAt, &a.RegisteredAt, &metaJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	return &a, nil
}

// DeleteAgent removes an agent by ID.
func (s *AgentStore) DeleteAgent(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = $1`, id)
	return err
}

// UpdateStatus sets the agent status (active, stopped, inactive, error).
func (s *AgentStore) UpdateStatus(agentID, status string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `UPDATE agents SET status = $2 WHERE id = $1`, agentID, status)
	return err
}

// SaveMetric inserts a metric data point for an agent.
func (s *AgentStore) SaveMetric(agentID, metricType string, ts time.Time, dataJSON string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_metrics (agent_id, metric_type, timestamp, data_json)
		VALUES ($1, $2, $3, $4)
	`, agentID, metricType, ts, dataJSON)
	return err
}

// GetRecentMetrics returns the most recent metric entries for an agent and metric type.
func (s *AgentStore) GetRecentMetrics(agentID, metricType string, limit int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 10
	}

	query := `
		SELECT data_json, timestamp
		FROM agent_metrics
		WHERE agent_id = $1
	`
	args := []interface{}{agentID}
	argIdx := 2

	if metricType != "" {
		query += ` AND metric_type = $` + itoa(argIdx)
		args = append(args, metricType)
		argIdx++
	}

	query += ` ORDER BY timestamp DESC LIMIT $` + itoa(argIdx)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var dataJSON string
		var ts time.Time
		if err := rows.Scan(&dataJSON, &ts); err != nil {
			return nil, err
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
			data = map[string]interface{}{"raw": dataJSON}
		}
		data["timestamp"] = ts.Format(time.RFC3339)
		results = append(results, data)
	}
	return results, rows.Err()
}

// MarkInactive sets agents to inactive if they haven't been seen since the cutoff time.
func (s *AgentStore) MarkInactive(cutoff time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE agents SET status = 'inactive'
		WHERE last_seen_at < $1 AND status = 'active'
	`, cutoff)
	return err
}

// scanAgent scans a single agent row.
func scanAgent(rows *sql.Rows) (models.Agent, error) {
	var a models.Agent
	var metaJSON string
	err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.HostIP, &a.OSType,
		&a.Version, &a.Status, &a.LastSeenAt, &a.RegisteredAt, &metaJSON)
	if err != nil {
		return a, err
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &a.Metadata)
	}
	return a, nil
}

// itoa converts a small int to a string without importing strconv.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
