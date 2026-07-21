/* Platform admin pages. */

async function pagePlatformBackups(main) {
  main.innerHTML = '<p class="loading">Đang tải backup config…</p>';
  try {
    const res = await api("/api/v1/admin/backups/targets");
    const history = await api("/api/v1/admin/backups/runs");
    const items = res.items || [];
    const target = items[0] || {};
    const runs = history.items || [];
    const tested = target.last_tested_at && !target.last_test_error;
    const configured = !!target.id;
    const targetState = !configured ? "Chưa cấu hình storage" : target.enabled && tested ? "Đang sẵn sàng" : target.enabled ? "Cần test lại target" : "Target đang tắt";
    const targetTone = !configured || !target.enabled ? "warn" : tested ? "ok" : "danger";
    const cronParts = String(target.schedule_cron || "0 3 * * *").trim().split(/\s+/);
    const cronMinute = /^(0|15|30|45)$/.test(cronParts[0]) ? cronParts[0] : "0";
    const cronUTCHour = /^(?:[01]?\d|2[0-3])$/.test(cronParts[1]) ? parseInt(cronParts[1], 10) : 20;
    const cronHour = String((cronUTCHour + 7) % 24);
    const dailyCron = cronParts.length === 5 && cronParts[2] === "*" && cronParts[3] === "*" && cronParts[4] === "*";
    const hourOptions = Array.from({ length: 24 }, function (_, hour) {
      const value = String(hour);
      return '<option value="' + value + '"' + (value === cronHour ? " selected" : "") + ">" + value.padStart(2, "0") + ":00</option>";
    }).join("");
    const minuteOptions = ["0", "15", "30", "45"].map(function (minute) {
      return '<option value="' + minute + '"' + (minute === cronMinute ? " selected" : "") + ">" + minute.padStart(2, "0") + "</option>";
    }).join("");
    main.innerHTML =
      '<div class="page-header"><h2 class="page-title">Backup Platform</h2>' +
      '<p class="page-subtitle">Quản lý backup toàn Platform: PostgreSQL, etcd/config và MinIO của các project.</p></div>' +
      '<div class="backup-status-grid">' +
      '<div class="card backup-status-card"><span class="muted">Backup local</span><strong>Đã sẵn sàng</strong><p class="muted">Snapshot etcd và config chạy trên control-plane.</p></div>' +
      '<div class="card backup-status-card"><span class="muted">Storage offsite</span><strong class="backup-state-' + esc(targetTone) + '">' + esc(targetState) + '</strong><p class="muted">' +
      (configured ? "Target: " + esc(target.name) + " · " + esc(target.bucket) : "Chưa phát sinh chi phí. Có thể gắn B2, R2 hoặc MinIO VPS sau.") +
      "</p></div>" +
      '<div class="card backup-status-card"><span class="muted">Khôi phục an toàn</span><strong>Restore cô lập</strong><p class="muted">Luôn restore sang vùng tạm trước, không ghi đè production.</p></div></div>' +
      '<div class="banner ' + (configured ? "warn" : "info") + '">' +
      (configured
        ? "Backup offsite chỉ chạy khi target Test thành công và được bật. Scheduler kiểm tra lịch mỗi phút; worker xử lý tuần tự."
        : "Bạn chưa cần mua storage ngay. Khi có VPS Storage/B2/R2: tạo bucket + Secret credential, cấu hình target tại đây, Test rồi bật lịch backup.") +
      "</div>" +
      '<details class="card detail-card backup-config"' + (configured ? "" : " open") + '><summary><span><strong>1. Cấu hình storage offsite</strong><span class="muted"> · S3-compatible: B2, R2, AWS S3, Wasabi hoặc MinIO VPS</span></span><span class="deploy-collapsible-chev" aria-hidden="true"></span></summary>' +
      '<div class="backup-config-body"><p class="muted">Credential chỉ nằm trong Kubernetes Secret; Console không lưu hoặc hiển thị access key.</p>' +
      '<form id="backup-target-form" class="login-form">' +
      '<div class="form-row"><label>Tên target<input name="name" required value="' + esc(target.name || "primary-offsite") + '" /></label>' +
      '<label>Provider<select name="provider" disabled><option>S3-compatible</option></select></label></div>' +
      '<label>Endpoint<input name="endpoint" required placeholder="https://s3.us-west-000.backblazeb2.com" value="' + esc(target.endpoint || "") + '" /></label>' +
      '<div class="form-row"><label>Region<input name="region" value="' + esc(target.region || "us-east-1") + '" /></label>' +
      '<label>Bucket<input name="bucket" required placeholder="platform-backups" value="' + esc(target.bucket || "") + '" /></label></div>' +
      '<div class="form-row"><label>Prefix<input name="prefix" value="' + esc(target.prefix || "platform-backups") + '" /></label>' +
      '<label>Credential Secret<input name="credentials_secret" required placeholder="backup-s3-credentials" value="' + esc(target.credentials_secret || "") + '" /></label></div>' +
      '<p class="muted">Secret trong namespace <code>' + esc(res.secret_namespace || "platform") + '</code>; bắt buộc keys: <code>access_key_id</code>, <code>secret_access_key</code>. Secret không được lưu hoặc hiển thị trên Console.</p>' +
      '<div class="form-row"><label>Lịch backup<select name="schedule_preset"><option value="daily"' + (dailyCron ? " selected" : "") + '>Hằng ngày</option><option value="weekly"' + (target.schedule_cron === "0 20 * * 0" ? " selected" : "") + '>Hằng tuần · Chủ nhật</option><option value="custom"' + (!dailyCron && target.schedule_cron !== "0 20 * * 0" ? " selected" : "") + '>Cron nâng cao</option></select></label>' +
      '<label>Giữ số bản thành công<input name="retention_count" type="number" min="1" max="1000" value="' + esc(String(target.retention_count || 3)) + '" /></label></div>' +
      '<div class="form-row backup-time-picker"><label>Giờ chạy (UTC+7)<select name="schedule_hour">' + hourOptions + '</select></label><label>Phút<select name="schedule_minute">' + minuteOptions + '</select></label></div>' +
      '<label class="backup-cron-custom">Cron nâng cao<input name="schedule_cron" value="' + esc(target.schedule_cron || "0 3 * * *") + '" /><span class="muted">Chỉ dùng khi chọn Cron nâng cao. Lịch thường được lưu theo UTC nhưng form hiển thị UTC+7.</span></label>' +
      '<label class="auto-deploy-toggle"><input name="enabled" type="checkbox"' + (target.enabled ? " checked" : "") + ' /> Bật backup theo lịch sau khi Test thành công</label>' +
      '<div class="form-actions"><button type="submit" class="btn-primary">Lưu target</button>' +
      (target.id ? '<button type="button" class="btn-ghost" id="backup-target-test">Test kết nối</button>' : "") +
      (target.id && tested && target.enabled ? '<button type="button" class="btn-ghost" id="backup-run-now">Backup ngay</button>' : "") +
      "</div></form>" +
      '<p class="muted" id="backup-target-status">' +
      (tested ? "✓ Test gần nhất: " + esc(fmtTime(target.last_tested_at)) : target.last_test_error ? "Lỗi test: " + esc(target.last_test_error) : "Chưa test target.") +
      "</p></div></details>" +
      '<div class="card detail-card backup-next-steps"><h3>2. Khi có Storage VPS hoặc cloud storage</h3>' +
      '<ol><li>Tạo bucket private và access key chỉ giới hạn bucket backup.</li><li>Tạo Secret trong namespace <code>' + esc(res.secret_namespace || "platform") + '</code> với <code>access_key_id</code> và <code>secret_access_key</code>.</li><li>Lưu target, Test kết nối, bật lịch và chạy một backup thủ công.</li><li>Thực hiện restore drill vào vùng cô lập trước khi coi backup là sẵn sàng.</li></ol>' +
      '<p class="muted">Hiện backup theo platform run; dữ liệu MinIO được tách theo namespace project/env trong từng run.</p></div>' +
      '<div class="card detail-card"><h3>Lịch sử backup</h3>' +
      '<table class="data-table"><thead><tr><th>Thời gian</th><th>Target</th><th>Loại</th><th>Trạng thái</th><th>Artifact</th><th>Lỗi</th></tr></thead><tbody>' +
      (runs.length ? runs.map(function (r) {
        return "<tr><td>" + esc(fmtTime(r.created_at)) + "</td><td>" + esc(r.target_name || "—") + "</td><td>" + esc(r.run_kind) + "</td><td>" + badgeStatus(r.status) + "</td><td>" + esc(String(r.artifact_count || 0)) + " · " + esc(formatBytesShort(r.total_bytes || 0)) + "</td><td>" + esc(r.error_message || "—") + "</td></tr>";
      }).join("") : '<tr><td colspan="6" class="muted">Chưa có backup run.</td></tr>') +
      "</tbody></table></div>";
    const form = main.querySelector("#backup-target-form");
    const presetSelect = form.querySelector('[name="schedule_preset"]');
    const timePicker = form.querySelector(".backup-time-picker");
    const cronCustom = form.querySelector(".backup-cron-custom");
    function updateScheduleFields() {
      const custom = presetSelect.value === "custom";
      timePicker.hidden = custom;
      cronCustom.hidden = !custom;
    }
    presetSelect.onchange = updateScheduleFields;
    updateScheduleFields();
    form.onsubmit = async function (e) {
      e.preventDefault();
      const fd = new FormData(form);
      try {
        const preset = fd.get("schedule_preset");
        const localHour = parseInt(fd.get("schedule_hour"), 10);
        const utcHour = (localHour + 17) % 24;
        const scheduledCron = preset === "custom"
          ? fd.get("schedule_cron")
          : String(fd.get("schedule_minute")) + " " + utcHour + " * * " + (preset === "weekly" ? "0" : "*");
        await api("/api/v1/admin/backups/targets", { method: "POST", body: {
          name: fd.get("name"), endpoint: fd.get("endpoint"), region: fd.get("region"), bucket: fd.get("bucket"),
          prefix: fd.get("prefix"), credentials_secret: fd.get("credentials_secret"), schedule_cron: scheduledCron,
          retention_days: 3650, retention_count: parseInt(fd.get("retention_count"), 10), encryption_enabled: true, enabled: fd.get("enabled") === "on"
        }});
        toastSuccess("Đã lưu backup target"); pagePlatformBackups(main);
      } catch (err) { toastError(err.message || "Không lưu được target"); }
    };
    const testBtn = main.querySelector("#backup-target-test");
    if (testBtn) testBtn.onclick = async function () {
      try { await api("/api/v1/admin/backups/targets/" + target.id + "/test", { method: "POST" }); toastSuccess("S3 bucket sẵn sàng"); pagePlatformBackups(main); }
      catch (err) { toastError(err.message || "Test target thất bại"); }
    };
    const runBtn = main.querySelector("#backup-run-now");
    if (runBtn) runBtn.onclick = async function () {
      runBtn.disabled = true;
      try {
        await api("/api/v1/admin/backups/runs", { method: "POST", body: { target_id: target.id } });
        toastSuccess("Backup đã được xếp hàng");
        pagePlatformBackups(main);
      } catch (err) {
        runBtn.disabled = false;
        toastError(err.message || "Không tạo được backup run");
      }
    };
  } catch (err) {
    main.innerHTML = '<p class="error-text">' + esc(err.message || "Không tải được Backup") + "</p>";
  }
}

