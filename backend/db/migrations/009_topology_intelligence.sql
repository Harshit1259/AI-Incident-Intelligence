-- 009_topology_intelligence.sql
-- Real topology and dependency intelligence layer.
-- Replaces the static config.ServiceDependencies map with a persistent DB-backed graph.
--
-- Three tables:
--   topology_nodes  — services, infra, pods, deployments, teams, databases, queues, caches, externals
--   topology_edges  — directed relationships (calls, depends_on, runs_on, owned_by, change_caused, …)
--   topology_node_health — point-in-time health snapshots per node (latest row used for live status)
--
-- Graph traversal (blast radius / upstream propagation) uses WITH RECURSIVE CTEs in the store layer.

BEGIN;

CREATE TABLE IF NOT EXISTS topology_nodes (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL DEFAULT 'default',
    node_type          TEXT NOT NULL DEFAULT 'service',
    -- service | infra | pod | deployment | team | database | queue | cache | external
    name               TEXT NOT NULL,
    display_name       TEXT NOT NULL DEFAULT '',
    tier               TEXT NOT NULL DEFAULT 'internal',
    -- critical | internal | external | infra
    owner_team         TEXT NOT NULL DEFAULT '',
    is_customer_facing BOOLEAN NOT NULL DEFAULT false,
    version            TEXT NOT NULL DEFAULT '',
    environment        TEXT NOT NULL DEFAULT 'production',
    metadata_json      TEXT NOT NULL DEFAULT '{}',
    auto_discovered    BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_topology_nodes_tenant_name
    ON topology_nodes (tenant_id, name);
CREATE INDEX IF NOT EXISTS idx_topology_nodes_tenant
    ON topology_nodes (tenant_id);
CREATE INDEX IF NOT EXISTS idx_topology_nodes_type
    ON topology_nodes (tenant_id, node_type);
CREATE INDEX IF NOT EXISTS idx_topology_nodes_team
    ON topology_nodes (tenant_id, owner_team);

CREATE TABLE IF NOT EXISTS topology_edges (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL DEFAULT 'default',
    from_node_id       TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    to_node_id         TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    relation           TEXT NOT NULL DEFAULT 'depends_on',
    -- calls | depends_on | runs_on | deployed_by | owned_by | change_caused | routes_to
    weight             REAL NOT NULL DEFAULT 1.0,
    confidence         INTEGER NOT NULL DEFAULT 50,
    propagation_factor REAL NOT NULL DEFAULT 0.7,
    -- propagation_factor: fraction of a failure signal that crosses this edge (0.0–1.0)
    latency_ms_p99     REAL NOT NULL DEFAULT 0,
    error_rate_pct     REAL NOT NULL DEFAULT 0,
    metadata_json      TEXT NOT NULL DEFAULT '{}',
    auto_discovered    BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_topology_edges_unique
    ON topology_edges (tenant_id, from_node_id, to_node_id, relation);
CREATE INDEX IF NOT EXISTS idx_topology_edges_from
    ON topology_edges (tenant_id, from_node_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_to
    ON topology_edges (tenant_id, to_node_id);

-- Point-in-time health snapshots. Only the latest row per node is used for live status.
-- The store trims rows older than 24 h on insert to avoid unbounded growth.
CREATE TABLE IF NOT EXISTS topology_node_health (
    id             BIGSERIAL PRIMARY KEY,
    node_id        TEXT NOT NULL REFERENCES topology_nodes(id) ON DELETE CASCADE,
    tenant_id      TEXT NOT NULL DEFAULT 'default',
    status         TEXT NOT NULL DEFAULT 'unknown',
    -- healthy | degraded | down | unknown
    cpu_pct        REAL NOT NULL DEFAULT 0,
    mem_pct        REAL NOT NULL DEFAULT 0,
    error_rate_pct REAL NOT NULL DEFAULT 0,
    latency_ms_p99 REAL NOT NULL DEFAULT 0,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_topology_node_health_node
    ON topology_node_health (node_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_topology_node_health_tenant
    ON topology_node_health (tenant_id, recorded_at DESC);

COMMIT;
