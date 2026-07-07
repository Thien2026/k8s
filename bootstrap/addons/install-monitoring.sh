#!/usr/bin/env bash
# Cài Monitoring addon (Prometheus + Grafana + Alertmanager).
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=/dev/null
source "$(dirname "$0")/../lib/addon-preflight.sh"

log "=== Addon: Monitoring (pin ${PROMETHEUS_STACK_CHART_VERSION:-?}) ==="
addon_require_core_bootstrap
addon_preflight_monitoring

if is_addon_done monitoring; then
  log "Monitoring addon đã xong (state file). Chạy lại install.sh để upgrade nếu cần."
  bash "$(addon_script monitoring install)"
  exit 0
fi

bash "$(addon_script monitoring install)"
