#!/usr/bin/env bash
# Gộp CSS + JS vào 1 file HTML — chỉ 1 request tải trang
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CSS="$(cat "$DIR/src/style.css")"

# Thứ tự: state → core/ui → pages → bootstrap
FILES=(
  src/core_state.js
  src/deploy_help.js
  src/k8s_ops_help.js
  src/core_format.js
  src/ui_dialog.js
  src/core_api.js
  src/ui_common.js
  src/ui_auth.js
  src/ui_sidebar.js
  src/core_router.js
  src/ui_deploy_render.js
  src/ui_deploy_history.js
  src/ui_deploy_activity.js
  src/ui_deploy_pipeline.js
  src/ui_pipeline_layout.js
  src/ui_env_helpers.js
  src/ui_platform_pages.js
  src/ui_project_overview.js
  src/ui_project_tabs.js
  src/ui_project_config.js
  src/ui_project_env.js
  src/ui_project_deploy.js
  src/ui_project_backups.js
  src/ui_project_hub.js
  src/ui_overview_charts.js
  src/ui_k8s.js
  src/app.js
)

JS=""
NL=$'\n'
for f in "${FILES[@]}"; do
  JS+="$(cat "$DIR/$f")"
  JS+="$NL"
done
# Tránh đóng thẻ <script> sớm khi nhúng inline
JS="${JS//<\/script>/<\\/script>}"
TMPJS="$(mktemp).js"
printf '%s' "$JS" >"$TMPJS"
if command -v node >/dev/null 2>&1; then
  node --check "$TMPJS"
else
  echo "WARN: node không có — bỏ qua syntax check (nên build trên máy dev trước)"
fi
rm -f "$TMPJS"
cat >"$DIR/dist/index.html" <<'HTMLEOF'
<!doctype html>
<html lang="vi">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta http-equiv="Cache-Control" content="no-store, no-cache, must-revalidate" />
    <title>Platform Console</title>
    <style>
HTMLEOF
# Append CSS/JS without shell expanding $ in source
{
  printf '%s\n' "${CSS}"
  echo '    </style>
  </head>
  <body>
    <div class="layout">
      <aside class="sidebar">
        <div class="sidebar-brand">
          <div class="sidebar-brand-text">
            <h1>Platform Console</h1>
            <p>K8s Explorer</p>
          </div>
          <button type="button" id="sidebar-toggle" class="sidebar-toggle" title="Thu gọn / mở rộng sidebar">‹</button>
        </div>
        <nav id="sidebar-nav" class="sidebar-nav loading">Đang tải menu…</nav>
        <div class="sidebar-footer">© 2026 <strong>7mlabs</strong></div>
      </aside>
      <button type="button" id="sidebar-fab" class="sidebar-fab" title="Mở sidebar">☰</button>
      <main id="main" class="main">
        <p class="loading">Đang tải…</p>
      </main>
    </div>
    <script>'
  printf '%s' "${JS}"
  echo '</script>
  </body>
</html>'
} >>"$DIR/dist/index.html"
echo "Built dist/index.html ($(wc -c <"$DIR/dist/index.html") bytes)"
