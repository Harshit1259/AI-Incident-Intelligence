package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ai-incident-platform/backend/internal/llm"
	"ai-incident-platform/backend/internal/models"
	"ai-incident-platform/backend/internal/store"
)

// PostMortemService creates and manages incident post-mortems.
type PostMortemService struct {
	llmClient *llm.Client
	pmStore   *store.PostMortemStore
	detailSvc *IncidentDetailService
}

func NewPostMortemService(
	llmClient *llm.Client,
	pmStore *store.PostMortemStore,
	detailSvc *IncidentDetailService,
) *PostMortemService {
	return &PostMortemService{
		llmClient: llmClient,
		pmStore:   pmStore,
		detailSvc: detailSvc,
	}
}

// Generate creates (or regenerates) a post-mortem for the given incident.
// It fetches the incident detail, assembles context, calls LLM, and persists the result.
func (s *PostMortemService) Generate(incidentID string) (*models.PostMortem, error) {
	detail, found := s.detailSvc.GetIncidentDetail(incidentID)
	if !found {
		return nil, fmt.Errorf("incident %s not found", incidentID)
	}

	pm := s.buildPostMortem(incidentID, detail)

	// Attempt LLM generation.
	if s.llmClient != nil && s.llmClient.IsConfigured() {
		llmPM, err := s.generateViaLLM(detail)
		if err != nil {
			log.Printf("postmortem: LLM generation failed for %s: %v — using template", incidentID, err)
		} else {
			// Merge LLM output into the post-mortem.
			pm.Title = llmPM.Title
			pm.ExecutiveSummary = llmPM.ExecutiveSummary
			pm.TimelineNarrative = llmPM.TimelineNarrative
			pm.RootCause = llmPM.RootCause
			pm.Impact = llmPM.Impact
			pm.Resolution = llmPM.Resolution
			pm.ActionItems = llmPM.ActionItems
			pm.Lessons = llmPM.Lessons
			pm.GeneratedBy = "llm"
		}
	}

	// Check if one already exists; if so update, otherwise insert.
	existing, err := s.pmStore.GetByID(pm.ID)
	if err != nil {
		return nil, fmt.Errorf("postmortem: lookup existing: %w", err)
	}

	if existing != nil {
		pm.CreatedAt = existing.CreatedAt
		if err := s.pmStore.Update(pm); err != nil {
			return nil, fmt.Errorf("postmortem: update: %w", err)
		}
	} else {
		if err := s.pmStore.Create(pm); err != nil {
			return nil, fmt.Errorf("postmortem: create: %w", err)
		}
	}

	return &pm, nil
}

// GetOrCreate returns the existing post-mortem for the incident, or generates one.
func (s *PostMortemService) GetOrCreate(incidentID string) (*models.PostMortem, error) {
	pm, err := s.pmStore.GetByIncidentID(incidentID)
	if err != nil {
		return nil, fmt.Errorf("postmortem: lookup: %w", err)
	}
	if pm != nil {
		return pm, nil
	}
	return s.Generate(incidentID)
}

// Update saves user edits to an existing post-mortem.
func (s *PostMortemService) Update(incidentID string, req models.PostMortemUpdateRequest) (*models.PostMortem, error) {
	pm, err := s.pmStore.GetByIncidentID(incidentID)
	if err != nil {
		return nil, fmt.Errorf("postmortem: lookup: %w", err)
	}
	if pm == nil {
		return nil, fmt.Errorf("no post-mortem found for incident %s", incidentID)
	}

	if req.Title != "" {
		pm.Title = req.Title
	}
	if req.Status != "" {
		pm.Status = req.Status
	}
	if req.ExecutiveSummary != "" {
		pm.ExecutiveSummary = req.ExecutiveSummary
	}
	if req.TimelineNarrative != "" {
		pm.TimelineNarrative = req.TimelineNarrative
	}
	if req.RootCause != "" {
		pm.RootCause = req.RootCause
	}
	if req.Impact != "" {
		pm.Impact = req.Impact
	}
	if req.Resolution != "" {
		pm.Resolution = req.Resolution
	}
	if req.ActionItems != nil {
		pm.ActionItems = req.ActionItems
	}
	if req.Lessons != "" {
		pm.Lessons = req.Lessons
	}
	pm.GeneratedBy = "manual"

	if err := s.pmStore.Update(*pm); err != nil {
		return nil, fmt.Errorf("postmortem: save: %w", err)
	}
	return pm, nil
}

// ─── internal ─────────────────────────────────────────────────────────────────

