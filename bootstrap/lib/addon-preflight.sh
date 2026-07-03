#!/usr/bin/env bash
# Preflight addon — pin chart version, kiểm tra tài nguyên, tránh upgrade lệch.
# shellcheck shell=bash

# Đổi ngưỡng: ADDON_<NAME>_MIN_MEM_MB / ADDON_<NAME>_MIN_DISK_GB trong config/env.sh
# Bỏ qua: SKIP_RESOURCE_CHECK=1 | Cài dù thiếu: FORCE_RESOURCE=1

addon_parse_memory_to_mi() {
  local v="${1:-}"
  [[ -z "$v" ]] && echo 0 && return
  if [[ "$v" =~ ^([0-9]+)Ki$ ]]; then
    echo $(( BASH_REMATCH[1] / 1024 ))
  elif [[ "$v" =~ ^([0-9]+)Mi$ ]]; then
    echo "${BASH_REMATCH[1]}"
  elif [[ "$v" =~ ^([0-9]+)Gi$ ]]; then
    echo $(( BASH_REMATCH[1] * 1024 ))
  else
    echo 0
  fi
}

addon_resource_defaults() {
  local addon="$1"
  case "$addon" in
    rancher)
      ADDON_MIN_MEM_MB="${ADDON_RANCHER_MIN_MEM_MB:-2048}"
      ADDON_MIN_DISK_GB="${ADDON_RANCHER_MIN_DISK_GB:-15}"
      ;;
    harbor)
      ADDON_MIN_MEM_MB="${ADDON_HARBOR_MIN_MEM_MB:-3072}"
      ADDON_MIN_DISK_GB="${ADDON_HARBOR_MIN_DISK_GB:-40}"
      ;;
    argocd)
      ADDON_MIN_MEM_MB="${ADDON_ARGOCD_MIN_MEM_MB:-1536}"
      ADDON_MIN_DISK_GB="${ADDON_ARGOCD_MIN_DISK_GB:-10}"
      ;;
    *)
      ADDON_MIN_MEM_MB="${ADDON_DEFAULT_MIN_MEM_MB:-2048}"
      ADDON_MIN_DISK_GB="${ADDON_DEFAULT_MIN_DISK_GB:-20}"
      ;;
  esac
}

