package services

// import_service.go
//
// Parses CSV uploads and bulk-upserts service catalog entries, business profiles,
// and MTTR baselines into the DB. Uses only stdlib encoding/csv — no external deps.
//
// CSV format contracts:
//   services.csv:        service_name, tier, environment, business_unit, owner, is_customer_facing, region
//   business_profiles.csv: service_name, hourly_revenue, employee_cost_per_hour, sla_penalty_per_minute,
//                            sla_threshold_minutes, infra_cost_per_hour, business_hour_multiplier, peak_multiplier,
//                            [users_per_hour, transactions_per_hour, avg_order_value, cost_model_type, currency, confidence_mode]
//   baselines.csv:       service_name, incident_type, average_mttr_minutes, [median_mttr_minutes, p90_mttr_minutes, sample_size]

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// ImportService handles CSV bulk-import for the tenant configuration layer.
type ImportService struct {
	serviceCatalogStore *store.ServiceCatalogStore
	bizImpactStore      *store.BusinessImpactStore
}

func NewImportService(
	sCatalogStore *store.ServiceCatalogStore,
	bizImpactStore *store.BusinessImpactStore,
) *ImportService {
	return &ImportService{
		serviceCatalogStore: sCatalogStore,
		bizImpactStore:      bizImpactStore,
	}
}

// ── Services CSV ─────────────────────────────────────────────────────────────

// ImportServices parses services.csv and upserts service catalog entries.
func (s *ImportService) ImportServices(tenantID string, r io.Reader) (*models.ImportResult, error) {
	result := &models.ImportResult{Errors: []string{}}

	rdr := csv.NewReader(r)
	rdr.TrimLeadingSpace = true

	headers, err := rdr.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	idx := buildColumnIndex(headers)

	var entries []models.ServiceCatalogEntry
	rowNum := 1
	for {
		rowNum++
		row, err := rdr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: parse error: %v", rowNum, err))
			result.Skipped++
			continue
		}
		result.TotalRows++

		svcName := csvStr(row, idx, "service_name")
		if svcName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: service_name is required", rowNum))
			result.Skipped++
			continue
		}

		env := csvStrDefault(row, idx, "environment", "prod")
		tier := strings.ToUpper(csvStrDefault(row, idx, "tier", "TIER_2"))
		if !validTier(tier) {
			tier = "TIER_2"
		}

		isFacing := false
		if v := csvStr(row, idx, "is_customer_facing"); v != "" {
			isFacing = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
		}

		now := time.Now()
		entries = append(entries, models.ServiceCatalogEntry{
			ID:               fmt.Sprintf("sc-%s-%s-%s", tenantID, sanitizeID(svcName), sanitizeID(env)),
			TenantID:         tenantID,
			ServiceName:      svcName,
			Environment:      env,
			BusinessUnit:     csvStr(row, idx, "business_unit"),
			Owner:            csvStr(row, idx, "owner"),
			IsCustomerFacing: isFacing,
			Tier:             tier,
			Region:           csvStr(row, idx, "region"),
			CreatedAt:        now,
			UpdatedAt:        now,
		})
		result.Imported++
	}

	if len(entries) > 0 {
		if err := s.serviceCatalogStore.BulkUpsert(entries); err != nil {
			return nil, fmt.Errorf("bulk upsert service catalog: %w", err)
		}
	}
	return result, nil
}

// ── Business Profiles CSV ────────────────────────────────────────────────────

// ImportBusinessProfiles parses business_profiles.csv and upserts financial profiles.
func (s *ImportService) ImportBusinessProfiles(tenantID string, r io.Reader) (*models.ImportResult, error) {
	result := &models.ImportResult{Errors: []string{}}

	rdr := csv.NewReader(r)
	rdr.TrimLeadingSpace = true

	headers, err := rdr.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	idx := buildColumnIndex(headers)

	rowNum := 1
	for {
		rowNum++
		row, err := rdr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: parse error: %v", rowNum, err))
			result.Skipped++
			continue
		}
		result.TotalRows++

		svcName := csvStr(row, idx, "service_name")
		if svcName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: service_name is required", rowNum))
			result.Skipped++
			continue
		}

		// Pull tier from service catalog if available.
		tier := "TIER_2"
		if cat, _ := s.serviceCatalogStore.GetByServiceName(tenantID, svcName); cat != nil && cat.Tier != "" {
			tier = cat.Tier
		}

		slaThreshold := csvInt(row, idx, "sla_threshold_minutes")
		if slaThreshold == 0 {
			slaThreshold = 15
		}

		now := time.Now()
		profile := models.ServiceProfile{
			ID:                     fmt.Sprintf("biz-%s-%s", tenantID, sanitizeID(svcName)),
			TenantID:               tenantID,
			Service:                svcName,
			Tier:                   tier,
			CostModelType:          csvStrDefault(row, idx, "cost_model_type", "mixed"),
			HourlyRevenue:          csvFloat(row, idx, "hourly_revenue"),
			EmployeeCostPerHour:    csvFloat(row, idx, "employee_cost_per_hour"),
			SLAPenaltyPerMinute:    csvFloat(row, idx, "sla_penalty_per_minute"),
			SLAThresholdMinutes:    slaThreshold,
			InfraCostPerHour:       csvFloat(row, idx, "infra_cost_per_hour"),
			BusinessHourMultiplier: csvFloatDefault(row, idx, "business_hour_multiplier", 1.0),
			PeakMultiplier:         csvFloatDefault(row, idx, "peak_multiplier", 1.0),
			UsersPerHour:           csvFloat(row, idx, "users_per_hour"),
			TransactionsPerHour:    csvFloat(row, idx, "transactions_per_hour"),
			AvgOrderValue:          csvFloat(row, idx, "avg_order_value"),
			ConfidenceMode:         csvStrDefault(row, idx, "confidence_mode", "balanced"),
			Currency:               csvStrDefault(row, idx, "currency", "USD"),
			CreatedAt:              now,
			UpdatedAt:              now,
		}

		if err := s.bizImpactStore.UpsertProfile(profile); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d (%s): %v", rowNum, svcName, err))
			result.Skipped++
			continue
		}
		result.Imported++
	}
	return result, nil
}

