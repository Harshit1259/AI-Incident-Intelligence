#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# NeurOps Agent — Interactive Installer
# Copyright (c) NeurOps 2025. All rights reserved.
#
# Supported: Ubuntu 18.04 / 20.04 / 22.04 / 24.04
#
# Usage:
#   sudo bash install.sh
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GRN='\033[0;32m'; YLW='\033[0;33m'
BLU='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

info()  { echo -e "${BLU}${BOLD}[INFO]${NC}  $*"; }
ok()    { echo -e "${GRN}${BOLD}[ OK ]${NC}  $*"; }
warn()  { echo -e "${YLW}${BOLD}[WARN]${NC}  $*"; }
fatal() { echo -e "${RED}${BOLD}[FAIL]${NC}  $*"; exit 1; }

BANNER="
 ███╗   ██╗███████╗██╗   ██╗██████╗  ██████╗  ██████╗ ██████╗ ███████╗
 ████╗  ██║██╔════╝██║   ██║██╔══██╗██╔═══██╗██╔═══██╗██╔══██╗██╔════╝
 ██╔██╗ ██║█████╗  ██║   ██║██████╔╝██║   ██║██║   ██║██████╔╝███████╗
 ██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██║   ██║██║   ██║██╔═══╝ ╚════██║
 ██║ ╚████║███████╗╚██████╔╝██║  ██║╚██████╔╝╚██████╔╝██║     ███████║
 ╚═╝  ╚═══╝╚══════╝ ╚═════╝ ╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝     ╚══════╝
                        Intelligent Observability Agent v1.0.0
"
echo -e "${BLU}${BOLD}${BANNER}${NC}"

# ── OS check ──────────────────────────────────────────────────────────────────
if ! command -v lsb_release &>/dev/null; then
    fatal "lsb_release not found. Only Ubuntu 18/20/22/24 are supported."
fi
OS_VERSION=$(lsb_release -rs)
case "$OS_VERSION" in
    18.04|20.04|22.04|24.04) ok "Ubuntu $OS_VERSION detected" ;;
    14.04|16.04) fatal "Ubuntu $OS_VERSION is no longer supported. Please upgrade to 18.04+." ;;
    *) warn "Ubuntu $OS_VERSION is untested. Proceeding with 22.04 binaries." ; OS_VERSION="22.04" ;;
esac

# ── Root check ────────────────────────────────────────────────────────────────
if [[ "$EUID" -ne 0 ]]; then
    fatal "This installer must be run as root (use sudo)."
fi

# ── Gather product server details ─────────────────────────────────────────────
echo ""
echo -e "${BLU}${BOLD}NeurOps Product Server Configuration${NC}"
echo ""
read -e -p "$(echo -e "${GRN}${BOLD}NeurOps Product Server IP   [127.0.0.1]: ${NC}")" \
    -i "127.0.0.1" PRODUCT_IP
read -e -p "$(echo -e "${GRN}${BOLD}Event Publisher Port (agent→product) [9441]: ${NC}")" \
    -i "9441" PUB_PORT
read -e -p "$(echo -e "${GRN}${BOLD}Event Subscriber Port (product→agent) [9440]: ${NC}")" \
    -i "9440" SUB_PORT
echo ""

# ── Port reachability check ───────────────────────────────────────────────────
info "Checking connectivity to ${PRODUCT_IP}:${PUB_PORT} and ${PRODUCT_IP}:${SUB_PORT} ..."

check_port() {
    local host=$1 port=$2
    (echo > /dev/tcp/"$host"/"$port") 2>/dev/null && echo "reachable" || echo "unreachable"
}

PUB_STATUS=$(check_port "$PRODUCT_IP" "$PUB_PORT")
SUB_STATUS=$(check_port "$PRODUCT_IP" "$SUB_PORT")

if [[ "$PUB_STATUS" == "unreachable" ]]; then
    fatal "Port ${PUB_PORT} is not reachable on ${PRODUCT_IP}. Please open firewall rule and retry."
fi
if [[ "$SUB_STATUS" == "unreachable" ]]; then
    fatal "Port ${SUB_PORT} is not reachable on ${PRODUCT_IP}. Please open firewall rule and retry."
fi
ok "Both ports are reachable"

# ── Install layout ────────────────────────────────────────────────────────────
INSTALL_ROOT="/neuroops"
AGENT_DIR="${INSTALL_ROOT}/neuroops-agent"

info "Installing to ${INSTALL_ROOT} ..."

mkdir -p "${INSTALL_ROOT}" "${INSTALL_ROOT}/tmp"

# ── Extract components ────────────────────────────────────────────────────────
info "Extracting Python runtime ..."
tar -zxf python-embedded-3.9-agent.tar.gz -C "${INSTALL_ROOT}/"

info "Copying Go runtime ..."
cp -r go "${INSTALL_ROOT}/"

info "Copying Go path (dependencies) ..."
cp -r gopath "${INSTALL_ROOT}/"

info "Copying agent package ..."
cp -r neuroops-agent "${INSTALL_ROOT}/"

