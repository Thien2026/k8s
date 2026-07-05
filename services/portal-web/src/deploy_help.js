/** Hướng dẫn deploy trên UI — đồng bộ docs/KHACH_DEPLOY.md + MICRO_DEPLOY.md */
var DEPLOY_HELP_SECTIONS = [
  {
    id: "steps",
    title: "4 bước",
    html:
      '<p class="deploy-help-lead">Monorepo <strong>backend + frontend</strong> — một project, hai image <code>api</code> + <code>web</code>, một domain.</p>' +
      '<div class="deploy-help-checklist">' +
      "<h4>Bước 1 — Nguồn GitHub</h4>" +
      "<ul class=\"deploy-help-list\">" +
      "<li>Tab <strong>Deploy / Git</strong> → chọn <strong>Repository</strong> + <strong>Branch</strong> (có <code>backend/</code> + <code>frontend/</code>).</li>" +
      "<li><strong>Deploy env = dev</strong> — đừng chọn prod lần đầu.</li>" +
      "<li>Bật <strong>Tự deploy lên cluster khi build xong</strong> (sau khi Workflow OK).</li>" +
      "</ul>" +
      "<h4>Bước 2 — Web + API riêng</h4>" +
      "<ul class=\"deploy-help-list\">" +
      "<li>Chọn <strong>Web + API riêng</strong> (không chọn Một website).</li>" +
      "<li>Banner repo → <strong>「Bước 2: Áp dụng api + web từ repo」</strong>.</li>" +
      "<li>Kiểm tra: <code>api</code> → <code>backend/</code>, ingress <code>/api</code> · <code>web</code> → <code>frontend/</code>, ingress <code>/</code>.</li>" +
      "</ul>" +
      "<h4>Bước 3 — Lưu &amp; đồng bộ GitHub (bắt buộc)</h4>" +
      "<ul class=\"deploy-help-list\">" +
      "<li>Bấm <strong>「Lưu &amp; đồng bộ GitHub」</strong> — không chỉ 「Chỉ lưu Console」.</li>" +
      "<li>Badge phải <strong>Workflow OK</strong> (không còn Cần đồng bộ).</li>" +
      "<li><strong>Cấu hình app</strong> → Build dev: <code>VITE_API_BASE=/api</code>.</li>" +
      "<li>Project cũ sau cập nhật platform → sync một lần, không cần đổi config.</li>" +
      "</ul>" +
      "<h4>Bước 4 — Push &amp; kiểm tra</h4>" +
      "<ul class=\"deploy-help-list\">" +
      "<li>Push commit → tab Deploy đợi <strong>success</strong>.</li>" +
      "<li>Mở <code>https://{slug}-dev…/</code> và <code>…/api/health</code> → <code>{\"status\":\"ok\"}</code>.</li>" +
      "</ul></div>" +
      '<p class="muted deploy-help-note">Repo mới: copy template <code>templates/back-front/</code> hoặc script <code>setup-back-front-pilot.sh</code> (team nội bộ).</p>',
  },
  {
    id: "dont",
    title: "Đừng làm",
    html:
      '<p class="deploy-help-lead">Hay gây lỗi — khách thường vấp ở đây.</p>' +
      "<table class=\"data-table deploy-help-table\"><thead><tr><th>Sai</th><th>Hậu quả</th></tr></thead><tbody>" +
      "<tr><td>Push trước <strong>Lưu &amp; đồng bộ GitHub</strong></td><td>Build/deploy lệch Console, 503</td></tr>" +
      "<tr><td>Chọn <strong>Một website</strong> khi repo api+web</td><td>Ingress sai, không có pod</td></tr>" +
      "<tr><td>Chỉ bấm <strong>Chỉ lưu Console</strong></td><td>Workflow GitHub chưa cập nhật</td></tr>" +
      "<tr><td><strong>Deploy lại</strong> bản khác kiểu chạy</td><td>Không đổi topology — dùng <strong>Đổi kiểu chạy…</strong></td></tr>" +
      "<tr><td>Thiếu <code>VITE_API_BASE=/api</code></td><td>Frontend gọi API sai URL</td></tr>" +
      "<tr><td>Deploy env = <strong>prod</strong> lần đầu</td><td>Push thẳng production — rủi ro</td></tr>" +
      "</tbody></table>",
  },
  {
    id: "env",
    title: "Env",
    html:
      "<table class=\"data-table deploy-help-table\"><thead><tr><th>Biến</th><th>Scope</th><th>Giá trị</th></tr></thead><tbody>" +
      "<tr><td><code>VITE_API_BASE</code></td><td>Build (dev)</td><td><code>/api</code></td></tr>" +
      "<tr><td><code>API_ROUTE_PREFIX</code></td><td>Runtime</td><td><code>/api</code></td></tr>" +
      "</tbody></table>" +
      '<p class="muted">Tab <strong>Cấu hình app</strong> → Build / Pod. Chọn Web + API thường tự điền.</p>' +
      '<p class="muted">Repo có <code>.platform/build.yaml</code> + <code>services.yaml</code>.</p>',
  },
  {
    id: "prod",
    title: "Lên prod",
    html:
      '<p><strong>Cách A — Promote (khuyến nghị)</strong></p>' +
      "<ol class=\"deploy-help-steps\">" +
      "<li>Dev deploy <strong>success</strong> — api + web <strong>cùng tag SHA</strong>.</li>" +
      "<li>Tab <strong>Promote Prod</strong> → checklist xanh.</li>" +
      "<li><strong>Promote</strong> — cùng image, không build lại.</li>" +
      "<li>Kiểm <code>…-prod…/</code> và <code>/api/health</code>.</li>" +
      "</ol>" +
      '<p><strong>Cách B — Deploy thẳng prod</strong></p>' +
      "<p class=\"muted\">Deploy env = prod → sync → mỗi push lên prod. Chỉ khi chủ đích.</p>",
  },
  {
    id: "rules",
    title: "Quy tắc",
    html:
      "<ul class=\"deploy-help-list\">" +
      "<li><strong>Deploy lại</strong> — đổi tag, <em>cùng</em> kiểu chạy.</li>" +
      "<li><strong>Đổi kiểu chạy…</strong> — single ↔ multi; deploy bản mới. <em>Không</em> rollback.</li>" +
      "<li>Sau đổi kiểu → <strong>Lưu &amp; đồng bộ GitHub</strong> trước khi push.</li>" +
      "<li>Promote multi = <strong>cùng tag</strong> cho api + web.</li>" +
      "</ul>",
  },
  {
    id: "troubleshoot",
    title: "Lỗi",
    html:
      "<table class=\"data-table deploy-help-table\"><thead><tr><th>Triệu chứng</th><th>Xử lý</th></tr></thead><tbody>" +
      "<tr><td>Badge <strong>Cần đồng bộ</strong></td><td>Bước 3 — Lưu &amp; đồng bộ GitHub</td></tr>" +
      "<tr><td>503, không có pod</td><td>Đổi kiểu chạy → sync → deploy mới</td></tr>" +
      "<tr><td>Build multi, Console single</td><td>Chưa sync sau đổi layout</td></tr>" +
      "<tr><td>Site trắng / API lỗi</td><td><code>VITE_API_BASE=/api</code> → sync lại</td></tr>" +
      "<tr><td>GitHub Actions fail</td><td>Log Actions — secret / Dockerfile path</td></tr>" +
      "<tr><td>Project cũ sau update platform</td><td>Sync workflow một lần</td></tr>" +
      "</tbody></table>" +
      '<p class="muted deploy-help-note">Báo support: gửi slug project, branch, screenshot tab Deploy/Git.</p>',
  },
];