function openCreateProjectDialog(providers, defaultProvider, userItems) {
  return new Promise(function (resolve) {
    const overlay = document.createElement("div");
    overlay.className = "ui-overlay";
    const memberOptions = userItems
      .map(function (u) {
        return '<label class="member-pick"><input type="checkbox" name="member" value="' + u.id + '" /> ' + esc(u.email) + "</label>";
      })
      .join("");
    overlay.innerHTML =
      '<div class="ui-dialog ui-dialog-wide" role="dialog" aria-modal="true">' +
      '<div class="ui-dialog-glow"></div>' +
      '<h3 class="ui-dialog-title">Tạo project mới</h3>' +
      '<form id="create-project-dialog-form" class="login-form dialog-form">' +
      '<div class="form-row"><label>Tên hiển thị<input name="name" required placeholder="Acme API" /></label>' +
      '<label>Slug<input name="slug" placeholder="acme — tự sinh nếu trống" pattern="[a-z0-9-]*" /></label></div>' +
      '<label>Mô tả<textarea name="description" rows="2" placeholder="Mô tả ngắn…"></textarea></label>' +
      '<div class="form-row">' + registrySelectHtml(providers, defaultProvider, defaultProvider) + "</div>" +
      '<div class="form-row"><label>Namespace dev<input name="namespace_dev" placeholder="acme-dev" /></label>' +
      '<label>Namespace prod<input name="namespace_prod" placeholder="acme-prod" /></label></div>' +
      '<p class="muted create-project-hint">Kiểu chạy (Một website / Web + API) chọn ở bước <strong>Deploy / Git</strong> sau khi gắn GitHub — Console tự gợi ý từ repo.</p>' +
      (memberOptions ? '<div class="member-picks compact"><span class="field-label">Thêm thành viên</span>' + memberOptions + "</div>" : "") +
      '<div class="ui-dialog-actions" style="margin-top:16px;padding-top:0;border:0">' +
      '<button type="button" class="btn-ghost ui-dialog-cancel">Huỷ</button>' +
      '<button type="submit" class="btn-primary">Tạo project</button></div></form></div>';

    function close(result) {
      overlay.classList.remove("show");
      setTimeout(function () {
        overlay.remove();
        resolve(result);
      }, 200);
    }

    overlay.querySelector(".ui-dialog-cancel").onclick = function () { close(null); };
    overlay.onclick = function (e) {
      if (e.target === overlay) close(null);
    };
    document.body.appendChild(overlay);
    requestAnimationFrame(function () { overlay.classList.add("show"); });

    const form = overlay.querySelector("#create-project-dialog-form");
    const regSel = overlay.querySelector("#registry-provider-select");
    const regHint = overlay.querySelector("#registry-picker-hint");
    function updateRegHint() {
      if (!regSel || !regHint) return;
      const pr = providers.find(function (x) { return x.name === regSel.value; });
      regHint.textContent = pr ? (pr.ready_hint || pr.description || "") : "";
    }
    if (regSel) {
      regSel.onchange = updateRegHint;
      updateRegHint();
    }
    form.onsubmit = async function (e) {
      e.preventDefault();
      const fd = new FormData(form);
      const memberIds = [];
      form.querySelectorAll('input[name="member"]:checked').forEach(function (cb) {
        memberIds.push(parseInt(cb.value, 10));
      });
      const submitBtn = form.querySelector('button[type="submit"]');
      submitBtn.disabled = true;
      try {
        const res = await withAppLoading(function () {
          return api("/api/v1/projects", {
            method: "POST",
            body: {
              name: fd.get("name"),
              slug: fd.get("slug"),
              description: fd.get("description"),
              namespace_dev: fd.get("namespace_dev"),
              namespace_prod: fd.get("namespace_prod"),
              registry_provider: fd.get("registry_provider") || "ghcr",
              member_ids: memberIds,
            },
          });
        }, { title: "Đang tạo project…", detail: "Khởi tạo namespace và registry" });
        close(res);
      } catch (err) {
        toastError(err.message);
        submitBtn.disabled = false;
      }
    };
    const firstInput = form.querySelector('input[name="name"]');
    if (firstInput) firstInput.focus();
  });
}

