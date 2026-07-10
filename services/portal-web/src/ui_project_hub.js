/* Project hub + my projects. */

async function pageMyProjects(main) {
  main.innerHTML = '<p class="loading">Đang tải dự án…</p>';
  const projects = await api("/api/v1/projects");
  if (!projects.length) {
    main.innerHTML =
      '<div class="page-header"><h2 class="page-title">Dự án của tôi</h2>' +
      '<p class="page-subtitle">Chưa được gán project nào — liên hệ Tech Lead hoặc Admin.</p></div>' +
      '<div class="card detail-card"><p class="muted">Admin/Tech Lead cần thêm bạn vào <code>project_members</code> cho project tương ứng.</p></div>';
    return;
  }

  const cards = projects
    .map(function (p) {
      const devNs = p.namespace_dev || "—";
      const prodNs = p.namespace_prod || "—";
      const slug = p.slug || p.name;
      return (
        '<div class="card project-card" style="margin-bottom:16px">' +
        '<h3><a href="#/project/' + esc(slug) + '" class="res-link">' + esc(p.name) + "</a></h3>" +
        (p.description ? '<p class="muted">' + esc(p.description) + "</p>" : "") +
        '<div class="meta-chips">' +
        chip("Dev", devNs) +
        chip("Prod", prodNs) +
        (p.registry && p.registry.image_prefix ? chip(p.registry.label || "Registry", p.registry.image_prefix) : "") +
        "</div>" +
        '<div class="action-bar" style="margin-top:12px">' +
        '<a href="#/project/' + esc(slug) + '" class="btn-primary">Mở dashboard</a>' +
        projectNsLink("pods", devNs, "Pods dev") +
        "</div></div>"
      );
    })
    .join("");

  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Dự án của tôi</h2>' +
    '<p class="page-subtitle">Workload trong namespace được gán — không có quyền Hạ tầng cluster.</p></div>' +
    cards;

  main.querySelectorAll(".project-ns-link").forEach(function (a) {
    a.onclick = function (e) {
      e.preventDefault();
      const ns = a.dataset.ns;
      const resource = a.dataset.resource;
      if (ns && ns !== "—") {
        state.namespace = ns;
        localStorage.setItem("filter-ns", ns);
      }
      location.hash = "#/" + resource;
    };
  });
}


async function pageProjectHub(main, slug, tab, addon) {
  tab = tab || "overview";
  addon = addon || "";
  if (tab !== "deploy") {
    stopDeployPoll();
  }
  main.innerHTML = '<p class="loading">Đang tải project…</p>';
  let data;
  try {
    data = await api("/api/v1/projects/" + encodeURIComponent(slug));
  } catch (err) {
    main.innerHTML =
      '<p class="error">Lỗi: ' +
      esc(errorMessage(err, "Không tải được project — thử đăng nhập lại")) +
      '</p><p class="muted" style="margin-top:8px"><button type="button" class="btn-ghost btn-sm" onclick="location.reload()">Tải lại</button></p>';
    return;
  }
  const p = data.project;
  if (!p) {
    main.innerHTML = '<p class="error">Lỗi: project không tồn tại hoặc API trả dữ liệu không hợp lệ.</p>';
    return;
  }
  state.projectCtx = p;
  if (
    state.namespace &&
    state.namespace !== p.namespace_dev &&
    state.namespace !== p.namespace_prod
  ) {
    state.namespace = "";
    localStorage.removeItem("filter-ns");
  }
  const canManage = canManagePlatformProjects();
  const env = state.projectEnv || "dev";
  const ns = env === "prod" ? p.namespace_prod : p.namespace_dev;

  if (tab === "addons") {
    if (addon === "redis") {
      await loadProjectAddonRedis(main, slug, p);
      return;
    }
    if (addon === "minio") {
      await loadProjectAddonMinio(main, slug, p);
      return;
    }
    await loadProjectAddonsHub(main, slug, p);
    return;
  }

  if (tab === "overview") {
    await loadProjectOverview(main, slug, p);
    return;
  }

  if (tab === "monitoring") {
    await loadProjectMonitoring(main, slug, p, env);
    return;
  }

  if (tab === "ops") {
    await loadProjectOps(main, slug, p);
    return;
  }

  if (tab === "runtime") {
    await loadProjectRuntimePage(main, slug, p);
    return;
  }

  if (await loadProjectWorkloadList(main, slug, p, tab, ns)) return;

  if (tab === "deploy-history") {
    await loadProjectDeployHistory(main, slug, p);
    return;
  }

  if (tab === "promote") {
    await loadProjectPromote(main, slug, p);
    return;
  }

  if (tab === "deploy") {
    await loadProjectDeploy(main, slug, p, data, env, ns);
    return;
  }

  if (tab === "env") {
    await loadProjectEnv(main, slug, p, env);
    return;
  }

  if (tab === "domains") {
    await loadProjectDomains(main, slug, p, data, canManage);
    return;
  }

  if (tab === "settings") {
    await loadProjectSettings(main, slug, p, data, canManage);
    return;
  }
}
