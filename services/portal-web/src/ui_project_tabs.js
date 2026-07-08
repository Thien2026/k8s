const WORKLOAD_HEALTH_LABELS = {
  healthy: "Ổn định",
  running: "Running",
  degraded: "Chưa Ready",
  restarting: "Đang restart",
  pending: "Đang khởi tạo",
  failed: "Lỗi",
  crash_loop: "CrashLoop",
  oom_killed: "OOMKilled",
  unknown: "Chưa rõ",
};

function workloadHealthBadge(health) {
  const h = String(health || "unknown");
  const label = WORKLOAD_HEALTH_LABELS[h] || h;
  let cls = "badge";
  if (h === "running" || h === "healthy") cls += " ok";
  else if (h === "restarting" || h === "pending" || h === "degraded") cls += " warn";
  else if (h === "crash_loop" || h === "oom_killed" || h === "failed") cls += " bad";
  else cls += " muted";
  return '<span class="' + cls + '">' + esc(label) + "</span>";
}

function renderWorkloadHealthSummary(summary) {
  summary = summary || {};
  const overall = summary.overall || "unknown";
  return (
    '<div class="workload-health-summary">' +
    '<div class="workload-health-overall">' +
    "<span class=\"muted\">Trạng thái chung</span>" +
    workloadHealthBadge(overall === "healthy" ? "healthy" : overall) +
    "</div>" +
    '<div class="workload-health-stats">' +
    '<div class="workload-stat"><span class="muted">Pod Running</span><strong>' + esc(summary.pods_running || 0) + "</strong></div>" +
    '<div class="workload-stat"><span class="muted">Pod lỗi</span><strong>' + esc(summary.pods_unhealthy || 0) + "</strong></div>" +
    '<div class="workload-stat"><span class="muted">CrashLoop</span><strong>' + esc(summary.pods_crash_loop || 0) + "</strong></div>" +
    '<div class="workload-stat"><span class="muted">OOM</span><strong>' + esc(summary.pods_oom || 0) + "</strong></div>" +
    '<div class="workload-stat"><span class="muted">Tổng restart</span><strong>' + esc(summary.total_restarts || 0) + "</strong></div>" +
    "</div>" +
    '<p class="muted workload-health-hint">K8s tự restart pod khi crash/OOM. 503 tạm thời là bình thường trong lúc pod mới lên.</p>' +
    "</div>"
  );
}

function renderWorkloadHealthHtml(data, slug) {
  const deps = data.deployments || [];
  const pods = data.pods || [];
  const events = data.events || [];
  const depRows = deps
    .map(function (d) {
      const restartBtn =
        data.can_restart
          ? ' <button type="button" class="btn-ghost btn-sm wl-restart-dep" data-dep="' + esc(d.name) + '">Restart</button>'
          : "";
      return (
        "<tr><td>" +
        esc(d.name) +
        "</td><td>" +
        esc(d.replicas || "—") +
        "</td><td>" +
        workloadHealthBadge(d.health) +
        "</td><td class=\"muted\">" +
        esc(d.status || "—") +
        "</td><td class=\"col-actions\">" +
        restartBtn +
        "</td></tr>"
      );
    })
    .join("");
  const podRows = pods
    .map(function (pod) {
      const term = pod.last_termination_reason ? ' <span class="muted">(' + esc(pod.last_termination_reason) + ")</span>" : "";
      const restarts =
        pod.restarts > 0
          ? '<span class="badge warn">' + esc(pod.restarts) + "</span>"
          : esc(pod.restarts || 0);
      return (
        "<tr><td>" +
        esc(pod.name) +
        "</td><td>" +
        workloadHealthBadge(pod.health) +
        term +
        "</td><td>" +
        restarts +
        "</td><td class=\"col-actions\">" +
        '<button type="button" class="btn-ghost btn-sm wl-pod-logs" data-pod="' +
        esc(pod.name) +
        '">Logs</button> ' +
        '<a class="btn-ghost btn-sm" href="' +
        esc(projectRoute(slug, "ops")) +
        '">Events</a>' +
        "</td></tr>"
      );
    })
    .join("");
  const eventBlock =
    events.length > 0
      ? '<ul class="workload-events-list">' +
        events.map(function (line) {
          return "<li>" + esc(line) + "</li>";
        }).join("") +
        "</ul>"
      : '<p class="muted">Không có event cảnh báo gần đây.</p>';
  return (
    '<div id="workload-health-root">' +
    renderWorkloadHealthSummary(data.summary) +
    '<div class="card workload-card"><div class="workload-card-head"><h3>Deployments</h3>' +
    (data.can_restart ? '<span class="muted">Owner/admin có thể restart rollout</span>' : "") +
    "</div>" +
    '<div class="table-wrap"><table class="data-table"><thead><tr><th>Tên</th><th>Replicas</th><th>Health</th><th>Status</th><th></th></tr></thead><tbody>' +
    (depRows || '<tr><td colspan="5" class="muted">Chưa có deployment</td></tr>') +
    "</tbody></table></div></div>" +
    '<div class="card workload-card"><h3>Pods</h3>' +
    '<div class="table-wrap"><table class="data-table"><thead><tr><th>Tên</th><th>Health</th><th>Restarts</th><th></th></tr></thead><tbody>' +
    (podRows || '<tr><td colspan="4" class="muted">Chưa có pod</td></tr>') +
    "</tbody></table></div></div>" +
    '<div class="card workload-card"><h3>Events gần đây</h3>' +
    eventBlock +
    '<p class="muted" style="margin-top:10px">Xem thêm tại <a href="' +
    esc(projectRoute(slug, "ops")) +
    '">Sổ lệnh K8s</a> → <code>kubectl get events</code></p></div>' +
    "</div>"
  );
}

