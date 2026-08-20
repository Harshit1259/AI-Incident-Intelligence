● Let me explore the full codebase to map the actual code paths before writing anything.                                       
                                         
● Explore(Map alert-to-dashboard workflow)                                                                                     
  ⎿  Done (27 tool uses · 100.2k tokens · 1m 38s)                   
  (ctrl+o to expand)                                                                                                           
                                                            
● Now I have the complete picture. Here is the full end-to-end workflow.                                                       
                                                                                                                               
  ---                                                                                                                          
  End-to-End Alert → Dashboard Workflow                                                                                        
                                                                                                                               
  ---                                                                                                                          
  Layer Map (bird's eye view)                               
                                                                                                                               
  EXTERNAL WORLD                 BACKEND PIPELINE                     FRONTEND
  ─────────────────   ──────────────────────────────────────────   ─────────────                                               
  Prometheus          ingest_handler.go                     
  AlertManager     →  normalizer.go / prometheus.go                                                                            
  PagerDuty           ingest_handler.go            → event_store.go                                                            
  Datadog                  ↓                                                                                                   
  GitHub/GitLab       correlation_service.go        → incident_store.go                                                        
  Generic webhook          ↓                                                                                                   
  NeuroOps Agent      incident_detail_service.go                                                                               
                      explain_service.go            → GET /api/v1/incidents                                                    
                      copilot_service.go            → GET /api/v1/incidents/{id}                                               
                      action_service.go        →     GET /api/v1/incidents/explain/{id}   →  Dashboard                         
                      action_handler.go             POST /api/v1/incidents/copilot/{id}                                        
                      incident_handler.go           POST /api/v1/ingest/...                                                    
                      source_handler.go                                                                                        
                                                                                                                               
  ---                                                                                                                          
  Phase 1 — Alert Enters the System                         
                                   
  Entry points (HTTP layer)
                                                                                                                               
  File: backend/internal/routes/routes.go                                                                                      
                                                                                                                               
  This is the router. Every inbound path is registered here. Ingest paths are open (CORS + rate-limit only), all read/write    
  incident paths are JWT-gated.                             
                                                                                                                               
  POST /api/v1/ingest/webhook       →  IngestHandler.GenericWebhook()
  POST /api/v1/ingest/prometheus    →  IngestHandler.PrometheusWebhook()
  POST /api/v1/ingest/pagerduty     →  IngestHandler.PagerDutyWebhook()                                                        
  POST /api/v1/ingest/datadog       →  IngestHandler.DatadogWebhook()
  POST /api/v1/ingest/github        →  IngestHandler.GitHubWebhook()                                                           
  POST /api/v1/ingest/gitlab        →  IngestHandler.GitLabWebhook()
  POST /api/v1/ingest/agent         →  AgentHandler.IngestBatch()   (HMAC signed)                                              
                                                                                                                               
  ---                                                                                                                          
  File: backend/internal/handlers/ingest_handler.go                                                                            
                                                            
  Struct: IngestHandler { eventStore, correlationService, sourceRegistryService }
                                                                                                                               
  Step-by-step per request:
                                                                                                                               
  1. resolveTenantForIngest(r)                                                                                                 
    - Check Authorization: Bearer <jwt> first → extract tenantID from claims
    - Fallback to X-Source-Token header → sourceRegistryService.FindByToken(token) → resolve tenantID + sourceID               
    - If neither → 401 Unauthorized. TenantID is never trusted from the request body.                                          
  2. GenericWebhook() (for direct JSON webhooks)                                                                               
    - json.Decode(r.Body) → models.IngestEvent                                                                                 
    - Stamp TenantID, Source = "webhook", default Severity = "medium", default ID = "evt-{nano}"                               
    - Converts to models.Event                                                                                                 
    - Calls correlationService.ProcessEvent(event)                                                                             
  3. PrometheusWebhook() (for AlertManager)                                                                                    
    - json.Decode(r.Body) → AlertManager payload { alerts: [...] }                                                             
    - For each alert: calls ingest.NormalizePrometheus(alert, tenantID)
    - Calls correlationService.ProcessEvent(event) for each result                                                             
  4. PagerDutyWebhook() / DatadogWebhook()                  
    - HMAC signature verified first (X-PagerDuty-Signature / DD-Signature)                                                     
    - Extracts alert fields from their proprietary JSON structures                                                             
    - Same terminal step: correlationService.ProcessEvent(event)                                                               
                                                                                                                               
  ---                                                                                                                          
  File: backend/internal/ingest/prometheus.go                                                                                  
                                                            
  Function: NormalizePrometheus(rawAlert, tenantID) models.Event
                                                                                                                               
  Input (AlertManager JSON per alert):                                                                                         
  {                                                                                                                            
    "fingerprint": "a1b2c3d4",                                                                                                 
    "status": "firing",                                     
    "labels": { "alertname": "HighCPU", "service": "payments", "severity": "critical" },
    "annotations": { "summary": "CPU above 90%", "description": "payments pod CPU at 94%" },
    "startsAt": "2026-04-22T10:00:00Z"                                                                                         
  }                                                                                                                            
                                                                                                                               
  What it does:                                                                                                                
  - Extracts fingerprint → use as Event.Fingerprint (already deduplicated by Prometheus)
  - Maps labels["service"] or falls back to labels["alertname"] → Event.Service                                                
  - Maps labels["severity"] → normalized to critical|high|medium|low           
  - Maps annotations["summary"] → Event.Title                                                                                  
  - Maps annotations["description"] → Event.Message         
  - Maps startsAt → Event.Timestamp                                                                                            
  - Sets Event.Source = "prometheus", Event.Type = "alert"                                                                     
   
  ---                                                                                                                          
  File: backend/internal/ingest/normalizer.go               
                                                                                                                               
  Function: NormalizeEvent(e *models.Event)                 
                                                                                                                               
  Called on every event before DB write. Fills in missing fields:
  - ID → "evt-" + strconv.FormatInt(time.Now().UnixNano(), 36) if empty                                                        
  - Severity → "medium" if empty or unrecognized                                                                               
  - Type → "alert" if empty                     
  - Timestamp → time.Now() if zero                                                                                             
  - Service → "unknown-service" if empty                    
  - Trims whitespace on Title, Message                                                                                         
   
  ---                                                                                                                          
  Phase 2 — Event Stored                                    
                                                                                                                               
  File: backend/internal/store/event_store.go               
                                                                                                                               
  Called by: ingest handler after normalization, before correlation                                                            
                                                                                                                               
  SaveEvent(e models.Event) error                                                                                              
  - SQL: INSERT INTO events (...) ON CONFLICT (id) DO UPDATE SET fingerprint = EXCLUDED.fingerprint, ...
  - Columns written: id, tenant_id, source, service, resource, environment, severity, type, title, message, labels_json,       
  timestamp, fingerprint, external_id                                                                                    
  - Table: events — this is the immutable append-only raw alert log                                                            
                                                                   
  FingerprintSeenInWindow(fingerprint, windowStart, excludeEventID) bool                                                       
  - SQL: SELECT 1 FROM events WHERE fingerprint=$1 AND timestamp > $2 AND id != $3 LIMIT 1                                     
  - Used by correlation service for 5-minute dedup window                                                                      
                                                                                                                               
  GetEventsByIDs(ids []string) []models.Event                                                                                  
  - SQL: SELECT ... FROM events WHERE id = ANY($1)          
  - Used later by detail assembly to build the incident timeline                                                               
                                                            
  ---                                                                                                                          
  Phase 3 — Correlation & Incident Creation                 
                                                                                                                               
  File: backend/internal/services/correlation_service.go
                                                                                                                               
  This is the brain. Every event passes through here exactly once.                                                             
   
  ProcessEvent(event models.Event) (incidentCreated bool, err error)                                                           
                                                            
  Step 1 — Compute fingerprint (if not already set by Prometheus):                                                             
  normalizeAlertTitle(title):                               
    strip UUIDs, IP addresses, pod names, numeric IDs from title                                                               
    → "pod abc-123-xyz CPU high" → "pod CPU high"               
  computeEventFingerprint(event):                                                                                              
    sha256("{source}|{service}|{severity}|{normalizedTitle}")                                                                  
    → first 16 hex chars → "a1b2c3d4e5f60102"                                                                                  
                                                                                                                               
  Step 2 — Suppression check:                                                                                                  
  - alertFeedbackService.IsSuppressed(fingerprint) → if true, drop event silently, return                                      
                                                                                                                               
  Step 3 — Deduplication:                                                                                                      
  - eventStore.FingerprintSeenInWindow(fingerprint, now-5min, eventID)                                                         
  - If same fingerprint already exists in last 5 minutes → drop (storm prevention), return
                                                                                                                               
  Step 4 — Merge into existing incident (fingerprint match):                                                                   
  - incidentStore.FindOpenIncidentByFingerprint(fingerprint)
  - If found → merge path:                                                                                                     
    - mergeEvent(incident, event): extend LastEventTime, EventCount++, escalate severity, +10 CorrelationScore, +5 Confidence,
  recalculate PriorityScore                                                                                                    
    - incidentStore.UpdateIncident(incident)                                                                                   
    - incidentStore.AddEventToIncident(incidentID, eventID)                                                                    
    - Return (no new incident)                                                                                                 
                                                                                                                               
  Step 5 — Merge into existing incident (time-window match):
  - incidentStore.FindOpenIncidentForService(service, now-5min)                                                                
  - Searches by service name and dependent services within 5-minute window
  - If found → same merge path as Step 4                                                                                       
                                                                                                                               
  Step 6 — Create new incident:                                                                                                
                                                                                                                               
  - createIncident(event):                                                                                                     
                                                            
  - a) Check knowledge base for matching pattern → if match, inherit RootCause, Reasoning, BaseConfidence                      
   
  - b) Build models.Incident:                                                                                                  
  ID:                   "inc-{nanotime}"                    
  Title:                event.Title or event.Message[:100]                                                                     
  Service:              event.Service                       
  Severity:             event.Severity                                                                                         
  Status:               "open"                              
  FirstEventTime:       event.Timestamp (RFC3339)                                                                              
  LastEventTime:        event.Timestamp                     
  EventCount:           1                                                                                                      
  EventIDs:             [event.ID]
  Fingerprint:          computed hash                                                                                          
  TenantID:             event.TenantID                      
  CorrelationPattern:   "new_incident" | "kb_match:{id}"                                                                       
  CorrelationScore:     50                                                                                                     
  Confidence:           45|55|65|75  (by severity)                                                                             
  RiskScore:            30|50|70|90  (by severity)                                                                             
  RootCauseSummary:     "Alert triggered for {service}: {title}"
  RootCauseType:        database|timeout|memory|cpu|network|service_alert                                                      
  Reasoning:            ["High severity {service} alert", "Severity: critical triggers immediate..."]                          
                                                                                                                               
  - c) incidentStore.AddIncident(incident) → writes to incidents table                                                         
  d) incidentStore.AddEventToIncident(incidentID, eventID) → writes to incident_events                                         
                                                                                                                               
  - e) Async side-effects (goroutines, don't block response):                                                                  
    - onboardingService.MarkMilestone(tenantID, "incident_created")                                                            
    - engineeringHealthService.RecordIncidentMetrics(incident, "detected", "")
    - autoResolveService.CheckAndResolve(incident) — auto-resolve if matches noise rule                                        
    - If severity == critical: whatsappService.Send(alert) + slackService.CreateIncidentChannel(incident)                      
                                                                                                                               
  ---                                                                                                                          
  File: backend/internal/store/incident_store.go                                                                               
                                                                                                                               
  AddIncident(i models.Incident) error                      
  - SQL: large INSERT INTO incidents (...) VALUES (...) with ~35 columns
  - Includes all correlation fields, intelligence fields, what-changed fields, JSON arrays                                     
   
  FindOpenIncidentByFingerprint(fingerprint string) (*Incident, bool)                                                          
  - SQL: SELECT ... FROM incidents WHERE fingerprint=$1 AND status IN ('open','acknowledged') ORDER BY last_event_time DESC 
  LIMIT 1                                                                                                                      
                                                            
  FindOpenIncidentForService(service string, since time.Time) (*Incident, bool)                                                
  - SQL: SELECT ... FROM incidents WHERE service=$1 AND status IN ('open','acknowledged') AND last_event_time > $2 ORDER BY 
  last_event_time DESC LIMIT 1                                                                                                 
   
  UpdateIncident(i models.Incident) error                                                                                      
  - SQL: UPDATE incidents SET last_event_time=..., event_count=..., severity=..., correlation_score=..., confidence=..., 
  risk_score=..., priority_score=... WHERE id=$1                                                                               
                                                
  ListIncidents(filter IncidentFilter) ([]IncidentListItem, int)                                                               
  - SQL: WHERE chain with tenant_id + optional status, severity, service LIKE, search FULLTEXT                                 
  - ORDER BY priority_score DESC, last_event_time DESC                                                                         
  - LIMIT/OFFSET for pagination                                                                                                
  - Returns IncidentListItem (lighter struct — no events, no full detail)                                                      
                                                            
  ---                                                                                                                          
  Phase 4 — API Read: List View
                                                                                                                               
  File: backend/internal/handlers/incident_handler.go       
                                                                                                                               
  ListIncidents(w, r)                                                                                                          
  - Reads query params: status, severity, service, search, page, pageSize, sort
  - Builds IncidentFilter                                                                                                      
  - Calls incidentService.ListIncidents(filter)             
  - Writes JSON response                                                                                                       
                                                            
  File: backend/internal/models/incident_list.go                                                                               
   
  IncidentListResponse:                                                                                                        
  {                                                         
    items:    []IncidentListItem  // lightweight, for table/card views
    page:     int                                                                                                              
    pageSize: int                                                                                                              
    total:    int                                                                                                              
    hasMore:  bool                                                                                                             
  }                                                         
   
  IncidentListItem — fields sent to dashboard list view:
  id, service, severity, status, title,                                                                                        
  first_event_time, last_event_time, event_count,
  confidence, risk_score, priority_score,                                                                                      
  root_cause_summary, root_cause_type,                      
  correlation_pattern, fingerprint,                                                                                            
  seen_before, recurring_count                              
                                                                                                                               
  ---                                                       
  Phase 5 — API Read: Incident Detail View
                                                                                                                               
  File: backend/internal/handlers/incident_handler.go
                                                                                                                               
  GetIncidentDetail(w, r)                                   
  - Extracts {id} from path
  - Calls incidentDetailService.GetIncidentDetail(id)                                                                          
  - Returns full models.IncidentDetail JSON          
                                                                                                                               
  File: backend/internal/services/incident_detail_service.go                                                                   
   
  GetIncidentDetail(id) models.IncidentDetail — assembles from many sources:                                                   
                                                            
  ┌──────┬───────────────────────────┬─────────────────────────────────────────────────────────────────┐                       
  │ Step │       What it does        │                      Store/Service called                       │
  ├──────┼───────────────────────────┼────────────────────────────────────────────────────────────────────┤                    
  │ 1    │ Fetch base incident       │ incidentStore.GetIncidentByID(id)                                  │
  ├──────┼───────────────────────────┼────────────────────────────────────────────────────────────────────┤
  │ 2    │ Fetch all linked events   │ eventStore.GetEventsByIDs(incident.EventIDs)                       │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 3    │ Wrap events into timeline  │ wraps each as TimelineEvent{event, signalType}                     │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 4    │ Build IncidentSummary      │ aggregates event count, severity distribution, confidence, risk    │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 5    │ Classify incident          │ classifyIncident(events) → database_failure, timeout, memory, etc. │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 6    │ Fetch linked change        │ changeStore.FindChangeInWindow(service, firstEvent, lastEvent)     │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 7    │ Build recommended actions  │ actionService.BuildActions(incident, insight)                      │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 8    │ Fetch status audit history │ historyStore.GetHistory(id)                                        │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 9    │ Build lifecycle timeline   │ reconstructs detected→ack→investigating→mitigated→resolved stages  │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 10   │ Fetch context logs         │ logStore.GetLogsAroundTime(service, timestamp, ±10min)             │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 11   │ Fetch context metrics      │ metricsStore.GetSnapshot(service, timestamp)                       │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 12   │ Build topology graph       │ topologyService.BuildGraph(service, incident)                      │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 13   │ Fetch pattern history      │ incidentMemoryService.GetPatternHistory(fingerprint)               │
  ├──────┼────────────────────────────┼────────────────────────────────────────────────────────────────────┤                   
  │ 14   │ Run AI enrichment          │ explainService.Explain(&detail) (see Phase 6)                      │
  └──────┴────────────────────────────┴────────────────────────────────────────────────────────────────────┘                   
                                                            
  ---                                                                                                                          
  File: backend/internal/models/incident_detail.go          
                                                  
  Full struct returned to the frontend:
                                                                                                                               
  IncidentDetail {
    Incident            // base record (id, service, severity, status, ...)                                                    
    Events              []TimelineEvent       // every raw alert in time order                                                 
    Summary             IncidentSummary       // stats: count, spread, duration                                                
    Insight             IncidentInsight       // why_this_is_likely, affected_area                                             
    Impact              ImpactAnalysis        // blast radius: impacted services count                                         
    Actions             []Action              // full recommended action list                                                  
    PrimaryAction       *Action               // best first step (low-risk, no approval)                                       
    DecisionCard        DecisionCard          // "what to do right now" card                                                   
    WhatChanged         WhatChanged           // linked deployment if found                                                    
    Narrative           string                // LLM or template explanation                                                   
    StatusAudit         []IncidentStatusAudit // who changed status + when                                                     
    Graph               GraphData             // service dependency graph                                                      
    Evidence            []string              // evidence sentences                                                            
    RecommendedNextStep string                // single concrete next step                                                     
    ResolutionSteps     []string              // KB playbook steps                                                             
    OccurrenceTimes     []string              // past occurrence timestamps                                                    
    ContextLogs         []ContextLogEntry     // error logs near incident time                                                 
    ContextMetrics      *ContextMetrics       // CPU/mem/disk at incident time                                                 
    Lifecycle           []LifecycleEntry      // stage-by-stage progression                                                    
    ActionExecutions    []ActionExecution     // audit trail of past actions                                                   
    TopologyGraph       *GraphData            // enriched topology with incident node                                          
    PatternHistory      *PatternHistory       // past resolutions for this pattern                                             
  }                                                                                                                            
                                                                                                                               
  ---                                                                                                                          
  Phase 6 — AI Enrichment                                                                                                      
                                                            
  File: backend/internal/services/explain_service.go
                                                                                                                               
  Explain(detail *IncidentDetail) — runs after detail assembly, mutates the detail in-place                                    
                                                                                                                               
  Path A (LLM configured):                                                                                                     
  1. AssembleForRCA(detail) → builds structured context: title, severity, events, root cause hypothesis, what changed, related
  incidents                                                                                                                    
  2. llmClient.Complete(systemPrompt, userPrompt) → calls OpenAI/Anthropic API
  3. Parse JSON response: { root_cause_summary, root_cause_type, confidence, risk_score, reasoning[], recommended_next_step,   
  narrative }                                                                                                                  
  4. Overwrite detail.Incident.RootCauseSummary, Confidence, RiskScore, Reasoning                                              
  5. buildEvidenceRefs(detail) → concrete data pointers: top 5 events + 3 log lines + change + metric spikes                   
  6. Set detail.Narrative, detail.RecommendedNextStep                                                                          
                                                                                                                               
  Path B (no LLM):                                                                                                             
  - buildTemplateNarrative(detail) → deterministic sentence assembly:                                                          
  "Payments API is experiencing a critical incident. Database connectivity failure detected. It is affecting 3 services. This  
  issue has been seen before 2 times."                      
                                                                                                                               
  ---                                                       
  File: backend/internal/services/copilot_service.go                                                                           
                                                            
  Answer(detail, question) models.CopilotAnswer
                                                                                                                               
  1. classifyCopilotIntent(question) — regex on question:                                                                      
    - "why" / "cause" / "happening" → intent = "why"                                                                           
    - "do first" / "next step" / "action" → intent = "first_action"                                                            
    - "changed" / "deploy" / "release" → intent = "change"                                                                     
    - "before" / "history" / "recurring" → intent = "history"                                                                  
    - else → intent = "general"                                                                                                
  2. If LLM available: full LLM call with assembled context + question
  3. Else: rule-based per-intent answer builder                                                                                
                                                                                                                               
  Returns:                                                                                                                     
  {                                                                                                                            
    "intent": "why",                                                                                                           
    "answer": "Payments is in critical state because of a database connectivity failure...",
    "suggested_followups": ["What changed recently?", "What should I do first?", "Has this happened before?"]                  
  }                                                                                                                            
                                                                                                                               
  ---                                                                                                                          
  Phase 7 — Action Execution                                                                                                   
                                                                                                                               
  File: backend/internal/services/action_service.go
                                                                                                                               
  BuildActions(incident, insight) []models.Action           

  Sources of actions (in priority order):                                                                                      
  1. Knowledge base match → pre-written resolution steps for this pattern
  2. Incident type → type-specific defaults (e.g., database_failure → "Check DB connections", "Restart connection pool")       
  3. Change-linked → "Rollback deployment on {service}: {version}"                                                      
  4. Recurrence-based → "Apply previous resolution: {last resolution steps}"                                                   
                                                                                                                               
  Each action has:                                                                                                             
                                                                                                                               
  Action {                                                                                                                     
    ID              string   // "act-{nano}"                
    Label           string   // "Check Database Connections"                                                                   
    Description     string   // full command or steps       
    Type            string   // diagnostic|remediation|investigation|coordination|verification                                 
    Severity        string   // mirrors incident severity                                     
    RiskLevel       string   // low|medium|high                                                                                
    RequiresApproval bool                      
    Status          string   // pending|approved|executing|done|rejected                                                       
  }                                                                                                                            
   
  File: backend/internal/handlers/action_handler.go                                                                            
                                                                                                                               
  ExecuteAction(w, r)
                                                                                                                               
  1. Decode { action_id, incident_id, approved } from request body                                                             
  2. Extract shell command from action description (pattern match for known safe commands)
  3. Find online agent: tries localhost:8765 then registered agent IPs                                                         
  4. POST { command } to http://{agent}:8765/execute (35s timeout)                                                             
  5. Write ActionAudit to action_audits table: command, output, status, executed_at
  6. Return:                                                                                                                   
  {                                                         
    "status": "executed",                                                                                                      
    "command": "df -h",                                     
    "output": "Filesystem  Size  Used  Avail  Use%\n/dev/sda1   100G   87G   13G   87%",
    "agent": "192.168.1.10"
  }                                                                                                                            
   
  ---                                                                                                                          
  Phase 8 — Source Management                               
                                                                                                                               
  File: backend/internal/handlers/source_handler.go
                                                                                                                               
  Manages the source_connections table — where does ingest come from?                                                          
   
  GET  /api/v1/sources              → list all source connections for tenant                                                   
  POST /api/v1/sources              → create source connection (returns X-Source-Token)                                        
  GET  /api/v1/sources/{id}         → get single source                                                                        
  PUT  /api/v1/sources/{id}         → update source                                                                            
  DELETE /api/v1/sources/{id}       → remove source                                                                            
  POST /api/v1/sources/{id}/rotate  → rotate X-Source-Token                                                                    
   
   
   
   =========================================================================================================================================================================                                                    
                                                                                                                               
● PART 1 — THE OUTSIDE WORLD: WHERE ALERTS ARE BORN                                                                            
                                         
  Before a single line of your code runs, the alert exists somewhere else entirely. Understanding the source changes how you   
  understand the ingest path.
                                                                                                                               
  1A. Prometheus + AlertManager (most common)                                                                                  
   
  What happens in the tool:                                                                                                    
                                                            
  You have a Prometheus rule file on your infrastructure server:         
  # /etc/prometheus/rules/payments.yml                      
  groups:                                                                                                                      
    - name: payments                                                                                              //todo : what if we have motadata             
      rules:        
        - alert: HighErrorRate                                                                                                 
          expr: rate(http_requests_total{status=~"5..",job="payments-api"}[5m]) > 0.05
          for: 2m                                                                     
          labels:                                                                                                              
            severity: critical
            service: payments-api                                                                                //what exacty is prometheus and how it works              
            environment: prod                               
          annotations:       
            summary: "High error rate on payments-api"                                                                         
            description: "Error rate is {{ $value | printf \"%.2f\" }}% over last 5 minutes"
                                                                                                                               
  Prometheus evaluates this expression every 15 seconds. When rate(http_requests_total...) stays above 0.05 for 2 continuous   
  minutes, Prometheus marks it FIRING and sends it to AlertManager.                                                            
                                                                                                                               
  AlertManager then batches it and POSTs to your configured webhook receiver — which is this platform's ingest endpoint. The   
  JSON it sends looks like:                                 
  {                                                                                                             //todo waht extactly is alertmanaeger and what it do               
    "alerts": [{                                            
      "status": "firing",
      "labels": {        
        "alertname": "HighErrorRate",
        "service": "payments-api",   
        "severity": "critical",   
        "instance": "pod-payments-7d9f8b-xk2pl",                                                                               
        "environment": "prod"                   
      },                                                                                                                       
      "annotations": {                                      
        "summary": "High error rate on payments-api",                                                                          
        "description": "Error rate is 8.34% over last 5 minutes"                                                               
      },                                                        
      "startsAt": "2026-04-23T14:32:00Z",                                                                                      
      "fingerprint": "a1b2c3d4e5f60001"                     
    }]                                                                                                                         
  }                                                                                                                            
                                                                                                                               
  How to configure AlertManager to send to this platform:                                                                      
  # alertmanager.yml                                                                                                           
  receivers:        
    - name: aiops                                                                                                              
      webhook_configs:                                      
        - url: "https://your-platform/api/v1/ingest/prometheus"
          http_config:                                         
            headers:                                                                                                           
              X-Source-Token: "your-source-token-from-platform"                                            //todo how to get this token
                                                                                                                               
  1B. The NeuroOps Agent (for self-hosted / on-premise hosts)                                                                  
                                                                                                                               
  The agent is a separate binary (neuroops-agent) that runs directly on each monitored host. It does not need Prometheus. It   
  watches the host itself — processes, logs, CPU, disk, memory — and sends events directly to the backend.                     
                                                                                                                               
  How to install the agent on a host:                                                                                          
  # On the monitored host
  cd neuroops-agent                                                                                                            
  ./neuroops-agent \                                        
    --server https://your-platform \                                                                                           
    --agent-id $(hostname) \        
    --enrollment-token enroll_<64hex>   # issued from the platform                               // todo how to get this token                              
                                                                                                                               
  On first run, the agent calls POST /api/v1/agents/register with the enrollment token. The backend issues a one-time          
  AgentSecret (agt_<64hex>). The agent stores this secret on disk. For every subsequent batch of events, the agent signs the   
  HTTP request using HMAC-SHA256 with that secret and sends to POST /api/v1/ingest/agent.                                      
                                                                                                                               
  The agent detects things like:                                                                                               
  - Process crashes (systemd unit entered failed state)
  - Disk usage > 90%                                                                                                           
  - CPU sustained > 85%                                     
  - Log lines matching error patterns                                                                                          
  - Port unreachable (connection refused)                   
                                                                                                                               
  It batches these into event objects and posts them every N seconds.
                                                                                                                               
  1C. Cloud Services — Datadog, PagerDuty, GitHub, GitLab                                                                      
                                                                                                                               
  These send webhooks automatically when their own alerts trigger. They each have dedicated ingest handlers                    
  (/api/v1/ingest/datadog, /api/v1/ingest/pagerduty, etc.) but they all convert their payload format into the same models.Event
   struct and feed it into the same correlation pipeline. The flow is identical after normalization.                           
                                                            
  ---                                                                                                                          
  PART 2 — SERVER STARTUP: HOW EVERYTHING IS WIRED
                                                                                                                               
  File: backend/cmd/server/main.go                          
                                                                                                                               
  Before any alert can arrive, main() must construct the entire dependency graph. This is the wiring layer — nothing processes 
  alerts here, but if this is wrong, nothing works.
                                                                                                                               
  What happens, in order:                                                                                                      
  
  main() calls loadDotEnv()           → reads .env file into os.Setenv                                                         
  main() calls config.Load()          → builds Config struct from env vars                                                     
  main() calls store.NewDB(cfg)       → opens postgres connection, runs all SQL migrations
  main() creates all stores:                                                                                                   
      NewEventStore(db)               → wraps db for events table
      NewIncidentStore(db)            → wraps db for incidents table                                                           
      NewAgentStore(db)               → wraps db for agents table
      NewAgentEnrollmentStore(db)     → wraps db for agent_enrollment_tokens table                                             
      ... (20+ stores total)                                                                                                   
  main() creates all services:
      NewCorrelationService(incidentStore, eventStore, changeStore)                                                            
      NewIncidentDetailService(incidentStore, eventStore, historyStore, logStore, agentStore)
      NewIncidentService(incidentStore, detailService, historyStore)                                                           
      NewExplainService(llmClient)                                                                                             
      ... (15+ services total)                                                                                                 
  main() calls correlationService.SetSlackService(...)   → optional wiring                                                     
  main() calls correlationService.SetWhatsAppService()  → optional wiring                                                      
  ... (all the Set* dependency injections)
  main() creates all handlers:                                                                                                 
      NewIngestHandler(eventStore, correlationService, sourceRegistryService)
      NewIncidentHandler(incidentService, incidentStore)                                                                       
      NewAgentHandler(agentStore, enrollmentStore, ingestionService, secretKey, devToken)
      ...                                                                                                                      
  main() calls routes.RegisterRoutes(mux, cfg, all handlers...)
  main() starts http.ListenAndServe(":8080", mux)                                                                              
                                                            
  The critical dependency chain for our alert: IngestHandler → CorrelationService → IncidentStore + EventStore. 
   
     PART 3 — THE ALERT ARRIVES: HTTP BOUNDARY                 
                                                                                                                               
  File: backend/internal/handlers/ingest_handler.go         
                                                                                                                               
  The HTTP server receives the AlertManager POST to /api/v1/ingest/prometheus.
                                                                                                                               
  Before PrometheusWebhook() even runs, the middleware stack executes:                                                         
   
  RequestID middleware        → generates request-id header, attaches to context                                               
  RequestLogger middleware    → logs: "POST /api/v1/ingest/prometheus 200 12ms"                                                
  CORS middleware             → checks Origin header, adds Access-Control headers                                              
                                                                                                                               
  Note: ingest endpoints do NOT require JWT auth. Instead they use X-Source-Token.                                             
                                                            
  Now PrometheusWebhook() runs (line 114):                                                                                     
                                                            
  Step 1 — Tenant Resolution (line 115, calls resolveTenantForIngest()):                                                       
  Check if request has valid Bearer JWT → call middleware.TenantFromRequestStrict(r)
    If yes → return tenantID from JWT claims (authenticated caller path)                                                       
    If no  → read r.Header.Get("X-Source-Token")                                                                               
             call sourceRegistryService.FindByToken(token)                                                                     
             If found → return (tenantID, sourceID, true)                                                                      
             If not   → return ("", "", false) → respond 401                                                                   
                                                                                                                               
  In our example, AlertManager sends X-Source-Token: src_abc123. The source registry finds the token in the DB, returns        
  tenantID = "default" and sourceID = "src_alerts_prod".                                                                       
                                                            
  Step 2 — Parse the AlertManager payload (line 121-133):                                                                      
                                                            
  The handler defines an inline anonymous struct matching AlertManager's schema exactly.                                       
  json.NewDecoder(r.Body).Decode(&payload) fills it.        
                                                                                                                               
  Step 3 — Loop over each alert (line 136-183):                                                                                
   
  For each alert in payload.Alerts:                                                                                            
                                                            
  ts, _ := time.Parse(time.RFC3339, a.StartsAt)
  // ts = 2026-04-23T14:32:00Z                                                                                                 
                                                                                                                               
  eventID = a.Fingerprint  // "a1b2c3d4e5f60001" from Prometheus                                                               
  // if fingerprint is empty: generate "prom-{nanotime}"                                                                       
                                                                                                                               
  service = a.Labels["service"]   // "payments-api"         
  // fallback: a.Labels["alertname"]                                                                                           
  // fallback: "unknown-service"                                                                                               
   
  severity = a.Labels["severity"] // "critical"                                                                                
  // fallback: "medium"                                     
                                                                                                                               
  title = a.Annotations["summary"]  // "High error rate on payments-api"
  // fallback: a.Labels["alertname"]                                                                                           
                                                                                                                               
  event = models.Event{
      ID:          "a1b2c3d4e5f60001",                                                                                         
      TenantID:    "default",          // stamped here — never from body
      Source:      "prometheus",                                                                                               
      Service:     "payments-api",                                                                                             
      Severity:    "critical",                                                                                                 
      Type:        "alert",                                                                                                    
      Title:       "High error rate on payments-api",       
      Message:     "Error rate is 8.34% over last 5 minutes",                                                                  
      Labels:      a.Labels,           // full labels map preserved
      Timestamp:   2026-04-23T14:32:00Z,                                                                                       
      Fingerprint: "a1b2c3d4e5f60001",                      
  }                                                                                                                            
                                                            
  Step 4 — First DB write + hand off to correlation (line 180-181):                                                            
  h.eventStore.SaveEvent(event)          // write to events table
  h.correlationService.ProcessEvent(event)  // begin correlation pipeline                                                      
                                                                         
  Step 5 — Record source health (line 185-187):                                                                                
  h.sourceRegistryService.RecordSuccess(sourceID, count)                                                                       
  // updates source_connections.last_event_at, event_count for health dashboard                                                
                                                                                                                               
  The handler returns 200 OK with an empty body. AlertManager considers it delivered.                                          
                                                                                                                               
  ---                                                                                                                          
  PART 4 — EVENT PERSISTENCE: THE IMMUTABLE LOG                                                                                
                                                                                                                               
  File: backend/internal/store/event_store.go — SaveEvent() (line 245)
                                                                                                                               
  This is the first database write. The events table is an append-only audit log.                                              
                                                                                                                               
  func (s *EventStore) SaveEvent(e models.Event) {                                                                             
      // Stamp "default" if tenantID somehow slipped through empty
      tenantID := e.TenantID  // "default"                                                                                     
                                                                                                                               
      _, _ = s.db.Exec(`                                                                                                       
          INSERT INTO events (id, tenant_id, source, service, severity, title, message, timestamp, fingerprint)                
          VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)                                                                                  
          ON CONFLICT (id) DO UPDATE
            SET fingerprint = EXCLUDED.fingerprint,                                                                            
                title       = EXCLUDED.title,               
                tenant_id   = EXCLUDED.tenant_id                                                                               
      `,                                                                                                                       
          "a1b2c3d4e5f60001",  // id
          "default",           // tenant_id                                                                                    
          "prometheus",        // source                                                                                       
          "payments-api",      // service
          "critical",          // severity                                                                                     
          "High error rate...",// title                     
          "Error rate is...",  // message
          2026-04-23T14:32:00Z, // timestamp                                                                                   
          "a1b2c3d4e5f60001",  // fingerprint
      )                                                                                                                        
  }                                                         
                                                                                                                               
  Why ON CONFLICT DO UPDATE? Because ProcessEvent() (next step) will call SaveEvent() again after computing a fingerprint. The 
  second call updates fingerprint and title in-place without creating a duplicate row.                                         
                                                                                                                               
  DB state after this write:                                
  events table:
    id          = "a1b2c3d4e5f60001"
    tenant_id   = "default"         
    source      = "prometheus"                                                                                                 
    service     = "payments-api"
    severity    = "critical"                                                                                                   
    title       = "High error rate on payments-api"         
    message     = "Error rate is 8.34% over last 5 minutes"
    timestamp   = 2026-04-23 14:32:00 UTC                                                                                      
    fingerprint = "a1b2c3d4e5f60001"     
                                                                                                                               
  ---                                                                                                                          
  PART 5 — CORRELATION ENGINE: THE BRAIN OF THE SYSTEM
                                                                                                                               
  File: backend/internal/services/correlation_service.go — ProcessEvent() (line 96)
                                                                                                                               
  This is the most important function in the entire backend. Everything that determines whether a new incident is created, or  
  an existing one is updated, happens here.                                                                                    
                                                                                                                               
  Step 5A — Fingerprint Computation (line 97-104)                                                                              
   
  if event.Fingerprint == "" {                                                                                                 
      event.Fingerprint = computeEventFingerprint(event)    
  }

  In our case, Prometheus already sent fingerprint = "a1b2c3d4e5f60001" so this branch is skipped. But for generic webhooks or 
  agent events that don't have a fingerprint, computeEventFingerprint() (line 147) runs:
                                                                                                                               
  func computeEventFingerprint(event models.Event) string { 
      src   = "prometheus"
      svc   = "payments-api"
      sev   = "critical"                                                                                                       
      title = normalizeAlertTitle("high error rate on payments-api")
      //   normalizeAlertTitle strips instance tokens: pod names, UUIDs, IPs, numeric IDs                                      
      //   "pod-payments-7d9f8b-xk2pl" → stripped (length >= 10, has dashes, has digits)                                       
      //   result: "high error rate on payments-api"                                                                           
                                                                                                                               
      raw = "prometheus|payments-api|critical|high error rate on payments-api"                                                 
      h   = sha256.Sum256([]byte(raw))                      
      return fmt.Sprintf("%x", h[:8])  // 16 hex chars: "3f8a1c2d4e5b6f70"                                                     
  }                                                                                                                            
                                                                                                                               
  This deterministic hash means 100 identical alerts from different pods all share the same fingerprint. That is how the dedup 
  window works.                                             
                                                                                                                               
  The second SaveEvent() call (line 104) updates the fingerprint in the events table row.                                      
   
  Step 5B — Suppression Check (line 107-110)                                                                                   
                                                            
  if s.alertFeedbackService != nil && s.alertFeedbackService.IsSuppressed(event.Fingerprint) {                                 
      log.Printf("correlation: suppressed event %s (fingerprint %s marked as noise)", ...)                                     
      return false                                                                                                             
  }                                                                                                                            
                                                                                                                               
  If an operator previously marked this fingerprint as noise (via the alert feedback UI), this event is silently dropped. No   
  incident created. This is how you teach the system which alerts are not worth seeing.
                                                                                                                               
  Step 5C — Dedup Window Check (line 112-117)                                                                                  
   
  dedupWindowStart = 14:27:00  // 5 minutes before event timestamp 14:32:00                                                    
                                                                                                                               
  s.eventStore.FingerprintSeenInWindow("a1b2c3d4e5f60001", dedupWindowStart, "a1b2c3d4e5f60001")                               
                                                                                                                               
  This runs the query (event_store.go line 273):                                                                               
  SELECT COUNT(*) FROM events                               
  WHERE fingerprint = 'a1b2c3d4e5f60001'                                                                                       
    AND timestamp   >= '2026-04-23 14:27:00'                
    AND id         != 'a1b2c3d4e5f60001'    
                                                                                                                               
  If count > 0 — another event with the same fingerprint arrived in the last 5 minutes — this event is suppressed (function
  returns false). This collapses alert storms: 50 identical alerts fire → 1 gets through, 49 are silently dropped.             
                                                            
  In our example this is the first alert, so count = 0 and we continue.                                                        
                                                            
  Step 5D — Fingerprint Merge (line 121)                                                                                       
                                                            
  existing := s.incidentStore.FindOpenIncidentByFingerprint("a1b2c3d4e5f60001")
                                                                                                                               
  SQL: SELECT * FROM incidents WHERE fingerprint = $1 AND status IN ('open','acknowledged') LIMIT 1                            
                                                                                                                               
  If this fingerprint has an open incident — meaning this exact alert has fired before and is still active — merge into it, not
   create a new one. This handles alerts that briefly resolve and immediately re-fire.
                                                                                                                               
  In our example, no open incident has this fingerprint → existing = nil.                                                      
   
  Step 5E — Time-Window Correlation (line 124-127)                                                                             
                                                            
  corrWindowStart = 14:27:00  // 5 minutes before event
                                                                                                                               
  existing = s.findRelatedIncident(event, corrWindowStart)                                                                     
                                                                                                                               
  findRelatedIncident() (line 357) checks three things:                                                                        
                                                            
  a) Same service (line 361):                                                                                                  
  SELECT * FROM incidents                                   
  WHERE service = 'payments-api'
    AND status IN ('open','acknowledged')
    AND last_event_time >= '2026-04-23 14:27:00'                                                                               
  ORDER BY last_event_time DESC                 
  LIMIT 1                                                                                                                      
                                                                                                                               
  b) Dependent services (line 363-370):
  relatedServices := getRelatedServices("payments-api")                                                                        
  // checks config.ServiceDependencies map                  
  // e.g. payments-api depends on: ["postgres", "redis", "auth-service"]                                                       
  // also finds services that depend ON payments-api                                                                           
                                                                                                                               
  Then queries: WHERE service IN ('postgres','redis','auth-service') AND status IN ('open','acknowledged') AND last_event_time 
  >= windowStart                                                                                                               
                                                            
  This is cross-service correlation: if postgres already has an open incident and now payments-api alerts, it merges them into 
  one — because the DB issue caused the payment service issue.
                                                                                                                               
  In our example, this is the first alert in 5 minutes for payments-api → existing = nil.                                      
   
  Step 5F — Create New Incident (line 135-137)                                                                                 
                                                            
  Both merge paths returned nil. We call createIncident(event) (line 264).                                                     
                                                            
  ---                                                                                                                          
  PART 6 — INCIDENT CREATION: BUILDING THE RECORD           
                                                                                                                               
  File: backend/internal/services/correlation_service.go — createIncident() (line 264)
                                                                                                                               
  This function builds the full models.Incident struct from the raw event.                                                     
   
  Step 6A — Root Cause & Reasoning (line 265-268)                                                                              
                                                            
  rootCause = buildDefaultRootCause(event)                                                                                     
                                                                                                                               
  buildDefaultRootCause() (line 473): strips prefixes from event.Title or event.Message to produce a clean root cause string.  
  For our event: "Error rate is 8.34% over last 5 minutes".                                                                    
                                                                                                                               
  reasoning = buildReasoning(event, rootCause)              
                                                                                                                               
  buildReasoning() (line 498): builds a []string of diagnostic reasoning lines by pattern-matching the event message:          
  ["Detected critical-severity event from prometheus on service payments-api",                                                 
   "Root cause analysis: Error rate is 8.34% over last 5 minutes"]                                                             
                                                                                                                               
  If the message contained "timeout": "Request timeout indicates either the downstream service is overloaded..." would be      
  appended.                                                                                                                    
  If it contained "connection refused": "Network connectivity failure detected..." would be appended.
                                                                                                                               
  Step 6B — Knowledge Base Enrichment (line 272-284)                                                                           
                                                                                                                               
  if s.knowledgeBase != nil {                                                                                                  
      metricName = event.Labels["metric.name"]  // "" for prometheus
      if entry := s.knowledgeBase.Match(event.Title, event.Message, metricName); entry != nil {                                
          rootCause          = entry.RootCause                                                                                 
          reasoning          = entry.Reasoning                                                                                 
          confidence         = entry.BaseConfidence                                                                            
          riskScore          = entry.BaseRiskScore                                                                             
          correlationPattern = "kb_match:" + entry.ID
      }                                                                                                                        
  }                                                         
                                                                                                                               
  If a previous incident for the same type of alert was resolved and recorded, the knowledge base can override the default root
   cause with the human-verified root cause from the last resolution. This is how the system learns.
                                                                                                                               
  Step 6C — Severity-Calibrated Scores (line 269-270)                                                                          
   
  confidence = initialConfidence("critical")  // → 75                                                                          
  riskScore  = initialRiskScore("critical")   // → 90       
                                                                                                                               
  From the switch table in correlation_service.go:
  - critical: confidence=75, riskScore=90                                                                                      
  - high: confidence=65, riskScore=70                                                                                          
  - medium: confidence=55, riskScore=50
  - low: confidence=45, riskScore=30                                                                                           
                                                            
  Step 6D — Root Cause Type Classification (line 526-542)                                                                      
   
  classifyRootCauseType(event)                                                                                                 
                                                                                                                               
  Scans event.Title + " " + event.Message for keywords:                                                                        
  - "database", "db", "sql", "redis" → "database"                                                                              
  - "timeout" → "timeout"                                                                                                      
  - "memory", "oom" → "memory"                              
  - "cpu" → "cpu"                                                                                                              
  - "network", "connect" → "network"                        
  - default → "service_alert"                                                                                                  
   
  Our message "Error rate is 8.34% over last 5 minutes" → "service_alert".                                                     
                                                            
  Step 6E — Priority Score (line 310, computePriorityScore() at line 629)                                                      
                                                            
  score = 0                                                                                                                    
  // severity: critical → +40                               
  score += 40                                                                                                                  
  // event_count: 1 event × 4 = 4 → +4
  score += 4                                                                                                                   
  // risk_score: 90 / 5 = 18 → +18                          
  score += 18                                                                                                                  
  // seen_before: false → +0                                
  // impact_count: 0 × 3 = 0 → +0                                                                                              
  // total = 62                                                                                                                
  incident.PriorityScore = 62
                                                                                                                               
  Step 6F — Build the Incident Struct (line 287-308)        
                                                                                                                               
  incident = models.Incident{                               
      ID:                 "inc-1745416320000000001",  // "inc-" + nanosecond timestamp                                         
      Title:              "High error rate on payments-api",                                                                   
      Service:            "payments-api",                                                                                      
      Severity:           "critical",                                                                                          
      Status:             "open",                           
      FirstEventTime:     "2026-04-23T14:32:00Z",                                                                              
      LastEventTime:      "2026-04-23T14:32:00Z",
      EventCount:         1,                                                                                                   
      EventIDs:           ["a1b2c3d4e5f60001"],                                                                                
      Fingerprint:        "a1b2c3d4e5f60001",
      TenantID:           "default",                                                                                           
      CorrelationPattern: "new_incident",                   
      CorrelationScore:   50,                                                                                                  
      CorrelationReason:  "First alert for service 'payments-api'",                                                            
      Confidence:         75,                                                                                                  
      RiskScore:          90,                                                                                                  
      RootCauseSummary:   "Error rate is 8.34% over last 5 minutes",                                                           
      RootCauseType:      "service_alert",                                                                                     
      Reasoning:          ["Detected critical-severity event from prometheus...", ...],
      PriorityScore:      62,                                                                                                  
  }                                                                                                                            
   
  ---                                                                                                                          
  PART 7 — INCIDENT PERSISTENCE: WRITING TO DATABASE        
                                                    
  File: backend/internal/store/incident_store.go — AddIncident() (line 25)
                                                                                                                               
  This executes a 33-column INSERT into the incidents table:                                                                   
                                                                                                                               
  INSERT INTO incidents (                                                                                                      
      id, service, severity, status,                        
      first_event_time, last_event_time,
      title,                                                                                                                   
      correlation_pattern, correlation_score, correlation_reason,
      confidence, risk_score, event_count,                                                                                     
      root_cause_summary, root_cause_type, reasoning_json,                                                                     
      what_changed_type, what_changed_service, what_changed_version,
      what_changed_description, what_changed_timestamp,                                                                        
      impacted_services_json, impact_count,                 
      seen_before, recurring_count, similar_incident_id, last_seen_at,                                                         
      fingerprint, tenant_id,                                                                                                  
      parent_incident_id, merged_incident_ids, is_merged,                                                                      
      priority_score                                                                                                           
  ) VALUES (                                                
      'inc-1745416320000000001', 'payments-api', 'critical', 'open',                                                           
      '2026-04-23T14:32:00Z', '2026-04-23T14:32:00Z',                                                                          
      'High error rate on payments-api',
      'new_incident', 50, 'First alert for service payments-api',                                                              
      75, 90, 1,                                                                                                               
      'Error rate is 8.34% over last 5 minutes', 'service_alert',
      '["Detected critical-severity event..."]',                                                                               
      '', '', '', '', '',                                   
      '[]', 0,                                                                                                                 
      false, 0, '', '',                                     
      'a1b2c3d4e5f60001', 'default',                                                                                           
      '', '[]', false,                                      
      62                                                                                                                       
  )                                                         
                                                                                                                               
  reasoning_json and impacted_services_json are JSON-serialized arrays stored as TEXT columns. They are deserialized back into 
  []string when reading.
                                                                                                                               
  There is also a call to AddEventToIncident() which inserts into the junction table:                                          
  INSERT INTO incident_events (incident_id, event_id)
  VALUES ('inc-1745416320000000001', 'a1b2c3d4e5f60001')                                                                       
  ON CONFLICT DO NOTHING                                                                                                       
                        
  DB state after both writes:                                                                                                  
  incidents table: one row fully populated                                                                                     
  incident_events table: one row linking incident to event
                                                                                                                               
  ---                                                       
  PART 8 — ASYNC SIDE-EFFECTS: NOTIFICATIONS & LEARNING                                                                        
                                                                                                                               
  Back in createIncident() (line 314-354), after the DB write, several goroutines are launched:
                                                                                                                               
  8A — Onboarding Milestone (line 315-318)                                                                                     
                                                                                                                               
  go func(tenantID string) {                                                                                                   
      s.onboardingService.MarkMilestone(tenantID, "incident_created")
  }(incident.TenantID)                                                                                                         
  Marks that this tenant has had their first incident — unlocks the next step in the onboarding UI wizard. Runs async so it
  doesn't delay the response.                                                                                                  
                                                            
  8B — Engineering Health Metrics (line 322-323)                                                                               
                                                                                                                               
  s.engineeringHealthService.RecordIncidentMetrics(incident, "detected", "")
  Records detected_at timestamp for MTTR calculation. When the incident is later resolved, resolved_at is recorded and MTTR =  
  resolved_at - detected_at.                                                                                                   
                                                                                                                               
  8C — Auto-Resolve Rules (line 327-331)                                                                                       
                                                            
  resolved, ruleName := s.autoResolveService.CheckAndResolve(incident)
  If there is a rule like "auto-resolve incidents with title matching /test/" this would immediately transition the incident to
   resolved and skip all notifications.                                                                                        
                                                                                                                               
  8D — WhatsApp Alert for Critical (line 335-339)                                                                              
                                                            
  if incident.Severity == "critical" && s.whatsappService != nil {
      go func(inc models.Incident) {
          s.whatsappService.SendIncidentAlert(inc.TenantID, inc)                                                               
      }(incident)
  }                                                                                                                            
  Fires only for critical severity. Async — does not block. 
                                                                                                                               
  8E — Slack Channel Creation for Critical (line 342-353)
                                                                                                                               
  if incident.Severity == "critical" && s.slackService != nil && s.slackService.IsConfigured() {
      go func(inc models.Incident) {                                                                                           
          channelID, _ := s.slackService.CreateIncidentChannel(inc)
          s.slackService.PostIncidentAlert(channelID, inc, inc.RootCauseSummary)                                               
          s.incidentStore.UpdateSlackChannelID(inc.ID, channelID)                                                              
      }(incident)                                                                                                              
  }                                                                                                                            
  Creates a Slack channel named #inc-payments-api-{id}, posts the alert, then saves the channel ID back to the DB so the UI can
   link to it.                                                                                                                 
   
  ---                                                                                                                          
  PART 9 — IF A SECOND ALERT FIRES (MERGE PATH)             
                                                                                                                               
  Imagine Prometheus fires another alert 2 minutes later for the same service (maybe a different rule, e.g. HighLatency).
                                                                                                                               
  ProcessEvent() runs again. This time at Step 5E, FindOpenIncidentForService("payments-api", windowStart) finds the open      
  incident we just created (it is within the 5-minute window).                                                                 
                                                                                                                               
  mergeEvent() (line 225) runs:                                                                                                
   
  // Extend the time window                                                                                                    
  incident.LastEventTime = "2026-04-23T14:34:00Z"  // new event timestamp                                                      
                                                                                                                               
  // Accumulate                                                                                                                
  incident.EventCount = 2                                                                                                      
  incident.EventIDs   = ["a1b2c3d4e5f60001", "evt-new-event-id"]
                                                                                                                               
  // Escalate severity (never downgrade)                                                                                       
  // new event: "high", existing: "critical" → no change                                                                       
                                                                                                                               
  // Update correlation metadata                            
  incident.CorrelationPattern = "time_window_service_grouping"                                                                 
  incident.CorrelationScore   = 60   // was 50, +10                                                                            
  incident.CorrelationReason  = "2 alerts grouped for service 'payments-api' within 5-min window"                              
                                                                                                                               
  // Confidence grows                                                                                                          
  incident.Confidence = 80  // was 75, +5                                                                                      
                                                                                                                               
  // Recompute priority
  incident.PriorityScore = 68  // recalculated                                                                                 
                                                                                                                               
  incidentStore.UpdateIncident(incident)  // UPDATE incidents SET ... WHERE id = ...                                           
  incidentStore.AddEventToIncident(incident.ID, newEvent.ID)  // new junction row                                              
                                                                                                                               
  Result: 2 alerts → 1 incident, not 2. This is the core value of the correlation engine.                                      
                                                                                                                               
  ---                                                                                                                          
  PART 10 — THE FRONTEND REQUESTS: LOADING THE DASHBOARD    
                                                                                                                               
  File: frontend/src/api/client.js
                                                                                                                               
  The React app uses apiRequest() as its universal fetch wrapper. On every call:                                               
                                                                                                                               
  // Step 1: Get JWT token                                                                                                     
  let token = getStoredToken()        // from localStorage  
  if (!token) {                                                                                                                
      token = await ensureAuthenticated()  // auto-login with stored credentials
  }                                                                                                                            
                                                            
  // Step 2: Make the request                                                                                                  
  response = await fetch(`${API_BASE_URL}/incidents`, {     
      headers: {                                                                                                               
          "Content-Type": "application/json",
          "Authorization": `Bearer ${token}`                                                                                   
      }                                                                                                                        
  })
                                                                                                                               
  // Step 3: Handle 401 (expired token)                     
  if (response.status === 401) {
      clearStoredToken()
      freshToken = await ensureAuthenticated()  // re-login, get new token
      // retry the same request with fresh token                                                                               
  }
                                                                                                                               
  // Step 4: Parse JSON and return                                                                                             
  payload = await response.json()
  return payload                                                                                                               
                                                            
  File: frontend/src/api/incidents.js                                                                                          
   
  getIncidents(filters)             → GET /api/v1/incidents?status=open&severity=critical                                      
  getIncidentDetail(incidentId)     → GET /api/v1/incidents/{id}                                                               
  updateIncidentStatus(id, action)  → POST /api/v1/incidents/{id}/ack                                                          
  getIncidentExplanation(id)        → GET /api/v1/incidents/explain/{id}                                                       
  askIncidentCopilot(id, question)  → POST /api/v1/incidents/copilot/{id}                                                      
                                                                                                                               
  ---                                                                                                                          
  PART 11 — BACKEND RECEIVES LIST REQUEST                                                                                      
                                                                                                                               
  File: backend/internal/handlers/incident_handler.go — ListIncidents() (line 27)
                                                                                                                               
  GET /api/v1/incidents?status=open&severity=critical&page=1&page_size=20                                                      
                                                                                                                               
  The middleware stack runs first:                                                                                             
  RequestID    → attach request ID                          
  RequestLogger → log the request 
  CORS         → add Access-Control headers                                                                                    
  RequireAuth  → validate JWT, extract tenantID + role from claims, attach to context                                          
  RequireMinRole(viewer) → check role level ≥ 1 (viewer = 1, operator = 2, admin = 3)                                          
                                                                                                                               
  Then ListIncidents():                                                                                                        
  filter = models.IncidentListFilter{                                                                                          
      TenantID:  "default",      // from JWT claims via middleware.TenantFromRequest(r)                                        
      Status:    "open",                                                                                                       
      Severity:  "critical",                                                                                                   
      Service:   "",             // not provided                                                                               
      Search:    "",             // not provided                                                                               
      Page:      1,                                                                                                            
      PageSize:  20,                                                                                                           
      SortBy:    "",             // default: priority_score DESC
      SortOrder: "",                                                                                                           
  }                                                                                                                            
   
  response, err = incidentService.ListIncidents(filter)                                                                        
                                                                                                                               
  File: backend/internal/services/incident_service.go — ListIncidents() (line 35)
                                                                                                                               
  normalizedFilter = normalizeIncidentListFilter(filter)    
  // validation: status must be in {open, acknowledged, resolved, ""}                                                          
  // validation: severity must be in {critical, high, medium, low, ""}                                                         
  // validation: sort_by must be in allowed set                                                                                
  // validation: search/service length limits                                                                                  
                                                            
  return incidentStore.ListIncidents(normalizedFilter)                                                                         
                                                            
  File: backend/internal/store/incident_store.go — ListIncidents()                                                             
                                                            
  Builds a dynamic SQL query:                                                                                                  
  SELECT id, service, severity, status, first_event_time, last_event_time,
         title, correlation_score, confidence, risk_score, event_count,                                                        
         root_cause_summary, root_cause_type, reasoning_json,          
         seen_before, recurring_count, fingerprint, priority_score, tenant_id, ...                                             
  FROM incidents                                                                  
  WHERE tenant_id = 'default'                                                                                                  
    AND status    = 'open'                                  
    AND severity  = 'critical'                                                                                                 
  ORDER BY priority_score DESC, last_event_time DESC        
  LIMIT 20 OFFSET 0                                 
                                                                                                                               
  A separate SELECT COUNT(*) runs the same WHERE clause to get the total for pagination.
                                                                                                                               
  Returns models.IncidentListResponse{ Items: [...], Total: 1, Page: 1, PageSize: 20, HasMore: false }.                        
                                                                                                                               
  The JSON is serialized and sent back. The dashboard renders the list.                                                        
                                                            
  ---                                                                                                                          
  PART 12 — USER CLICKS AN INCIDENT: LOADING THE DETAIL     
                                                                                                                               
  File: backend/internal/handlers/incident_handler.go — GetIncidentDetail() (line 77)
                                                                                                                               
  GET /api/v1/incidents/inc-1745416320000000001                                                                                
   
  incidentID, _, ok = parseIncidentActionPath(request.URL.Path)                                                                
  // path: "api/v1/incidents/inc-1745416320000000001"                                                                          
  // parts[3] = "inc-1745416320000000001"
  // incidentID = "inc-1745416320000000001"                                                                                    
                                                            
  detail, found = incidentHandler.incidentService.GetIncidentDetail(incidentID)                                                
                                                            
  → calls incidentDetailService.GetIncidentDetail(incidentID) (line 513 of incident_detail_service.go)                         
                                                            
  File: backend/internal/services/incident_detail_service.go — GetIncidentDetail() (line 513)                                  
                                                            
  This is the most complex function in the codebase. It assembles the full detail response from many DB queries and pure logic 
  functions. Here is every step in order:                   
                                                                                                                               
  1. incidentStore.GetIncidentByID(id)                      
     → SELECT * FROM incidents WHERE id = 'inc-...' AND tenant_id = 'default'                                                  
     → returns models.Incident struct
                                                                                                                               
  2. eventStore.GetEventsByIDs(incident.EventIDs)           
     → SELECT id, source, type, service, severity, title, message, timestamp                                                   
       FROM events WHERE id IN ('a1b2c3d4e5f60001')                                                                            
       ORDER BY timestamp ASC                                                                                                  
     → returns []models.Event, sorted chronologically                                                                          
                                                                                                                               
  3. classifyIncidentPattern(events)                        
     → scans event messages for keywords: "database", "timeout", "failure", "latency"                                          
     → returns pattern string: "service_alert" in our case                                                                     
                                                                                                                               
  4. buildInsight(pattern, incident, events)                                                                                   
     → returns models.IncidentInsight with:                                                                                    
       IncidentType:      "service_failure"                                                                                    
       LikelyRootCause:   incident.RootCauseSummary                                                                            
       WhyThisIsLikely:   [...reasoning lines...]
       RecommendedChecks: ["Inspect service error logs...", ...]                                                               
       SuggestedAction:   "Start with application logs..."                                                                     
       Confidence:        "high"  (incident.Confidence=75 → "high")                                                            
                                                                                                                               
  5. buildTimeline(events)                                                                                                     
     → for each event, classifySignalType(event.Message)                                                                       
       and classifyStage(index, total, signalType)                                                                             
       result: [{Event, SignalType:"failure", StageType:"only", GapFromPrevious:"", StoryLabel:"Failures escalated"}]          
                                                                                                                               
  6. buildImpact(incident) → ImpactAnalysis with impacted services list                                                        
                                                                                                                               
  7. BuildActions(incident, insight) (action_service.go)                                                                       
     → generates []models.Action based on RootCauseType and Insight
     → e.g. Action{Label:"Check Error Logs", Type:"diagnostic", RiskLevel:"low", RequiresApproval:false}                       
                                                                                                                               
  8. GetPrimaryAction(actions)                                                                                                 
     → first diagnostic action with low risk and no approval required                                                          
                                                                                                                               
  9. buildNarrative(incident, timeline)
     → "payments-api generated 1 correlated signals. The incident started with failure behavior.                               
        Most likely cause: Error rate is 8.34% over last 5 minutes."                                                           
                                                                                                                               
  10. buildGraph(incident)                                                                                                     
      → models.GraphData with nodes=[{ID:"payments-api",NodeType:"service",Severity:"critical"}]                               
        + dependency nodes from config.ServiceDependencies["payments-api"]                                                     
        + edges between them                                                                                                   
                                                                                                                               
  11. buildEvidence(incident, insight, timeline)                                                                               
      → []string of evidence bullets assembled from:        
        - incident.CorrelationReason                                                                                           
        - incident.RootCauseSummary                                                                                            
        - incident.Reasoning
        - insight.WhyThisIsLikely                                                                                              
        - timeline story labels with timestamps                                                                                
        - what_changed info if any
                                                                                                                               
  12. buildRecommendedNextStep(primaryAction, insight)                                                                         
      → primaryAction.Label if available, else insight.SuggestedAction
                                                                                                                               
  13. buildResolutionSteps(incident, insight, events)                                                                          
      → checks root cause keywords for context-specific steps:                                                                 
        - "connection refused" → ["Verify the target service is running: systemctl status ...", ...]                           
        - "timeout" → ["Check downstream service health...", ...]                                                              
        - generic fallback → ["Review service logs for error patterns", ...]                                                   
                                                                                                                               
  14. historyStore.GetByIncidentID(incident.ID)                                                                                
      → SELECT * FROM incident_status_history WHERE incident_id = ?                                                            
      → buildStatusAudit() wraps into []models.IncidentStatusAudit
                                                                                                                               
  15. knowledgeBase enrichment (if available)                                                                                  
      → override resolutionSteps with human-verified steps from past resolutions                                               
                                                                                                                               
  16. buildLifecycle(incident, statusAudit)                 
      → ["detected at 14:32:00 by correlation engine",                                                                         
         "acknowledged at 14:35:12 by operator John",                                                                          
         ...]                                                                                                                  
                                                                                                                               
  17. incidentStore.CountSimilarByService("payments-api", incidentID)                                                          
      → SELECT COUNT(*) FROM incidents WHERE service = ? AND id != ? AND status = 'resolved'
      → realRecurringCount = 2 (this has happened before)                                                                      
      → seenBefore = true                                                                                                      
                                                                                                                               
  18. enrichContextLogs(&detail, incident)                                                                                     
      → logStore.GetLogsForService(service, windowStart, windowEnd)                                                            
      → attaches recent error/warn log lines from agents as []ContextLogEntry                                                  
                                                                                                                               
  19. enrichContextMetrics(&detail, incident)                                                                                  
      → agentStore.GetMetricsForService(service, timestamp)                                                                    
      → attaches CPU%, memory%, disk% at incident time as *ContextMetrics
                                                                                                                               
  20. enrichActionExecutions(&detail, incident)                                                                                
      → actionExecutionStore.GetByIncidentID(incidentID)                                                                       
      → attaches audit trail of actions taken during this incident                                                             
                                                                                                                               
  Final assembled object models.IncidentDetail returned to the handler, serialized to JSON, sent to the frontend.              
                                                                                                                               
  ---                                                                                                                          
  PART 13 — AI ENRICHMENT LAYER                             
                               
  File: backend/internal/services/explain_service.go — Explain() (line 64)
                                                                                                                               
  When the frontend calls GET /api/v1/incidents/explain/{id}, the ExplainHandler calls explainService.Explain(detail).         
                                                                                                                               
  Priority 1: Knowledge Base match                                                                                             
     knowledgeBase.MatchByIncident(title, rootCause, reasoning)                                                                
     → if entry found: use human-verified narrative from past resolution                                                       
     → return immediately, no LLM call                                                                                         
                                                                                                                               
  Priority 2: LLM-powered RCA                                                                                                  
     if llmClient != nil && llmClient.IsConfigured():       
       AssembleForRCA(detail)                                                                                                  
         → builds SystemPrompt: "You are an incident RCA expert..."
         → builds UserPrompt: structured JSON of incident + events + logs + metrics                                            
       llmClient.CompleteWithSystem(systemPrompt, userPrompt)                                                                  
         → HTTP POST to OpenAI/Anthropic API                                                                                   
         → expects JSON response:                                                                                              
           {                                                                                                                   
             "root_cause_summary": "...",                                                                                      
             "root_cause_type": "...",                                                                                         
             "confidence": 88,
             "risk_score": 92,                                                                                                 
             "reasoning": ["...", "..."],                   
             "recommended_next_step": "...",                                                                                   
             "narrative": "..."
           }                                                                                                                   
       parseJSON(rawText)                                   
         → handles markdown code fences (```json ... ```)                                                                      
       enriches detail.Incident with LLM-produced fields
       buildEvidenceRefs(detail)                                                                                               
         → creates []EvidenceRef pointing to actual DB records (not LLM text):                                                 
           [{type:"event", id:"a1b2c3d4e5f60001", timestamp:"...", label:"[CRITICAL] prometheus on payments-api"}]             
           [{type:"metric", id:"metric-cpu", label:"High CPU utilization", detail:"CPU at 94.2%"}]                             
                                                                                                                               
  Priority 3: Template fallback                                                                                                
     if LLM fails or is not configured:                                                                                        
       buildTemplateNarrative(detail)                                                                                          
         → "payments-api is experiencing a critical incident.
            Error rate is 8.34% over last 5 minutes.                                                                           
            This issue has been seen 2 times before."                                                                          
                                                                                                                               
  ---                                                                                                                          
  PART 14 — INCIDENT STATUS UPDATES: THE OPERATOR ACTS      
                                                                                                                               
  File: backend/internal/handlers/incident_handler.go — UpdateIncidentStatus() (line 93)
                                                                                                                               
  POST /api/v1/incidents/inc-.../ack (requires JWT with operator role or above)
                                                                                                                               
  Middleware: RequireMinRole(operator) rejects viewers with 403.                                                               
   
  incidentID = "inc-1745416320000000001"                                                                                       
  action     = "ack"   // from URL path                     

  incidentService.UpdateIncidentStatus(incidentID, "ack")

  File: backend/internal/services/incident_service.go — UpdateIncidentStatus() (line 48)

  nextStatus = mapIncidentActionToStatus("ack")  // → "acknowledged"

  currentIncident = incidentStore.GetIncidentByID(id)  // fetch current state

  updatedIncident = incidentStore.UpdateIncidentStatus(id, "acknowledged")                                                     
  // UPDATE incidents SET status = 'acknowledged' WHERE id = ?
                                                                                                                               
  // Audit log                                              
  audit.Log("default", "operator", "incident.acknowledged", "incident", id,
            {"previous_status":"open","new_status":"acknowledged","action":"ack"})                                             
                                                                                                                               
  // Status history                                                                                                            
  historyStore.AddRecord(id, "open", "acknowledged", "Acknowledged by operator", "operator")                                   
  // INSERT INTO incident_status_history (incident_id, previous_status, new_status, note, changed_by, changed_at)              
   
  // Engineering health                                                                                                        
  engineeringHealthService.RecordIncidentMetrics(updatedIncident, "ack", "operator")
  // records ack_at timestamp for time-to-acknowledge metric                                                                   
                                                                                                                               
  // If resolving: async side-effects
  if nextStatus == "resolved" {                                                                                                
      go businessImpactService.AutoGenerateBaseline(tenantID, service)                                                         
      go incidentMemoryService.RecordResolution(id, tenantID, "")                                                              
      // RecordResolution saves the resolution pattern to incident_memory table                                                
      // so future similar incidents can benefit from this resolution's steps                                                  
  }                                                                                                                            
                                                                                                                               
  ---                                                                                                                          
  PART 15 — COMPLETE FILE-BY-FILE FLOW SUMMARY              
                                                                                                                               
  EXTERNAL TOOL (Prometheus / Agent / Datadog / PagerDuty)
    │                                                                                                                          
    │  POST /api/v1/ingest/prometheus                       
    │  Header: X-Source-Token: src_abc123                                                                                      
    │  Body: { "alerts": [...AlertManager payload...] }                                                                        
    │                                                                                                                          
    ▼                                                                                                                          
  backend/internal/routes/routes.go                         
    withOps middleware:                                                                                                        
      middleware/request_id.go     → attach X-Request-ID
      middleware/request_logger.go → log method, path, status, duration                                                        
      CORS headers                                          
    ↓ (no JWT required on ingest routes)                                                                                       
    │                                                                                                                          
    ▼                                                                                                                          
  backend/internal/handlers/ingest_handler.go                                                                                  
    PrometheusWebhook()                                     
      resolveTenantForIngest()  → sourceRegistryService.FindByToken(token)                                                     
      parse AlertManager JSON                                                                                                  
      for each alert:                                                                                                          
        build models.Event{}                                                                                                   
        stamp TenantID (NEVER from body)                                                                                       
    ↓                                                                                                                          
    ▼
  backend/internal/store/event_store.go                                                                                        
    SaveEvent()                                                                                                                
      INSERT INTO events (...) ON CONFLICT DO UPDATE
    ↓                                                                                                                          
    ▼                                                       
  backend/internal/services/correlation_service.go
    ProcessEvent()
      computeEventFingerprint()  → SHA-256 of "source|service|severity|normalized_title"                                       
      SaveEvent() again          → update fingerprint column                                                                   
      IsSuppressed()             → check alert_feedback table                                                                  
      FingerprintSeenInWindow()  → dedup check in events table                                                                 
      FindOpenIncidentByFingerprint() → fingerprint merge check                                                                
      findRelatedIncident()      → time-window + cross-service merge check                                                     
      │                                                                                                                        
      ├── [existing found] → mergeEvent()                                                                                      
      │     UpdateIncident()    → UPDATE incidents SET event_count++, last_event_time=...                                      
      │     AddEventToIncident()→ INSERT INTO incident_events                                                                  
      │                                                                                                                        
      └── [no match] → createIncident()                                                                                        
            computePriorityScore()                          
            knowledgeBase.Match()  → optional KB enrichment                                                                    
            buildDefaultRootCause()                                                                                            
            buildReasoning()
            classifyRootCauseType()                                                                                            
            incidentStore.AddIncident()                     
              → INSERT INTO incidents (33 columns)                                                                             
              → INSERT INTO incident_events                                                                                    
            goroutine: onboardingService.MarkMilestone()
            goroutine: whatsappService.SendIncidentAlert()   [critical only]                                                   
            goroutine: slackService.CreateIncidentChannel()  [critical only]                                                   
    ↓                                                                                                                          
    ▼                                                                                                                          
  [INCIDENT NOW IN DATABASE — OPEN STATUS]                  
    ↓                                                                                                                          
    ▼
  frontend/src/api/incidents.js                                                                                                
    getIncidents(filters)                                   
      → apiRequest("/incidents?status=open&severity=critical")                                                                 
         client.js: attach JWT, fetch, handle 401 retry                                                                        
    ↓                                                                                                                          
    ▼                                                                                                                          
  backend/internal/handlers/incident_handler.go                                                                                
    ListIncidents()                                                                                                            
      middleware.TenantFromRequest()   → extract tenantID from JWT
      incidentService.ListIncidents(filter)                                                                                    
    ↓                                                       
    ▼                                                                                                                          
  backend/internal/services/incident_service.go             
    ListIncidents()
      normalizeIncidentListFilter()   → validate/sanitize inputs
      incidentStore.ListIncidents()                                                                                            
    ↓                                                                                                                          
    ▼                                                                                                                          
  backend/internal/store/incident_store.go                                                                                     
    ListIncidents()                                         
      SELECT ... FROM incidents WHERE tenant_id=? AND status=? ORDER BY priority_score DESC
      SELECT COUNT(*) FROM incidents WHERE ...                                                                                 
      → models.IncidentListResponse{ Items, Total, Page, HasMore }
    ↓                                                                                                                          
    ▼                                                       
  [DASHBOARD SHOWS INCIDENT LIST]                                                                                              
    ↓                                                       
    ▼
  frontend/src/api/incidents.js                                                                                                
    getIncidentDetail(incidentId)
      → apiRequest("/incidents/inc-...")                                                                                       
    ↓                                                       
    ▼
  backend/internal/handlers/incident_handler.go
    GetIncidentDetail()                                                                                                        
      parseIncidentActionPath()  → extract ID from URL
      incidentService.GetIncidentDetail(id)                                                                                    
    ↓                                                       
    ▼
  backend/internal/services/incident_service.go                                                                                
    GetIncidentDetail()
      → incidentDetailService.GetIncidentDetail(id)                                                                            
    ↓                                                       
    ▼
  backend/internal/services/incident_detail_service.go
    GetIncidentDetail()                                                                                                        
      incidentStore.GetIncidentByID()       → SELECT from incidents
      eventStore.GetEventsByIDs()           → SELECT from events                                                               
      classifyIncidentPattern(events)       → keyword scan → pattern string
      buildInsight(pattern, incident)       → IncidentInsight with checks + suggestions                                        
      buildTimeline(events)                 → []TimelineEvent with signal/stage labels
      buildImpact(incident)                 → ImpactAnalysis                                                                   
      BuildActions(incident, insight)       → []Action (action_service.go)                                                     
      GetPrimaryAction(actions)             → best low-risk diagnostic action                                                  
      buildNarrative(incident, timeline)    → plain-English description                                                        
      buildGraph(incident)                  → GraphData nodes+edges
      buildEvidence(incident,insight,tl)    → []string evidence bullets                                                        
      buildRecommendedNextStep()            → single best action label
      buildResolutionSteps()                → []string step-by-step playbook                                                   
      historyStore.GetByIncidentID()        → []IncidentStatusAudit                                                            
      knowledgeBase.MatchByIncident()       → override with KB steps if available                                              
      buildLifecycle(incident,statusAudit)  → []LifecycleEntry stages                                                          
      CountSimilarByService()               → seen_before flag                                                                 
      enrichContextLogs()                   → attach agent log lines                                                           
      enrichContextMetrics()                → attach CPU/mem/disk values                                                       
      enrichActionExecutions()              → attach action audit trail                                                        
      → return models.IncidentDetail (fully assembled)                                                                         
    ↓                                                                                                                          
    ▼                                                       
  backend/internal/services/explain_service.go  [optional, on /explain endpoint]
    Explain()                                                                                                                  
      knowledgeBase.MatchByIncident()  → priority 1: human-verified narrative
      llmClient.CompleteWithSystem()   → priority 2: LLM-generated RCA                                                         
      buildTemplateNarrative()         → priority 3: template fallback
      buildEvidenceRefs()              → concrete data pointers (event IDs, log refs)                                          
    ↓                                                                                                                          
    ▼                                                                                                                          
  [DASHBOARD SHOWS FULL INCIDENT DETAIL]                                                                                       
    Timeline, Actions, AI Narrative, Evidence, Graph, Lifecycle, Resolution Steps                                              
    ↓                                                                                                                          
    ▼                                                                                                                          
  Operator clicks "Acknowledge"                                                                                                
  frontend/src/api/incidents.js  → updateIncidentStatus(id, "ack")
    ↓                                                                                                                          
  backend/internal/handlers/incident_handler.go  → UpdateIncidentStatus()
    ↓                                                                                                                          
  backend/internal/services/incident_service.go  → UpdateIncidentStatus()                                                      
    incidentStore.UpdateIncidentStatus()        → UPDATE incidents SET status='acknowledged'
    audit.Log()                                 → INSERT INTO audit_logs                                                       
    historyStore.AddRecord()                    → INSERT INTO incident_status_history                                          
    engineeringHealthService.RecordMetrics()    → record ack_at timestamp
    [if resolve] → goroutine: incidentMemoryService.RecordResolution()                                                         
                               → INSERT INTO incident_memory                                                                   
                               → future similar incident learns from this resolution                                           
                                                                                                                               
  ---                                                       
  The Three Invariants Everything Is Built Around                                                                              
                                                                                                                               
  1. TenantID is stamped at the HTTP boundary, never trusted from the body.
  ingest_handler.go line 85: e.TenantID = tenantID. The tenantID from the JWT or source token overwrites anything the caller   
  put in the body. This is how multi-tenancy stays airtight across 20+ services.                                               
                                                                                                                               
  2. Fingerprint = same alert, regardless of which pod it came from.                                                           
  correlation_service.go line 147: normalizeAlertTitle() strips UUIDs, pod names, IP addresses before hashing. 100 pods firing
  the same alert → one fingerprint → one dedup suppression → 1 event through, 99 dropped.                                      
                                                            
  3. One incident per service per 5-minute window.                                                                             
  FindOpenIncidentForService() + the merge path means no matter how many alerts fire for payments-api in a burst, they are all
  correlated into one incident record with a rising EventCount, CorrelationScore, and Confidence — not 100 separate incidents  
  flooding the dashboard.
                                                                                                                               
