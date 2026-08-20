package handlers

import (
	"net/http"

	"ai-incident-platform/backend/internal/api"
	"ai-incident-platform/backend/internal/middleware"
	"ai-incident-platform/backend/internal/models"
)

// tenantRegionStore is the minimal store interface for region lookups.
type tenantRegionStore interface {
	GetDataRegion(tenantID string) string
}

// RegionHandler serves data residency information.
//
//   GET /api/v1/region           — public; returns this deployment's region info
//   GET /api/v1/region/tenant    — authenticated; returns tenant's data residency badge
type RegionHandler struct {
	deploymentRegion string
	tenantStore      tenantRegionStore
}

func NewRegionHandler(deploymentRegion string, ts tenantRegionStore) *RegionHandler {
	if deploymentRegion == "" {
		deploymentRegion = "us"
	}
	return &RegionHandler{deploymentRegion: deploymentRegion, tenantStore: ts}
}

// HandleDeploymentRegion handles GET /api/v1/region (public — no auth required).
// Used by the login page to show "This server stores data in EU (Frankfurt)".
func (h *RegionHandler) HandleDeploymentRegion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	meta, ok := models.AllRegions[h.deploymentRegion]
	if !ok {
		meta = models.AllRegions[models.RegionUS]
	}

	badge := models.DataResidencyBadge{
		DeploymentRegion: meta,
		Compliant:        true, // deployment is always in its own region
		GDPRCompliant:    meta.GDPRScope,
		BadgeText:        "Data processed in " + meta.DisplayName,
	}

	w.Header().Set("X-Deployment-Region", h.deploymentRegion)
	api.WriteJSON(w, http.StatusOK, badge)
}

// HandleTenantRegion handles GET /api/v1/region/tenant (requires auth).
// Returns the full data residency badge for the authenticated tenant:
// "Your data is stored in EU (Frankfurt)" — drives the UI badge.
func (h *RegionHandler) HandleTenantRegion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	claims, ok := middleware.ClaimsFromContext(r)
	if !ok {
		api.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantRegion := h.tenantStore.GetDataRegion(claims.TenantID)
	if tenantRegion == "" {
		tenantRegion = h.deploymentRegion
	}

	tenantMeta, ok := models.AllRegions[tenantRegion]
	if !ok {
		tenantMeta = models.AllRegions[models.RegionUS]
	}
	deployMeta, ok := models.AllRegions[h.deploymentRegion]
	if !ok {
		deployMeta = models.AllRegions[models.RegionUS]
	}

	compliant := tenantRegion == h.deploymentRegion
	badge := models.DataResidencyBadge{
		DeploymentRegion: deployMeta,
		TenantRegion:     &tenantMeta,
		Compliant:        compliant,
		GDPRCompliant:    tenantMeta.GDPRScope,
		BadgeText:        "Your data is stored in " + tenantMeta.DisplayName,
	}

	w.Header().Set("X-Data-Region", tenantRegion)
	api.WriteJSON(w, http.StatusOK, badge)
}
