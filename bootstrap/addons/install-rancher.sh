#!/usr/bin/env bash
# Cài Rancher addon — wrapper an toàn (pin version + preflight).
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=/dev/null
source "$(dirname "$0")/../lib/addon-preflight.sh"

log "=== Addon: Rancher (pin ${RANCHER_CHART_VERSION:-?}) ==="
addon_require_core_bootstrap
addon_preflight_rancher

if is_addon_done rancher; then
  log "Rancher addon đã xong — chỉ redeploy Console nếu cần token."
  if [[ -f "${ROOT_DIR}/config/rancher.env" ]] && grep -q 'RANCHER_TOKEN=token:' "${ROOT_DIR}/config/rancher.env" 2>/dev/null; then
    FORCE_BUILD=1 bash "$(core_step 08-portal)" || true
  fi
  exit 0
fi

bash "$(addon_script rancher install)"
