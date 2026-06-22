#!/usr/bin/env bash
# Truy cập Rancher UI nhanh từ VN — traffic đi qua SSH vào VPS (EU), không tải 7MB JS qua đường dài.
#
# Cách dùng:
#   1. ./scripts/rancher-tunnel.sh
#   2. Mở https://rancher.161-97-176-55.sslip.io:8443/dashboard/auth/login
#   3. Ctrl+C để tắt tunnel (script tự gỡ /etc/hosts nếu đã thêm)
#
set -euo pipefail

VPS="${VPS:-root@161.97.176.55}"
RANCHER_HOST="${RANCHER_HOST:-rancher.161-97-176-55.sslip.io}"
LOCAL_PORT="${LOCAL_PORT:-8443}"
HOSTS_LINE="127.0.0.1 ${RANCHER_HOST}"
HOSTS_FILE="/etc/hosts"
ADDED_HOSTS=0

cleanup() {
  if [[ "${ADDED_HOSTS}" -eq 1 ]]; then
    echo ""
    echo "Gỡ ${RANCHER_HOST} khỏi /etc/hosts..."
    sudo sed -i.bak "/${RANCHER_HOST}/d" "${HOSTS_FILE}" 2>/dev/null || \
      sudo sed -i '' "/${RANCHER_HOST}/d" "${HOSTS_FILE}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

if ! grep -q "${RANCHER_HOST}" "${HOSTS_FILE}" 2>/dev/null; then
  echo "Thêm ${HOSTS_LINE} vào ${HOSTS_FILE} (cần sudo)..."
  echo "${HOSTS_LINE}" | sudo tee -a "${HOSTS_FILE}" >/dev/null
  ADDED_HOSTS=1
fi

echo ""
echo "Tunnel: localhost:${LOCAL_PORT} → VPS:443"
echo "Mở:     https://${RANCHER_HOST}:${LOCAL_PORT}/dashboard/auth/login"
echo "User:   admin  |  Password: xem /root/k8s/config/rancher-admin.env trên VPS"
echo ""
echo "Giữ terminal này mở. Ctrl+C để thoát."
echo ""

exec ssh -N -o ServerAliveInterval=60 \
  -L "${LOCAL_PORT}:127.0.0.1:443" \
  "${VPS}"
