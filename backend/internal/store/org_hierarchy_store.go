package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// OrgHierarchyStore manages projects and workspaces.
type OrgHierarchyStore struct {
	db *sql.DB
}

// NewOrgHierarchyStore returns a new OrgHierarchyStore.
func NewOrgHierarchyStore(db *sql.DB) *OrgHierarchyStore {
	return &OrgHierarchyStore{db: db}
}

// ── Projects ──────────────────────────────────────────────────────────────────

// CreateProject inserts a new project record.
func (s *OrgHierarchyStore) CreateProject(p models.Project) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO projects (id, tenant_id, name, slug, description, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		p.ID, p.TenantID, p.Name, p.Slug, p.Description, p.CreatedBy, now, now,
	)
	return err
}

// GetProject returns the project with the given id scoped to tenantID.
func (s *OrgHierarchyStore) GetProject(id, tenantID string) (*models.Project, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, slug, description, created_by, created_at, updated_at
		FROM projects
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanProject(row)
}

// ListProjects returns all projects for a tenant ordered by creation date.
func (s *OrgHierarchyStore) ListProjects(tenantID string) ([]models.Project, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, slug, description, created_by, created_at, updated_at
		FROM projects
		WHERE tenant_id = $1
		ORDER BY created_at ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// UpdateProject updates the mutable fields of a project.
func (s *OrgHierarchyStore) UpdateProject(p models.Project) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE projects SET name = $1, slug = $2, description = $3, updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5`,
		p.Name, p.Slug, p.Description, p.ID, p.TenantID,
	)
	return err
}

// DeleteProject removes a project and all its workspaces (cascade handled here).
func (s *OrgHierarchyStore) DeleteProject(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, 
		`DELETE FROM workspaces WHERE project_id = $1 AND tenant_id = $2`,
		id, tenantID,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx, 
		`DELETE FROM projects WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ── Workspaces ────────────────────────────────────────────────────────────────

// CreateWorkspace inserts a new workspace record.
func (s *OrgHierarchyStore) CreateWorkspace(w models.Workspace) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO workspaces (id, tenant_id, project_id, name, slug, description, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		w.ID, w.TenantID, w.ProjectID, w.Name, w.Slug, w.Description, w.CreatedBy, now, now,
	)
	return err
}

// GetWorkspace returns the workspace with the given id scoped to tenantID.
func (s *OrgHierarchyStore) GetWorkspace(id, tenantID string) (*models.Workspace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, project_id, name, slug, description, created_by, created_at, updated_at
		FROM workspaces
		WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return scanWorkspace(row)
}

// ListWorkspaces returns all workspaces for a given project scoped to tenantID.
func (s *OrgHierarchyStore) ListWorkspaces(projectID, tenantID string) ([]models.Workspace, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, project_id, name, slug, description, created_by, created_at, updated_at
		FROM workspaces
		WHERE project_id = $1 AND tenant_id = $2
		ORDER BY created_at ASC`,
		projectID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Workspace
	for rows.Next() {
		w, err := scanWorkspace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// UpdateWorkspace updates the mutable fields of a workspace.
func (s *OrgHierarchyStore) UpdateWorkspace(w models.Workspace) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE workspaces SET name = $1, slug = $2, description = $3, updated_at = NOW()
		WHERE id = $4 AND tenant_id = $5`,
		w.Name, w.Slug, w.Description, w.ID, w.TenantID,
	)
	return err
}

// DeleteWorkspace removes a workspace record.
func (s *OrgHierarchyStore) DeleteWorkspace(id, tenantID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`DELETE FROM workspaces WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	return err
}

// ── scan helpers ──────────────────────────────────────────────────────────────

func scanProject(row interface{ Scan(...interface{}) error }) (*models.Project, error) {
	var p models.Project
	err := row.Scan(&p.ID, &p.TenantID, &p.Name, &p.Slug, &p.Description,
		&p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func scanWorkspace(row interface{ Scan(...interface{}) error }) (*models.Workspace, error) {
	var w models.Workspace
	err := row.Scan(&w.ID, &w.TenantID, &w.ProjectID, &w.Name, &w.Slug,
		&w.Description, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &w, nil
}