mkdir -p "${AGENT_DIR}/logs" "${AGENT_DIR}/config" "${AGENT_DIR}/plugins/metric" \
         "${AGENT_DIR}/plugins/runbook" "${AGENT_DIR}/plugins/topology"

# ── Copy platform binary ──────────────────────────────────────────────────────
UBUNTU_DIR="ubuntu-${OS_VERSION}"
info "Copying agent binary for Ubuntu ${OS_VERSION} ..."
if [[ -f "${UBUNTU_DIR}/neuroops-agent" ]]; then
    cp "${UBUNTU_DIR}/neuroops-agent"  "${AGENT_DIR}/neuroops-agent"
    cp "${UBUNTU_DIR}/neuroops-upgrader" "${AGENT_DIR}/neuroops-upgrader" 2>/dev/null || true
else
    # Fall back to the included binary
    warn "Platform binary not found in ${UBUNTU_DIR}/ — using bundled binary"
fi
chmod +x "${AGENT_DIR}/neuroops-agent" "${AGENT_DIR}/neuroops-upgrader" 2>/dev/null || true

# ── Copy network utilities ────────────────────────────────────────────────────
cp traceroute /usr/bin/traceroute 2>/dev/null || true
cp trip       /usr/bin/trip       2>/dev/null || true

# ── IBM MQ soft-links (for pymqi) ────────────────────────────────────────────
if ls "${INSTALL_ROOT}/python-embedded/mqm/lib64/libmq"* &>/dev/null; then
    ln -sf "${INSTALL_ROOT}/python-embedded/mqm/lib64/libmq"* /usr/lib/x86_64-linux-gnu/ 2>/dev/null || true
fi

# ── Install Python SDK ────────────────────────────────────────────────────────
info "Installing NeurOps Python SDK ..."
"${INSTALL_ROOT}/python-embedded/bin/pip3" install neuroopssdk \
    --no-cache-dir --no-index --find-links=. 2>/dev/null || \
"${INSTALL_ROOT}/python-embedded/bin/pip3" install \
    "${AGENT_DIR}/sdk/python" --no-cache-dir 2>/dev/null || true

# ── Install Go SDK ────────────────────────────────────────────────────────────
info "Installing NeurOps Go SDK ..."
rm -rf "${INSTALL_ROOT}/gopath/src/neuroopssdk"
tar -xf go-sdk.tar.gz -C "${INSTALL_ROOT}/gopath/src/" 2>/dev/null || \
    cp -r sdk/go "${INSTALL_ROOT}/gopath/src/neuroopssdk" 2>/dev/null || true

# ── Patch agent.json with product host and ports ─────────────────────────────
info "Configuring agent.json ..."
AGENT_JSON="${AGENT_DIR}/config/agent.json"
if [[ -f "${AGENT_JSON}" ]]; then
    sed -i "s/\"127.0.0.1\"/\"${PRODUCT_IP}\"/g"     "${AGENT_JSON}"
    sed -i "s/\"neuroops.event.publisher.port\": 9441/\"neuroops.event.publisher.port\": ${PUB_PORT}/g" "${AGENT_JSON}"
    sed -i "s/\"neuroops.event.subscriber.port\": 9440/\"neuroops.event.subscriber.port\": ${SUB_PORT}/g" "${AGENT_JSON}"
else
    warn "agent.json not found at ${AGENT_JSON} — default config will be used"
fi

# ── Install systemd services ──────────────────────────────────────────────────
info "Installing systemd services ..."
cp services/neuroops-agent.service    /lib/systemd/system/neuroops-agent.service
cp services/neuroops-upgrader.service /lib/systemd/system/neuroops-upgrader.service

systemctl daemon-reload
systemctl enable neuroops-agent.service
systemctl enable neuroops-upgrader.service

# ── Start the agent ───────────────────────────────────────────────────────────
info "Starting NeurOps Agent ..."
systemctl start neuroops-upgrader.service

sleep 3

if systemctl is-active --quiet neuroops-upgrader.service; then
    ok "neuroops-upgrader service is running"
else
    warn "neuroops-upgrader failed to start. Check: journalctl -u neuroops-upgrader"
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GRN}${BOLD}══════════════════════════════════════════════════════════${NC}"
echo -e "${GRN}${BOLD}  NeurOps Agent v1.0.0 installed successfully!${NC}"
echo -e "${GRN}${BOLD}══════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  Product server : ${BOLD}${PRODUCT_IP}${NC}"
echo -e "  Publisher port : ${BOLD}${PUB_PORT}${NC}  (agent → product)"
echo -e "  Subscriber port: ${BOLD}${SUB_PORT}${NC}  (product → agent)"
echo -e "  Install path   : ${BOLD}${AGENT_DIR}${NC}"
echo -e "  Health endpoint: ${BOLD}http://localhost:8765/health${NC}"
echo -e "  Log directory  : ${BOLD}${AGENT_DIR}/logs/${NC}"
echo ""
echo -e "  Manage the agent:"
echo -e "    ${BOLD}systemctl status  neuroops-agent${NC}"
echo -e "    ${BOLD}systemctl restart neuroops-agent${NC}"
echo -e "    ${BOLD}systemctl stop    neuroops-agent${NC}"
echo ""
