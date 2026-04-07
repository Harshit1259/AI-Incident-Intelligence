package models

type CopilotRequest struct {
	Question string `json:"question"`
}

type CopilotAnswer struct {
	Answer             string   `json:"answer"`
	Intent             string   `json:"intent"`
	SuggestedFollowups []string `json:"suggested_followups"`
}

type ActivityItem struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
}