function bindWorkloadHealthPage(slug, env, ns) {
  const root = document.getElementById("workload-health-root");
  if (!root) return;
  root.querySelectorAll(".wl-restart-dep").forEach(function (btn) {
    btn.onclick = async function () {
      const dep = btn.dataset.dep || "app";
      if (!(await uiConfirm("Restart deployment \"" + dep + "\"? Pod sẽ được tạo lại.", { title: "Restart deployment" }))) return;
      btn.disabled = true;
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/workload/restart", {
          method: "POST",
          body: { environment: env, deployment: dep },
        });
        toastSuccess(res.message || "Đã gửi lệnh restart");
        setTimeout(function () {
          pageProjectHub(document.getElementById("main"), slug, "runtime");
        }, 1500);
      } catch (err) {
        toastError(err.message || "Restart thất bại");
        btn.disabled = false;
      }
    };
  });
  root.querySelectorAll(".wl-pod-logs").forEach(function (btn) {
    btn.onclick = async function () {
      const pod = btn.dataset.pod;
      if (!pod) return;
      btn.disabled = true;
      try {
        const data = await api("/api/v1/k8s/pods/" + encodeURIComponent(pod) + "/logs" + qs({ namespace: ns, tail: 200 }));
        const text = data.logs || data.log || data.output || "";
        const overlay = document.createElement("div");
        overlay.className = "ui-overlay";
        overlay.innerHTML =
          '<div class="ui-dialog ui-dialog-wide" role="dialog" aria-modal="true">' +
          '<div class="ui-dialog-glow"></div>' +
          '<h3 class="ui-dialog-title">Logs · ' + esc(pod) + "</h3>" +
          logDetailsBlock({ text: text, open: true, summary: "200 dòng cuối" }) +
          '<div class="ui-dialog-actions" style="margin-top:16px;padding-top:0;border:0">' +
          '<button type="button" class="btn-primary wl-log-close">Đóng</button></div></div>';
        function close() {
          overlay.classList.remove("show");
          setTimeout(function () { overlay.remove(); }, 200);
        }
        overlay.querySelector(".wl-log-close").onclick = close;
        overlay.onclick = function (e) { if (e.target === overlay) close(); };
        document.body.appendChild(overlay);
        requestAnimationFrame(function () { overlay.classList.add("show"); });
        bindDeployLogCopyButtons(overlay);
      } catch (err) {
        toastError(err.message || "Không đọc được log");
      } finally {
        btn.disabled = false;
      }
    };
  });
}

