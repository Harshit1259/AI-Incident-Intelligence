package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"github.com/lib/pq"
)

// TopologyStore manages persistent topology nodes, edges, and health snapshots.
// Graph traversal (blast radius, upstream propagation) uses WITH RECURSIVE CTEs
// so the DB does the heavy work rather than the application.
type TopologyStore struct {
	db *sql.DB
}

func NewTopologyStore(db *sql.DB) *TopologyStore {
	return &TopologyStore{db: db}
}

// ─── Nodes ────────────────────────────────────────────────────────────────────

// UpsertNode creates or updates a topology node.
// The node ID is caller-supplied and must be stable (e.g. "{tenantID}:{name}").
func (s *TopologyStore) UpsertNode(n models.TopologyNode) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	meta := "{}"
	if len(n.Metadata) > 0 {
		b, _ := json.Marshal(n.Metadata)
		meta = string(b)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO topology_nodes
		    (id, tenant_id, node_type, name, display_name, tier, owner_team,
		     is_customer_facing, version, environment, metadata_json, auto_discovered, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, NOW())
		ON CONFLICT (tenant_id, name)
		DO UPDATE SET
		    node_type          = EXCLUDED.node_type,
		    display_name       = EXCLUDED.display_name,
		    tier               = EXCLUDED.tier,
		    owner_team         = EXCLUDED.owner_team,
		    is_customer_facing = EXCLUDED.is_customer_facing,
		    version            = EXCLUDED.version,
		    environment        = EXCLUDED.environment,
		    metadata_json      = EXCLUDED.metadata_json,
		    auto_discovered    = EXCLUDED.auto_discovered,
		    updated_at         = NOW()`,
		n.ID, n.TenantID, n.NodeType, n.Name, n.DisplayName, n.Tier, n.OwnerTeam,
		n.IsCustomerFacing, n.Version, n.Environment, meta, n.AutoDiscovered,
	)
	return err
}

// GetNodes returns all nodes for a tenant with their latest health snapshot joined.
func (s *TopologyStore) GetNodes(tenantID string) ([]models.TopologyNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.tenant_id, n.node_type, n.name, n.display_name,
		       n.tier, n.owner_team, n.is_customer_facing, n.version, n.environment,
		       n.metadata_json, n.auto_discovered, n.created_at, n.updated_at,
		       h.status, h.cpu_pct, h.mem_pct, h.error_rate_pct, h.latency_ms_p99, h.recorded_at
		FROM topology_nodes n
		LEFT JOIN LATERAL (
		    SELECT status, cpu_pct, mem_pct, error_rate_pct, latency_ms_p99, recorded_at
		    FROM topology_node_health
		    WHERE node_id = n.id
		    ORDER BY recorded_at DESC
		    LIMIT 1
		) h ON true
		WHERE n.tenant_id = $1
		ORDER BY n.node_type, n.name ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNodes(rows)
}

// GetNodeByName returns a single node matched by tenant + name.
func (s *TopologyStore) GetNodeByName(tenantID, name string) (*models.TopologyNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT n.id, n.tenant_id, n.node_type, n.name, n.display_name,
		       n.tier, n.owner_team, n.is_customer_facing, n.version, n.environment,
		       n.metadata_json, n.auto_discovered, n.created_at, n.updated_at,
		       h.status, h.cpu_pct, h.mem_pct, h.error_rate_pct, h.latency_ms_p99, h.recorded_at
		FROM topology_nodes n
		LEFT JOIN LATERAL (
		    SELECT status, cpu_pct, mem_pct, error_rate_pct, latency_ms_p99, recorded_at
		    FROM topology_node_health
		    WHERE node_id = n.id
		    ORDER BY recorded_at DESC
		    LIMIT 1
		) h ON true
		WHERE n.tenant_id = $1 AND n.name = $2`,
		tenantID, name,
	)
	nodes, err := scanNodeRows(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return nodes, nil
}