// buildPostMortem creates a template-based post-mortem from incident detail.
func (s *PostMortemService) buildPostMortem(incidentID string, detail models.IncidentDetail) models.PostMortem {
	inc := detail.Incident
	date := time.Now().Format("2006-01-02")

	// Build a simple timeline from events.
	timelineLines := make([]string, 0, len(detail.Events))
	for _, te := range detail.Events {
		timelineLines = append(timelineLines, fmt.Sprintf("- [%s] %s — %s",
			te.Event.Timestamp.Format("15:04:05"),
			te.StoryLabel,
			te.Event.Title,
		))
	}
	timelineNarrative := strings.Join(timelineLines, "\n")
	if timelineNarrative == "" {
		timelineNarrative = fmt.Sprintf("Incident started at %s and affected the %s service.", inc.FirstEventTime, inc.Service)
	}

	// Build action items from reasoning.
	actionItems := make([]string, 0)
	for _, r := range inc.Reasoning {
		if r != "" {
			actionItems = append(actionItems, r)
		}
	}
	if len(actionItems) == 0 {
		actionItems = []string{
			"Investigate root cause and implement permanent fix",
			"Add monitoring and alerting for early detection",
			"Update runbook with resolution steps",
		}
	}

	// PM id: pm-{incidentID without "inc-" prefix}
	pmID := "pm-" + strings.TrimPrefix(incidentID, "inc-")

	return models.PostMortem{
		ID:         pmID,
		IncidentID: incidentID,
		TenantID:   inc.TenantID,
		Title:      fmt.Sprintf("Post-Mortem: %s — %s", inc.Service, date),
		Status:     "draft",
		ExecutiveSummary: fmt.Sprintf(
			"On %s, the %s service experienced a %s incident. %s",
			inc.FirstEventTime, inc.Service, inc.Severity, inc.RootCauseSummary,
		),
		TimelineNarrative: timelineNarrative,
		RootCause:         inc.RootCauseSummary,
		Impact: fmt.Sprintf(
			"Severity: %s. Affected services: %d. Risk score: %d.",
			inc.Severity, inc.ImpactCount, inc.RiskScore,
		),
		Resolution: fmt.Sprintf(
			"Incident was %s after investigation. What changed: %s %s@%s",
			inc.Status, inc.WhatChangedType, inc.WhatChangedService, inc.WhatChangedVersion,
		),
		ActionItems: actionItems,
		Lessons:     "Review monitoring coverage and ensure runbooks are up to date.",
		GeneratedBy: "template",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// llmPostMortemResponse is the JSON structure we ask the LLM to produce.
type llmPostMortemResponse struct {
	Title             string   `json:"title"`
	ExecutiveSummary  string   `json:"executive_summary"`
	TimelineNarrative string   `json:"timeline_narrative"`
	RootCause         string   `json:"root_cause"`
	Impact            string   `json:"impact"`
	Resolution        string   `json:"resolution"`
	ActionItems       []string `json:"action_items"`
	Lessons           string   `json:"lessons"`
}

func (s *PostMortemService) generateViaLLM(detail models.IncidentDetail) (llmPostMortemResponse, error) {
	inc := detail.Incident

	// Build event summary.
	eventLines := make([]string, 0, len(detail.Events))
	for _, te := range detail.Events {
		eventLines = append(eventLines, fmt.Sprintf("[%s] %s: %s",
			te.Event.Timestamp.Format(time.RFC3339),
			te.StoryLabel,
			te.Event.Title,
		))
	}
	eventSummary := strings.Join(eventLines, "\n")
	if eventSummary == "" {
		eventSummary = "No detailed event timeline available."
	}

	systemPrompt := "You are a senior SRE writing an incident post-mortem. Return valid JSON only."

	userPrompt := fmt.Sprintf(`Write a post-mortem for this incident.

Incident ID: %s
Service: %s
Severity: %s
Status: %s
First Event: %s
Last Event: %s
Root Cause Summary: %s
Root Cause Type: %s
What Changed: %s %s @ %s — %s
Impacted Services Count: %d
Risk Score: %d

Event Timeline:
%s

Return ONLY a JSON object in this exact format:
{
  "title": "Post-Mortem: %s — %s",
  "executive_summary": "...",
  "timeline_narrative": "...",
  "root_cause": "...",
  "impact": "...",
  "resolution": "...",
  "action_items": ["...", "..."],
  "lessons": "..."
}`,
		inc.ID, inc.Service, inc.Severity, inc.Status,
		inc.FirstEventTime, inc.LastEventTime,
		inc.RootCauseSummary, inc.RootCauseType,
		inc.WhatChangedType, inc.WhatChangedService, inc.WhatChangedVersion, inc.WhatChangedDescription,
		inc.ImpactCount, inc.RiskScore,
		eventSummary,
		inc.Service, time.Now().Format("2006-01-02"),
	)

	raw, err := s.llmClient.CompleteWithSystem(systemPrompt, userPrompt)
	if err != nil {
		return llmPostMortemResponse{}, fmt.Errorf("llm call: %w", err)
	}

	// Strip markdown code fences if present.
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.SplitN(raw, "\n", 2)
		if len(lines) == 2 {
			raw = lines[1]
		}
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}

	var result llmPostMortemResponse
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return llmPostMortemResponse{}, fmt.Errorf("parse llm response: %w", err)
	}

	return result, nil
}
