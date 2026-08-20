package services

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type DependencyService struct {
	store *store.DependencyStore
}

func NewDependencyService(s *store.DependencyStore) *DependencyService {
	return &DependencyService{store: s}
}

// AttributeIncident checks if an incident can be attributed to a known dependency.
func (s *DependencyService) AttributeIncident(incident models.Incident) (*models.DependencyAttribution, error) {
	deps, err := s.store.GetByService(incident.Service)
	if err != nil {
		return nil, err
	}

	textToSearch := strings.ToLower(incident.Title + " " + incident.RootCauseSummary)

	for _, dep := range deps {
		depNameLower := strings.ToLower(dep.DependencyName)
		vendorLower := strings.ToLower(dep.Vendor)

		// Check if the incident mentions this dependency
		if strings.Contains(textToSearch, depNameLower) ||
			(vendorLower != "" && strings.Contains(textToSearch, vendorLower)) {

			attr := &models.DependencyAttribution{
				IncidentID:     incident.ID,
				DependencyName: dep.DependencyName,
				Vendor:         dep.Vendor,
			}

			switch dep.DependencyType {
			case "vendor", "third_party":
				attr.Attribution = "vendor"
				attr.Confidence = 80
				attr.Reason = fmt.Sprintf("Incident text mentions vendor dependency %q (%s)",
					dep.DependencyName, dep.Vendor)
			default:
				attr.Attribution = "internal"
				attr.Confidence = 70
				attr.Reason = fmt.Sprintf("Incident text mentions internal dependency %q",
					dep.DependencyName)
			}

			slog.Info("dependency: attributed incident", "incident_id", incident.ID, "dependency_name", dep.DependencyName, "attribution", attr.Attribution, "confidence", attr.Confidence)
			return attr, nil
		}
	}

	// No match found
	return &models.DependencyAttribution{
		IncidentID:  incident.ID,
		Attribution: "unknown",
		Confidence:  30,
		Reason:      "No matching dependency found in catalog",
	}, nil
}

func (s *DependencyService) GetCatalog(tenantID string) ([]models.DependencyCatalogEntry, error) {
	return s.store.GetAll(tenantID)
}

func (s *DependencyService) AddEntry(entry models.DependencyCatalogEntry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("dep-%d", time.Now().UnixNano())
	}
	if entry.DependencyType == "" {
		entry.DependencyType = "internal"
	}
	return s.store.Create(entry)
}

func (s *DependencyService) DeleteEntry(id string) error {
	return s.store.Delete(id)
}
