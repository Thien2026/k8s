/* Project overview cards. */

function projectDeployBadgeFromSummary(dep, loading) {
  if (loading) return '<span class="badge muted project-ov-badge-skel">…</span>';
  dep = dep || {};
  if (!dep.status || dep.status === "none") return '<span class="badge muted">Chưa deploy</span>';
  const build = String(dep.build_status || "").toLowerCase();
  const deploySt = String(dep.deploy_status || "").toLowerCase();
  if (build === "running" || build === "pending" || deploySt === "running" || deploySt === "pending") {
    return '<span class="badge warn"><span class="live-dot"></span> Pipeline</span>';
  }
  const rt = String(dep.runtime_status || "").toLowerCase();
  const st = String(dep.status || "").toLowerCase();
  if (st === "failed" || deploySt === "failed" || build === "failed") {
    return '<span class="badge bad">Lỗi</span>';
  }
  if (st === "success" || rt === "running") {
    const tag = String(dep.image_tag || "").trim();
    const short = tag.length > 12 ? tag.slice(0, 12) + "…" : tag;
    return '<span class="badge ok">OK' + (short ? " · " + esc(short) : "") + "</span>";
  }
  return '<span class="badge muted">' + esc(dep.status) + "</span>";
}

function renderProjectEnvOverviewCard(slug, envKey, label, ns, counts, deploy, loading) {
  counts = counts || {};
  const pods = counts.pods != null ? counts.pods : null;
  const deps = counts.deployments != null ? counts.deployments : null;
  const podText = pods != null ? pods + " pods" : "… pods";
  const depText = deps != null ? deps + " deployments" : "… deployments";
  const skel = loading ? " project-ov-loading" : "";
  return (
    '<div class="card detail-card project-ov-env project-ov-env-' +
    esc(envKey) +
    skel +
    '">' +
    '<div class="project-ov-env-accent"></div>' +
    '<div class="project-ov-env-body">' +
    '<div class="project-ov-env-head"><div><h3>' +
    esc(label) +
    '</h3><p class="project-ov-ns">' +
    esc(ns || "—") +
    "</p></div>" +
    projectDeployBadgeFromSummary(deploy, loading) +
    "</div>" +
    '<div class="project-ov-metric-row">' +
    '<span class="project-ov-metric"><span class="project-ov-metric-icon" aria-hidden="true">▣</span>' +
    esc(String(podText)) +
    "</span>" +
    '<span class="project-ov-metric"><span class="project-ov-metric-icon" aria-hidden="true">⬡</span>' +
    esc(String(depText)) +
    "</span></div>" +
    '<div class="project-ov-env-links">' +
    '<a class="project-ov-link" href="#/project/' +
    esc(slug) +
    "/runtime?env=" +
    esc(envKey) +
    '">Runtime</a>' +
    '<a class="project-ov-link" href="#/project/' +
    esc(slug) +
    "/monitoring?env=" +
    esc(envKey) +
    '">Biểu đồ</a>' +
    '<a class="project-ov-link project-ov-link-primary" href="#/project/' +
    esc(slug) +
    "/deploy?env=" +
    esc(envKey) +
    '">Deploy</a>' +
    "</div></div></div>"
  );
}

function renderProjectOverviewHtml(p, slug, ov, loading) {
  ov = ov || {};
  const dev = ov.dev || {};
  const prod = ov.prod || {};
  return (
    projectHeader(p, "Trang chủ project") +
    '<p class="project-ov-lead muted">So sánh <strong>Dev</strong> và <strong>Prod</strong> — metric chi tiết ở tab Monitoring.</p>' +
    '<div class="project-ov-env-grid">' +
    renderProjectEnvOverviewCard(slug, "dev", "Dev", p.namespace_dev, dev, dev.deploy, loading) +
    renderProjectEnvOverviewCard(slug, "prod", "Prod", p.namespace_prod, prod, prod.deploy, loading) +
    "</div>" +
    (p.registry && p.registry.image_prefix
      ? '<div class="card detail-card project-ov-registry"><span class="muted">Registry</span> <code>' +
        esc(p.registry.image_prefix) +
        "</code></div>"
      : "") +
    '<div class="card detail-card"><h3>Đi nhanh</h3>' +
    renderProjectQuickNav(slug) +
    "</div>"
  );
}

async function loadProjectOverview(main, slug, p) {
  main.innerHTML = renderProjectOverviewHtml(p, slug, null, true);
  try {
    const ov = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/overview" + projectQs());
    main.innerHTML = renderProjectOverviewHtml(p, slug, ov, false);
  } catch (err) {
    main.innerHTML =
      renderProjectOverviewHtml(p, slug, null, false) +
      '<p class="error" style="margin-top:12px">Không tải số liệu cluster: ' +
      esc(errorMessage(err)) +
      "</p>";
  }
}

function renderProjectQuickNav(slug) {
  const items = [
    { href: "#/project/" + slug + "/deploy", icon: "⎇", title: "Deploy / Git", sub: "Repo, pipeline, push" },
    { href: "#/project/" + slug + "/runtime", icon: "▶", title: "Runtime", sub: "Health & restart" },
    { href: "#/project/" + slug + "/monitoring", icon: "📈", title: "Monitoring", sub: "CPU/RAM timeline" },
    { href: "#/project/" + slug + "/ops", icon: "⌘", title: "Sổ lệnh K8s", sub: "Log & kubectl" },
    { href: "#/project/" + slug + "/addons", icon: "🧩", title: "Addons", sub: "Redis, DB…" },
    { href: "#/project/" + slug + "/domains", icon: "🌐", title: "Domains", sub: "URL & Ingress" },
    { href: "#/project/" + slug + "/env", icon: "🔑", title: "Cấu hình app", sub: "Env build & pod" },
  ];
  return (
    '<div class="project-ov-quick">' +
    items
      .map(function (it) {
        return (
          '<a class="project-ov-quick-card" href="' +
          esc(it.href) +
          '"><span class="project-ov-quick-icon" aria-hidden="true">' +
          it.icon +
          "</span><strong>" +
          esc(it.title) +
          '</strong><span class="muted">' +
          esc(it.sub) +
          "</span></a>"
        );
      })
      .join("") +
    "</div>"
  );
}
