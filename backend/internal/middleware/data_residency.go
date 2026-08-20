package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// tenantRegionReader is the minimal store interface needed for residency checks.
type tenantRegionReader interface {
	GetDataRegion(tenantID string) string
}

// regionCacheEntry holds a cached tenant region with a TTL.
type regionCacheEntry struct {
	region    string
	expiresAt time.Time
}

// DataResidencyMiddleware enforces that every authenticated request is served
// by the correct regional deployment.
//
// Enforcement: if the tenant's data_region differs from this deployment's
// DATA_REGION (cfg.DataRegion), the request is rejected with 403.
// This guarantees EU tenants' data never leaves the EU deployment.
type DataResidencyMiddleware struct {
	deploymentRegion string
	store            tenantRegionReader
	cache            sync.Map // map[string]*regionCacheEntry
	cacheTTL         time.Duration
}

// NewDataResidencyMiddleware creates the middleware.
// deploymentRegion is the DATA_REGION env value for this server (e.g. "eu").
func NewDataResidencyMiddleware(deploymentRegion string, store tenantRegionReader) *DataResidencyMiddleware {
	if deploymentRegion == "" {
		deploymentRegion = "us"
	}
	return &DataResidencyMiddleware{
		deploymentRegion: deploymentRegion,
		store:            store,
		cacheTTL:         5 * time.Minute,
	}
}

// Enforce wraps an authenticated handler. It must be applied AFTER auth middleware
// (so ClaimsFromContext works) but BEFORE the actual handler.
func (m *DataResidencyMiddleware) Enforce(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r)
		if !ok {
			// No auth claims — pass through (auth middleware handles the 401).
			next.ServeHTTP(w, r)
			return
		}

		tenantRegion := m.cachedRegion(claims.TenantID)

		// Always stamp the response so the UI knows which region served it.
		w.Header().Set("X-Data-Region", tenantRegion)
		w.Header().Set("X-Deployment-Region", m.deploymentRegion)

		if tenantRegion != "" && tenantRegion != m.deploymentRegion {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":             "data_residency_violation",
				"message":           "Your tenant's data is stored in the " + tenantRegion + " region. Please use the " + tenantRegion + " endpoint.",
				"tenant_region":     tenantRegion,
				"deployment_region": m.deploymentRegion,
			})
			return
		}

		next.ServeHTTP(w, r)
	}
}

// cachedRegion returns the tenant's data_region from an in-memory cache (5-min TTL)
// so we don't hit the DB on every request.
func (m *DataResidencyMiddleware) cachedRegion(tenantID string) string {
	if v, ok := m.cache.Load(tenantID); ok {
		entry := v.(*regionCacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return entry.region
		}
	}
	region := m.store.GetDataRegion(tenantID)
	m.cache.Store(tenantID, &regionCacheEntry{
		region:    region,
		expiresAt: time.Now().Add(m.cacheTTL),
	})
	return region
}

// InvalidateRegionCache evicts a tenant's cached region (call after SetDataRegion).
func (m *DataResidencyMiddleware) InvalidateRegionCache(tenantID string) {
	m.cache.Delete(tenantID)
}
