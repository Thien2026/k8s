#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../../lib/common.sh"

log "Cài RKE2 server (single node)"

if [[ "$(id -u)" -ne 0 ]]; then
  log "Chạy với sudo trên VPS."
  exit 1
fi

if systemctl is-active --quiet rke2-server 2>/dev/null; then
  log "rke2-server đã chạy — bỏ qua cài đặt."
else
  if [[ -n "${RKE2_VERSION:-}" ]]; then
    curl -sfL https://get.rke2.io | INSTALL_RKE2_VERSION="${RKE2_VERSION}" sh -
  else
    curl -sfL https://get.rke2.io | sh -
  fi
  systemctl enable rke2-server
  systemctl start rke2-server
fi

log "Đợi rke2-server ready (có thể 1–3 phút)..."
for i in $(seq 1 60); do
  if systemctl is-active --quiet rke2-server && [[ -f /etc/rancher/rke2/rke2.yaml ]]; then
    break
  fi
  sleep 5
done

if ! systemctl is-active --quiet rke2-server; then
  log "rke2-server chưa active — xem: journalctl -u rke2-server -n 50"
  exit 1
fi

log "RKE2 server OK"
mark_step_done "$0"
