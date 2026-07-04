/** Nội dung hướng dẫn deploy — đồng bộ với docs/MICRO_DEPLOY.md */
var DEPLOY_HELP_SECTIONS = [
  {
    id: "steps",
    title: "4 bước",
    html:
      '<p class="deploy-help-lead">Monorepo <strong>backend + frontend</strong> — một project Console, hai image <code>api</code> + <code>web</code>, một domain.</p>' +
      '<ol class="deploy-help-steps">' +
      "<li><strong>Nguồn GitHub</strong> — chọn repo + branch. <em>Deploy env = dev</em> (prod chỉ khi chủ đích deploy thẳng production).</li>" +
      "<li><strong>Chốt kiểu chạy</strong> — chọn <strong>Web + API riêng</strong>. Có <code>.platform/services.yaml</code> → 「Áp dụng từ repo」.</li>" +
      "<li><strong>Lưu &amp; đồng bộ GitHub</strong> — bắt buộc sau mỗi lần đổi kiểu. Badge phải <strong>Workflow OK</strong>.</li>" +
      "<li><strong>Push</strong> — theo dõi 4 bước deploy; kiểm tra <code>/</code> và <code>/api/health</code>.</li>" +
      "</ol>" +
      '<p class="muted deploy-help-note">「Chỉ lưu Console」= nháp — chưa được push cho đến khi sync workflow.</p>',
  },
  {
    id: "env",
    title: "Env & contract",
    html:
      "<table class=\"data-table deploy-help-table\"><thead><tr><th>Biến</th><th>Scope</th><th>Giá trị</th></tr></thead><tbody>" +
      "<tr><td><code>VITE_API_BASE</code></td><td>Build (dev)</td><td><code>/api</code></td></tr>" +
      "<tr><td><code>API_ROUTE_PREFIX</code></td><td>Runtime</td><td><code>/api</code> (mặc định)</td></tr>" +
      "</tbody></table>" +
      '<p class="muted">Tab <strong>Cấu hình app</strong> → block Build / Pod. Contract đọc từ <code>.platform/build.yaml</code> trên repo.</p>',
  },
  {
    id: "prod",
    title: "Lên prod",
    html:
      '<p><strong>Cách A — Promote (khuyến nghị)</strong></p>' +
      "<ol class=\"deploy-help-steps\">" +
      "<li>Dev deploy <strong>success</strong> (multi, cùng tag api+web).</li>" +
      "<li>Tab <strong>Promote Prod</strong> → checklist xanh (workflow, env, domain…).</li>" +
      "<li>Promote — cùng image tag, không build lại.</li>" +
      "</ol>" +
      '<p><strong>Cách B — Deploy thẳng prod</strong></p>' +
      "<p class=\"muted\">Deploy env = <strong>prod</strong> → sync → mỗi push deploy production. Cẩn thận.</p>",
  },
  {
    id: "rules",
    title: "Quy tắc",
    html:
      "<ul class=\"deploy-help-list\">" +
      "<li><strong>Deploy lại</strong> — đổi tag, <em>cùng</em> kiểu chạy (single hoặc multi).</li>" +
      "<li><strong>Đổi kiểu chạy…</strong> — đổi single ↔ multi; deploy bản mới. <em>Không</em> dùng rollback.</li>" +
      "<li>Sau đổi kiểu → <strong>bắt buộc</strong> 「Lưu &amp; đồng bộ GitHub」 trước khi push.</li>" +
      "<li>Promote multi = <strong>cùng tag</strong> cho api + web.</li>" +
      "</ul>",
  },
  {
    id: "troubleshoot",
    title: "Lỗi thường gặp",
    html:
      "<table class=\"data-table deploy-help-table\"><thead><tr><th>Triệu chứng</th><th>Xử lý</th></tr></thead><tbody>" +
      "<tr><td>503, không có pod</td><td>Layout lệch — 「Đổi kiểu chạy」+ sync + deploy mới</td></tr>" +
      "<tr><td>Badge <strong>Cần đồng bộ</strong></td><td>Bấm 「Lưu &amp; đồng bộ GitHub」</td></tr>" +
      "<tr><td>Build multi, Console single</td><td>Chưa sync sau đổi layout</td></tr>" +
      "<tr><td>Frontend gọi API sai</td><td><code>VITE_API_BASE=/api</code> ở Cấu hình app</td></tr>" +
      "<tr><td>Project cũ sau cập nhật platform</td><td>Sync workflow một lần (không cần đổi config)</td></tr>" +
      "</tbody></table>",
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
    '" title="Hướng dẫn deploy (micro thường)" aria-label="Hướng dẫn deploy">' +
    "<span aria-hidden=\"true\">?</span></button>"
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
      '<p class="muted deploy-help-sub">Micro thường · api + web · Phase 1</p>' +
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