async function loadProjectRuntimePage(main, slug, p) {
  const env = state.projectEnv || "dev";
  const envLabel = env.toUpperCase();
  main.innerHTML =
    projectHeader(p, "Runtime · sức khỏe workload") +
    projectEnvToolbar(slug, p, function () {
      pageProjectHub(main, slug, "runtime");
    }) +
    '<div class="card" id="workload-health-page"><p class="loading">Đang tải health…</p></div>';
  try {
    const data = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/workload-health" + projectQs({ environment: env })
    );
    const page = document.getElementById("workload-health-page");
    if (!page) return;
    page.innerHTML = "<h3>Workload · " + esc(envLabel) + " · <code>" + esc(data.namespace || "") + "</code></h3>" + renderWorkloadHealthHtml(data, slug);
    bindWorkloadHealthPage(slug, env, data.namespace);
  } catch (err) {
    const page = document.getElementById("workload-health-page");
    if (page) page.innerHTML = '<p class="error-text">' + esc(err.message) + "</p>";
  }
}

function addonStatusBadge(status) {
  const s = String(status || "pending");
  let cls = "badge";
  if (s === "running") cls += " ok";
  else if (s === "provisioning" || s === "pending") cls += " warn";
  else if (s === "failed") cls += " bad";
  else cls += " muted";
  return '<span class="' + cls + '">' + esc(s) + "</span>";
}

function addonIcon(engine) {
  if (engine === "redis") return "⚡";
  if (engine === "postgres") return "🐘";
  return "🧩";
}

async function loadProjectAddonsHub(main, slug, p) {
  main.innerHTML =
    projectHeader(p, "Addons · data services") +
    projectEnvToolbar(slug, p, function () {
      pageProjectHub(main, slug, "addons");
    }) +
    '<div id="addons-hub-root" class="card"><p class="loading">Đang tải addons…</p></div>';
  try {
    const data = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons");
    const root = document.getElementById("addons-hub-root");
    if (!root) return;
    const env = state.projectEnv || "dev";
    const installed = (data.items || []).filter(function (it) {
      return it.environment === env;
    });
    const catalog = data.catalog || [];
    const cards = catalog
      .map(function (cat) {
        const inst = installed.find(function (it) { return it.engine === cat.engine; });
        const href = projectAddonsRoute(slug, cat.engine);
        const action = !cat.available
          ? '<span class="badge muted">Sắp có</span>'
          : inst
            ? '<a class="btn-ghost btn-sm" href="' + esc(href) + '">Quản lý</a>'
            : data.can_manage
              ? '<button type="button" class="btn-primary btn-sm addon-add-btn" data-engine="' + esc(cat.engine) + '">+ Thêm</button>'
              : '<span class="muted">Chỉ owner</span>';
        return (
          '<div class="addon-catalog-card' + (cat.available ? "" : " addon-catalog-card-disabled") + '">' +
          '<div class="addon-catalog-icon" aria-hidden="true">' + addonIcon(cat.engine) + "</div>" +
          "<div><strong>" + esc(cat.label) + "</strong>" +
          '<p class="muted addon-catalog-desc">' + esc(cat.description) + "</p>" +
          (inst ? '<p class="addon-catalog-status">' + addonStatusBadge(inst.status) + "</p>" : "") +
          "</div><div class=\"addon-catalog-action\">" + action + "</div></div>"
        );
      })
      .join("");
    root.innerHTML =
      "<h3>Catalog · " + esc(env.toUpperCase()) + "</h3>" +
      '<p class="muted">Mỗi addon = instance riêng, connection riêng, quota riêng.</p>' +
      '<div class="addon-catalog-grid">' + cards + "</div>" +
      (installed.length
        ? '<h4 style="margin:20px 0 10px">Đã gắn</h4><ul class="addon-installed-list">' +
          installed.map(function (it) {
            return (
              "<li><a href=\"" + esc(projectAddonsRoute(slug, it.engine)) + '">' +
              addonIcon(it.engine) + " " + esc(it.engine) + " · " + addonStatusBadge(it.status) + "</a></li>"
            );
          }).join("") +
          "</ul>"
        : "");
    root.querySelectorAll(".addon-add-btn").forEach(function (btn) {
      btn.onclick = async function () {
        const engine = btn.dataset.engine;
        if (!(await uiConfirm("Thêm " + engine + " cho môi trường " + env.toUpperCase() + "?", { title: "Thêm addon" }))) return;
        btn.disabled = true;
        try {
          const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/" + encodeURIComponent(engine), {
            method: "POST",
            body: { environment: env, max_memory_mb: 128, max_clients: 100 },
          });
          toastSuccess(res.message || "Đã thêm addon");
          location.hash = projectAddonsRoute(slug, engine);
          await navigate();
        } catch (err) {
          toastError(err.message || "Không thêm được addon");
          btn.disabled = false;
        }
      };
    });
  } catch (err) {
    const root = document.getElementById("addons-hub-root");
    if (root) root.innerHTML = '<p class="error-text">' + esc(err.message) + "</p>";
  }
}

