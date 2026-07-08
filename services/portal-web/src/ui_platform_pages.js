/* Platform admin pages. */

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
        const res = await api("/api/v1/projects", {
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
  main.innerHTML = '<p class="loading">Đang tải…</p>';
  const projects = await api("/api/v1/projects");
  const providersRes = await api("/api/v1/registry/providers").catch(function () { return { items: [], default: "ghcr" }; });
  const providers = providersRes.items || [];
  const defaultProvider = providersRes.default || "ghcr";
  const users = canManagePlatformProjects() ? await api("/api/v1/team/users").catch(() => ({ items: [] })) : { items: [] };
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
      rows +=
        "<tr><td><a class=\"res-link\" href=\"#/project/" + esc(p.slug) + '">' + esc(p.name) + "</a></td>" +
        "<td><code>" + esc(p.slug) + "</code></td>" +
        "<td>" + esc(p.namespace_dev) + "</td><td>" + esc(p.namespace_prod) + "</td>" +
        "<td>" + esc((p.registry && p.registry.label) || p.registry_provider || "ghcr") + "</td>" +
        '<td class="table-actions">' + delBtn + "</td></tr>";
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
            "DB platform — lịch sử deploy, env, domains (GitHub/GHCR cloud giữ nguyên)",
          ],
          confirmText: "Xóa vĩnh viễn",
          danger: true,
        });
        if (!ok) return;
        try {
          const res = await api("/api/v1/projects/" + encodeURIComponent(slug), {
            method: "DELETE",
            body: { purge_k8s: true },
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
  }

  renderList();
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
    '<div class="form-row"><label>Mật khẩu<input name="password" type="password" minlength="12" required placeholder="≥12 ký tự, chữ + số" /></label>' +
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
