package models

type ImpactAnalysis struct {
	PrimaryService   string   `json:"primary_service"`
	Downstream       []string `json:"downstream"`
	ImpactLevel      string   `json:"impact_level"`
	AffectedServices []string `json:"affected_services"`
	ImpactCount      int      `json:"impact_count"`
}