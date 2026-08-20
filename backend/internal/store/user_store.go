package store

// user_store.go — Phase 1, Week 3
// Data access layer for users and tenants.

import (
	"context"
	"database/sql"
	"time"

	"ai-incident-platform/backend/internal/models"
)

// UserStore manages user persistence.
type UserStore struct {
	db *sql.DB
}

func NewUserStore(db *sql.DB) *UserStore {
	return &UserStore{db: db}
}

// CreateUser inserts a new user record.
func (s *UserStore) CreateUser(user models.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, tenant_id, email, password_hash, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID,
		user.TenantID,
		user.Email,
		user.PasswordHash,
		user.Role,
		now,
		now,
	)
	return err
}

// GetUserByEmail returns the user matching the given email, or false if not found.
func (s *UserStore) GetUserByEmail(email string) (models.User, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, password_hash, role,
		       created_at, updated_at
		FROM users
		WHERE email = $1
		LIMIT 1`,
		email,
	)

	var u models.User
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return models.User{}, false
	}
	return u, true
}

// GetUserByID returns the user matching the given ID.
func (s *UserStore) GetUserByID(id string) (models.User, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, password_hash, role,
		       created_at, updated_at
		FROM users
		WHERE id = $1
		LIMIT 1`,
		id,
	)

	var u models.User
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return models.User{}, false
	}
	return u, true
}

// EmailExists returns true if the email is already registered.
func (s *UserStore) EmailExists(email string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email = $1`, email).Scan(&count)
	return count > 0
}

// CountUsers returns total user count (used to auto-grant first user admin role).
func (s *UserStore) CountUsers() int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count
}

// UpdatePasswordHash updates a user's password hash (used for hash migration).
func (s *UserStore) UpdatePasswordHash(userID, newHash string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		newHash, userID,
	)
	return err
}

// GetUsersByTenant returns all users belonging to a tenant, ordered by creation date.
// Never returns password hashes.
func (s *UserStore) GetUsersByTenant(tenantID string) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, email, role, created_at, updated_at
		FROM users
		WHERE tenant_id = $1
		ORDER BY created_at DESC LIMIT 500`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// TenantExists returns true when a row exists in the tenants table for id.
func (s *UserStore) TenantExists(id string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var count int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants WHERE id = $1`, id).Scan(&count)
	return count > 0
}

// GetUserBySSOUserID returns the user with the given SSO subject (sso_user_id) within a tenant.
func (s *UserStore) GetUserBySSOUserID(ssoUserID, tenantID string) (models.User, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, password_hash, role, created_at, updated_at
		FROM users
		WHERE sso_user_id = $1 AND tenant_id = $2
		LIMIT 1`,
		ssoUserID, tenantID,
	)
	var u models.User
	err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return models.User{}, false
	}
	u.SSOUserID = ssoUserID
	return u, true
}

// UpdateSSOFields updates the sso_user_id, sso_provider_id, and display_name columns.
func (s *UserStore) UpdateSSOFields(id, ssoUserID, ssoProviderID, displayName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET sso_user_id = $1, sso_provider_id = $2, display_name = $3, updated_at = NOW()
		WHERE id = $4`,
		ssoUserID, ssoProviderID, displayName, id,
	)
	return err
}

// CreateUserSSO inserts a user provisioned via SSO (no password, with SSO fields).
func (s *UserStore) CreateUserSSO(user models.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, tenant_id, email, password_hash, role, sso_user_id, sso_provider_id, display_name, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		user.ID, user.TenantID, user.Email, "",
		user.Role, user.SSOUserID, user.SSOProviderID, user.DisplayName,
		now, now,
	)
	return err
}

// UpdateUserRole changes the role of a user.
func (s *UserStore) UpdateUserRole(userID, tenantID, role string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, 
		`UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3`,
		role, userID, tenantID,
	)
	return err
}
