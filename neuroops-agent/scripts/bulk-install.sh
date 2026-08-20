#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# NeurOps Agent — Bulk / Silent Installer
# Copyright (c) NeurOps 2025. All rights reserved.
#
# Designed for mass deployment via SSH, Ansible, Chef, Puppet, or any
# configuration management tool. Takes all parameters as positional args.
#
# Usage:
#   sudo bash bulk-install.sh <product_ip> <pub_port> <sub_port>
#
# Example:
#   sudo bash bulk-install.sh 192.168.10.5 9441 9440
# ─────────────────────────────────────────────────────────────────────────────
set -euo pipefail

PRODUCT_IP="${1:?Usage: $0 <product_ip> <pub_port> <sub_port>}"
PUB_PORT="${2:-9441}"
SUB_PORT="${3:-9440}"

INSTALL_ROOT="/neuroops"
AGENT_DIR="${INSTALL_ROOT}/neuroops-agent"

echo "[neuroops-installer] Installing NeurOps Agent → product=${PRODUCT_IP} pub=${PUB_PORT} sub=${SUB_PORT}"

# Determine Ubuntu version
OS_VERSION=$(lsb_release -rs 2>/dev/null || echo "22.04")
case "$OS_VERSION" in
    18.04|20.04|22.04|24.04) : ;;
    *) OS_VERSION="22.04" ;;   # default
esac

# Extract and install
mkdir -p "${INSTALL_ROOT}" "${INSTALL_ROOT}/tmp"
tar -zxf python-embedded-3.9-agent.tar.gz -C "${INSTALL_ROOT}/"
cp -r go gopath neuroops-agent "${INSTALL_ROOT}/"
mkdir -p "${AGENT_DIR}"/{logs,config,plugins/{metric,runbook,topology}}

UBUNTU_DIR="ubuntu-${OS_VERSION}"
[[ -f "${UBUNTU_DIR}/neuroops-agent" ]] && cp "${UBUNTU_DIR}/neuroops-agent" "${AGENT_DIR}/neuroops-agent"
[[ -f "${UBUNTU_DIR}/neuroops-upgrader" ]] && cp "${UBUNTU_DIR}/neuroops-upgrader" "${AGENT_DIR}/neuroops-upgrader"
chmod +x "${AGENT_DIR}"/neuroops-{agent,upgrader} 2>/dev/null || true

cp traceroute /usr/bin/ 2>/dev/null || true
cp trip       /usr/bin/ 2>/dev/null || true

"${INSTALL_ROOT}/python-embedded/bin/pip3" install neuroopssdk \
    --no-cache-dir --no-index --find-links=. --quiet 2>/dev/null || true
rm -rf "${INSTALL_ROOT}/gopath/src/neuroopssdk"
tar -xf go-sdk.tar.gz -C "${INSTALL_ROOT}/gopath/src/" 2>/dev/null || \
    cp -r sdk/go "${INSTALL_ROOT}/gopath/src/neuroopssdk" 2>/dev/null || true

# Patch config
AGENT_JSON="${AGENT_DIR}/config/agent.json"
[[ -f "${AGENT_JSON}" ]] && {
    sed -i "s/\"127.0.0.1\"/\"${PRODUCT_IP}\"/g" "${AGENT_JSON}"
    sed -i "s/\"neuroops.event.publisher.port\": 9441/\"neuroops.event.publisher.port\": ${PUB_PORT}/g" "${AGENT_JSON}"
    sed -i "s/\"neuroops.event.subscriber.port\": 9440/\"neuroops.event.subscriber.port\": ${SUB_PORT}/g" "${AGENT_JSON}"
}

# Register and start
cp services/neuroops-agent.service    /lib/systemd/system/
cp services/neuroops-upgrader.service /lib/systemd/system/
systemctl daemon-reload
systemctl enable neuroops-agent.service neuroops-upgrader.service
systemctl start  neuroops-upgrader.service

echo "[neuroops-installer] Installation complete. Agent started."