async function loadProjectAddonRedis(main, slug, p) {
  const env = state.projectEnv || "dev";
  main.innerHTML =
    projectHeader(p, "Redis · addon") +
    projectEnvToolbar(slug, p, function () {
      pageProjectHub(main, slug, "addons", "redis");
    }) +
    '<div id="addon-redis-root" class="card"><p class="loading">Đang tải Redis…</p></div>';
  try {
    const data = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis" + projectQs({ environment: env })
    );
    const root = document.getElementById("addon-redis-root");
    if (!root) return;
    if (!data.installed) {
      root.innerHTML =
        "<h3>Redis · " + esc(env.toUpperCase()) + "</h3>" +
        '<p class="muted">Chưa bật Redis cho môi trường này.</p>' +
        (data.can_manage
          ? '<button type="button" class="btn-primary" id="addon-redis-enable">+ Bật Redis</button>' +
            '<p class="muted" style="margin-top:10px"><a href="' + esc(projectAddonsRoute(slug)) + '">← Về catalog</a></p>'
          : "") +
        "";
      const enableBtn = document.getElementById("addon-redis-enable");
      if (enableBtn) {
        enableBtn.onclick = async function () {
          try {
            await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis", {
              method: "POST",
              body: { environment: env, max_memory_mb: 128, max_clients: 100 },
            });
            toastSuccess("Đã ghi nhận — provision cluster bước kế tiếp");
            await loadProjectAddonRedis(main, slug, p);
          } catch (err) {
            toastError(err.message || "Lỗi");
          }
        };
      }
      return;
    }
    const addon = data.addon || {};
    root.innerHTML =
      "<h3>Redis · " + esc(env.toUpperCase()) + " · <code>" + esc(data.namespace || "") + "</code></h3>" +
      '<div class="addon-redis-section" id="addon-sec-overview">' +
      "<h4>Tổng quan</h4>" +
      '<p>Trạng thái: ' + addonStatusBadge(addon.status) + "</p>" +
      '<p class="muted">Release: <code>' + esc(addon.k8s_release || "—") + "</code></p>" +
      "</div>" +
      '<div class="addon-redis-section" id="addon-sec-connection">' +
      "<h4>Connection</h4>" +
      (addon.has_connection
        ? '<p class="muted">REDIS_URL đã sẵn sàng — hiển thị đầy đủ sau bước provision.</p>'
        : '<p class="muted">Chưa có connection — provision Helm sẽ sinh Secret.</p>') +
      "</div>" +
      '<div class="addon-redis-section" id="addon-sec-quota">' +
      "<h4>Quota</h4>" +
      '<p>RAM: <strong>' + esc(addon.max_memory_mb || 128) + " MB</strong> · Max clients: <strong>" + esc(addon.max_clients || 100) + "</strong></p>" +
      '<p class="muted">Chỉnh quota + apply Helm — bước kế tiếp Phase 10a.</p></div>' +
      '<p class="muted" style="margin-top:12px"><a href="' + esc(projectAddonsRoute(slug)) + '">← Về catalog addons</a></p>';
  } catch (err) {
    const root = document.getElementById("addon-redis-root");
    if (root) root.innerHTML = '<p class="error-text">' + esc(err.message) + "</p>";
  }
}

/* --- hub tabs: monitoring / ops / history / promote --- */

