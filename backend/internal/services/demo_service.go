package services

import (
	"fmt"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

type DemoService struct {
	eventStore         *store.EventStore
	correlationService *CorrelationService
	devStore           *store.DevStore
}

func NewDemoService(eventStore *store.EventStore, correlationService *CorrelationService, devStore *store.DevStore) *DemoService {
	return &DemoService{
		eventStore:         eventStore,
		correlationService: correlationService,
		devStore:           devStore,
	}
}

func (demoService *DemoService) RunScenario(name string) error {
	if err := demoService.devStore.ResetAll(); err != nil {
		return err
	}

	switch name {
	case "checkout_timeout", "checkout_timeout_cascade":
		return demoService.runCheckoutTimeoutScenario()
	case "payments_database", "payments_database_failure":
		return demoService.runPaymentsDatabaseScenario()
	case "inventory_degradation", "inventory_service_degradation":
		return demoService.runInventoryDegradationScenario()
	default:
		return fmt.Errorf("unknown scenario: %s", name)
	}
}

func (demoService *DemoService) runCheckoutTimeoutScenario() error {
	baseTime := time.Now().UTC()

	events := []models.Event{
		{
			ID:        fmt.Sprintf("event-%d-1", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "checkout-api",
			Severity:  "high",
			Title:     "Checkout API latency degraded",
			Message:   "P99 checkout request latency increased above 2s threshold",
			Timestamp: baseTime,
		},
		{
			ID:        fmt.Sprintf("event-%d-2", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "checkout-api",
			Severity:  "critical",
			Title:     "Checkout requests timing out",
			Message:   "checkout-api requests are timing out — error rate 42%",
			Timestamp: baseTime.Add(2 * time.Minute),
		},
		{
			ID:        fmt.Sprintf("event-%d-3", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "checkout-api",
			Severity:  "critical",
			Title:     "Checkout timeout spike — downstream dependency unresponsive",
			Message:   "checkout-api cannot reach order-service — connection timeout spike detected",
			Timestamp: baseTime.Add(4 * time.Minute),
		},
	}

	return demoService.ingestScenarioEvents(events)
}

func (demoService *DemoService) runPaymentsDatabaseScenario() error {
	baseTime := time.Now().UTC()

	events := []models.Event{
		{
			ID:        fmt.Sprintf("event-%d-1", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "payments-api",
			Severity:  "critical",
			Title:     "Payments database connection refused",
			Message:   "payments-api: database connection refused by primary node on port 5432",
			Timestamp: baseTime,
		},
		{
			ID:        fmt.Sprintf("event-%d-2", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "payments-api",
			Severity:  "critical",
			Title:     "Payments requests failing — database outage",
			Message:   "payments requests failing due to database outage — all writes rejected",
			Timestamp: baseTime.Add(1 * time.Minute),
		},
		{
			ID:        fmt.Sprintf("event-%d-3", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "payments-api",
			Severity:  "critical",
			Title:     "Payments API error rate 100%",
			Message:   "payments-api error rate at 100% — PostgreSQL primary unreachable",
			Timestamp: baseTime.Add(3 * time.Minute),
		},
	}

	return demoService.ingestScenarioEvents(events)
}

func (demoService *DemoService) runInventoryDegradationScenario() error {
	baseTime := time.Now().UTC()

	events := []models.Event{
		{
			ID:        fmt.Sprintf("event-%d-1", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "inventory-api",
			Severity:  "medium",
			Title:     "Inventory query latency increased",
			Message:   "inventory-api query latency increased — P95 above 800ms",
			Timestamp: baseTime,
		},
		{
			ID:        fmt.Sprintf("event-%d-2", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "inventory-api",
			Severity:  "high",
			Title:     "Inventory API requests timing out",
			Message:   "inventory-api requests timing out — Redis cache unavailable",
			Timestamp: baseTime.Add(3 * time.Minute),
		},
		{
			ID:        fmt.Sprintf("event-%d-3", baseTime.UnixNano()),
			Source:    "prometheus",
			Type:      "alert",
			Service:   "inventory-api",
			Severity:  "critical",
			Title:     "Inventory service failure spike",
			Message:   "inventory-api failure spike — service degraded, Redis OOM condition detected",
			Timestamp: baseTime.Add(5 * time.Minute),
		},
	}

	return demoService.ingestScenarioEvents(events)
}

func (demoService *DemoService) ingestScenarioEvents(events []models.Event) error {
	for _, event := range events {
		// Use SaveEvent (Phase 1) which stores the fingerprint column
		demoService.eventStore.SaveEvent(event)
		// ProcessEvent handles dedup + correlation → creates/merges incidents
		_ = demoService.correlationService.ProcessEvent(event)
	}
	return nil
}
