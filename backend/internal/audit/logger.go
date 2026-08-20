package audit

import (
	"encoding/json"
	"log"

	"ai-incident-platform/backend/internal/store"
)

var auditStore *store.AuditLogStore

// SetStore wires the audit log store. Must be called at startup.
func SetStore(s *store.AuditLogStore) {
	auditStore = s
}

// Log records an audit entry asynchronously (fire-and-forget).
func Log(tenantID, actor, action, resourceType, resourceID string, details map[string]interface{}) {
	if auditStore == nil {
		return
	}
	detailsJSON := "{}"
	if details != nil {
		b, err := json.Marshal(details)
		if err == nil {
			detailsJSON = string(b)
		}
	}
	go func() {
		if err := auditStore.Log(store.AuditLogEntry{
			TenantID:     tenantID,
			Actor:        actor,
			Action:       action,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			DetailsJSON:  detailsJSON,
		}); err != nil {
			log.Printf("audit: failed to log %s: %v", action, err)
		}
	}()
}