async function loadProjectMonitoring(main, slug, p, env) {
    const hashQ = new URLSearchParams(location.hash.split("?")[1] || "");
    const win = hashQ.get("window") || "6h";
    const ov = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/overview" + projectQs());
    const mon = ov.monitoring || {};
    const metrics = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/monitoring" + projectQs({ env: env, window: win })
    ).catch(function () { return {}; });
    let grafanaBase = mon.grafana_url || "";
    if (!grafanaBase) {
      const infra = await api("/api/v1/infra/links").catch(function () { return { items: [] }; });
      const g = (infra.items || []).find(function (x) { return x && x.key === "grafana" && x.url; });
      grafanaBase = g && g.url ? g.url : "";
    }
    const devUrl = mon.dev_dashboard_url || grafanaNamespaceDashboardUrl(grafanaBase, p.namespace_dev);
    const prodUrl = mon.prod_dashboard_url || grafanaNamespaceDashboardUrl(grafanaBase, p.namespace_prod);
    const activeUrl = env === "prod" ? prodUrl : devUrl;
    const cpu = Number(metrics.cpu_cores_5m || 0);
    const memMiB = Number(metrics.memory_mib || 0);
    const restarts = Number(metrics.restarts_1h || 0);
    const pods = Number(metrics.running_pods || 0);
    const cpuSeries = monitoringPoints(metrics.cpu_series, 1);
    const memSeries = monitoringPoints(metrics.memory_series, 1024 * 1024);
    const cpuStats = timelineStats(cpuSeries);
    const memStats = timelineStats(memSeries);
    const insight = monitoringInsight(cpu, memMiB, restarts);
    const topCPU = (metrics.top_cpu_pods || []).map(function (r) {
      return { name: r.name || "unknown", value: Number(r.value || 0).toFixed(4) + " cores" };
    });
    const topMem = (metrics.top_mem_pods || []).map(function (r) {
      return { name: r.name || "unknown", value: Number(r.value || 0).toFixed(1) + " MiB" };
    });
    main.innerHTML =
      '<div class="monitor-page">' +
      projectHeader(p, "Biểu đồ CPU/RAM · Prometheus") +
      '<p class="monitor-page-lead muted">Timeline metric namespace — khác tab Tổng quan (chỉ xem trạng thái & lối tắt).</p>' +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "monitoring"); }) +
      monitoringWindowToolbar(win, slug, env) +
      '<div class="card detail-card monitor-summary">' +
      '<div class="monitor-summary-head"><h3>Tóm tắt nhanh</h3><span class="badge ' + (insight.tone === "warn" ? "badge-warn" : "badge-ok") + '">' + (insight.tone === "warn" ? "Cần chú ý" : "Ổn định") + "</span></div>" +
      '<p class="muted">' + esc(insight.text) + "</p></div>" +
      '<div class="stat-grid">' +
      statBox(pods.toFixed(0), "Running Pods", "g1") +
      statBox(cpu.toFixed(2), "CPU trung bình 5 phút", "g2") +
      statBox(memMiB.toFixed(0) + " MiB", "Memory đang dùng", "g3") +
      statBox(restarts.toFixed(0), "Số restart 1 giờ", "g4") +
      "</div>" +
      '<div class="card detail-card"><h3>CPU timeline (' + esc(win) + ')</h3>' +
      lineChart(cpuSeries, { color: "#46d6ff", unit: "cores", digits: 3 }) +
      timelineStatsChips(cpuStats, "cores", 3) +
      timelinePeakText(cpuStats, "cores", 3) +
      '<p class="muted">Nguồn: rate(container_cpu_usage_seconds_total[5m]) · namespace hiện tại.</p></div>' +
      '<div class="card detail-card"><h3>Memory timeline (' + esc(win) + ')</h3>' +
      lineChart(memSeries, { color: "#c084fc", unit: "MiB", digits: 1 }) +
      timelineStatsChips(memStats, "MiB", 1) +
      timelinePeakText(memStats, "MiB", 1) +
      '<p class="muted">Nguồn: container_memory_working_set_bytes · đơn vị MiB.</p></div>' +
      '<div class="dash-grid-bottom">' +
      '<div class="card detail-card"><h3>Top Pods CPU</h3>' +
      renderTable(
        [{ key: "name", label: "Pod" }, { key: "value", label: "CPU (cores)" }],
        topCPU,
        ""
      ) +
      "</div>" +
      '<div class="card detail-card"><h3>Top Pods Memory</h3>' +
      renderTable(
        [{ key: "name", label: "Pod" }, { key: "value", label: "Memory (MiB)" }],
        topMem,
        ""
      ) +
      "</div></div>" +
      (activeUrl
        ? '<div class="card detail-card"><h3>Grafana nâng cao</h3><p class="muted">Cần điều tra sâu? Mở dashboard namespace hoặc Grafana full.</p><div class="meta-chips">' +
          (devUrl ? '<a class="chip chip-link" href="' + esc(devUrl) + '" target="_blank" rel="noopener">Dashboard Dev</a>' : "") +
          (prodUrl ? '<a class="chip chip-link" href="' + esc(prodUrl) + '" target="_blank" rel="noopener">Dashboard Prod</a>' : "") +
          '<a class="chip chip-link" href="' + esc(activeUrl) + '" target="_blank" rel="noopener">Namespace hiện tại</a>' +
          (grafanaBase ? '<a class="chip chip-link" href="' + esc(grafanaBase) + '" target="_blank" rel="noopener">Grafana full</a>' : "") +
          "</div></div>"
        : "") +
      (metrics.warning ? '<div class="card detail-card"><p class="muted">' + esc(metrics.warning) + "</p></div>" : "") +
      "</div>";
    bindInteractiveCharts(main);
}

