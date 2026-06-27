#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

log "Preflight — kiểm tra VPS trước khi cài RKE2"

require_cmd curl
require_cmd systemctl || true

if [[ "$(id -u)" -ne 0 ]]; then
  log "Chạy script này trên VPS với sudo hoặc root."
  exit 1
fi

. /etc/os-release
if [[ "${ID}" != "ubuntu" && "${ID}" != "debian" && "${ID}" != "rhel" && "${ID}" != "centos" && "${ID}" != "rocky" ]]; then
  log "Cảnh báo: OS ${ID} — script test trên Ubuntu. Có thể cần chỉnh tay."
fi

# Tắt swap (RKE2 khuyến nghị)
if swapon --show | grep -q .; then
  log "Tắt swap..."
  swapoff -a
  sed -i.bak '/ swap / s/^\(.*\)$/#\1/' /etc/fstab || true
fi

# Module kernel
modprobe overlay || true
modprobe br_netfilter || true

cat >/etc/sysctl.d/99-kubernetes.conf <<'EOF'
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF
sysctl --system >/dev/null

log "RAM: $(free -h | awk '/Mem:/ {print $2}')"
log "Disk: $(df -h / | awk 'NR==2 {print $4 " free"}')"
log "IP public (config): ${NODE_PUBLIC_IP}"
log "Domain: ${DOMAIN}"
log "Preflight OK"

mark_step_done "$0"