function canViewAddons() {
  const r = state.user && state.user.role;
  return r === "admin" || r === "tech_lead";
}

function canPatchAddons() {
  return state.user && state.user.role === "admin";
}

/** Route phụ thuộc addon Rancher — ẩn khi tắt plugin. */
const RANCHER_ROUTE_KEYS = new Set([
  "clusters", "projects", "add-worker", "namespaces", "nodes", "events",
  "deployments", "statefulsets", "daemonsets", "jobs", "cronjobs", "pods",
  "services", "ingresses", "horizontalpodautoscalers", "persistentvolumeclaims",
  "persistentvolumes", "storageclasses", "configmaps", "secrets",
]);

function isRancherDependentRoute(parsed) {
  if (!parsed) return false;
  if (parsed.type === "view") return true;
  if (parsed.type === "project") {
    return ["pods", "deployments", "services", "ingresses", "logs"].indexOf(parsed.tab) >= 0;
  }
  return RANCHER_ROUTE_KEYS.has(parsed.key);
}

async function syncNavAfterPluginChange(pluginName, enabled) {
  await buildSidebar();
  if (pluginName === "rancher" && !enabled && isRancherDependentRoute(parseRoute())) {
    toastInfo("Đã ẩn menu K8s — chuyển về Addons");
    location.hash = "#/addons";
    return;
  }
  markActiveNav(parseRoute());
}

async function pageAddons(main) {
  main.innerHTML = '<p class="loading">Đang tải…</p>';
  const data = await api("/api/v1/admin/plugins");
  const items = data.items || [];
  const canPatch = canPatchAddons();

  const categoryLabel = { core: "Core", registry: "Registry", cluster: "Cluster", addon: "Addon" };

  let cards = "";
  items.forEach(function (p) {
    const readyBadge = p.ready
      ? '<span class="badge ok">Sẵn sàng</span>'
      : p.enabled
        ? '<span class="badge warn">Chưa sẵn sàng</span>'
        : '<span class="badge muted">Tắt</span>';
    const coreTag = p.core ? '<span class="plugin-tag core">Core</span>' : "";
    const cat = categoryLabel[p.category] || p.category;
    const toggle =
      p.core
        ? '<span class="muted">Luôn bật</span>'
        : canPatch
          ? '<label class="plugin-toggle"><input type="checkbox" class="plugin-enable" data-name="' + esc(p.name) + '"' +
            (p.enabled ? " checked" : "") + " /><span>" + (p.enabled ? "Đang bật" : "Đang tắt") + "</span></label>"
          : '<span class="muted">' + (p.enabled ? "Đang bật" : "Đang tắt") + " (chỉ admin đổi)</span>";
    const showInstallGuide = !p.core && (p.install_command || p.bootstrap);
    let bootstrap = "";
    if (showInstallGuide) {
      const title = p.needs_bootstrap
        ? "Cài lần đầu trên VPS"
        : p.ready
          ? "Đã cài — hướng dẫn / cài lại"
          : "Cài trên VPS";
      const boxClass =
        "plugin-install-box" + (p.needs_bootstrap ? "" : p.ready ? " plugin-install-box--done" : "");
      bootstrap =
        '<div class="' + boxClass + '">' +
        '<p class="plugin-install-title">' + esc(title) + "</p>" +
        (p.prereq_note ? '<p class="muted plugin-install-prereq">' + esc(p.prereq_note) + "</p>" : "") +
        (p.chart_version ? '<p class="muted">Chart pin: <code>' + esc(p.chart_version) + "</code></p>" : "") +
        (p.check_command
          ? '<p class="muted plugin-install-step"><strong>1. Kiểm tra tài nguyên</strong></p>' +
            '<pre class="plugin-install-cmd plugin-install-cmd-sm"><code id="check-cmd-' + esc(p.name) + '">' +
            esc(p.check_command) + "</code></pre>" +
            '<button type="button" class="btn-sm btn-copy-install" data-cmd-id="check-cmd-' + esc(p.name) +
            '">Copy</button>'
          : "") +
        '<p class="muted plugin-install-step"><strong>2. Cài addon (tmux trên VPS)</strong></p>' +
        '<pre class="plugin-install-cmd"><code id="install-cmd-' + esc(p.name) + '">' +
        esc(p.install_command || ("./bootstrap/addons/run.sh " + p.name)) + "</code></pre>" +
        '<button type="button" class="btn-sm btn-copy-install" data-cmd-id="install-cmd-' + esc(p.name) +
        '">Copy lệnh cài</button>' +
        (p.ready
          ? '<p class="muted plugin-install-hint">Addon đang chạy — chỉ cần khi cài VPS mới hoặc cài lại (--force).</p>'
          : '<p class="muted plugin-install-hint">Dùng tmux — SSH rớt không mất tiến trình. Xong bấm <strong>Làm mới trạng thái</strong>.</p>') +
        "</div>";
    }
    cards +=
      '<article class="plugin-card' + (p.enabled ? " enabled" : "") + (p.core ? " is-core" : "") + '">' +
      '<div class="plugin-card-head"><div><h3>' + esc(p.label) + coreTag + "</h3>" +
      '<span class="plugin-cat">' + esc(cat) + "</span></div>" + readyBadge + "</div>" +
      "<p>" + esc(p.description) + "</p>" +
      (p.ready_hint ? '<p class="plugin-hint muted">' + esc(p.ready_hint) + "</p>" : "") +
      bootstrap +
      '<div class="plugin-card-foot">' + toggle + "</div></article>";
  });

  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Addons</h2>' +
    '<p class="page-subtitle">Bật/tắt module — menu cập nhật ngay. Cài engine lần đầu: SSH VPS + script pin version.</p>' +
    '<button type="button" class="btn-sm" id="addons-refresh">Làm mới trạng thái</button></div>' +
    '<div class="plugin-grid">' + (cards || '<p class="muted">Chưa có plugin</p>') + "</div>";

  const refreshBtn = document.getElementById("addons-refresh");
  if (refreshBtn) {
    refreshBtn.onclick = async function () {
      await pageAddons(main);
      await buildSidebar();
      toastInfo("Đã làm mới — kiểm tra badge Sẵn sàng");
    };
  }

  main.querySelectorAll(".btn-copy-install").forEach(function (btn) {
    btn.onclick = function () {
      const el = document.getElementById(btn.dataset.cmdId);
      if (!el) return;
      navigator.clipboard.writeText(el.textContent || "").then(
        function () { toastSuccess("Đã copy lệnh cài"); },
        function () { toastError("Không copy được — chọn thủ công"); }
      );
    };
  });

  main.querySelectorAll(".plugin-enable").forEach(function (cb) {
    cb.onchange = async function () {
      const name = cb.dataset.name;
      const enabled = cb.checked;
      const label = cb.parentElement.querySelector("span");
      try {
        await api("/api/v1/admin/plugins/" + encodeURIComponent(name), {
          method: "PATCH",
          body: { enabled: enabled },
        });
        if (label) label.textContent = enabled ? "Đang bật" : "Đang tắt";
        toastSuccess(enabled ? "Đã bật " + name + " — menu đã cập nhật" : "Đã tắt " + name);
        await syncNavAfterPluginChange(name, enabled);
        await pageAddons(main);
      } catch (err) {
        cb.checked = !enabled;
        if (label) label.textContent = !enabled ? "Đang bật" : "Đang tắt";
        toastError(err.message);
      }
    };
  });
}

