#!/usr/bin/env bash
# validate-package.sh — CI gate for release tarballs
#
# Usage: ./scripts/validate-package.sh <path-to-tarball>
#
# Exits 0 on pass, 1 on failure.
# Prints every violation found before exiting so the developer sees all issues
# at once rather than fixing them one at a time.

set -euo pipefail

TARBALL="${1:-}"
if [[ -z "$TARBALL" ]]; then
  echo "Usage: $0 <path-to-tarball>" >&2
  exit 1
fi

if [[ ! -f "$TARBALL" ]]; then
  echo "✗  File not found: $TARBALL" >&2
  exit 1
fi

echo "Inspecting: $TARBALL"

# List all paths inside the tarball (strip leading ./ for consistency)
CONTENTS="$(tar -tzf "$TARBALL" | sed 's|^\./||')"

FAILURES=0

check() {
  local label="$1"
  local pattern="$2"
  local matches
  matches="$(echo "$CONTENTS" | grep -E "$pattern" || true)"
  if [[ -n "$matches" ]]; then
    echo ""
    echo "✗  FORBIDDEN [$label]:"
    echo "$matches" | sed 's/^/     /'
    FAILURES=$((FAILURES + 1))
  fi
}

# ── Secrets ──────────────────────────────────────────────────────────────────
check "secrets: .env file"          '(^|/)\.env$'
check "secrets: .env.local"         '(^|/)\.env\.'
check "secrets: private keys"       '\.(pem|key|p12|pfx)$'
check "secrets: credential files"   '(credentials|service.account).*\.json$'

# ── Source control ────────────────────────────────────────────────────────────
check "vcs: .git directory"         '(^|/)\.git(/|$)'
check "vcs: .gitignore"             '(^|/)\.gitignore$'

# ── Build tooling (not for customers) ────────────────────────────────────────
check "deps: node_modules"          '(^|/)node_modules/'
check "deps: go module cache"       '(^|/)pkg/mod/'
check "deps: go build cache"        '(^|/)\.cache/'

# ── Build artifacts that should not ship ─────────────────────────────────────
check "artifacts: *.test binary"    '\.test$'
check "artifacts: coverage files"   '\.(coverprofile|out)$'

# ── Logs and runtime state ────────────────────────────────────────────────────
check "runtime: log files"          '\.(log|log\.[0-9])$'
check "runtime: .agent.id"          '(^|/)\.agent\.id$'
check "runtime: cache-position"     'cache-position\.json$'

# ── Dev / backup junk ────────────────────────────────────────────────────────
check "junk: backup files"          '\.(bak|orig|swp|swo)$'
check "junk: backup directories"    '(^|/)\.backup_'
check "junk: tilde backups"         '~$'
check "junk: planning docs"         '(6MothPlan|Claude_plan|whatsLeftExecutionPlan)'
check "junk: malformed dirs"        '^\{'

# ── Large binaries that belong in agent package, not platform package ─────────
check "binary: agent binary"        '(^|/)neuroops-agent$'
check "binary: upgrader binary"     '(^|/)neuroops-upgrader$'

# ── IDE artefacts ─────────────────────────────────────────────────────────────
check "ide: .idea"                  '(^|/)\.idea/'
check "ide: .vscode"                '(^|/)\.vscode/'

# ── Result ────────────────────────────────────────────────────────────────────
echo ""
if [[ $FAILURES -gt 0 ]]; then
  echo "✗  Validation FAILED — $FAILURES rule(s) violated."
  echo "   Fix the packaging step (Makefile package target) and re-run."
  exit 1
else
  echo "✓  All checks passed. Tarball is clean."
fi
