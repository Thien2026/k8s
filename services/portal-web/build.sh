#!/usr/bin/env bash
# Gộp CSS + JS vào 1 file HTML — chỉ 1 request tải trang
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CSS="$(cat "$DIR/src/style.css")"
HELP="$(cat "$DIR/src/deploy_help.js")"
K8SHELP="$(cat "$DIR/src/k8s_ops_help.js")"
JS="$(cat "$DIR/src/app.js")"
JS="${HELP}"$'\n'"${K8SHELP}"$'\n'"${JS}"
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
cat >"$DIR/dist/index.html" <<EOF
<!doctype html>
<html lang="vi">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Platform Console</title>
    <style>
${CSS}
    </style>
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
    <script>
${JS}
    </script>
  </body>
</html>
EOF
echo "Built dist/index.html ($(wc -c <"$DIR/dist/index.html") bytes)"
