package services

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// SCIMService implements SCIM 2.0 user and group provisioning.
type SCIMService struct {
	scimStore *store.SCIMStore
	userStore *store.UserStore
}

// NewSCIMService creates a new SCIMService.
func NewSCIMService(scimStore *store.SCIMStore, userStore *store.UserStore) *SCIMService {
	return &SCIMService{scimStore: scimStore, userStore: userStore}
}

// ── Users ─────────────────────────────────────────────────────────────────────

// GetUser returns a user as a SCIM User resource.
func (s *SCIMService) GetUser(tenantID, scimID string) (*models.SCIMUser, error) {
	u, err := s.scimStore.GetUserBySCIMID(scimID, tenantID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("scim: user not found")
		}
		return nil, fmt.Errorf("scim: get user: %w", err)
	}
	return userToSCIM(u), nil
}

// ListUsers returns a paginated list of SCIM User resources.
func (s *SCIMService) ListUsers(tenantID, filter string, startIndex, count int) ([]models.SCIMUser, int, error) {
	if count <= 0 {
		count = 100
	}
	users, total, err := s.scimStore.ListUsersByTenant(tenantID, startIndex, count)
	if err != nil {
		return nil, 0, fmt.Errorf("scim: list users: %w", err)
	}

	// Basic filter support: userName eq "value"
	var filtered []models.User
	if filter != "" {
		filter = strings.ToLower(strings.TrimSpace(filter))
		for _, u := range users {
			if strings.Contains(filter, strings.ToLower(u.Email)) {
				filtered = append(filtered, u)
			}
		}
		users = filtered
		total = len(users)
	}

	scimUsers := make([]models.SCIMUser, 0, len(users))
	for _, u := range users {
		scimUsers = append(scimUsers, *userToSCIM(&u))
	}
	return scimUsers, total, nil
}

// CreateUser provisions a new user from a SCIM User resource.
func (s *SCIMService) CreateUser(tenantID string, scimUser models.SCIMUser) (models.SCIMUser, error) {
	email := primaryEmail(scimUser.Emails)
	if email == "" {
		email = scimUser.UserName
	}
	displayName := scimUser.Name.Formatted
	if displayName == "" {
		displayName = strings.TrimSpace(scimUser.Name.GivenName + " " + scimUser.Name.FamilyName)
	}

	// Check if user exists by externalId.
	if scimUser.ExternalID != "" {
		existing, err := s.scimStore.GetUserBySCIMExternalID(scimUser.ExternalID, tenantID)
		if err == nil && existing != nil {
			// Update and return.
			if err := s.scimStore.UpdateUserSCIMFields(existing.ID, tenantID, email, displayName, scimUser.ExternalID, scimUser.Active); err != nil {
				return models.SCIMUser{}, fmt.Errorf("scim: update existing user: %w", err)
			}
			existing.Email = email
			existing.DisplayName = displayName
			_ = s.scimStore.LogSync(tenantID, "create_or_update", scimUser.ExternalID, existing.ID, "success", "upserted existing user")
			return *userToSCIM(existing), nil
		}
	}

	newUser := models.User{
		ID:             generateID(),
		TenantID:       tenantID,
		Email:          email,
		PasswordHash:   "",
		Role:           models.RoleViewer,
		SCIMExternalID: scimUser.ExternalID,
		DisplayName:    displayName,
	}

	if err := s.userStore.CreateUserSSO(newUser); err != nil {
		_ = s.scimStore.LogSync(tenantID, "create", scimUser.ExternalID, "", "failure", err.Error())
		return models.SCIMUser{}, fmt.Errorf("scim: create user: %w", err)
	}

	_ = s.scimStore.LogSync(tenantID, "create", scimUser.ExternalID, newUser.ID, "success", "provisioned")

	result := *userToSCIM(&newUser)
	result.ID = newUser.ID
	return result, nil
}

// UpdateUser performs a full replace of a SCIM User resource.
func (s *SCIMService) UpdateUser(tenantID, scimID string, scimUser models.SCIMUser) (models.SCIMUser, error) {
	existing, err := s.scimStore.GetUserBySCIMID(scimID, tenantID)
	if err != nil {
		return models.SCIMUser{}, errors.New("scim: user not found")
	}

	email := primaryEmail(scimUser.Emails)
	if email == "" {
		email = scimUser.UserName
	}
	displayName := scimUser.Name.Formatted
	if displayName == "" {
		displayName = strings.TrimSpace(scimUser.Name.GivenName + " " + scimUser.Name.FamilyName)
	}

	if err := s.scimStore.UpdateUserSCIMFields(existing.ID, tenantID, email, displayName, scimUser.ExternalID, scimUser.Active); err != nil {
		_ = s.scimStore.LogSync(tenantID, "replace", scimUser.ExternalID, existing.ID, "failure", err.Error())
		return models.SCIMUser{}, fmt.Errorf("scim: update user: %w", err)
	}

	existing.Email = email
	existing.DisplayName = displayName
	existing.SCIMExternalID = scimUser.ExternalID
	_ = s.scimStore.LogSync(tenantID, "replace", scimUser.ExternalID, existing.ID, "success", "full replace")
	return *userToSCIM(existing), nil
}