// GetNodeByID returns a single node matched by primary key.
// Prefer GetNodeByIDForTenant on tenant-scoped API paths to prevent
// cross-tenant reads when node IDs are guessable.
func (s *TopologyStore) GetNodeByID(id string) (*models.TopologyNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT n.id, n.tenant_id, n.node_type, n.name, n.display_name,
		       n.tier, n.owner_team, n.is_customer_facing, n.version, n.environment,
		       n.metadata_json, n.auto_discovered, n.created_at, n.updated_at,
		       h.status, h.cpu_pct, h.mem_pct, h.error_rate_pct, h.latency_ms_p99, h.recorded_at
		FROM topology_nodes n
		LEFT JOIN LATERAL (
		    SELECT status, cpu_pct, mem_pct, error_rate_pct, latency_ms_p99, recorded_at
		    FROM topology_node_health
		    WHERE node_id = n.id
		    ORDER BY recorded_at DESC
		    LIMIT 1
		) h ON true
		WHERE n.id = $1`, id,
	)
	node, err := scanNodeRows(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return node, err
}

// GetNodeByIDForTenant returns a node only if it belongs to the given tenant.
// Returns nil, nil when the node exists but belongs to a different tenant.
func (s *TopologyStore) GetNodeByIDForTenant(id, tenantID string) (*models.TopologyNode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT n.id, n.tenant_id, n.node_type, n.name, n.display_name,
		       n.tier, n.owner_team, n.is_customer_facing, n.version, n.environment,
		       n.metadata_json, n.auto_discovered, n.created_at, n.updated_at,
		       h.status, h.cpu_pct, h.mem_pct, h.error_rate_pct, h.latency_ms_p99, h.recorded_at
		FROM topology_nodes n
		LEFT JOIN LATERAL (
		    SELECT status, cpu_pct, mem_pct, error_rate_pct, latency_ms_p99, recorded_at
		    FROM topology_node_health
		    WHERE node_id = n.id
		    ORDER BY recorded_at DESC
		    LIMIT 1
		) h ON true
		WHERE n.id = $1 AND n.tenant_id = $2`, id, tenantID,
	)
	node, err := scanNodeRows(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return node, err
}

// DeleteNode removes a node and all its edges (via FK cascade).
func (s *TopologyStore) DeleteNode(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM topology_nodes WHERE id = $1`, id)
	return err
}

// ─── Edges ────────────────────────────────────────────────────────────────────

// UpsertEdge creates or updates a topology edge.
func (s *TopologyStore) UpsertEdge(e models.TopologyEdge) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO topology_edges
		    (id, tenant_id, from_node_id, to_node_id, relation,
		     weight, confidence, propagation_factor, latency_ms_p99, error_rate_pct, auto_discovered, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, NOW())
		ON CONFLICT (tenant_id, from_node_id, to_node_id, relation)
		DO UPDATE SET
		    weight             = EXCLUDED.weight,
		    confidence         = EXCLUDED.confidence,
		    propagation_factor = EXCLUDED.propagation_factor,
		    latency_ms_p99     = EXCLUDED.latency_ms_p99,
		    error_rate_pct     = EXCLUDED.error_rate_pct,
		    auto_discovered    = EXCLUDED.auto_discovered,
		    updated_at         = NOW()`,
		e.ID, e.TenantID, e.FromNodeID, e.ToNodeID, e.Relation,
		e.Weight, e.Confidence, e.PropagationFactor, e.LatencyMsP99, e.ErrorRatePct, e.AutoDiscovered,
	)
	return err
}

// GetEdges returns all edges for a tenant with denormalized node names.
func (s *TopologyStore) GetEdges(tenantID string) ([]models.TopologyEdge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.tenant_id, e.from_node_id, e.to_node_id, e.relation,
		       e.weight, e.confidence, e.propagation_factor, e.latency_ms_p99, e.error_rate_pct,
		       e.auto_discovered, e.created_at, e.updated_at,
		       fn.name AS from_name, tn.name AS to_name
		FROM topology_edges e
		JOIN topology_nodes fn ON fn.id = e.from_node_id
		JOIN topology_nodes tn ON tn.id = e.to_node_id
		WHERE e.tenant_id = $1
		ORDER BY fn.name, tn.name ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdges(rows)
}

