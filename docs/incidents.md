# How Incidents Are Created

## Correlation Engine Flow

```
Event arrives
    ↓
Compute fingerprint (SHA-256 of source|service|severity|normalized_title)
    ↓
Check: is this fingerprint suppressed? (alert feedback, 5+ noise marks)
    ↓ no
Check: is there an OPEN incident with same fingerprint? (any time)
    ↓ yes → MERGE into existing incident (increase event count, update timeline)
    ↓ no
Check: is there an OPEN incident for same SERVICE within 5 min?
    ↓ yes → MERGE
    ↓ no
Check: is there an OPEN incident for a DEPENDENT service within 5 min?
    ↓ yes → MERGE (cross-service correlation)
    ↓ no
CREATE new incident
    ↓
Match against Knowledge Base (502 entries)
    ↓
Set root cause, reasoning, resolution steps, confidence, risk score
    ↓
Compute priority score (severity + events + risk + recurrence + impact)
    ↓
Check auto-resolve rules → if match, auto-resolve immediately
    ↓
Store in PostgreSQL
```

## Incident Lifecycle

```
OPEN → ACKNOWLEDGED → RESOLVED
  ↑                        |
  └────── REOPENED ────────┘
```

- **Open**: New incident, needs attention
- **Acknowledged**: Engineer is investigating (set when "Open Investigation" clicked)
- **Resolved**: Issue fixed (triggers auto-baseline generation)
- **Reopened**: Issue recurred after resolution

## Knowledge Base Matching

The 502-entry knowledge base maps incident patterns to:

| Field | Example |
|-------|---------|
| Root Cause | "Database connection pool is exhausted" |
| Reasoning | ["All connections checked out", "Slow queries hold connections", "Leaks gradually exhaust pool"] |
| Resolution Steps | ["Check pool: SELECT count(*) FROM pg_stat_activity", "Kill idle: SELECT pg_terminate_backend(pid)", ...] |
| Impact | "All functionality depending on the database fails" |
| Prevention | "Add connection pool monitoring, implement circuit breaker" |
| Confidence | 85 |
| Risk Score | 80 |

## Priority Scoring

```
priority_score = severity_weight (0-40)
               + event_count_weight (0-20)
               + risk_score / 5 (0-20)
               + recurrence_bonus (0-10)
               + impact_count * 3 (0-10)
```

Incidents are sorted by priority_score DESC in the UI.

## Business Impact

When an incident is viewed:
```
actual_loss = revenue × impact% × duration × severity_mult × tier_mult
counterfactual_loss = same formula with baseline MTTR (2-phase escalation)
avoided_loss = counterfactual - actual (what your team saved)
```

Plus projections: "If unresolved +15/30/60 min → $X exposure"