function renderDeployHelpButton(sectionId, extraClass) {
  sectionId = sectionId || "steps";
  extraClass = extraClass || "";
  return (
    '<button type="button" class="btn-help deploy-help-trigger ' +
    extraClass +
    '" data-help-section="' +
    esc(sectionId) +
    '" title="Hướng dẫn deploy (api + web)" aria-label="Hướng dẫn deploy">' +
    "<span aria-hidden=\"true\">?</span></button>"
  );
}

/** Card cố định trên tab Deploy / Git — không cần mở file doc */
function renderDeployHelpInlineCard(slug, platformDomain) {
  platformDomain = platformDomain || "platform.7mlabs.com";
  slug = (slug || "").trim();
  const devHost = slug ? slug + "-dev." + platformDomain : "{slug}-dev." + platformDomain;
  const devUrl = "https://" + devHost + "/";
  const healthUrl = "https://" + devHost + "/api/health";
  return (
    '<div class="deploy-help-inline-card">' +
    '<div class="deploy-help-inline-head">' +
    "<span><strong>Hướng dẫn deploy</strong> · api + web · Phase 1</span>" +
    renderDeployHelpButton("steps", "btn-help-inline") +
    "</div>" +
    '<ol class="deploy-help-inline-steps">' +
    "<li>Repo + branch · <em>Deploy env = dev</em></li>" +
    "<li><strong>Web + API riêng</strong> → 「Áp dụng api + web từ repo」</li>" +
    "<li><strong>Lưu &amp; đồng bộ GitHub</strong> → badge Workflow OK</li>" +
    "<li>Push → kiểm <a href=\"" +
    esc(healthUrl) +
    '" target="_blank" rel="noopener"><code>/api/health</code></a> + <a href="' +
    esc(devUrl) +
    '" target="_blank" rel="noopener"><code>/</code></a></li>' +
    "</ol>" +
    '<div class="deploy-help-inline-actions">' +
    '<button type="button" class="btn-ghost btn-sm deploy-help-trigger" data-help-section="dont">Đừng làm gì?</button>' +
    '<button type="button" class="btn-ghost btn-sm deploy-help-trigger" data-help-section="troubleshoot">Lỗi thường gặp</button>' +
    '<button type="button" class="btn-primary btn-sm deploy-help-trigger" data-help-section="steps">Xem đầy đủ</button>' +
    "</div></div>"
  );
}