function canManageGitOps() {
  return state.user && state.user.role === "admin";
}

function renderGitOpsProjectCard(slug, pub, status, canScaffold) {
  pub = pub || {};
  status = status || {};
  if (!pub.enabled && !pub.configured) {
    return renderDeployCollapsibleCard(
      slug,
      "gitops",
      '<div class="deploy-collapsible-summary-inner">' +
        '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
        '<div class="deploy-collapsible-title-row"><h3 style="margin:0">GitOps <span class="badge muted">Chưa bật</span></h3></div>' +
        "</div>",
      '<p class="muted">Admin chưa cấu hình repo GitOps — vẫn deploy bình thường qua Rancher. ' +
        (canManageGitOps() ? '<a href="#/gitops">Cấu hình GitOps →</a>' : "Hỏi admin nếu cần Argo CD.") +
        "</p>",
      false,
      { extraClass: "gitops-project-card" }
    );
  }
  const devBadge = status.dev_scaffolded
    ? '<span class="badge ok">dev ✓</span>'
    : '<span class="badge warn">dev chưa scaffold</span>';
  const prodBadge = status.prod_scaffolded
    ? '<span class="badge ok">prod ✓</span>'
    : '<span class="badge warn">prod chưa scaffold</span>';
  let argoHtml = "";
  ["dev", "prod"].forEach(function (env) {
    const st = status["argocd_" + env];
    if (!st) return;
    argoHtml +=
      '<div class="meta-chips" style="margin-top:6px">' +
      chip("Argo " + env, (st.sync || "—") + " / " + (st.health || "—")) +
      (st.url ? '<a class="chip-link" href="' + esc(st.url) + '" target="_blank" rel="noopener">Mở Argo</a>' : "") +
      "</div>";
  });
  const scaffoldBtn =
    canScaffold && pub.configured
      ? '<button type="button" class="btn-primary btn-sm" id="gitops-scaffold-btn">Tạo scaffold GitOps</button>'
      : "";
  const needsAttention = !status.dev_scaffolded || !status.prod_scaffolded;
  const summary =
    '<div class="deploy-collapsible-summary-inner">' +
    '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
    '<div class="deploy-collapsible-title-row">' +
    "<h3 style=\"margin:0\">GitOps " +
    (pub.configured ? '<span class="badge ok">Đã cấu hình</span>' : '<span class="badge warn">Thiếu PAT</span>') +
    "</h3>" +
  '<div class="meta-chips deploy-collapsible-summary-chips">' +
    devBadge +
    prodBadge +
    "</div></div>" +
    '<div class="deploy-collapsible-summary-actions">' +
    scaffoldBtn +
    "</div></div>";
  const body =
    '<p class="muted">Repo: <code>' +
    esc(pub.repo_url || status.repo_url || "—") +
    "</code> · branch <code>" +
    esc(pub.repo_branch || "main") +
    "</code></p>" +
    argoHtml +
    '<p class="muted" style="margin-top:8px;font-size:12px">Scaffold tạo <code>apps/' +
    esc(slug) +
    "/overlays/dev|prod</code> trên repo GitOps + đăng ký Argo CD (nếu bật).</p>";
  return renderDeployCollapsibleCard(
    slug,
    "gitops",
    summary,
    body,
    needsAttention,
    { extraClass: "gitops-project-card" }
  );
}

function bindGitOpsProjectCard(main, slug) {
  const btn = main.querySelector("#gitops-scaffold-btn");
  if (!btn) return;
  btn.onclick = async function () {
    if (!(await uiConfirm("Push manifest GitOps cho project " + slug + "?", { title: "Scaffold GitOps" }))) return;
    btn.disabled = true;
    btn.textContent = "Đang push…";
    try {
      const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/gitops/scaffold", { method: "POST", body: {} });
      toastSuccess("Đã scaffold " + (res.files || []).length + " file");
      const mainEl = $("#main");
      if (mainEl) pageProjectHub(mainEl, slug, "deploy");
    } catch (err) {
      toastError(errorMessage(err, "Scaffold thất bại"));
      btn.disabled = false;
      btn.textContent = "Tạo scaffold GitOps";
    }
  };
}

async function pageGitOps(main) {
  if (!canManageGitOps()) {
    main.innerHTML =
      '<div class="page-header"><h2 class="page-title">GitOps</h2></div>' +
      '<div class="card"><p class="error-text">Chỉ admin được cấu hình GitOps platform.</p></div>';
    return;
  }
  main.innerHTML = '<p class="loading">Đang tải…</p>';
  let cfg;
  let pub;
  try {
    [cfg, pub] = await Promise.all([
      api("/api/v1/admin/gitops"),
      api("/api/v1/gitops/public"),
    ]);
  } catch (err) {
    main.innerHTML = '<p class="error">' + esc(errorMessage(err)) + "</p>";
    return;
  }
  const tokenHint = cfg.token_configured
    ? '<span class="badge ok">PAT đã lưu</span>'
    : '<span class="badge warn">Chưa có PAT</span>';
  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">GitOps</h2>' +
    '<p class="page-subtitle">Repo manifest chung cho platform — CI ghi tag, Argo CD sync cluster. Một repo cho tất cả project.</p></div>' +
    '<div class="card gitops-settings-card">' +
    "<h3>Cấu hình repo <span id=\"gitops-token-badge\">" + tokenHint + "</span></h3>" +
    '<form id="gitops-settings-form" class="login-form" style="max-width:560px">' +
    '<label>Repo URL<input name="repo_url" type="url" required placeholder="https://github.com/org/gitopt" value="' + esc(cfg.repo_url || "") + '" /></label>' +
    '<div class="form-row">' +
    '<label>Branch<input name="repo_branch" value="' + esc(cfg.repo_branch || "main") + '" /></label>' +
    '<label>Base path<input name="base_path" value="' + esc(cfg.base_path || "apps") + '" placeholder="apps" /></label>' +
    "</div>" +
    '<label>PAT (GitHub)<input name="push_token" type="password" autocomplete="new-password" placeholder="' +
    (cfg.token_configured ? "Để trống giữ PAT hiện tại" : "ghp_… quyền repo") +
    '" /></label>' +
    '<p class="muted" style="font-size:12px">PAT cần quyền <code>contents:write</code> trên repo GitOps. Platform inject secret <code>PLATFORM_GITOPS_TOKEN</code> vào workflow project khi setup GitHub.</p>' +
  '<div class="gitops-form-actions">' +
    '<button type="submit" class="btn-primary">Lưu cấu hình</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="gitops-test-btn">Kiểm tra kết nối</button>' +
    "</div></form>" +
    (pub.argocd_enabled && pub.argocd_url
      ? '<p class="muted" style="margin-top:16px">Argo CD: <a href="' + esc(pub.argocd_url) + '" target="_blank" rel="noopener">' + esc(pub.argocd_url) + "</a></p>"
      : '<p class="muted" style="margin-top:16px">Argo CD: chưa bật — set <code>ARGOCD_NAMESPACE</code> trên VPS.</p>') +
    "</div>" +
    '<div class="card" style="margin-top:16px"><h3>Hướng dẫn nhanh</h3><ol class="deploy-steps">' +
    "<li>Tạo repo GitHub (vd. <code>gitopt</code>) — public hoặc private.</li>" +
    "<li>Điền URL + PAT ở trên → <strong>Kiểm tra kết nối</strong> → <strong>Lưu</strong>.</li>" +
    "<li>Vào từng project → tab <strong>Deploy / Git</strong> → <strong>Tạo scaffold GitOps</strong>.</li>" +
    "<li>Push code — CI sync tag vào overlay → Argo deploy (dev auto-sync).</li>" +
    "</ol></div>";

  const form = document.getElementById("gitops-settings-form");
  form.onsubmit = async function (e) {
    e.preventDefault();
    const fd = new FormData(form);
    const body = {
      repo_url: (fd.get("repo_url") || "").toString().trim(),
      repo_branch: (fd.get("repo_branch") || "main").toString().trim(),
      base_path: (fd.get("base_path") || "apps").toString().trim(),
    };
    const tok = (fd.get("push_token") || "").toString().trim();
    if (tok) body.push_token = tok;
    try {
      await api("/api/v1/admin/gitops", { method: "PATCH", body: body });
      toastSuccess("Đã lưu cấu hình GitOps");
      pageGitOps(main);
    } catch (err) {
      toastError(errorMessage(err));
    }
  };
  document.getElementById("gitops-test-btn").onclick = async function () {
    const fd = new FormData(form);
    const tok = (fd.get("push_token") || "").toString().trim();
    if (tok) {
      try {
        await api("/api/v1/admin/gitops", {
          method: "PATCH",
          body: {
            repo_url: (fd.get("repo_url") || "").toString().trim(),
            repo_branch: (fd.get("repo_branch") || "main").toString().trim(),
            base_path: (fd.get("base_path") || "apps").toString().trim(),
            push_token: tok,
          },
        });
      } catch (err) {
        toastError(errorMessage(err));
        return;
      }
    }
    try {
      const res = await api("/api/v1/admin/gitops/test", { method: "POST", body: {} });
      toastSuccess(res.message || "Kết nối OK");
    } catch (err) {
      toastError(errorMessage(err, "Kiểm tra thất bại"));
    }
  };
}