// PatchUser applies SCIM PATCH operations to a user resource.
func (s *SCIMService) PatchUser(tenantID, scimID string, ops models.SCIMPatchOp) (models.SCIMUser, error) {
	existing, err := s.scimStore.GetUserBySCIMID(scimID, tenantID)
	if err != nil {
		return models.SCIMUser{}, errors.New("scim: user not found")
	}

	email := existing.Email
	displayName := existing.DisplayName
	active := true // default active

	for _, op := range ops.Operations {
		opLower := strings.ToLower(op.Op)
		switch op.Path {
		case "active":
			if opLower == "replace" || opLower == "add" {
				if v, ok := op.Value.(bool); ok {
					active = v
				}
			}
		case "emails":
			if m, ok := op.Value.(map[string]interface{}); ok {
				if v, ok := m["value"].(string); ok {
					email = v
				}
			}
		case "name.formatted":
			if v, ok := op.Value.(string); ok {
				displayName = v
			}
		}
	}

	if err := s.scimStore.UpdateUserSCIMFields(existing.ID, tenantID, email, displayName, existing.SCIMExternalID, active); err != nil {
		_ = s.scimStore.LogSync(tenantID, "patch", existing.SCIMExternalID, existing.ID, "failure", err.Error())
		return models.SCIMUser{}, fmt.Errorf("scim: patch user: %w", err)
	}

	existing.Email = email
	existing.DisplayName = displayName
	_ = s.scimStore.LogSync(tenantID, "patch", existing.SCIMExternalID, existing.ID, "success", "patched")
	return *userToSCIM(existing), nil
}

// DeleteUser deactivates a user (soft delete — preserves audit history).
func (s *SCIMService) DeleteUser(tenantID, scimID string) error {
	existing, err := s.scimStore.GetUserBySCIMID(scimID, tenantID)
	if err != nil {
		return errors.New("scim: user not found")
	}
	// Deactivate by blanking SSO fields and marking inactive in display_name.
	if err := s.scimStore.UpdateUserSCIMFields(existing.ID, tenantID, existing.Email, "[deactivated]", "", false); err != nil {
		return fmt.Errorf("scim: deactivate user: %w", err)
	}
	_ = s.scimStore.LogSync(tenantID, "delete", existing.SCIMExternalID, existing.ID, "success", "deactivated")
	return nil
}

// ── Groups ────────────────────────────────────────────────────────────────────

// GetGroup returns a SCIM Group resource.
func (s *SCIMService) GetGroup(tenantID, groupID string) (*models.SCIMGroup, error) {
	g, err := s.scimStore.GetGroup(groupID, tenantID)
	if err != nil {
		return nil, errors.New("scim: group not found")
	}
	return g, nil
}

// ListGroups returns a paginated list of SCIM Group resources.
func (s *SCIMService) ListGroups(tenantID string, startIndex, count int) ([]models.SCIMGroup, int, error) {
	if count <= 0 {
		count = 100
	}
	return s.scimStore.ListGroups(tenantID, startIndex, count)
}

// CreateGroup provisions a new SCIM group.
func (s *SCIMService) CreateGroup(tenantID string, g models.SCIMGroup) (models.SCIMGroup, error) {
	if g.ID == "" {
		g.ID = generateID()
	}
	g.Schemas = []string{"urn:ietf:params:scim:schemas:core:2.0:Group"}
	g.Meta = models.SCIMMeta{
		ResourceType: "Group",
		Created:      time.Now().UTC(),
		LastModified: time.Now().UTC(),
	}

	if err := s.scimStore.CreateGroupForTenant(tenantID, g); err != nil {
		_ = s.scimStore.LogSync(tenantID, "create_group", g.ExternalID, g.ID, "failure", err.Error())
		return models.SCIMGroup{}, fmt.Errorf("scim: create group: %w", err)
	}

	_ = s.scimStore.LogSync(tenantID, "create_group", g.ExternalID, g.ID, "success", "provisioned")
	return g, nil
}