function openDeployHelpDialog(sectionId) {
  sectionId = sectionId || "steps";
  return new Promise(function (resolve) {
    const overlay = document.createElement("div");
    overlay.className = "ui-overlay";
    const tabs = DEPLOY_HELP_SECTIONS.map(function (s) {
      return (
        '<button type="button" class="deploy-help-tab' +
        (s.id === sectionId ? " active" : "") +
        '" data-section="' +
        esc(s.id) +
        '">' +
        esc(s.title) +
        "</button>"
      );
    }).join("");
    const panels = DEPLOY_HELP_SECTIONS.map(function (s) {
      return (
        '<div class="deploy-help-panel' +
        (s.id === sectionId ? " active" : "") +
        '" data-section="' +
        esc(s.id) +
        '">' +
        s.html +
        "</div>"
      );
    }).join("");
    overlay.innerHTML =
      '<div class="ui-dialog ui-dialog-help" role="dialog" aria-modal="true" aria-labelledby="deploy-help-title">' +
      '<div class="ui-dialog-glow"></div>' +
      '<div class="deploy-help-header">' +
      '<h3 class="ui-dialog-title" id="deploy-help-title">Hướng dẫn deploy</h3>' +
      '<p class="muted deploy-help-sub">Micro thường · api + web · làm trực tiếp trên Console</p>' +
      "</div>" +
      '<nav class="deploy-help-tabs" role="tablist">' +
      tabs +
      "</nav>" +
      '<div class="deploy-help-body">' +
      panels +
      "</div>" +
      '<div class="ui-dialog-actions deploy-help-actions">' +
      '<button type="button" class="btn-primary ui-dialog-ok">Đóng</button></div></div>';

    function close() {
      overlay.classList.remove("show");
      setTimeout(function () {
        overlay.remove();
        document.removeEventListener("keydown", onKey);
        resolve();
      }, 200);
    }

    function onKey(e) {
      if (e.key === "Escape") close();
    }

    overlay.querySelectorAll(".deploy-help-tab").forEach(function (tab) {
      tab.onclick = function () {
        const id = tab.dataset.section;
        overlay.querySelectorAll(".deploy-help-tab").forEach(function (t) {
          t.classList.toggle("active", t.dataset.section === id);
        });
        overlay.querySelectorAll(".deploy-help-panel").forEach(function (p) {
          p.classList.toggle("active", p.dataset.section === id);
        });
      };
    });

    overlay.querySelector(".ui-dialog-ok").onclick = close;
    overlay.onclick = function (e) {
      if (e.target === overlay) close();
    };
    document.body.appendChild(overlay);
    document.addEventListener("keydown", onKey);
    requestAnimationFrame(function () {
      overlay.classList.add("show");
      overlay.querySelector(".ui-dialog-ok").focus();
    });
  });
}

function bindDeployHelpTriggers(root) {
  (root || document).querySelectorAll(".deploy-help-trigger").forEach(function (btn) {
    if (btn.dataset.helpBound === "1") return;
    btn.dataset.helpBound = "1";
    btn.onclick = function (e) {
      e.preventDefault();
      e.stopPropagation();
      openDeployHelpDialog(btn.dataset.helpSection || "steps");
    };
  });
}