function registryChip(p) {
  const reg = p.registry || {};
  const name = reg.label || (p.registry_provider === "harbor" ? "Harbor" : "GHCR");
  const prefix = reg.image_prefix ? " · " + reg.image_prefix : "";
  return chip(name, (reg.image_prefix || p.slug || "") + prefix);
}

function registrySelectHtml(providers, selected, defaultProvider) {
  const items = providers || [];
  const def = defaultProvider || "ghcr";
  if (!items.length) {
    return '<input type="hidden" name="registry_provider" value="ghcr" />';
  }
  let opts = "";
  items.forEach(function (pr) {
    if (!pr.available) return;
    const isSel = (selected || def) === pr.name;
    const hint = pr.ready ? "" : " — chưa sẵn sàng";
    opts +=
      '<option value="' + esc(pr.name) + '"' + (isSel ? " selected" : "") + ">" +
      esc(pr.label) + hint + "</option>";
  });
  return (
    '<label class="field-stack">Registry' +
    '<div class="select-wrap">' +
    '<select name="registry_provider" id="registry-provider-select">' +
    opts +
    '</select><span class="select-chev" aria-hidden="true">▾</span></div></label>' +
    '<p class="muted registry-picker-hint" id="registry-picker-hint"></p>'
  );
}

async function pagePlatformProjects(main) {
  showAppLoading("Đang tải danh sách project…");
  let projects;
  let providersRes;
  let users;
  try {
    projects = await api("/api/v1/projects");
    providersRes = await api("/api/v1/registry/providers").catch(function () { return { items: [], default: "ghcr" }; });
    users = canManagePlatformProjects() ? await api("/api/v1/team/users").catch(() => ({ items: [] })) : { items: [] };
  } finally {
    hideAppLoading();
  }
  const providers = providersRes.items || [];
  const defaultProvider = providersRes.default || "ghcr";
  const userItems = users.items || [];

  let page = 1;
  let limit = 10;

  function renderList() {
    const total = projects.length;
    const start = (page - 1) * limit;
    const slice = projects.slice(start, start + limit);
    let rows = "";
    slice.forEach(function (p) {
      const delBtn = canDeleteProject()
        ? '<button type="button" class="btn-ghost btn-sm btn-danger-text project-del-btn" data-slug="' + esc(p.slug) + '" data-name="' + esc(p.name) + '" data-ns-dev="' + esc(p.namespace_dev) + '" data-ns-prod="' + esc(p.namespace_prod) + '">Xóa</button>'
        : "";
      const quotaBtn =
        '<button type="button" class="btn-ghost btn-sm project-quota-btn" data-slug="' + esc(p.slug) + '" data-name="' + esc(p.name) + '">Quota</button>';
      rows +=
        "<tr><td><a class=\"res-link\" href=\"#/project/" + esc(p.slug) + '">' + esc(p.name) + "</a></td>" +
        "<td><code>" + esc(p.slug) + "</code></td>" +
        "<td>" + esc(p.namespace_dev) + "</td><td>" + esc(p.namespace_prod) + "</td>" +
        "<td>" + esc((p.registry && p.registry.label) || p.registry_provider || "ghcr") + "</td>" +
        '<td class="table-actions">' + quotaBtn + delBtn + "</td></tr>";
    });

    main.innerHTML =
      '<div class="page-header page-header-row">' +
      '<div><h2 class="page-title">Quản lý Projects</h2>' +
      '<p class="page-subtitle">' + total + " project · registry GHCR/Harbor</p></div>" +
      '<button type="button" class="btn-primary" id="open-create-project">+ Tạo project</button></div>' +
      '<div class="card"><div class="table-wrap"><table><thead><tr><th>Tên</th><th>Slug</th><th>Dev NS</th><th>Prod NS</th><th>Registry</th><th></th></tr></thead><tbody>' +
      (rows || '<tr><td colspan="6" class="muted">Chưa có project</td></tr>') +
      "</tbody></table></div>" +
      (total > 0 ? renderPagination("platform-projects", total, page, limit, function (p, l) {
        page = p;
        limit = l;
        renderList();
      }) : "") +
      "</div>";

    const openBtn = document.getElementById("open-create-project");
    if (openBtn) {
      openBtn.onclick = async function () {
        const res = await openCreateProjectDialog(providers, defaultProvider, userItems);
        if (!res) return;
        if (res.warnings && res.warnings.length) {
          await uiAlert({
            title: "Tạo project thành công",
            message: "Đã tạo project " + res.slug,
            details: res.warnings,
            variant: "success",
          });
        } else {
          toastSuccess("Đã tạo project " + res.slug);
        }
        location.hash = "#/project/" + res.slug + "/deploy";
      };
    }

    main.querySelectorAll(".project-del-btn").forEach(function (btn) {
      btn.onclick = async function () {
        const slug = btn.dataset.slug;
        const name = btn.dataset.name;
        const nsDev = btn.dataset.nsDev;
        const nsProd = btn.dataset.nsProd;
        const ok = await uiConfirm({
          title: "Xóa project \"" + name + "\"?",
          message: "Hành động không hoàn tác. Metadata project sẽ bị xóa khỏi platform.",
          details: [
            "Namespace dev: " + nsDev + " — xóa pod, deployment, ingress, secret",
            "Namespace prod: " + nsProd + " — xóa toàn bộ workload",
            "Harbor project (nếu dùng Harbor) — xóa image trên VPS",
            "GitOps scaffold apps/{slug} — xóa manifest cũ (tránh tạo lại bị dính env)",
            "DB platform — lịch sử deploy, env, domains (GitHub/GHCR cloud giữ nguyên)",
          ],
          confirmText: "Xóa vĩnh viễn",
          danger: true,
        });
        if (!ok) return;
        try {
          const res = await withAppLoading(function () {
            return api("/api/v1/projects/" + encodeURIComponent(slug), {
              method: "DELETE",
              body: { purge_k8s: true },
              timeout: 300000,
            });
          }, {
            title: "Đang xóa project…",
            detail: "Dọn namespace, Harbor, GitOps — có thể mất 1–3 phút",
          });
          const idx = projects.findIndex(function (p) { return p.slug === slug; });
          if (idx >= 0) projects.splice(idx, 1);
          if (res.warnings && res.warnings.length) {
            await uiAlert({
              title: "Đã xóa project",
              message: slug,
              details: res.warnings.concat(res.note ? [res.note] : []),
              variant: "success",
            });
          } else {
            toastSuccess("Đã xóa project " + slug);
          }
          if (page > 1 && (page - 1) * limit >= projects.length) page--;
          renderList();
        } catch (err) {
          toastError(err.message);
        }
      };
    });

    main.querySelectorAll(".project-quota-btn").forEach(function (btn) {
      btn.onclick = function () {
        openProjectQuotaDialog(btn.dataset.slug, btn.dataset.name);
      };
    });
  }

  renderList();
}