async function loadProjectOps(main, slug, p) {
    const envOps = state.projectEnv || "dev";
    const nsOps = envOps === "prod" ? p.namespace_prod : p.namespace_dev;
    main.innerHTML =
      projectHeader(p, "Sổ lệnh K8s · terminal read-only", { help: "k8sops" }) +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "ops"); }) +
      '<div id="k8s-ops-root"></div>';
    await pageK8sOps(document.getElementById("k8s-ops-root"), {
      scope: "project",
      namespace: nsOps,
      slug: slug,
      embed: true,
    });
    bindK8sOpsHelpTriggers(main);
}

async function loadProjectWorkloadList(main, slug, p, tab, ns) {
  if (!PROJECT_WORKLOADS.some(function (w) { return w.tab === tab; })) return false;
    const wl = PROJECT_WORKLOADS.find(function (w) { return w.tab === tab; });
    state.namespace = ns;
    localStorage.setItem("filter-ns", ns);
    const list = await api("/api/v1/k8s/" + tab + qs({ namespace: ns, limit: 50 }));
    main.innerHTML =
      projectHeader(p, wl.label + " · " + ns) +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, tab); }) +
      '<div class="card">' +
      renderTable(
        tab === "pods"
          ? [
              { key: "name", label: "Name" },
              { key: "status", label: "Status" },
              { key: "restarts", label: "Restarts" },
              { key: "node", label: "Node" },
            ]
          : tab === "deployments"
            ? [
                { key: "name", label: "Name" },
                { key: "replicas", label: "Replicas" },
                { key: "status", label: "Status" },
              ]
            : [
                { key: "name", label: "Name" },
                { key: "status", label: "Status" },
              ],
        list.items || [],
        tab
      ) +
      "</div>";
  return true;
}

async function loadProjectDeployHistory(main, slug, p) {
    const env = state.projectEnv || "dev";
    const envLabel = env.toUpperCase();
    main.innerHTML =
      projectHeader(p, "Lịch sử deploy") +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "deploy-history"); }) +
      '<div class="card" id="deploy-history-page"><h3>Lịch sử · ' + esc(envLabel) + '</h3><p class="loading">Đang tải lịch sử…</p></div>';
    try {
      const activity = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env, scope: "history" })
      );
      state.deployActivityCache[deployHistoryPageKey(slug, env)] = activity;
      const hist = renderDeployHistoryContent(activity, { slug: slug, expectedEnv: env });
      const cur = activity.current;
      let body = "";
      if (cur) {
        body +=
          '<p class="muted" style="margin-bottom:12px">Bản mới nhất (cũng xem tại tab <a href="' +
          esc(projectRoute(slug, "deploy")) +
          '">Deploy / Git</a>):</p>' +
          renderDeployPipelineItem(cur, true, { env: env });
      }
      body +=
        '<h4 style="margin:20px 0 10px">Các bản trước (' +
        hist.count +
        ")</h4>" +
        '<p class="muted deploy-history-note" style="margin:0 0 10px;font-size:12px">Mỗi commit = 1 tag · badge = kiểu deploy lúc đó. <strong>Deploy lại</strong> chỉ cùng kiểu chạy. Đổi kiểu → tab Deploy / Git → <strong>Đổi kiểu chạy…</strong></p>' +
        '<div id="deploy-history-list">' +
        hist.itemsHtml +
        "</div>" +
        hist.pagerHtml;
      document.getElementById("deploy-history-page").innerHTML =
        "<h3>Lịch sử · " + esc(envLabel) + "</h3>" + renderDeployProfileContext(activity) + body;
      bindDeployHistoryPagination(slug, env);
      bindDeployActivityActions(slug, env);
    } catch (err) {
      document.getElementById("deploy-history-page").innerHTML =
        '<p class="error-text">' + esc(err.message) + "</p>";
    }
}

