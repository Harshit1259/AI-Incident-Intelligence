package handlers

import (
	"fmt"
	"net/http"
)

// HandleInstallScript serves a dynamically-generated agent install script.
// The script picks up SERVER_URL from the request host so it works behind
// any reverse proxy without hardcoding the domain.
//
// Usage (shown in onboarding wizard):
//
//	curl -fsSL http://<host>/install.sh | TENANT_ID=<id> sh
func HandleInstallScript(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto == "https" {
		scheme = proto
	}
	baseURL := scheme + "://" + r.Host

	tenantID := r.URL.Query().Get("tenant_id")
	if tenantID == "" {
		tenantID = "${TENANT_ID:-default}"
	}

	script := fmt.Sprintf(`#!/usr/bin/env sh
# ──────────────────────────────────────────────────────────────────────────────
# NeuroOps Agent — Auto Installer
# Generated for: %s
# ──────────────────────────────────────────────────────────────────────────────
set -e

SERVER_URL="${SERVER_URL:-%s}"
TENANT_ID="${TENANT_ID:-%s}"
INSTALL_DIR="${INSTALL_DIR:-/opt/neuroops-agent}"

echo ""
echo "  NeuroOps Agent Installer"
echo "  Server  : ${SERVER_URL}"
echo "  Tenant  : ${TENANT_ID}"
echo ""

# ── OS detection ──────────────────────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "${ARCH}" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: ${ARCH}"; exit 1 ;;
esac

# ── Download agent binary ─────────────────────────────────────────────────────
AGENT_URL="${SERVER_URL}/api/v1/agents/download?os=${OS}&arch=${ARCH}"
echo "→ Downloading agent binary..."

if command -v curl >/dev/null 2>&1; then
  DOWNLOADER="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
  DOWNLOADER="wget -qO-"
else
  echo "Error: curl or wget required"; exit 1
fi

mkdir -p "${INSTALL_DIR}/config" "${INSTALL_DIR}/logs"

${DOWNLOADER} "${AGENT_URL}" > "${INSTALL_DIR}/neuroops-agent" 2>/dev/null || {
  echo "Warning: Could not download agent binary from ${AGENT_URL}"
  echo "Manual install: visit ${SERVER_URL}/agents for instructions"
  exit 0
}
chmod +x "${INSTALL_DIR}/neuroops-agent"

# ── Write config ──────────────────────────────────────────────────────────────
cat > "${INSTALL_DIR}/config/agent.json" <<CONFIG
{
  "server_url": "${SERVER_URL}",
  "tenant_id":  "${TENANT_ID}",
  "log_dir":    "${INSTALL_DIR}/logs",
  "interval":   30
}
CONFIG

# ── Install systemd service (Linux only) ──────────────────────────────────────
if [ "${OS}" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
  cat > /tmp/neuroops-agent.service <<SERVICE
[Unit]
Description=NeuroOps Observability Agent
After=network.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/neuroops-agent --config ${INSTALL_DIR}/config/agent.json
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
SERVICE

  if [ "$(id -u)" = "0" ]; then
    mv /tmp/neuroops-agent.service /etc/systemd/system/
    systemctl daemon-reload
    systemctl enable neuroops-agent
    systemctl start  neuroops-agent
    echo "✓ neuroops-agent service installed and started"
  else
    echo "→ Run the following as root to install as a system service:"
    echo "  sudo mv /tmp/neuroops-agent.service /etc/systemd/system/"
    echo "  sudo systemctl daemon-reload && sudo systemctl enable --now neuroops-agent"
  fi
else
  echo "→ Start the agent manually:"
  echo "  ${INSTALL_DIR}/neuroops-agent --config ${INSTALL_DIR}/config/agent.json &"
fi

echo ""
echo "  ✓ NeuroOps Agent installed to ${INSTALL_DIR}"
echo "  ✓ Reporting to ${SERVER_URL} as tenant ${TENANT_ID}"
echo ""
`, baseURL, baseURL, tenantID)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline; filename=install.sh")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, script)
}
