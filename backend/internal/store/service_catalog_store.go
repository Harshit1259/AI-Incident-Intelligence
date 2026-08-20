package store

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// ServiceCatalogStore manages the per-tenant service registry.
type ServiceCatalogStore struct {
	db *sql.DB
}

func NewServiceCatalogStore(db *sql.DB) *ServiceCatalogStore {
	return &ServiceCatalogStore{db: db}
}

func (s *ServiceCatalogStore) Upsert(e models.ServiceCatalogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`INSERT INTO service_catalog
		 (id, tenant_id, service_name, environment, business_unit, owner,
		  is_customer_facing, tier, region, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (id) DO UPDATE SET
		   service_name       = EXCLUDED.service_name,
		   environment        = EXCLUDED.environment,
		   business_unit      = EXCLUDED.business_unit,
		   owner              = EXCLUDED.owner,
		   is_customer_facing = EXCLUDED.is_customer_facing,
		   tier               = EXCLUDED.tier,
		   region             = EXCLUDED.region,
		   updated_at         = EXCLUDED.updated_at`,
		e.ID, e.TenantID, e.ServiceName, e.Environment, e.BusinessUnit, e.Owner,
		e.IsCustomerFacing, e.Tier, e.Region, e.CreatedAt, e.UpdatedAt,
	)
	return err
}

func (s *ServiceCatalogStore) GetByServiceName(tenantID, serviceName string) (*models.ServiceCatalogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var e models.ServiceCatalogEntry
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service_name, environment, business_unit, owner,
		        is_customer_facing, tier, region, created_at, updated_at
		 FROM service_catalog
		 WHERE tenant_id = $1 AND service_name = $2
		 ORDER BY CASE WHEN environment = 'prod' THEN 0 ELSE 1 END
		 LIMIT 1`,
		tenantID, serviceName,
	).Scan(
		&e.ID, &e.TenantID, &e.ServiceName, &e.Environment, &e.BusinessUnit, &e.Owner,
		&e.IsCustomerFacing, &e.Tier, &e.Region, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &e, err
}

func (s *ServiceCatalogStore) GetByID(id string) (*models.ServiceCatalogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var e models.ServiceCatalogEntry
	err := s.db.QueryRowContext(ctx, 
		`SELECT id, tenant_id, service_name, environment, business_unit, owner,
		        is_customer_facing, tier, region, created_at, updated_at
		 FROM service_catalog WHERE id = $1`,
		id,
	).Scan(
		&e.ID, &e.TenantID, &e.ServiceName, &e.Environment, &e.BusinessUnit, &e.Owner,
		&e.IsCustomerFacing, &e.Tier, &e.Region, &e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &e, err
}

func (s *ServiceCatalogStore) ListByTenant(tenantID string) ([]models.ServiceCatalogEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, 
		`SELECT id, tenant_id, service_name, environment, business_unit, owner,
		        is_customer_facing, tier, region, created_at, updated_at
		 FROM service_catalog
		 WHERE tenant_id = $1
		 ORDER BY service_name, environment`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []models.ServiceCatalogEntry
	for rows.Next() {
		var e models.ServiceCatalogEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.ServiceName, &e.Environment, &e.BusinessUnit, &e.Owner,
			&e.IsCustomerFacing, &e.Tier, &e.Region, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *ServiceCatalogStore) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM service_catalog WHERE id = $1`, id)
	return err
}

// BulkUpsert inserts or updates a slice of entries in a single transaction.
func (s *ServiceCatalogStore) BulkUpsert(entries []models.ServiceCatalogEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if len(entries) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(
		`INSERT INTO service_catalog
		 (id, tenant_id, service_name, environment, business_unit, owner,
		  is_customer_facing, tier, region, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 ON CONFLICT (id) DO UPDATE SET
		   service_name       = EXCLUDED.service_name,
		   environment        = EXCLUDED.environment,
		   business_unit      = EXCLUDED.business_unit,
		   owner              = EXCLUDED.owner,
		   is_customer_facing = EXCLUDED.is_customer_facing,
		   tier               = EXCLUDED.tier,
		   region             = EXCLUDED.region,
		   updated_at         = EXCLUDED.updated_at`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for _, e := range entries {
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		e.UpdatedAt = now
		if _, err := stmt.Exec(
			e.ID, e.TenantID, e.ServiceName, e.Environment, e.BusinessUnit, e.Owner,
			e.IsCustomerFacing, e.Tier, e.Region, e.CreatedAt, e.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
