package models

type IncidentInsight struct {
	IncidentType      string   `json:"incident_type"`
	LikelyRootCause   string   `json:"likely_root_cause"`
	WhyThisIsLikely   []string `json:"why_this_is_likely"`
	RecommendedChecks []string `json:"recommended_checks"`
	SuggestedAction   string   `json:"suggested_action"`
	Confidence        string   `json:"confidence"`
}