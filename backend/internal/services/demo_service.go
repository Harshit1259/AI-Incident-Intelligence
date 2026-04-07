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
	case "checkout_timeout":
		return demoService.runCheckoutTimeoutScenario()
	case "payments_database":
		return demoService.runPaymentsDatabaseScenario()
	case "inventory_degradation":
		return demoService.runInventoryDegradationScenario()
	default:
		return fmt.Errorf("unknown scenario")
	}
}

func (demoService *DemoService) runCheckoutTimeoutScenario() error {
	baseTime := time.Now().UTC()

	events := []models.Event{
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "checkout-api",
			Severity:  "critical",
			Message:   "checkout latency increased",
			Timestamp: baseTime,
		},
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()+1),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "checkout-api",
			Severity:  "critical",
			Message:   "checkout requests timed out",
			Timestamp: baseTime.Add(2 * time.Minute),
		},
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()+2),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "checkout-api",
			Severity:  "critical",
			Message:   "checkout timeout spike detected",
			Timestamp: baseTime.Add(4 * time.Minute),
		},
	}

	return demoService.ingestScenarioEvents(events)
}

func (demoService *DemoService) runPaymentsDatabaseScenario() error {
	baseTime := time.Now().UTC()

	events := []models.Event{
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "payments-api",
			Severity:  "critical",
			Message:   "database connection refused by primary node",
			Timestamp: baseTime,
		},
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()+1),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "payments-api",
			Severity:  "critical",
			Message:   "payments requests failing due to database outage",
			Timestamp: baseTime.Add(1 * time.Minute),
		},
	}

	return demoService.ingestScenarioEvents(events)
}

func (demoService *DemoService) runInventoryDegradationScenario() error {
	baseTime := time.Now().UTC()

	events := []models.Event{
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "inventory-api",
			Severity:  "critical",
			Message:   "inventory query latency increased",
			Timestamp: baseTime,
		},
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()+1),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "inventory-api",
			Severity:  "critical",
			Message:   "inventory requests timed out",
			Timestamp: baseTime.Add(3 * time.Minute),
		},
		{
			ID:        fmt.Sprintf("event-%d", time.Now().UnixNano()+2),
			Source:    "demo-generator",
			Type:      "alert",
			Service:   "inventory-api",
			Severity:  "critical",
			Message:   "inventory failure spike detected",
			Timestamp: baseTime.Add(5 * time.Minute),
		},
	}

	return demoService.ingestScenarioEvents(events)
}

func (demoService *DemoService) ingestScenarioEvents(events []models.Event) error {
	for _, event := range events {
		if err := demoService.eventStore.AddEvent(event); err != nil {
			return err
		}

		demoService.correlationService.ProcessEvent(event)
		
	}
				return nil

}