// ── Baselines CSV ─────────────────────────────────────────────────────────────

// ImportBaselines parses baselines.csv and upserts MTTR baselines.
func (s *ImportService) ImportBaselines(tenantID string, r io.Reader) (*models.ImportResult, error) {
	result := &models.ImportResult{Errors: []string{}}

	rdr := csv.NewReader(r)
	rdr.TrimLeadingSpace = true

	headers, err := rdr.Read()
	if err != nil {
		return nil, fmt.Errorf("read CSV header: %w", err)
	}
	idx := buildColumnIndex(headers)

	rowNum := 1
	for {
		rowNum++
		row, err := rdr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: parse error: %v", rowNum, err))
			result.Skipped++
			continue
		}
		result.TotalRows++

		svcName := csvStr(row, idx, "service_name")
		if svcName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d: service_name is required", rowNum))
			result.Skipped++
			continue
		}

		incidentType := csvStrDefault(row, idx, "incident_type", "auto")
		avgMTTR := csvFloat(row, idx, "average_mttr_minutes")
		if avgMTTR <= 0 {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d (%s): average_mttr_minutes must be > 0", rowNum, svcName))
			result.Skipped++
			continue
		}

		medianMTTR := csvFloatDefault(row, idx, "median_mttr_minutes", avgMTTR)
		p90MTTR := csvFloatDefault(row, idx, "p90_mttr_minutes", avgMTTR*1.5)
		sampleSize := csvInt(row, idx, "sample_size")

		now := time.Now()
		baseline := models.IncidentBaseline{
			ID:                fmt.Sprintf("bl-%s-%s-%s", tenantID, sanitizeID(svcName), sanitizeID(incidentType)),
			TenantID:          tenantID,
			Service:           svcName,
			IncidentType:      incidentType,
			AvgMTTRMinutes:    avgMTTR,
			MedianMTTRMinutes: medianMTTR,
			P90MTTRMinutes:    p90MTTR,
			SampleSize:        sampleSize,
			CreatedAt:         now,
			UpdatedAt:         now,
		}

		if err := s.bizImpactStore.UpsertBaseline(baseline); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("row %d (%s): %v", rowNum, svcName, err))
			result.Skipped++
			continue
		}
		result.Imported++
	}
	return result, nil
}

// ── CSV utilities ─────────────────────────────────────────────────────────────

func buildColumnIndex(headers []string) map[string]int {
	idx := make(map[string]int, len(headers))
	for i, h := range headers {
		idx[strings.TrimSpace(strings.ToLower(h))] = i
	}
	return idx
}

func csvStr(row []string, idx map[string]int, key string) string {
	i, ok := idx[key]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func csvStrDefault(row []string, idx map[string]int, key, def string) string {
	v := csvStr(row, idx, key)
	if v == "" {
		return def
	}
	return v
}

func csvFloat(row []string, idx map[string]int, key string) float64 {
	v := csvStr(row, idx, key)
	if v == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

func csvFloatDefault(row []string, idx map[string]int, key string, def float64) float64 {
	v := csvStr(row, idx, key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func csvInt(row []string, idx map[string]int, key string) int {
	v := csvStr(row, idx, key)
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}

func validTier(t string) bool {
	switch t {
	case "TIER_0", "TIER_1", "TIER_2", "TIER_3":
		return true
	}
	return false
}

// sanitizeID replaces characters that are problematic in composite IDs.
func sanitizeID(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}
