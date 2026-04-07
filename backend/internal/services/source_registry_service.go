package services

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"ai-incident-platform/backend/internal/models"
)

type SourceRegistryService struct {
	mutex   sync.RWMutex
	sources map[string]models.SourceConnection
}

func NewSourceRegistryService() *SourceRegistryService {
	return &SourceRegistryService{
		sources: make(map[string]models.SourceConnection),
	}
}

func (sourceRegistryService *SourceRegistryService) CreateSource(name string, sourceType string) models.SourceConnection {
	sourceID := fmt.Sprintf("source-%d", time.Now().UnixNano())
	token := randomHex(16)
	endpoint := endpointForSourceType(sourceType)

	source := models.SourceConnection{
		ID:          sourceID,
		Name:        name,
		Type:        sourceType,
		Token:       token,
		Endpoint:    endpoint,
		Status:      "connected",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		LastEventAt: "",
		LastError:   "",
		TotalEvents: 0,
	}

	sourceRegistryService.mutex.Lock()
	sourceRegistryService.sources[sourceID] = source
	sourceRegistryService.mutex.Unlock()

	return source
}

func (sourceRegistryService *SourceRegistryService) ListSources() []models.SourceConnection {
	sourceRegistryService.mutex.RLock()
	defer sourceRegistryService.mutex.RUnlock()

	result := make([]models.SourceConnection, 0, len(sourceRegistryService.sources))
	for _, source := range sourceRegistryService.sources {
		result = append(result, source)
	}
	return result
}

func (sourceRegistryService *SourceRegistryService) FindByToken(sourceType string, token string) (models.SourceConnection, bool) {
	sourceRegistryService.mutex.RLock()
	defer sourceRegistryService.mutex.RUnlock()

	for _, source := range sourceRegistryService.sources {
		if source.Type == sourceType && source.Token == token {
			return source, true
		}
	}

	return models.SourceConnection{}, false
}

func (sourceRegistryService *SourceRegistryService) RecordSuccess(sourceID string, eventCount int) {
	sourceRegistryService.mutex.Lock()
	defer sourceRegistryService.mutex.Unlock()

	source, exists := sourceRegistryService.sources[sourceID]
	if !exists {
		return
	}

	source.Status = "healthy"
	source.LastEventAt = time.Now().UTC().Format(time.RFC3339)
	source.LastError = ""
	source.TotalEvents += eventCount
	sourceRegistryService.sources[sourceID] = source
}

func (sourceRegistryService *SourceRegistryService) RecordError(sourceID string, errorMessage string) {
	sourceRegistryService.mutex.Lock()
	defer sourceRegistryService.mutex.Unlock()

	source, exists := sourceRegistryService.sources[sourceID]
	if !exists {
		return
	}

	source.Status = "error"
	source.LastError = errorMessage
	sourceRegistryService.sources[sourceID] = source
}

func endpointForSourceType(sourceType string) string {
	switch sourceType {
	case "prometheus":
		return "/api/v1/ingest/prometheus"
	default:
		return "/api/v1/ingest/webhook"
	}
}

func randomHex(byteLength int) string {
	buffer := make([]byte, byteLength)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}