addon_check_resources() {
  local addon="$1"
  addon_resource_defaults "$addon"

  if [[ "${SKIP_RESOURCE_CHECK:-}" == "1" ]]; then
    log "WARN: SKIP_RESOURCE_CHECK=1 — bỏ qua kiểm tra tài nguyên"
    return 0
  fi

  log "Kiểm tra tài nguyên trước khi cài addon: ${addon}"
  log "  Ngưỡng tối thiểu: RAM available ≥ ${ADDON_MIN_MEM_MB} MiB, disk ≥ ${ADDON_MIN_DISK_GB} GiB"

  local mem_avail_mb=0
  if [[ -r /proc/meminfo ]]; then
    mem_avail_mb="$(awk '/MemAvailable:/{printf "%d", $2/1024}' /proc/meminfo)"
  elif command -v free >/dev/null 2>&1; then
    mem_avail_mb="$(free -m | awk '/^Mem:/{print $7}')"
  fi

  local k8s_mem_mi=0 k8s_cpu_m=0
  if kubectl get nodes >/dev/null 2>&1; then
    local raw_mem raw_cpu
    raw_mem="$(kubectl get nodes -o jsonpath='{.items[0].status.allocatable.memory}' 2>/dev/null || true)"
    raw_cpu="$(kubectl get nodes -o jsonpath='{.items[0].status.allocatable.cpu}' 2>/dev/null || true)"
    k8s_mem_mi="$(addon_parse_memory_to_mi "$raw_mem")"
    k8s_cpu_m="${raw_cpu%m}"
    [[ "$k8s_cpu_m" =~ ^[0-9]+$ ]] || k8s_cpu_m=0
  fi

  local top_line used_mi=0
  top_line="$(kubectl top nodes --no-headers 2>/dev/null | head -1 || true)"
  if [[ -n "$top_line" ]]; then
    used_mi="$(echo "$top_line" | awk '{print $3}' | sed 's/Mi//')"
    [[ "$used_mi" =~ ^[0-9]+$ ]] || used_mi=0
  fi

  local disk_path="/var/lib/rancher"
  [[ -d "$disk_path" ]] || disk_path="/"
  local disk_avail_gb=0
  disk_avail_gb="$(df -BG "$disk_path" 2>/dev/null | awk 'NR==2{gsub(/G/,""); print $4}')"
  [[ "$disk_avail_gb" =~ ^[0-9]+$ ]] || disk_avail_gb=0

  log "  Host MemAvailable:     ${mem_avail_mb} MiB"
  if [[ "$k8s_mem_mi" -gt 0 ]]; then
    log "  Node allocatable RAM:  ${k8s_mem_mi} MiB, CPU: ${k8s_cpu_m}m"
    if [[ "$used_mi" -gt 0 ]]; then
      log "  Node RAM đang dùng:    ~${used_mi} MiB (ước tính còn ~$((k8s_mem_mi - used_mi)) MiB)"
    else
      log "  (Chưa có metrics-server — chỉ xem allocatable, không có top nodes)"
    fi
  fi
  log "  Disk khả dụng (${disk_path}): ${disk_avail_gb} GiB"

  local fail=0
  if [[ "$mem_avail_mb" -lt "$ADDON_MIN_MEM_MB" ]]; then
    log "✗ RAM host thấp: cần ≥${ADDON_MIN_MEM_MB} MiB MemAvailable, hiện ${mem_avail_mb} MiB"
    fail=1
  fi
  if [[ "$disk_avail_gb" -lt "$ADDON_MIN_DISK_GB" ]]; then
    log "✗ Disk thấp: cần ≥${ADDON_MIN_DISK_GB} GiB trống, hiện ${disk_avail_gb} GiB"
    fail=1
  fi
  if [[ "$k8s_mem_mi" -gt 0 && "$used_mi" -gt 0 ]]; then
    local est_free=$((k8s_mem_mi - used_mi))
    if [[ "$est_free" -lt "$ADDON_MIN_MEM_MB" ]]; then
      log "✗ RAM cluster ước tính còn ~${est_free} MiB — có thể không đủ cho pod ${addon}"
      fail=1
    fi
  fi

  if [[ "$fail" -eq 1 ]]; then
    if [[ "${FORCE_RESOURCE:-}" == "1" ]]; then
      log "WARN: FORCE_RESOURCE=1 — tiếp tục dù thiếu tài nguyên (pod có thể Pending/OOM)"
      return 0
    fi
    log ""
    log "Gợi ý:"
    log "  • Nâng RAM VPS hoặc giảm workload đang chạy"
    log "  • Harbor: tiếp tục dùng GHCR (không cài Harbor on-prem)"
    log "  • Chỉ xem, không cài: ./bootstrap/addons/run.sh check ${addon}"
    log "  • Bỏ qua: SKIP_RESOURCE_CHECK=1 hoặc FORCE_RESOURCE=1 ./bootstrap/addons/run.sh ${addon}"
    exit 1
  fi

  log "✓ Tài nguyên đủ để cài ${addon}"
}

addon_check_resources_only() {
  local addon="$1"
  export KUBECONFIG="${KUBECONFIG:-${ROOT_DIR}/kubeconfig/rke2.yaml}"
  export PATH="/var/lib/rancher/rke2/bin:${PATH}"
  if ! kubectl get nodes >/dev/null 2>&1; then
    log "Cluster chưa sẵn sàng — chạy ./bootstrap/run.sh đến bước 02-kubeconfig trước."
    exit 1
  fi
  case "$addon" in
    rancher) addon_preflight_rancher_chart_only ;;
    harbor)  addon_preflight_harbor_chart_only ;;
    argocd)  addon_preflight_argocd_chart_only ;;
    *) log "Addon không hỗ trợ check: ${addon}"; exit 1 ;;
  esac
  addon_check_resources "$addon"
  log "Kết luận: có thể thử cài ${addon} (chưa chạy helm install)."
}

