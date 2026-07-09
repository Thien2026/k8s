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

function renderAddonRedisDashboard(addon, canManage) {
  const pod = addon.pod;
  let podHtml = '<p class="muted">Chưa thấy pod Redis trên cluster.</p>';
  if (pod && pod.name) {
    podHtml =
      "<p>Pod: <code>" + esc(pod.name) + "</code> · " + esc(pod.status || "—") +
      " · restarts <strong>" + esc(pod.restarts || 0) + "</strong>" +
      (pod.ready ? ' <span class="badge ok">ready</span>' : ' <span class="badge warn">not ready</span>') +
      "</p>";
  }
  return (
    podHtml +
    (canManage
      ? '<div class="addon-redis-ops" style="margin-top:10px">' +
        '<button type="button" class="btn-ghost btn-sm" id="addon-redis-restart">Restart pod</button> ' +
        '<button type="button" class="btn-ghost btn-sm" id="addon-redis-load-logs">Xem logs</button>' +
        "</div>" +
        '<pre class="addon-redis-logs hidden" id="addon-redis-logs"></pre>'
      : "")
  );
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

function renderAddonRedisConnection(addon, canManage) {
  let html = "";
  if (addon.has_connection && addon.connection_url_masked) {
    html +=
      '<div class="addon-connection-box">' +
      '<p class="muted">REDIS_URL · trong cluster</p>' +
      '<div class="addon-connection-row">' +
      '<code class="addon-connection-url">' + esc(addon.connection_url_masked) + "</code>" +
      (canManage
        ? '<button type="button" class="btn-ghost btn-sm" id="addon-redis-copy-url">Copy</button>'
        : "") +
      "</div>";
    if (addon.connection_url_external_masked) {
      html +=
        '<p class="muted" style="margin-top:12px">REDIS_URL · dev từ máy local (NodePort + DNS <code>*.redis.{domain}</code>)</p>' +
        '<div class="addon-connection-row">' +
        '<code class="addon-connection-url">' + esc(addon.connection_url_external_masked) + "</code>" +
        (canManage && addon.connection_url_external
          ? '<button type="button" class="btn-ghost btn-sm" id="addon-redis-copy-url-ext">Copy</button>'
          : "") +
        "</div>" +
        (addon.external_hostname
          ? '<p class="muted addon-connection-meta">Host: <code>' + esc(addon.external_hostname) + "</code>" +
            (addon.external_port ? " · port <code>" + esc(addon.external_port) + "</code>" : "") +
            " · CF DNS only, whitelist IP nếu expose.</p>"
          : "");
    }
    html +=
      (addon.connection_secret
        ? '<p class="muted addon-connection-meta">Secret K8s: <code>' + esc(addon.connection_secret) + "</code></p>"
        : "") +
      '<p class="muted addon-connection-hint">App trong cluster dùng REDIS_URL runtime. Dev local dùng URL external (nếu có).</p>' +
      (canManage ? '<button type="button" class="btn-ghost btn-sm" id="addon-redis-reprovision">Re-provision Redis</button>' : "") +
      "</div>";
    return html;
  }
  if (addon.status === "provisioning" || addon.status === "pending") {
    return '<p class="loading">Đang provision Redis trên cluster…</p>';
  }
  return (
    '<p class="muted">Chưa có connection trên cluster.</p>' +
    (canManage ? '<button type="button" class="btn-primary btn-sm" id="addon-redis-reprovision">Provision Redis</button>' : "")
  );
}

function renderAddonRedisQuota(addon, canManage) {
  const mem = addon.max_memory_mb || 128;
  const clients = addon.max_clients || 100;
  if (!canManage) {
    return (
      "<p>RAM: <strong>" + esc(mem) + " MB</strong> · Max clients: <strong>" + esc(clients) + "</strong></p>" +
      '<p class="muted">Chỉ owner/admin chỉnh quota.</p>'
    );
  }
  return (
    '<div class="addon-quota-form">' +
    '<label class="addon-quota-field">RAM (MB)<input type="number" id="addon-redis-mem" min="64" max="512" step="1" value="' + esc(mem) + '"></label>' +
    '<label class="addon-quota-field">Max clients<input type="number" id="addon-redis-clients" min="10" max="1000" step="1" value="' + esc(clients) + '"></label>' +
    '<button type="button" class="btn-primary btn-sm" id="addon-redis-quota-apply">Lưu & apply</button>' +
    "</div>" +
    '<p class="muted">Apply = re-provision Redis với quota mới (redis.conf + memory limit pod).</p>'
  );
}

function bindAddonRedisActions(main, slug, p, env, canManage, addon) {
  if (!canManage) return;
  const fullURL = addon && addon.connection_url ? String(addon.connection_url) : "";
  const fullExtURL = addon && addon.connection_url_external ? String(addon.connection_url_external) : "";
  const copyBtn = document.getElementById("addon-redis-copy-url");
  if (copyBtn && fullURL) {
    copyBtn.onclick = function () {
      copyText(fullURL, "Đã copy REDIS_URL");
    };
  } else if (copyBtn) {
    copyBtn.disabled = true;
    copyBtn.title = "Chưa có REDIS_URL";
  }
  const copyExtBtn = document.getElementById("addon-redis-copy-url-ext");
  if (copyExtBtn && fullExtURL) {
    copyExtBtn.onclick = function () {
      copyText(fullExtURL, "Đã copy REDIS_URL external");
    };
  }
  const restartBtn = document.getElementById("addon-redis-restart");
  if (restartBtn) {
    restartBtn.onclick = async function () {
      const ok = await uiConfirm("Restart pod Redis?", { title: "Restart Redis" });
      if (!ok) return;
      restartBtn.disabled = true;
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis/restart", {
          method: "POST",
          body: { environment: env },
        });
        toastSuccess(res.message || "Đang restart Redis");
        await loadProjectAddonRedis(main, slug, p);
      } catch (err) {
        toastError(err.message || "Restart thất bại");
        restartBtn.disabled = false;
      }
    };
  }
  const logsBtn = document.getElementById("addon-redis-load-logs");
  const logsPre = document.getElementById("addon-redis-logs");
  if (logsBtn && logsPre) {
    logsBtn.onclick = async function () {
      logsBtn.disabled = true;
      logsPre.classList.remove("hidden");
      logsPre.textContent = "Đang tải logs…";
      try {
        const res = await api(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis/logs" + projectQs({ environment: env, tail: 300 })
        );
        logsPre.textContent = res.logs || "(trống)";
      } catch (err) {
        logsPre.textContent = err.message || "Không tải được logs";
      } finally {
        logsBtn.disabled = false;
      }
    };
  }
  async function runProvision(extraBody) {
    const ok = await uiConfirm(
      "Re-provision Redis? Mật khẩu mới sẽ được tạo, REDIS_URL cập nhật và app restart để nhận env.",
      { title: "Re-provision Redis" }
    );
    if (!ok) return;
    const buttons = document.querySelectorAll("#addon-redis-reprovision, #addon-redis-reprovision-top, #addon-redis-quota-apply");
    buttons.forEach(function (el) { el.disabled = true; });
    try {
      const body = Object.assign({ environment: env }, extraBody || {});
      const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis/provision", {
        method: "POST",
        body: body,
      });
      toastSuccess(res.message || "Đã re-provision Redis");
      await loadProjectAddonRedis(main, slug, p);
    } catch (err) {
      toastError(err.message || "Re-provision thất bại");
      buttons.forEach(function (el) { el.disabled = false; });
    }
  }
  ["addon-redis-reprovision", "addon-redis-reprovision-top"].forEach(function (id) {
    const btn = document.getElementById(id);
    if (btn) btn.onclick = function () { runProvision(); };
  });
  const quotaBtn = document.getElementById("addon-redis-quota-apply");
  if (quotaBtn) {
    quotaBtn.onclick = function () {
      const mem = parseInt(document.getElementById("addon-redis-mem").value, 10);
      const clients = parseInt(document.getElementById("addon-redis-clients").value, 10);
      if (!(mem >= 64 && mem <= 512)) {
        toastError("RAM phải từ 64–512 MB");
        return;
      }
      if (!(clients >= 10 && clients <= 1000)) {
        toastError("Max clients phải từ 10–1000");
        return;
      }
      runProvision({ max_memory_mb: mem, max_clients: clients });
    };
  }
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
        ? '<h4 class="addon-installed-title">Đã gắn</h4><ul class="addon-installed-list">' +
          installed.map(function (it) {
            return (
              '<li><a class="addon-installed-link" href="' + esc(projectAddonsRoute(slug, it.engine)) + '">' +
              '<span class="addon-installed-engine">' + addonIcon(it.engine) + " " + esc(it.engine) + "</span>" +
              '<span class="addon-installed-status">' + addonStatusBadge(it.status) + "</span></a></li>"
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
            toastSuccess("Đã bật Redis");
            await loadProjectAddonRedis(main, slug, p);
          } catch (err) {
            toastError(err.message || "Lỗi");
          }
        };
      }
      return;
    }
    const addon = data.addon || {};
    const canManage = !!data.can_manage;
    root.innerHTML =
      "<h3>Redis · " + esc(env.toUpperCase()) + " · <code>" + esc(data.namespace || "") + "</code></h3>" +
      '<div class="toolbar addon-redis-nav" style="margin:10px 0 14px">' +
      '<a class="btn-ghost btn-sm" href="#addon-sec-dashboard">Dashboard</a> ' +
      '<a class="btn-ghost btn-sm" href="#addon-sec-connection">Connection</a> ' +
      '<a class="btn-ghost btn-sm" href="#addon-sec-quota">Quota</a>' +
      (canManage ? ' <button type="button" class="btn-ghost btn-sm" id="addon-redis-reprovision-top">Re-provision</button>' : "") +
      "</div>" +
      '<div class="addon-redis-section" id="addon-sec-dashboard">' +
      "<h4>Dashboard</h4>" +
      '<p>Trạng thái: ' + addonStatusBadge(addon.status) + "</p>" +
      '<p class="muted">Release: <code>' + esc(addon.k8s_release || "—") + "</code></p>" +
      '<p class="muted">Namespace: <code>' + esc(data.namespace || "—") + "</code></p>" +
      renderAddonRedisDashboard(addon, canManage) +
      "</div>" +
      '<div class="addon-redis-section" id="addon-sec-connection">' +
      "<h4>Connection</h4>" +
      renderAddonRedisConnection(addon, canManage) +
      "</div>" +
      '<div class="addon-redis-section" id="addon-sec-quota">' +
      "<h4>Quota</h4>" +
      renderAddonRedisQuota(addon, canManage) +
      "</div>" +
      '<p class="addon-back-wrap"><a class="addon-back-link" href="' + esc(projectAddonsRoute(slug)) + '">← Về catalog addons</a></p>';
    bindAddonRedisActions(main, slug, p, env, canManage, addon);
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
