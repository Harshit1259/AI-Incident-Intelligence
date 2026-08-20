package models

// Supported data residency regions.
const (
	RegionUS   = "us"   // us-east-1 / US East (N. Virginia)
	RegionEU   = "eu"   // eu-central-1 / EU (Frankfurt)
	RegionAPAC = "apac" // ap-southeast-1 / Asia Pacific (Singapore)
)

// RegionMeta describes a supported region.
type RegionMeta struct {
	Code        string `json:"code"`         // "us" | "eu" | "apac"
	DisplayName string `json:"display_name"` // "EU (Frankfurt)"
	CloudRegion string `json:"cloud_region"` // "eu-central-1"
	City        string `json:"city"`         // "Frankfurt"
	Continent   string `json:"continent"`    // "Europe"
	GDPRScope   bool   `json:"gdpr_scope"`   // true for EU (GDPR applies)
}

// AllRegions lists every supported residency region.
var AllRegions = map[string]RegionMeta{
	RegionUS: {
		Code:        RegionUS,
		DisplayName: "US East (Virginia)",
		CloudRegion: "us-east-1",
		City:        "Ashburn, VA",
		Continent:   "North America",
		GDPRScope:   false,
	},
	RegionEU: {
		Code:        RegionEU,
		DisplayName: "EU (Frankfurt)",
		CloudRegion: "eu-central-1",
		City:        "Frankfurt",
		Continent:   "Europe",
		GDPRScope:   true,
	},
	RegionAPAC: {
		Code:        RegionAPAC,
		DisplayName: "Asia Pacific (Singapore)",
		CloudRegion: "ap-southeast-1",
		City:        "Singapore",
		Continent:   "Asia Pacific",
		GDPRScope:   false,
	},
}

// ValidRegion returns true when code is a known region.
func ValidRegion(code string) bool {
	_, ok := AllRegions[code]
	return ok
}

// DataResidencyBadge is returned by GET /api/v1/region and embedded in
// tenant detail responses. Drives the "Your data is stored in EU (Frankfurt)" badge.
type DataResidencyBadge struct {
	DeploymentRegion RegionMeta `json:"deployment_region"` // this server's region
	TenantRegion     *RegionMeta `json:"tenant_region,omitempty"` // tenant's assigned region (when known)
	Compliant        bool       `json:"compliant"`   // tenant_region == deployment_region
	GDPRCompliant    bool       `json:"gdpr_compliant"`
	BadgeText        string     `json:"badge_text"`  // "Your data is stored in EU (Frankfurt)"
}