/* ── Admin: xem/sửa quota tài nguyên per-project (dev + prod) ── */
async function openProjectQuotaDialog(slug, name) {
  let data;
  try {
    data = await withAppLoading(function () {
      return api("/api/v1/admin/projects/" + encodeURIComponent(slug) + "/quota");
    }, { title: "Đang tải quota…" });
  } catch (err) {
    toastError(err.message || "Không tải được quota");
    return;
  }

  function envFields(env, q) {
    q = q || {};
    const p = env + "-";
    return (
      '<div class="quota-env-block"><h4 class="quota-env-title">' +
      (env === "prod" ? "Prod" : "Dev") +
      (q.is_override ? ' <span class="quota-badge-override">override</span>' : "") +
      "</h4>" +
      '<div class="quota-grid">' +
      '<label>Storage (GB)<input type="number" min="1" max="2000" id="q-' + p + 'storage" value="' + esc(String(q.storage_gb || 0)) + '" /></label>' +
      '<label>RAM (MB)<input type="number" min="128" max="65536" id="q-' + p + 'mem" value="' + esc(String(q.memory_mb || 0)) + '" /></label>' +
      '<label>CPU (mCPU)<input type="number" min="100" max="64000" id="q-' + p + 'cpu" value="' + esc(String(q.cpu_m || 0)) + '" /></label>' +
      '<label>Max pods<input type="number" min="1" max="500" id="q-' + p + 'pods" value="' + esc(String(q.max_pods || 0)) + '" /></label>' +
      '<label>Max PVC<input type="number" min="1" max="200" id="q-' + p + 'pvc" value="' + esc(String(q.max_pvcs || 0)) + '" /></label>' +
      "</div></div>"
    );
  }

  const overlay = document.createElement("div");
  overlay.className = "ui-overlay";
  overlay.innerHTML =
    '<div class="ui-dialog ui-dialog-default quota-dialog" role="dialog" aria-modal="true">' +
    '<div class="ui-dialog-glow"></div>' +
    '<h3 class="ui-dialog-title">Quota tài nguyên · ' + esc(name || slug) + "</h3>" +
    '<p class="ui-dialog-message muted">Trần tài nguyên mỗi môi trường (ResourceQuota trên namespace). Storage áp cứng qua K8s. Lưu = áp ngay.</p>' +
    '<div class="quota-unit-note">' +
    '<strong>Đơn vị:</strong> Storage = GiB (1 GiB = 1.024 MiB) · RAM = MiB · CPU = mCPU (1.000 mCPU = 1 CPU core) · ' +
    'Max pods/PVC = số lượng tối đa.' +
    "</div>" +
    envFields("dev", data.dev) +
    envFields("prod", data.prod) +
    '<div class="ui-dialog-actions">' +
    '<button type="button" class="btn-ghost quota-cancel">Đóng</button>' +
    '<button type="button" class="btn-primary quota-save">Lưu (áp ResourceQuota)</button>' +
    "</div></div>";

  function close() {
    overlay.classList.remove("show");
    setTimeout(function () { overlay.remove(); }, 200);
  }
  overlay.querySelector(".quota-cancel").onclick = close;
  overlay.onclick = function (e) { if (e.target === overlay) close(); };

  function readEnv(env) {
    const p = env + "-";
    const num = function (id) { return Number(overlay.querySelector("#q-" + p + id).value); };
    return {
      environment: env,
      storage_gb: num("storage"),
      memory_mb: num("mem"),
      cpu_m: num("cpu"),
      max_pods: num("pods"),
      max_pvcs: num("pvc"),
    };
  }

  overlay.querySelector(".quota-save").onclick = async function () {
    try {
      await withAppLoading(async function () {
        for (const env of ["dev", "prod"]) {
          await api("/api/v1/admin/projects/" + encodeURIComponent(slug) + "/quota", {
            method: "PATCH",
            body: readEnv(env),
          });
        }
      }, { title: "Đang lưu quota + áp ResourceQuota…" });
      toastSuccess("Đã cập nhật quota " + (name || slug));
      close();
    } catch (err) {
      toastError(err.message || "Lưu quota thất bại");
    }
  };

  document.body.appendChild(overlay);
  requestAnimationFrame(function () { overlay.classList.add("show"); });
}

