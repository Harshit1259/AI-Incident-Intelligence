package models

// Role constants — use these instead of inline strings everywhere.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// roleLevel assigns a numeric weight to each role so the hierarchy can be
// evaluated with a single integer comparison rather than cascading if-chains.
//
//	admin (3) > operator (2) > viewer (1)
var roleLevel = map[string]int{
	RoleViewer:   1,
	RoleOperator: 2,
	RoleAdmin:    3,
}

// RoleSatisfies returns true when userRole meets or exceeds minRole in the hierarchy.
// An unrecognised role has level 0 and never satisfies any requirement.
func RoleSatisfies(userRole, minRole string) bool {
	uLevel := roleLevel[userRole] // 0 if unknown
	mLevel := roleLevel[minRole]
	if mLevel == 0 {
		return false // unknown minimum is always unsatisfiable
	}
	return uLevel >= mLevel
}

// ValidRole returns true when role is one of the recognised role strings.
func ValidRole(role string) bool {
	_, ok := roleLevel[role]
	return ok
}

// ─── Permission matrix ───────────────────────────────────────────────────────
//
// This table is the canonical reference for what each role may do.
// Route registrars enforce this by accepting requireOperator/requireAdmin
// closures and applying them per HTTP method.
//
// ┌────────────────────────────────┬────────┬──────────┬────────┐
// │ Resource / Action              │ admin  │ operator │ viewer │
// ├────────────────────────────────┼────────┼──────────┼────────┤
// │ Incidents — read               │   ✓    │    ✓     │   ✓    │
// │ Incidents — ack/resolve/reopen │   ✓    │    ✓     │   ✗    │
// │ Events — read                  │   ✓    │    ✓     │   ✓    │
// │ Events — write                 │   ✓    │    ✓     │   ✗    │
// │ Sources — read                 │   ✓    │    ✓     │   ✓    │
// │ Sources — write                │   ✓    │    ✓     │   ✗    │
// │ SLOs — read                    │   ✓    │    ✓     │   ✓    │
// │ SLOs — write                   │   ✓    │    ✓     │   ✗    │
// │ On-call — read                 │   ✓    │    ✓     │   ✓    │
// │ On-call — write                │   ✓    │    ✓     │   ✗    │
// │ Analytics / ROI / Health       │   ✓    │    ✓     │   ✓    │
// │ Topology / Memory / Risk       │   ✓    │    ✓     │   ✓    │
// │ Runbooks — read                │   ✓    │    ✓     │   ✓    │
// │ Runbooks — write               │   ✓    │    ✓     │   ✗    │
// │ Dependencies — read            │   ✓    │    ✓     │   ✓    │
// │ Dependencies — write           │   ✓    │    ✓     │   ✗    │
// │ Compliance — read              │   ✓    │    ✓     │   ✓    │
// │ Postmortem — write             │   ✓    │    ✓     │   ✗    │
// │ Action execute / approve       │   ✓    │    ✓     │   ✗    │
// │ Alert feedback                 │   ✓    │    ✓     │   ✗    │
// │ Auto-resolve rules — write     │   ✓    │    ✓     │   ✗    │
// │ Biz profiles — write           │   ✓    │    ✓     │   ✗    │
// │ Agent — read                   │   ✓    │    ✓     │   ✗    │  sensitive (IPs)
// │ Agent — write / enroll         │   ✓    │    ✓     │   ✗    │
// │ Audit log — read               │   ✓    │    ✓     │   ✗    │  operational sensitivity
// │ Policies — read                │   ✓    │    ✓     │   ✗    │  internal governance
// │ Policies — write               │   ✓    │    ✗     │   ✗    │
// │ Config — read                  │   ✓    │    ✓     │   ✗    │
// │ Config — write                 │   ✓    │    ✗     │   ✗    │
// │ Billing — read                 │   ✓    │    ✓     │   ✗    │  financial data
// │ Billing — write / checkout     │   ✓    │    ✓     │   ✗    │
// │ Tenant admin — read            │   ✓    │    ✓     │   ✗    │
// │ Tenant admin — write           │   ✓    │    ✗     │   ✗    │
// │ WhatsApp config — read/write   │   ✓    │    ✓     │   ✗    │
// │ Import / bulk operations       │   ✓    │    ✗     │   ✗    │
// └────────────────────────────────┴────────┴──────────┴────────┘