✻ Churned for 4m 29s                                        
                                                                                          
───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
❯                                                                                                                              
───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ accept edits on (shift+tab to cycle)                                                                                      
                                                                                                                               
  frontend/src/api/incidents.js  → updateIncidentStatus(id, "ack")
    ↓
  backend/internal/handlers/incident_handler.go  → UpdateIncidentStatus()
    ↓
  backend/internal/services/incident_service.go  → UpdateIncidentStatus()
    incidentStore.UpdateIncidentStatus()        → UPDATE incidents SET status='acknowledged'
    audit.Log()                                 → INSERT INTO audit_logs
    historyStore.AddRecord()                    → INSERT INTO incident_status_history
    engineeringHealthService.RecordMetrics()    → record ack_at timestamp
    [if resolve] → goroutine: incidentMemoryService.RecordResolution()
                               → INSERT INTO incident_memory
                               → future similar incident learns from this resolution

  ---
  The Three Invariants Everything Is Built Around

  1. TenantID is stamped at the HTTP boundary, never trusted from the body.
  ingest_handler.go line 85: e.TenantID = tenantID. The tenantID from the JWT or source token overwrites anything the caller
  put in the body. This is how multi-tenancy stays airtight across 20+ services.

  2. Fingerprint = same alert, regardless of which pod it came from.
  correlation_service.go line 147: normalizeAlertTitle() strips UUIDs, pod names, IP addresses before hashing. 100 pods firing
  the same alert → one fingerprint → one dedup suppression → 1 event through, 99 dropped.

  3. One incident per service per 5-minute window.
  FindOpenIncidentForService() + the merge path means no matter how many alerts fire for payments-api in a burst, they are all
  correlated into one incident record with a rising EventCount, CorrelationScore, and Confidence — not 100 separate incidents
  flooding the dashboard.