async function loadProjectPromote(main, slug, p) {
    if (!canManagePlatformProjects()) {
      main.innerHTML =
      projectHeader(p, "Promote Prod", { help: "deploy" }) +
        '<div class="card"><p class="error-text">Chỉ admin/tech_lead được promote lên prod.</p></div>';
      bindDeployHelpTriggers(main);
      return;
    }
    main.innerHTML =
      projectHeader(p, "Promote Prod", { help: "deploy" }) +
      '<div class="card" id="promote-page"><p class="loading">Đang kiểm tra checklist…</p></div>';
    bindDeployHelpTriggers(main);
    try {
      const [readiness, activityDev] = await Promise.all([
        api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote-readiness"),
        api(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: "dev", scope: "current" })
        ),
      ]);
      rememberPromoteReadiness(slug, readiness);
      const tag = promotableDevImageTag(activityDev, readiness);
      let html = renderDeployPromotePrep(readiness, slug);
      if (tag) {
        html +=
          '<div class="deploy-promote-bar" style="margin-top:16px">' +
          '<div class="deploy-promote-bar-inner">' +
          '<span class="muted">Image dev sẵn sàng: <code>' +
          esc(tag.slice(0, 12)) +
          "</code>" +
          (activityDev.current && activityDev.current.status !== "success"
            ? ' <span class="badge warn" style="margin-left:6px">từ lịch sử success</span>'
            : "") +
          "</span>" +
          '<button type="button" class="btn-primary" id="deploy-promote-btn" data-tag="' +
          esc(tag) +
          '"' +
          (readiness.ready ? "" : ' disabled title="Hoàn tất checklist"') +
          ">Promote lên Prod →</button></div></div>";
      } else {
        html +=
          '<p class="muted" style="margin-top:14px">Chưa có bản dev <strong>success</strong> để promote. Deploy ổn trên tab <a href="' +
          esc(projectRoute(slug, "deploy")) +
          '">Deploy / Git</a> trước.</p>';
      }
      html +=
        '<p class="muted" style="margin-top:14px;font-size:12px">Promote = cùng image tag lên prod, không chạy GitHub Actions. Theo dõi prod tại tab Deploy hoặc Runtime.</p>';
      document.getElementById("promote-page").innerHTML = html;
      bindPromotePrepLinks(slug);
      bindDeployHelpTriggers(main);
      const promoteBtn = document.getElementById("deploy-promote-btn");
      if (promoteBtn) {
        promoteBtn.onclick = async function () {
          const r = state.deployPromoteReadiness[slug];
          if (r && !r.ready) {
            toastError("Hoàn tất checklist Promote trước");
            return;
          }
          const t = promoteBtn.dataset.tag;
          if (!t) return;
          const ok = await uiConfirm({
            title: "Promote lên Prod",
            message: "Deploy image " + t.slice(0, 7) + " từ DEV lên PROD?",
            details: ["Cùng image tag — không build lại", "Namespace prod: " + p.namespace_prod],
            confirmText: "Promote",
          });
          if (!ok) return;
          setButtonLoading(promoteBtn, true, "Đang promote…");
          try {
            await api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote", {
              method: "POST",
              body: { image_tag: t },
            });
            navigateAfterPromote(slug, t);
          } catch (err) {
            toastError(err.message || "Promote prod thất bại");
          } finally {
            setButtonLoading(promoteBtn, false, "Promote lên Prod →");
          }
        };
      }
    } catch (err) {
      document.getElementById("promote-page").innerHTML = '<p class="error-text">' + esc(err.message) + "</p>";
    }
}
