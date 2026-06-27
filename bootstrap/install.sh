#!/usr/bin/env bash
# Cài Platform từ source — một lệnh, idempotent (chạy lại an toàn).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ ! -f config/env.sh ]]; then
  if [[ -f config/env.sh.example ]]; then
    cp config/env.sh.example config/env.sh
    echo "Đã tạo config/env.sh — sửa DOMAIN, IP, mật khẩu rồi chạy lại:"
    echo "  $0"
    exit 1
  fi
  echo "Thiếu config/env.sh"
  exit 1
fi

chmod +x bootstrap/run.sh bootstrap/core/steps/*.sh bootstrap/addons/*.sh 2>/dev/null || true

echo "=== Platform install (core 00–08) ==="
echo "Log: bootstrap/logs/"
echo ""

while true; do
  output=$(./bootstrap/run.sh next 2>&1) || {
    echo "$output"
    exit 1
  }
  echo "$output"
  if [[ "$output" == *"Tất cả bước đã xong"* ]]; then
    break
  fi
done

echo ""
echo "✓ Core xong. Console: https://platform.\${DOMAIN} (xem config/env.sh)"
echo ""
echo "Addon (tuỳ chọn):"
echo "  ./bootstrap/addons/run.sh rancher"
echo "  ./bootstrap/addons/run.sh harbor"
echo ""
echo "Cập nhật Console sau khi sửa code:"
echo "  FORCE_BUILD=1 ./bootstrap/run.sh 08 --force"