✻ Churned for 4m 29s

───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
❯  
───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ accept edits on (shift+tab to cycle)
    ▼
  Operator clicks "Acknowledge"
  frontend/src/api/incidents.js  → updateIncidentStatus(id, "ack")
    ↓
  backend/internal/handlers/incident_handler.go  → UpdateIncidentStatus()
    ↓
  backend/internal/services/incident_service.go  → UpdateIncidentStatus()
    incidentStore.UpdateIncidentStatus()        → UPDATE incidents SET status='acknowledged'
    audit.Log()                                 → INSERT INTO audit_logs
    historyStore.AddRecord()                    → INSERT INTO incident_status_history
    engineeringHealthService.RecordMetrics()    → record ack_at timestamp
    [if resolve] → goroutine: incidentMemoryService.RecordResolution()
                               → INSERT INTO incident_memory
                               → future similar incident learns from this resolution

  ---
  The Three Invariants Everything Is Built Around

  1. TenantID is stamped at the HTTP boundary, never trusted from the body.
  ingest_handler.go line 85: e.TenantID = tenantID. The tenantID from the JWT or source token overwrites anything the caller
  put in the body. This is how multi-tenancy stays airtight across 20+ services.

  2. Fingerprint = same alert, regardless of which pod it came from.
  correlation_service.go line 147: normalizeAlertTitle() strips UUIDs, pod names, IP addresses before hashing. 100 pods firing
  the same alert → one fingerprint → one dedup suppression → 1 event through, 99 dropped.

  3. One incident per service per 5-minute window.
  FindOpenIncidentForService() + the merge path means no matter how many alerts fire for payments-api in a burst, they are all
  correlated into one incident record with a rising EventCount, CorrelationScore, and Confidence — not 100 separate incidents
  flooding the dashboard.

✻ Churned for 4m 29s

───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
❯  
───────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏵⏵ accept edits on (shift+tab to cycle)

