package services

import (
	"fmt"
	"log"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type AutoResolveService struct {
	store         *store.AutoResolveStore
	incidentStore *store.IncidentStore
}

func NewAutoResolveService(s *store.AutoResolveStore, is *store.IncidentStore) *AutoResolveService {
	return &AutoResolveService{store: s, incidentStore: is}
}

// CheckAndResolve checks if an incident matches any auto-resolve rules.
// Returns (true, ruleName) if the incident was auto-resolved.
func (s *AutoResolveService) CheckAndResolve(incident models.Incident) (bool, string) {
	patternText := strings.ToLower(incident.Title + " " + incident.RootCauseSummary)
	rules, err := s.store.FindMatchingRules(incident.Service, incident.Severity, patternText)
	if err != nil {
		log.Printf("auto_resolve: error finding rules: %v", err)
		return false, ""
	}

	for _, rule := range rules {
		if rule.Action == "resolve" {
			_, err := s.incidentStore.UpdateIncidentStatus(incident.ID, "resolved")
			if err != nil {
				log.Printf("auto_resolve: failed to resolve incident %s: %v", incident.ID, err)
				continue
			}
			_ = s.store.IncrementFired(rule.ID)
			log.Printf("auto_resolve: resolved incident %s via rule %q", incident.ID, rule.Name)
			return true, rule.Name
		}
	}

	return false, ""
}

func (s *AutoResolveService) GetRules(tenantID string, limit, offset int) ([]models.AutoResolveRule, error) {
	return s.store.GetRules(tenantID, limit, offset)
}

func (s *AutoResolveService) CreateRule(req models.AutoResolveRuleRequest, tenantID string) (*models.AutoResolveRule, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("rule name is required")
	}
	if req.Action == "" {
		req.Action = "resolve"
	}
	if req.CooldownMinutes <= 0 {
		req.CooldownMinutes = 30
	}

	rule := models.AutoResolveRule{
		ID:              fmt.Sprintf("ar-%d", time.Now().UnixNano()),
		TenantID:        tenantID,
		Name:            req.Name,
		Pattern:         req.Pattern,
		Service:         req.Service,
		Severity:        req.Severity,
		Action:          req.Action,
		CooldownMinutes: req.CooldownMinutes,
		Enabled:         true,
		CreatedAt:       time.Now(),
	}

	if err := s.store.Create(rule); err != nil {
		return nil, err
	}

	return &rule, nil
}

func (s *AutoResolveService) DeleteRule(id string) error {
	return s.store.DeleteRule(id)
}

func (s *AutoResolveService) ToggleRule(id string, enabled bool) error {
	rule, err := s.store.GetRuleByID(id)
	if err != nil {
		return err
	}
	if rule == nil {
		return fmt.Errorf("rule not found: %s", id)
	}
	rule.Enabled = enabled
	return s.store.UpdateRule(*rule)
}
