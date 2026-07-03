#!/usr/bin/env bash
# Cài Argo CD addon — wrapper an toàn (pin version + preflight).
set -euo pipefail
source "$(dirname "$0")/../lib/common.sh"
# shellcheck source=/dev/null
source "$(dirname "$0")/../lib/addon-preflight.sh"

log "=== Addon: Argo CD (pin ${ARGOCD_CHART_VERSION:-?}) ==="
addon_require_core_bootstrap
addon_preflight_argocd

if is_addon_done argocd; then
  log "Argo CD addon đã xong."
  exit 0
fi

bash "$(addon_script argocd install)"
