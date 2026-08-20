package services

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type WhatsAppService struct {
	store      *store.WhatsAppStore
	httpClient *http.Client
}

func NewWhatsAppService(s *store.WhatsAppStore) *WhatsAppService {
	return &WhatsAppService{
		store:      s,
		httpClient: &http.Client{},
	}
}

func (s *WhatsAppService) SendIncidentAlert(tenantID string, incident models.Incident) error {
	config, err := s.store.GetByTenant(tenantID)
	if err != nil {
		return fmt.Errorf("whatsapp: get config: %w", err)
	}
	if config == nil || !config.Enabled {
		return nil
	}

	// Check severity threshold
	if !s.shouldNotify(config.NotifyOn, incident.Severity) {
		return nil
	}

	// Build message
	message := fmt.Sprintf(
		"\xf0\x9f\x9a\xa8 %s INCIDENT\n\nService: %s\nTitle: %s\nStatus: %s\n\nRoot Cause: %s\n\nView: /incidents/%s",
		strings.ToUpper(incident.Severity),
		incident.Service,
		incident.Title,
		incident.Status,
		incident.RootCauseSummary,
		incident.ID,
	)

	// Mock: log the message instead of actually calling WhatsApp Cloud API
	// In production, POST to https://graph.facebook.com/v18.0/{phone_number_id}/messages
	for _, recipient := range config.Recipients {
		log.Printf("whatsapp: [MOCK] sending to %s via phone_number_id=%s: %s",
			recipient, config.PhoneNumberID, truncateWhatsApp(message, 100))
	}

	log.Printf("whatsapp: sent alert to %d recipients for incident %s",
		len(config.Recipients), incident.ID)
	return nil
}

func (s *WhatsAppService) GetConfig(tenantID string) (*models.WhatsAppConfig, error) {
	return s.store.GetByTenant(tenantID)
}

func (s *WhatsAppService) SaveConfig(config models.WhatsAppConfig) error {
	if config.ID == "" {
		config.ID = fmt.Sprintf("wa-%s", config.TenantID)
	}
	return s.store.Upsert(config)
}

func (s *WhatsAppService) shouldNotify(notifyOn, severity string) bool {
	sevWeight := severityWeightWA(severity)
	switch strings.ToLower(notifyOn) {
	case "critical":
		return sevWeight >= 4
	case "high":
		return sevWeight >= 3
	case "all":
		return true
	default:
		return sevWeight >= 4 // default to critical only
	}
}

func severityWeightWA(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func truncateWhatsApp(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