addon_preflight_rancher_chart_only() {
  addon_assert_chart_pinned RANCHER_CHART_VERSION Rancher
  addon_assert_helm_chart_match rancher cattle-system "${RANCHER_CHART_VERSION}"
}

addon_preflight_harbor_chart_only() {
  if [[ -z "${HARBOR_CHART_VERSION:-}" ]]; then
    HARBOR_CHART_VERSION="1.19.1"
    log "WARN: HARBOR_CHART_VERSION chưa set trong env.sh — dùng mặc định ${HARBOR_CHART_VERSION} khi check"
  fi
  addon_assert_chart_pinned HARBOR_CHART_VERSION Harbor
  addon_assert_helm_chart_match harbor harbor "${HARBOR_CHART_VERSION}"
}

addon_preflight_argocd_chart_only() {
  addon_assert_chart_pinned ARGOCD_CHART_VERSION ArgoCD
  addon_assert_helm_chart_match argocd argocd "${ARGOCD_CHART_VERSION}"
}

addon_require_core_bootstrap() {
  kube_ready
  helm_ready
  if is_step_done "$(core_step "08-portal")"; then
    return 0
  fi
  # Tương thích VPS cũ: có thể mất state file nhưng portal đã chạy.
  if kubectl -n platform get deploy portal-api >/dev/null 2>&1 && kubectl -n platform get deploy portal-web >/dev/null 2>&1; then
    log "Không thấy state 08-portal nhưng portal đang chạy — tiếp tục addon."
    return 0
  fi
  log "Thiếu core bootstrap 08-portal — chạy ./bootstrap/run.sh next đến khi Console OK."
  exit 1
}

addon_assert_chart_pinned() {
  local var="$1"
  local label="$2"
  # shellcheck disable=SC2154
  local val="${!var:-}"
  if [[ -z "${val}" ]]; then
    log "Thiếu ${var} trong config/env.sh — pin version ${label} (xem config/env.sh.example)."
    exit 1
  fi
  log "Chart ${label} pin: ${val}"
}

addon_assert_helm_chart_match() {
  local release="$1"
  local ns="$2"
  local expected="$3"
  if ! helm status "${release}" -n "${ns}" >/dev/null 2>&1; then
    return 0
  fi
  local current
  current="$(helm list -n "${ns}" -f "^${release}$" -o json 2>/dev/null | sed -n 's/.*"chart":"[^-]*-\([^"]*\)".*/\1/p' | head -1)"
  if [[ -z "${current}" ]]; then
    return 0
  fi
  if [[ "${current}" != "${expected}" && "${FORCE_VERSION:-}" != "1" ]]; then
    log "✗ ${release} đang chart ${current}, env pin ${expected}."
    log "  Không tự upgrade để tránh lệch phiên bản."
    log "  Nếu cố ý: FORCE_VERSION=1 ./bootstrap/addons/run.sh ${release}"
    exit 1
  fi
  log "Helm ${release} chart ${current} khớp pin ${expected}"
}

addon_preflight_rancher() {
  addon_check_resources rancher
  addon_preflight_rancher_chart_only
  local k8s_minor
  k8s_minor="$(kubectl version --short 2>/dev/null | awk '/Server Version/{print $3}' | sed 's/^v//' | cut -d. -f1,2)"
  if [[ -n "${k8s_minor}" ]]; then
    log "Kubernetes server: v${k8s_minor} — Rancher chart ${RANCHER_CHART_VERSION}"
  fi
}

addon_preflight_harbor() {
  addon_check_resources harbor
  addon_preflight_harbor_chart_only
}

addon_preflight_argocd() {
  addon_check_resources argocd
  addon_preflight_argocd_chart_only
}