async function pagePlatformPolicy(main) {
  if (!state.user || state.user.role !== "admin") {
    main.innerHTML =
      '<div class="page-header"><h2 class="page-title">Platform Policy</h2></div>' +
      '<div class="card"><p class="error-text">Chỉ admin được xem/sửa policy.</p></div>';
    return;
  }
  let pol;
  try {
    pol = await api("/api/v1/admin/policy");
  } catch (err) {
    main.innerHTML =
      '<div class="page-header"><h2 class="page-title">Platform Policy</h2></div>' +
      '<div class="card"><p class="error-text">' + esc(err.message) + "</p></div>";
    return;
  }

  async function stepUpThen() {
    const dlg = await uiFormDialog({
      title: "Xác thực 2 lớp",
      message: "Nhập mật khẩu đăng nhập + Policy Unlock passphrase để lưu thay đổi.",
      confirmText: "Mở khoá & tiếp tục",
      fields: [
        { id: "password", label: "Mật khẩu đăng nhập admin", type: "password", autocomplete: "current-password" },
        { id: "unlock", label: "Policy Unlock passphrase", type: "password", autocomplete: "off" },
      ],
    });
    if (!dlg.ok) return null;
    try {
      const res = await api("/api/v1/admin/policy/step-up", {
        method: "POST",
        body: {
          password: dlg.values.password,
          unlock_passphrase: dlg.values.unlock,
        },
        loadingTitle: "Đang xác thực…",
      });
      return res.step_up_token;
    } catch (err) {
      toastError(err.message);
      return null;
    }
  }

  function policySummary(title, badgeHtml) {
    return (
      '<div class="deploy-collapsible-summary-inner">' +
      '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
      '<div class="deploy-collapsible-title-row">' +
      '<h3 style="margin:0">' +
      esc(title) +
      "</h3>" +
      (badgeHtml ? '<span class="deploy-collapsible-no-toggle">' + badgeHtml + "</span>" : "") +
      "</div></div>"
    );
  }

  const unlockBadge = pol.unlock_configured
    ? '<span class="badge ok">đã cấu hình</span>'
    : '<span class="badge bad">chưa có</span>';

  const infra = pol.infra || {};
  let infraHtml =
    '<div class="policy-infra">' +
    '<p class="policy-help"><strong>Policy là gì?</strong> Trần tối đa mỗi project được xin. App của dev tự giới hạn upload; Platform canh sức chứa MinIO (PVC). Console upload chỉ là quản trị phụ.</p>';
  if (infra.disk_ready) {
    const pct = Math.round(Number(infra.disk_used_pct) || 0);
    const warnClass = pct >= 85 ? "policy-infra-warn" : pct >= 70 ? "policy-infra-caution" : "";
    infraHtml +=
      '<div class="policy-infra-grid ' +
      warnClass +
      '">' +
      '<div><span class="muted">Disk trống</span><strong>' +
      esc(String(infra.disk_free_gib)) +
      " / " +
      esc(String(infra.disk_total_gib)) +
      " GiB</strong></div>" +
      '<div><span class="muted">Disk đã dùng</span><strong>' +
      esc(String(pct)) +
      "%</strong></div>" +
      '<div><span class="muted">Đã cấp MinIO addon</span><strong>' +
      esc(String(infra.minio_allocated_gb || 0)) +
      " GiB</strong></div>" +
      '<div><span class="muted">RAM / CPU trống (ước lượng)</span><strong>' +
      esc(String(infra.memory_free_gib)) +
      " GiB · " +
      esc(String(infra.cpu_free_cores)) +
      " core</strong></div>" +
      "</div>";
  }
  infraHtml +=
    '<p class="muted policy-infra-hint">' +
    esc(infra.hint || "Đang ước lượng hạ tầng…") +
    "</p></div>";

  const overviewBody =
    infraHtml +
    "<p>Unlock: " +
    unlockBadge +
    (pol.updated_at ? ' · Cập nhật: <code>' + esc(pol.updated_at) + "</code>" : "") +
    "</p>" +
    '<p class="muted">Sửa policy / đổi Unlock cần mật khẩu login + Policy Unlock passphrase.</p>';

  const redisBody =
    '<p class="policy-help">Trần <strong>mỗi</strong> Redis addon (dev/prod). Project không xin được cao hơn. Nên ≤ RAM node trống.</p>' +
    '<div class="form-row">' +
    '<label>Max RAM (MB)<input form="policy-form" name="redis_max_memory_mb" type="number" min="64" max="4096" value="' +
    esc(String(pol.redis_max_memory_mb)) +
    '" required /></label>' +
    '<label>Max clients<input form="policy-form" name="redis_max_clients" type="number" min="10" max="10000" value="' +
    esc(String(pol.redis_max_clients)) +
    '" required /></label></div>';

  const minioBody =
    '<p class="policy-help"><strong>Max storage</strong> = PVC + hard quota bucket mỗi MinIO project. ' +
    "<strong>Max object</strong> = trần 1 file (project Quota ≤ trần này; Console + <code>S3_MAX_OBJECT_MB</code>). " +
    "<strong>Upload Console</strong> chỉ giới hạn nút Files trên Console.</p>" +
    (infra.disk_ready
      ? '<p class="muted">Gợi ý: trần storage ≪ disk trống (~' +
        esc(String(infra.disk_free_gib)) +
        " GiB), đã cấp MinIO " +
        esc(String(infra.minio_allocated_gb || 0)) +
        " GiB.</p>"
      : "") +
    '<div class="form-row">' +
    '<label>Max storage (GB)<input form="policy-form" name="minio_max_storage_gb" type="number" min="1" max="2000" value="' +
    esc(String(pol.minio_max_storage_gb)) +
    '" required /></label>' +
    '<label>Max RAM pod (MB)<input form="policy-form" name="minio_max_memory_mb" type="number" min="128" max="8192" value="' +
    esc(String(pol.minio_max_memory_mb)) +
    '" required /></label></div>' +
    '<div class="form-row">' +
    '<label>Max object (MiB)<input form="policy-form" name="minio_max_object_mb" type="number" min="1" max="51200" value="' +
    esc(String(pol.minio_max_object_mb != null ? pol.minio_max_object_mb : 5120)) +
    '" required /></label>' +
    '<label>Upload Console (MiB)<input form="policy-form" name="minio_console_upload_mb" type="number" min="1" max="512" value="' +
    esc(String(pol.minio_console_upload_mb)) +
    '" required /></label></div>';

  const ingressBody =
    '<p class="policy-help">Giới hạn body HTTP qua Ingress hostname app (upload multipart qua web). Không áp dụng khi app gọi MinIO trong cluster.</p>' +
    '<div class="form-row">' +
    '<label>proxy-body-size<input form="policy-form" name="ingress_proxy_body_size" type="text" value="' +
    esc(pol.ingress_proxy_body_size || "32m") +
    '" required placeholder="32m" /></label></div>';

  const securityBody =
    '<p class="muted">Passphrase lớp 2 — không phải mật khẩu đăng nhập. Chỉ admin biết passphrase mới sửa được policy.</p>' +
    '<button type="button" class="btn-ghost" id="policy-rotate-unlock"' +
    (pol.unlock_configured ? "" : " disabled") +
    ">Đổi Unlock passphrase</button>";

  const applyBody =
    '<p class="muted">Lưu toàn bộ trần Redis · MinIO · Ingress (cần xác thực 2 lớp).</p>' +
    '<form id="policy-form" class="policy-form">' +
    '<button type="submit" class="btn-primary"' +
    (pol.unlock_configured ? "" : " disabled") +
    ">Lưu policy</button></form>";

  const slug = "platform-policy";
  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Platform Policy</h2>' +
    '<p class="page-subtitle">Trần cứng toàn platform — sửa cần mật khẩu login + Policy Unlock</p></div>' +
    '<div id="policy-root" class="policy-root">' +
    renderDeployCollapsibleCard(
      slug,
      "policy-overview",
      policySummary("Tổng quan", unlockBadge),
      overviewBody,
      true,
      { id: "policy-sec-overview" }
    ) +
    renderDeployCollapsibleCard(
      slug,
      "policy-redis",
      policySummary(
        "Redis",
        '<span class="badge muted">' +
          esc(String(pol.redis_max_memory_mb)) +
          " MB · " +
          esc(String(pol.redis_max_clients)) +
          " clients</span>"
      ),
      redisBody,
      true,
      { id: "policy-sec-redis" }
    ) +
    renderDeployCollapsibleCard(
      slug,
      "policy-minio",
      policySummary(
        "MinIO",
        '<span class="badge muted">' +
          esc(String(pol.minio_max_storage_gb)) +
          " Gi · obj " +
          esc(String(pol.minio_max_object_mb != null ? pol.minio_max_object_mb : 5120)) +
          " Mi · console " +
          esc(String(pol.minio_console_upload_mb)) +
          " MiB</span>"
      ),
      minioBody,
      true,
      { id: "policy-sec-minio" }
    ) +
    renderDeployCollapsibleCard(
      slug,
      "policy-ingress",
      policySummary(
        "Ingress",
        '<span class="badge muted">' + esc(pol.ingress_proxy_body_size || "32m") + "</span>"
      ),
      ingressBody,
      false,
      { id: "policy-sec-ingress" }
    ) +
    renderDeployCollapsibleCard(
      slug,
      "policy-security",
      policySummary("Bảo mật Unlock", unlockBadge),
      securityBody,
      false,
      { id: "policy-sec-security" }
    ) +
    renderDeployCollapsibleCard(
      slug,
      "policy-apply",
      policySummary("Áp dụng", '<span class="badge muted">Lưu · step-up</span>'),
      applyBody,
      true,
      { id: "policy-sec-apply" }
    ) +
    "</div>";

  bindDeployCollapsibleCards(document.getElementById("policy-root"), slug);

  document.getElementById("policy-form").onsubmit = async function (e) {
    e.preventDefault();
    const fd = new FormData(e.target);
    // Inputs dùng form="policy-form" nằm ngoài thẻ form — FormData vẫn lấy được.
    const body = {
      redis_max_memory_mb: Number(fd.get("redis_max_memory_mb")),
      redis_max_clients: Number(fd.get("redis_max_clients")),
      minio_max_storage_gb: Number(fd.get("minio_max_storage_gb")),
      minio_max_memory_mb: Number(fd.get("minio_max_memory_mb")),
      minio_max_object_mb: Number(fd.get("minio_max_object_mb")),
      minio_console_upload_mb: Number(fd.get("minio_console_upload_mb")),
      ingress_proxy_body_size: String(fd.get("ingress_proxy_body_size") || "").trim(),
    };
    const token = await stepUpThen();
    if (!token) return;
    try {
      await api("/api/v1/admin/policy", {
        method: "PATCH",
        body: body,
        headers: { "X-Platform-Step-Up": token },
        loadingTitle: "Đang lưu policy…",
      });
      toastSuccess("Đã lưu Platform Policy");
      pagePlatformPolicy(main);
    } catch (err) {
      toastError(err.message);
    }
  };

  const rotateBtn = document.getElementById("policy-rotate-unlock");
  if (rotateBtn) {
    rotateBtn.onclick = async function () {
      const token = await stepUpThen();
      if (!token) return;
      const dlg = await uiFormDialog({
        title: "Đổi Policy Unlock",
        message: "Passphrase mới (tối thiểu 10 ký tự).",
        confirmText: "Đổi passphrase",
        fields: [{ id: "next", label: "Passphrase mới", type: "password", autocomplete: "new-password" }],
      });
      if (!dlg.ok) return;
      try {
        await api("/api/v1/admin/policy/unlock-passphrase", {
          method: "POST",
          body: { new_unlock_passphrase: dlg.values.next },
          headers: { "X-Platform-Step-Up": token },
        });
        toastSuccess("Đã đổi Unlock passphrase");
        pagePlatformPolicy(main);
      } catch (err) {
        toastError(err.message);
      }
    };
  }
}