// DeleteEdge removes an edge by ID.
func (s *TopologyStore) DeleteEdge(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM topology_edges WHERE id = $1`, id)
	return err
}

// ─── Graph Traversal ─────────────────────────────────────────────────────────

// GetDownstream performs a breadth-first traversal of all nodes reachable
// downstream from nodeID up to maxDepth hops. Confidence decays by the edge's
// propagation_factor at each hop. Cycles are prevented via path-array exclusion.
func (s *TopologyStore) GetDownstream(nodeID, tenantID string, maxDepth int) ([]models.TraversalHop, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE downstream AS (
		    SELECT
		        e.to_node_id                                  AS node_id,
		        1                                             AS depth,
		        CAST(e.confidence AS REAL)                    AS propagated_conf,
		        e.relation                                    AS edge_relation,
		        e.propagation_factor                          AS prop_factor,
		        ARRAY[e.from_node_id, e.to_node_id]          AS path
		    FROM topology_edges e
		    WHERE e.from_node_id = $1 AND e.tenant_id = $2

		    UNION ALL

		    SELECT
		        e.to_node_id,
		        d.depth + 1,
		        d.propagated_conf * e.propagation_factor,
		        e.relation,
		        e.propagation_factor,
		        d.path || e.to_node_id
		    FROM topology_edges e
		    JOIN downstream d ON d.node_id = e.from_node_id
		    WHERE e.tenant_id = $2
		      AND d.depth < $3
		      AND NOT (e.to_node_id = ANY(d.path))
		)
		SELECT DISTINCT ON (node_id)
		    node_id,
		    depth,
		    CAST(propagated_conf AS INTEGER),
		    edge_relation,
		    path
		FROM downstream
		ORDER BY node_id, depth ASC`,
		nodeID, tenantID, maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTraversalHops(rows)
}