// UpdateGroup performs a full replace of a SCIM Group resource.
func (s *SCIMService) UpdateGroup(tenantID, groupID string, g models.SCIMGroup) (models.SCIMGroup, error) {
	existing, err := s.scimStore.GetGroup(groupID, tenantID)
	if err != nil {
		return models.SCIMGroup{}, errors.New("scim: group not found")
	}
	existing.DisplayName = g.DisplayName
	existing.ExternalID = g.ExternalID
	existing.Members = g.Members

	if err := s.scimStore.UpdateGroup(tenantID, *existing); err != nil {
		_ = s.scimStore.LogSync(tenantID, "replace_group", g.ExternalID, groupID, "failure", err.Error())
		return models.SCIMGroup{}, fmt.Errorf("scim: update group: %w", err)
	}

	existing.Meta.LastModified = time.Now().UTC()
	_ = s.scimStore.LogSync(tenantID, "replace_group", g.ExternalID, groupID, "success", "full replace")
	return *existing, nil
}

// PatchGroup applies SCIM PATCH operations to a group.
func (s *SCIMService) PatchGroup(tenantID, groupID string, ops models.SCIMPatchOp) (models.SCIMGroup, error) {
	existing, err := s.scimStore.GetGroup(groupID, tenantID)
	if err != nil {
		return models.SCIMGroup{}, errors.New("scim: group not found")
	}

	for _, op := range ops.Operations {
		opLower := strings.ToLower(op.Op)
		switch op.Path {
		case "displayName":
			if opLower == "replace" || opLower == "add" {
				if v, ok := op.Value.(string); ok {
					existing.DisplayName = v
				}
			}
		case "members":
			switch opLower {
			case "add":
				if members, ok := parseMembersOp(op.Value); ok {
					existing.Members = append(existing.Members, members...)
				}
			case "remove":
				existing.Members = []models.SCIMRef{}
			case "replace":
				if members, ok := parseMembersOp(op.Value); ok {
					existing.Members = members
				}
			}
		}
	}

	if err := s.scimStore.UpdateGroup(tenantID, *existing); err != nil {
		return models.SCIMGroup{}, fmt.Errorf("scim: patch group: %w", err)
	}

	existing.Meta.LastModified = time.Now().UTC()
	_ = s.scimStore.LogSync(tenantID, "patch_group", existing.ExternalID, groupID, "success", "patched")
	return *existing, nil
}

// DeleteGroup removes a SCIM group.
func (s *SCIMService) DeleteGroup(tenantID, groupID string) error {
	g, err := s.scimStore.GetGroup(groupID, tenantID)
	if err != nil {
		return errors.New("scim: group not found")
	}
	if err := s.scimStore.DeleteGroup(groupID, tenantID); err != nil {
		return fmt.Errorf("scim: delete group: %w", err)
	}
	_ = s.scimStore.LogSync(tenantID, "delete_group", g.ExternalID, groupID, "success", "deleted")
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// userToSCIM converts a models.User to a SCIM User resource.
func userToSCIM(u *models.User) *models.SCIMUser {
	su := &models.SCIMUser{
		Schemas:    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:         u.ID,
		ExternalID: u.SCIMExternalID,
		UserName:   u.Email,
		Name: models.SCIMName{
			Formatted: u.DisplayName,
		},
		Emails: []models.SCIMEmail{
			{Value: u.Email, Type: "work", Primary: true},
		},
		Active: u.DisplayName != "[deactivated]",
		Meta: models.SCIMMeta{
			ResourceType: "User",
			Created:      u.CreatedAt,
			LastModified: u.UpdatedAt,
		},
	}
	return su
}

// primaryEmail returns the primary email from a SCIM email list.
func primaryEmail(emails []models.SCIMEmail) string {
	for _, e := range emails {
		if e.Primary && e.Value != "" {
			return e.Value
		}
	}
	if len(emails) > 0 {
		return emails[0].Value
	}
	return ""
}

// parseMembersOp attempts to parse an op.Value into []SCIMRef for group member operations.
func parseMembersOp(value interface{}) ([]models.SCIMRef, bool) {
	switch v := value.(type) {
	case []interface{}:
		var refs []models.SCIMRef
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				ref := models.SCIMRef{}
				if val, ok := m["value"].(string); ok {
					ref.Value = val
				}
				if display, ok := m["display"].(string); ok {
					ref.Display = display
				}
				refs = append(refs, ref)
			}
		}
		return refs, true
	}
	return nil, false
}