async function pageAudit(main) {
  const data = await api("/api/v1/admin/audit");
  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Audit Log</h2>' +
    '<p class="page-subtitle">100 sự kiện gần nhất — ai làm gì</p></div>' +
    renderTable(
      [
        { key: "created_at", label: "Thời gian" },
        { key: "email", label: "User" },
        { key: "action", label: "Action" },
        { key: "resource", label: "Resource" },
        { key: "ip_address", label: "IP" },
      ],
      data.items || []
    );
}

async function pageUsers(main) {
  const data = await api("/api/v1/admin/users");
  const items = data.items || [];
  const roleOptions = [
    { v: "dev", l: "Developer" },
    { v: "tech_lead", l: "Tech Lead" },
    { v: "readonly", l: "Read-only" },
    { v: "admin", l: "Admin" },
  ];
  let rows = "";
  items.forEach(function (u) {
    const isSelf = state.user && state.user.id === u.id;
    const roleSel =
      '<select class="user-role-select" data-id="' + u.id + '"' + (isSelf ? " disabled" : "") + ">" +
      roleOptions
        .map(function (o) {
          return '<option value="' + o.v + '"' + (u.role === o.v ? " selected" : "") + ">" + o.l + "</option>";
        })
        .join("") +
      "</select>";
    const status = u.active
      ? '<span class="badge ok">Active</span>'
      : '<span class="badge bad">Disabled</span>';
  const btn = u.active
      ? '<button type="button" class="btn-sm btn-danger user-disable" data-id="' + u.id + '"' + (isSelf ? " disabled" : "") + ">Vô hiệu</button>"
      : '<button type="button" class="btn-sm btn-ok user-enable" data-id="' + u.id + '">Kích hoạt</button>';
    rows +=
      "<tr><td>" + esc(u.email) + "</td><td>" + esc(u.display_name || "—") + "</td><td>" + roleSel +
      "</td><td>" + status + '</td><td class="actions-cell">' + btn + "</td></tr>";
  });
  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Quản lý user</h2>' +
    '<p class="page-subtitle">Tạo tài khoản, gán role — chỉ admin</p></div>' +
    '<div class="card" style="margin-bottom:20px"><h3>Giải thích Roles</h3>' +
    roleHelpHtml() +
    "</div>" +
    '<div class="card" style="margin-bottom:20px"><h3>Tạo user mới</h3>' +
    '<form id="create-user-form" class="login-form" style="max-width:520px">' +
    '<div class="form-row"><label>Email<input name="email" type="email" required /></label>' +
    '<label>Tên hiển thị<input name="display_name" type="text" placeholder="Nguyễn Văn A" /></label></div>' +
    '<div class="form-row"><label>Mật khẩu<input name="password" type="password" minlength="10" required placeholder="≥10 ký tự, chữ + số" /></label>' +
    '<label>Role<select name="role">' +
    roleOptions.map(function (o) { return '<option value="' + o.v + '">' + o.l + "</option>"; }).join("") +
    "</select></label></div>" +
    '<button type="submit" class="btn-primary">Tạo user</button></form></div>' +
    '<div class="card"><h3>Danh sách (' + items.length + ")</h3>" +
    '<div class="table-wrap"><table><thead><tr><th>Email</th><th>Tên</th><th>Role</th><th>Trạng thái</th><th></th></tr></thead><tbody>' +
    (rows || '<tr><td colspan="5" class="muted">Chưa có user</td></tr>') +
    "</tbody></table></div></div>";

  $("#create-user-form").onsubmit = async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      await api("/api/v1/admin/users", {
        method: "POST",
        body: {
          email: fd.get("email"),
          display_name: fd.get("display_name"),
          password: fd.get("password"),
          role: fd.get("role"),
        },
      });
      e.target.reset();
      pageUsers(main);
    } catch (err) {
      toastError(err.message);
    }
  };

  main.querySelectorAll(".user-role-select").forEach((sel) => {
    sel.onchange = async () => {
      try {
        await api("/api/v1/admin/users/" + sel.dataset.id, {
          method: "PATCH",
          body: { role: sel.value },
        });
      } catch (err) {
        toastError(err.message);
        pageUsers(main);
      }
    };
  });
  main.querySelectorAll(".user-disable").forEach((btn) => {
    btn.onclick = async () => {
      if (!(await uiConfirm("Vô hiệu user này? Họ sẽ không đăng nhập được.", { danger: true, title: "Vô hiệu user" }))) return;
      try {
        await api("/api/v1/admin/users/" + btn.dataset.id, {
          method: "PATCH",
          body: { active: false },
        });
        pageUsers(main);
      } catch (err) {
        toastError(err.message);
      }
    };
  });
  main.querySelectorAll(".user-enable").forEach((btn) => {
    btn.onclick = async () => {
      try {
        await api("/api/v1/admin/users/" + btn.dataset.id, {
          method: "PATCH",
          body: { active: true },
        });
        pageUsers(main);
      } catch (err) {
        toastError(err.message);
      }
    };
  });
}