// GetUpstream performs a breadth-first traversal of all nodes that reach nodeID
// through upstream edges (reverse direction).
func (s *TopologyStore) GetUpstream(nodeID, tenantID string, maxDepth int) ([]models.TraversalHop, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		WITH RECURSIVE upstream AS (
		    SELECT
		        e.from_node_id                                AS node_id,
		        1                                             AS depth,
		        CAST(e.confidence AS REAL)                    AS propagated_conf,
		        e.relation                                    AS edge_relation,
		        e.propagation_factor                          AS prop_factor,
		        ARRAY[e.to_node_id, e.from_node_id]          AS path
		    FROM topology_edges e
		    WHERE e.to_node_id = $1 AND e.tenant_id = $2

		    UNION ALL

		    SELECT
		        e.from_node_id,
		        u.depth + 1,
		        u.propagated_conf * e.propagation_factor,
		        e.relation,
		        e.propagation_factor,
		        u.path || e.from_node_id
		    FROM topology_edges e
		    JOIN upstream u ON u.node_id = e.to_node_id
		    WHERE e.tenant_id = $2
		      AND u.depth < $3
		      AND NOT (e.from_node_id = ANY(u.path))
		)
		SELECT DISTINCT ON (node_id)
		    node_id,
		    depth,
		    CAST(propagated_conf AS INTEGER),
		    edge_relation,
		    path
		FROM upstream
		ORDER BY node_id, depth ASC`,
		nodeID, tenantID, maxDepth,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTraversalHops(rows)
}

// ─── Health ───────────────────────────────────────────────────────────────────

// RecordHealth inserts a health snapshot for a node and trims old rows (keep 100).
func (s *TopologyStore) RecordHealth(h models.NodeHealthInput, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO topology_node_health (node_id, tenant_id, status, cpu_pct, mem_pct, error_rate_pct, latency_ms_p99)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		h.NodeID, tenantID, h.Status, h.CPUPct, h.MemPct, h.ErrorRatePct, h.LatencyMsP99,
	)
	if err != nil {
		return err
	}
	// Trim: keep only the 100 most recent rows per node.
	_, _ = s.db.ExecContext(ctx, `
		DELETE FROM topology_node_health
		WHERE node_id = $1
		  AND id NOT IN (
		      SELECT id FROM topology_node_health
		      WHERE node_id = $1
		      ORDER BY recorded_at DESC
		      LIMIT 100
		  )`, h.NodeID,
	)
	return nil
}

// ─── Auto-discovery ──────────────────────────────────────────────────────────

// AutoDiscoverFromIncidents mines the incidents table to create topology_nodes
// for every distinct service seen. It also creates depends_on edges using the
// impacted_services_json field as a proxy for call relationships.
// Returns the number of new or updated nodes created.
func (s *TopologyStore) AutoDiscoverFromIncidents(tenantID string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Fetch distinct services with their incident stats.
	rows, err := s.db.QueryContext(ctx, `
		SELECT service,
		       COUNT(*)         AS incident_count,
		       MAX(severity)    AS max_severity,
		       MAX(last_event_time) AS last_incident,
		       STRING_AGG(DISTINCT impacted_services_json, ',') AS all_impacted
		FROM incidents
		WHERE tenant_id = $1 AND service <> ''
		GROUP BY service`, tenantID,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type svcRow struct {
		name        string
		count       int
		maxSev      string
		lastSeen    string
		allImpacted string
	}
	var services []svcRow
	for rows.Next() {
		var r svcRow
		if err := rows.Scan(&r.name, &r.count, &r.maxSev, &r.lastSeen, &r.allImpacted); err != nil {
			continue
		}
		services = append(services, r)
	}

	created := 0
	for _, svc := range services {
		nodeID := fmt.Sprintf("%s:%s", tenantID, svc.name)
		tier := autoTier(svc.name)
		n := models.TopologyNode{
			ID:             nodeID,
			TenantID:       tenantID,
			NodeType:       "service",
			Name:           svc.name,
			DisplayName:    humanizeLabel(svc.name),
			Tier:           tier,
			Environment:    "production",
			AutoDiscovered: true,
			UpdatedAt:      time.Now(),
		}
		if err := s.UpsertNode(n); err == nil {
			created++
		}

		// Parse impacted services and create edges.
		// The JSON is a concatenation of per-incident arrays; extract unique service names.
		impacted := parseImpactedJSON(svc.allImpacted)
		for _, imp := range impacted {
			if imp == "" || imp == svc.name {
				continue
			}
			impNodeID := fmt.Sprintf("%s:%s", tenantID, imp)
			// Ensure the impacted node exists.
			impN := models.TopologyNode{
				ID: impNodeID, TenantID: tenantID, NodeType: "service",
				Name: imp, DisplayName: humanizeLabel(imp),
				Tier: autoTier(imp), Environment: "production", AutoDiscovered: true,
			}
			_ = s.UpsertNode(impN)

			edgeID := fmt.Sprintf("%s:%s->%s:impacts", tenantID, svc.name, imp)
			edge := models.TopologyEdge{
				ID: edgeID, TenantID: tenantID,
				FromNodeID: nodeID, ToNodeID: impNodeID,
				Relation: "impacts", Weight: 0.7, Confidence: 60,
				PropagationFactor: 0.6, AutoDiscovered: true,
			}
			_ = s.UpsertEdge(edge)
		}
	}

	return created, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

func scanNodes(rows *sql.Rows) ([]models.TopologyNode, error) {
	var result []models.TopologyNode
	for rows.Next() {
		n, err := scanNodeFromRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *n)
	}
	return result, rows.Err()
}

// scanNodeRows scans a single *sql.Row (from QueryRow).
func scanNodeRows(row *sql.Row) (*models.TopologyNode, error) {
	return scanNodeFromScanner(row)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNodeFromRow(s scanner) (*models.TopologyNode, error) {
	return scanNodeFromScanner(s)
}

func scanNodeFromScanner(s scanner) (*models.TopologyNode, error) {
	var n models.TopologyNode
	var metaJSON string
	var hStatus, hCPU, hMem, hErr, hLat, hAt sql.NullString
	err := s.Scan(
		&n.ID, &n.TenantID, &n.NodeType, &n.Name, &n.DisplayName,
		&n.Tier, &n.OwnerTeam, &n.IsCustomerFacing, &n.Version, &n.Environment,
		&metaJSON, &n.AutoDiscovered, &n.CreatedAt, &n.UpdatedAt,
		&hStatus, &hCPU, &hMem, &hErr, &hLat, &hAt,
	)
	if err != nil {
		return nil, err
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &n.Metadata)
	}
	if hStatus.Valid {
		snap := &models.NodeHealthSnap{Status: hStatus.String}
		fmt.Sscanf(hCPU.String, "%f", &snap.CPUPct)
		fmt.Sscanf(hMem.String, "%f", &snap.MemPct)
		fmt.Sscanf(hErr.String, "%f", &snap.ErrorRatePct)
		fmt.Sscanf(hLat.String, "%f", &snap.LatencyMsP99)
		if hAt.Valid {
			snap.RecordedAt, _ = time.Parse(time.RFC3339, hAt.String)
		}
		n.Health = snap
	}
	return &n, nil
}

func scanEdges(rows *sql.Rows) ([]models.TopologyEdge, error) {
	var result []models.TopologyEdge
	for rows.Next() {
		var e models.TopologyEdge
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.FromNodeID, &e.ToNodeID, &e.Relation,
			&e.Weight, &e.Confidence, &e.PropagationFactor, &e.LatencyMsP99, &e.ErrorRatePct,
			&e.AutoDiscovered, &e.CreatedAt, &e.UpdatedAt,
			&e.FromNodeName, &e.ToNodeName,
		); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func scanTraversalHops(rows *sql.Rows) ([]models.TraversalHop, error) {
	var result []models.TraversalHop
	for rows.Next() {
		var h models.TraversalHop
		var path pq.StringArray
		if err := rows.Scan(&h.NodeID, &h.Depth, &h.PropagatedConfidence, &h.EdgeRelation, &path); err != nil {
			return nil, err
		}
		h.Path = []string(path)
		result = append(result, h)
	}
	return result, rows.Err()
}

// ─── Discovery helpers ────────────────────────────────────────────────────────

func autoTier(name string) string {
	switch {
	case containsAny(name, "payment", "checkout", "order", "billing"):
		return "critical"
	case containsAny(name, "api", "gateway", "auth", "frontend"):
		return "critical"
	case containsAny(name, "worker", "cron", "batch", "queue"):
		return "internal"
	case containsAny(name, "db", "database", "postgres", "mysql", "redis", "cache"):
		return "infra"
	default:
		return "internal"
	}
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if len(s) >= len(p) {
			for i := 0; i <= len(s)-len(p); i++ {
				if s[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}

func humanizeLabel(id string) string {
	out := make([]byte, 0, len(id))
	capitalizeNext := true
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c == '-' || c == '_' || c == '.' {
			out = append(out, ' ')
			capitalizeNext = true
		} else if capitalizeNext {
			if c >= 'a' && c <= 'z' {
				out = append(out, c-32)
			} else {
				out = append(out, c)
			}
			capitalizeNext = false
		} else {
			out = append(out, c)
		}
	}
	return string(out)
}

// parseImpactedJSON extracts service names from concatenated impacted_services_json arrays.
// The field can look like: `["svcA","svcB"],["svcB","svcC"]` (concatenated across incidents).
func parseImpactedJSON(raw string) []string {
	if raw == "" {
		return nil
	}
	// Wrap fragments into a valid JSON array for bulk parse.
	wrapped := "[" + raw + "]"
	var nested [][]string
	if err := json.Unmarshal([]byte(wrapped), &nested); err != nil {
		return nil
	}
	seen := map[string]bool{}
	var unique []string
	for _, arr := range nested {
		for _, s := range arr {
			if s != "" && !seen[s] {
				seen[s] = true
				unique = append(unique, s)
			}
		}
	}
	return unique
}
