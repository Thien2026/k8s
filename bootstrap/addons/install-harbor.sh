#!/usr/bin/env bash
# Cài Harbor addon — wrapper an toàn (pin version + preflight).
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=/dev/null
source "$(dirname "$0")/../lib/addon-preflight.sh"

log "=== Addon: Harbor (pin ${HARBOR_CHART_VERSION:-?}) ==="
addon_require_core_bootstrap
addon_preflight_harbor

if is_addon_done harbor; then
  log "Harbor addon đã xong — redeploy Console để đọc harbor.env..."
  FORCE_BUILD=1 bash "$(core_step 08-portal)" || true
  exit 0
fi

bash "$(addon_script harbor install)"
