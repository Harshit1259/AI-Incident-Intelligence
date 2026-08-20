package store

import (
	"context"
	"time"
	"database/sql"

	"ai-incident-platform/backend/internal/models"
)

type DependencyStore struct {
	db *sql.DB
}

func NewDependencyStore(db *sql.DB) *DependencyStore {
	return &DependencyStore{db: db}
}

func (s *DependencyStore) Create(d models.DependencyCatalogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO dependency_catalog (id, tenant_id, service, dependency_name, dependency_type, vendor, health_url, status_page_url)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		d.ID, d.TenantID, d.Service, d.DependencyName, d.DependencyType, d.Vendor, d.HealthURL, d.StatusPageURL,
	)
	return err
}

func (s *DependencyStore) GetByService(service string) ([]models.DependencyCatalogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, dependency_name, dependency_type, vendor, health_url, status_page_url, created_at
		 FROM dependency_catalog WHERE service = $1 ORDER BY dependency_name ASC`,
		service,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.DependencyCatalogEntry
	for rows.Next() {
		var d models.DependencyCatalogEntry
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Service, &d.DependencyName, &d.DependencyType,
			&d.Vendor, &d.HealthURL, &d.StatusPageURL, &d.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func (s *DependencyStore) GetAll(tenantID string) ([]models.DependencyCatalogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service, dependency_name, dependency_type, vendor, health_url, status_page_url, created_at
		 FROM dependency_catalog WHERE tenant_id = $1 ORDER BY service, dependency_name ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []models.DependencyCatalogEntry
	for rows.Next() {
		var d models.DependencyCatalogEntry
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Service, &d.DependencyName, &d.DependencyType,
			&d.Vendor, &d.HealthURL, &d.StatusPageURL, &d.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, d)
	}
	return results, rows.Err()
}

func (s *DependencyStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM dependency_catalog WHERE id = $1`, id)
	return err
}
