const $ = (sel) => document.querySelector(sel);
const state = {
  page: {},
  limit: 50,
  namespace: localStorage.getItem("filter-ns") || "",
  clusterId: localStorage.getItem("cluster-id") || "",
  search: "",
  user: null,
  projectCtx: null,
  projectEnv: localStorage.getItem("project-env") || "dev",
  navToken: 0,
  deployPoll: null,
  deployWasLive: false,
  deployHistoryPage: {},
  deployActivityCache: {},
  deployPromoteReadiness: {},
  deployServingTag: "",
  promoteFollow: null,
  onLoginPage: false,
};

function handleSessionLost(msg) {
  stopDeployPoll();
  state.user = null;
  state.projectCtx = null;
  if (state.onLoginPage) {
    if (msg) {
      setLoginError(msg);
      toastError(msg);
    }
    return;
  }
  showLoginPage(msg);
}

function stopDeployPoll() {
  if (state.deployPoll) {
    clearInterval(state.deployPoll);
    state.deployPoll = null;
  }
  state.deployWasLive = false;
}

function nextNavToken() {
  stopDeployPoll();
  state.navToken += 1;
  return state.navToken;
}

function isNavTokenActive(token) {
  return token === state.navToken;
}

function getJoinGate() {
  return sessionStorage.getItem("join-gate") || "";
}

function setJoinGate(v) {
  if (v) sessionStorage.setItem("join-gate", v);
  else sessionStorage.removeItem("join-gate");
}

function qs(extra, opts) {
  opts = opts || {};
  const p = new URLSearchParams();
  if (!opts.project) {
    if (state.clusterId) p.set("cluster_id", state.clusterId);
    if (state.namespace) p.set("namespace", state.namespace);
  }
  if (extra) {
    Object.keys(extra).forEach((k) => {
      if (extra[k] != null && extra[k] !== "") p.set(k, extra[k]);
    });
  }
  const s = p.toString();
  return s ? "?" + s : "";
}

function projectQs(extra) {
  return qs(extra, { project: true });
}

function errorMessage(err, fallback) {
  fallback = fallback || "Không tải được dữ liệu — thử tải lại trang";
  if (!err) return fallback;
  const msg = String(err.message || err.error || err).trim();
  return msg || fallback;
}

async function api(path, opts) {
  opts = opts || {};
  const isAuth = path.indexOf("/auth/") >= 0;
  const timeoutMs = opts.timeout != null ? opts.timeout : 60000;
  const ctrl = new AbortController();
  const timer = timeoutMs > 0 ? setTimeout(function () { ctrl.abort(); }, timeoutMs) : null;
  async function doFetch() {
    return fetch(path, {
      method: opts.method || "GET",
      credentials: "include",
      signal: opts.signal || ctrl.signal,
      headers: Object.assign(
        { "Content-Type": "application/json" },
        opts.headers || {}
      ),
      body: opts.body != null ? JSON.stringify(opts.body) : undefined,
    });
  }
  try {
    let res = await doFetch();
    if (res.status === 401 && !isAuth && path !== "/api/v1/auth/refresh" && !opts.noRefresh) {
      const ref = await fetch("/api/v1/auth/refresh", { method: "POST", credentials: "include" });
      if (ref.ok) {
        res = await doFetch();
      }
    }
    const ct = res.headers.get("content-type") || "";
    const data = ct.includes("json") ? await res.json() : await res.text();
    if (!res.ok) {
      const err = typeof data === "object" && data.error ? data.error : res.statusText;
      if (res.status === 401 && !isAuth && !opts.silent401) {
        handleSessionLost(err);
      }
      throw new Error(err);
    }
    return data;
  } catch (err) {
    if (err && err.name === "AbortError") {
      throw new Error("Yêu cầu quá thời gian chờ — thử lại sau");
    }
    throw err;
  } finally {
    if (timer) clearTimeout(timer);
  }
}

function esc(s) {
  if (s == null) return "";
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/* ── Toast & Dialog (thay alert/confirm native) ── */
const TOAST_ICONS = { ok: "✓", error: "✕", info: "ℹ", warn: "!" };

function ensureToastStack() {
  let stack = document.getElementById("toast-stack");
  if (!stack) {
    stack = document.createElement("div");
    stack.id = "toast-stack";
    stack.className = "toast-stack";
    stack.setAttribute("aria-live", "polite");
    document.body.appendChild(stack);
  }
  return stack;
}

function toast(message, type, ms) {
  type = type || "info";
  ms = ms == null ? 4200 : ms;
  const stack = ensureToastStack();
  const el = document.createElement("div");
  el.className = "toast toast-" + type;
  el.innerHTML =
    '<span class="toast-icon" aria-hidden="true">' + (TOAST_ICONS[type] || "ℹ") + "</span>" +
    '<span class="toast-body">' + esc(message) + "</span>" +
    '<button type="button" class="toast-close" aria-label="Đóng">×</button>';
  const close = function () {
    el.classList.remove("show");
    setTimeout(function () { el.remove(); }, 280);
  };
  el.querySelector(".toast-close").onclick = close;
  stack.appendChild(el);
  requestAnimationFrame(function () { el.classList.add("show"); });
  if (ms > 0) setTimeout(close, ms);
}

function toastSuccess(m) { toast(m, "ok"); }
function toastError(m) { toast(m, "error", 6200); }
function toastInfo(m) { toast(m, "info"); }
function toastWarn(m) { toast(m, "warn", 5600); }

function formatDialogDetails(details) {
  if (!details) return "";
  const items = Array.isArray(details) ? details : String(details).split("\n");
  if (!items.length) return "";
  return (
    '<ul class="ui-dialog-details">' +
    items.filter(Boolean).map(function (d) { return "<li>" + esc(d) + "</li>"; }).join("") +
    "</ul>"
  );
}

function openDialog(opts) {
  return new Promise(function (resolve) {
    const overlay = document.createElement("div");
    overlay.className = "ui-overlay";
    const variant = opts.variant || "default";
    const cancelBtn = opts.cancelText !== false
      ? '<button type="button" class="btn-ghost ui-dialog-cancel">' + esc(opts.cancelText || "Huỷ") + "</button>"
      : "";
    overlay.innerHTML =
      '<div class="ui-dialog ui-dialog-' + esc(variant) + '" role="dialog" aria-modal="true">' +
      '<div class="ui-dialog-glow"></div>' +
      '<h3 class="ui-dialog-title">' + esc(opts.title || "Thông báo") + "</h3>" +
      (opts.message ? '<p class="ui-dialog-message">' + esc(opts.message) + "</p>" : "") +
      formatDialogDetails(opts.details) +
      '<div class="ui-dialog-actions">' +
      cancelBtn +
      '<button type="button" class="' + (opts.danger ? "btn-danger" : "btn-primary") + ' ui-dialog-ok">' +
      esc(opts.confirmText || "OK") +
      "</button></div></div>";

    function close(result) {
      overlay.classList.remove("show");
      setTimeout(function () {
        overlay.remove();
        document.removeEventListener("keydown", onKey);
        resolve(result);
      }, 200);
    }

    function onKey(e) {
      if (e.key === "Escape" && opts.cancelText !== false) close(false);
    }

    overlay.querySelector(".ui-dialog-ok").onclick = function () { close(true); };
    const cancel = overlay.querySelector(".ui-dialog-cancel");
    if (cancel) cancel.onclick = function () { close(false); };
    overlay.onclick = function (e) {
      if (e.target === overlay && opts.cancelText !== false) close(false);
    };
    document.body.appendChild(overlay);
    document.addEventListener("keydown", onKey);
    requestAnimationFrame(function () { overlay.classList.add("show"); });
    overlay.querySelector(".ui-dialog-ok").focus();
  });
}

function uiConfirm(message, opts) {
  opts = opts || {};
  if (typeof message === "object") {
    opts = message;
    message = opts.message;
  }
  return openDialog({
    title: opts.title || "Xác nhận",
    message: message,
    confirmText: opts.confirmText || "Đồng ý",
    cancelText: opts.cancelText || "Huỷ",
    danger: !!opts.danger,
    variant: opts.danger ? "danger" : "default",
  });
}

function uiAlert(opts) {
  if (typeof opts === "string") opts = { message: opts };
  return openDialog({
    title: opts.title || "Thông báo",
    message: opts.message,
    details: opts.details,
    confirmText: opts.confirmText || "OK",
    cancelText: false,
    variant: opts.variant || "default",
  });
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const diffMs = Date.now() - d.getTime();
  if (diffMs < 0) return d.toLocaleString("vi-VN", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit", hour12: false });
  const mins = Math.floor(diffMs / 60000);
  let rel;
  if (mins < 1) rel = "vừa xong";
  else if (mins < 60) rel = mins + " phút trước";
  else {
    const hrs = Math.floor(mins / 60);
    if (hrs < 48) rel = hrs + " giờ trước";
    else rel = Math.floor(hrs / 24) + " ngày trước";
  }
  const local = d.toLocaleString("vi-VN", {
    day: "2-digit",
    month: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
  return rel + " · " + local;
}

function resourceLink(resource, row) {
  const ns = row.namespace || "_";
  return (
    '<a class="res-link" href="#/view/' +
    esc(resource) +
    "/" +
    esc(ns) +
    "/" +
    esc(row.name) +
    '">' +
    esc(row.name) +
    "</a>"
  );
}

function renderTable(columns, rows, resource) {
  if (!rows.length) {
    return '<p class="muted">Không có dữ liệu.</p>';
  }
  const head = columns.map((c) => "<th>" + esc(c.label) + "</th>").join("");
  const body = rows
    .map((row) => {
      const cells = columns
        .map((c) => {
          if (c.key === "name" && resource && row.name) {
            return "<td>" + resourceLink(resource, row) + "</td>";
          }
          return "<td>" + (c.render ? c.render(row) : esc(row[c.key] ?? "—")) + "</td>";
        })
        .join("");
      return "<tr>" + cells + "</tr>";
    })
    .join("");
  return (
    '<div class="table-wrap"><table><thead><tr>' +
    head +
    "</tr></thead><tbody>" +
    body +
    "</tbody></table></div>"
  );
}

function renderPagination(route, total, page, limit, onChange) {
  const pages = Math.max(1, Math.ceil(total / limit));
  const start = total === 0 ? 0 : (page - 1) * limit + 1;
  const end = Math.min(page * limit, total);
  const id = "pager-" + route.replace(/\W/g, "");
  setTimeout(() => {
    const prev = document.getElementById(id + "-prev");
    const next = document.getElementById(id + "-next");
    const sel = document.getElementById(id + "-limit");
    if (prev) prev.onclick = () => onChange(page - 1, limit);
    if (next) next.onclick = () => onChange(page + 1, limit);
    if (sel) sel.onchange = () => onChange(1, parseInt(sel.value, 10));
  }, 0);
  return (
    '<div class="pagination" id="' +
    id +
    '">' +
    '<span class="muted">' +
    start +
    "–" +
    end +
    " / " +
    total +
    "</span>" +
    '<div style="display:flex;gap:8px;align-items:center">' +
    '<select id="' +
    id +
    '-limit">' +
    [25, 50, 100]
      .map(
        (n) =>
          '<option value="' +
          n +
          '"' +
          (n === limit ? " selected" : "") +
          ">" +
          n +
          "/trang</option>"
      )
      .join("") +
    "</select>" +
    '<button id="' +
    id +
    '-prev"' +
    (page <= 1 ? " disabled" : "") +
    ">← Trước</button>" +
    '<span class="muted">' +
    page +
    "/" +
    pages +
    "</span>" +
    '<button id="' +
    id +
    '-next"' +
    (page >= pages ? " disabled" : "") +
    ">Sau →</button>" +
    "</div></div>"
  );
}

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

function projectNsLink(resource, ns, label) {
  if (!ns || ns === "—") return "";
  return (
    '<a href="#/' + esc(resource) + '" class="btn-ghost project-ns-link" data-resource="' + esc(resource) + '" data-ns="' + esc(ns) + '">' +
    esc(label) +
    "</a> "
  );
}

function defaultHomeRoute() {
  const u = state.user;
  if (!u) return "overview";
  if (u.role === "dev" || u.role === "readonly") return "my-projects";
  return "overview";
}

function canWriteK8s() {
  const r = state.user && state.user.role;
  return r === "admin" || r === "tech_lead" || r === "dev";
}

function deployIsTerminal(item) {
  if (!item) return false;
  const s = (item.status || "").toLowerCase();
  return s === "success" || s === "failed";
}

function deployIsLive(item) {
  return !!(item && item.live && !deployIsTerminal(item));
}

function deployStageIcon(status) {
  if (status === "success") return '<span class="pipe-icon ok">✓</span>';
  if (status === "failed") return '<span class="pipe-icon bad">✕</span>';
  if (status === "skipped") return '<span class="pipe-icon skip">—</span>';
  if (status === "running") return '<span class="pipe-icon run">◌</span>';
  return '<span class="pipe-icon wait">○</span>';
}

function renderBuildSteps(steps) {
  if (!steps || !steps.length) return "";
  const visible = filterDisplayBuildSteps(steps);
  if (!visible.length) return "";
  return (
    '<div class="build-steps">' +
    visible
      .map(function (s) {
        const icon =
          s.status === "success"
            ? "✓"
            : s.status === "failed"
              ? "✕"
              : s.status === "skipped"
                ? "—"
                : s.status === "running"
                  ? "◌"
                  : "○";
        const label =
          s.status === "skipped"
            ? '<span class="muted"> (bỏ qua)</span>'
            : "";
        const mainCls = isMainBuildStep(s.name) ? " build-step-main" : "";
        return (
          '<div class="build-step build-step-' + esc(s.status || "pending") + mainCls + '">' +
          '<span class="build-step-icon">' + icon + "</span>" +
          "<span>" + esc(s.name) + label + "</span></div>"
        );
      })
      .join("") +
    "</div>"
  );
}

function isMainBuildStep(name) {
  const n = (name || "").toLowerCase();
  return (
    n.indexOf("build and push") >= 0 ||
    n.indexOf("buildpack") >= 0 ||
    n.indexOf("docker/build-push") >= 0
  );
}

function filterDisplayBuildSteps(steps) {
  return steps.filter(function (s) {
    const n = (s.name || "").toLowerCase().trim();
    if (n === "set up job" || n === "complete job") return false;
    if (n.indexOf("post ") === 0) return false;
    return true;
  });
}

function buildStepsAllSuccess(steps) {
  const visible = filterDisplayBuildSteps(steps || []);
  if (!visible.length) return false;
  return visible.every(function (s) {
    const st = (s.status || "").toLowerCase();
    return st === "success" || st === "skipped";
  });
}

function buildStatusEffective(item) {
  if (!item) return "pending";
  const bs = (item.build_status || "").toLowerCase();
  if (bs === "success" || bs === "failed") return bs;
  if (buildStepsAllSuccess(item.build_steps)) return "success";
  return bs || "pending";
}

function runtimeLogPlaceholder(item) {
  if (!item) return "▶ Đang chờ pod trên cluster...\n";
  if (item.error_message) {
    return "✕ " + item.error_message + "\n";
  }
  const signals = item.runtime_signals || [];
  const k8s = signals.find(function (s) {
    return s.id === "k8s";
  });
  const ev = signals.find(function (s) {
    return s.id === "events";
  });
  if (item.runtime_status === "failed" || item.status === "failed") {
    if (k8s && k8s.detail) return "✕ " + k8s.detail + "\n";
    if (ev && ev.detail) return "✕ " + ev.detail + "\n";
    if (item.runtime_detail) return "✕ " + item.runtime_detail + "\n";
    return "✕ Runtime thất bại — xem tầng kiểm tra bên trên.\n";
  }
  if (item.pod_name) return "▶ Pod " + item.pod_name + " đang khởi động...\n";
  if (item.runtime_detail) return "▶ " + item.runtime_detail + "\n";
  return "▶ Đang chờ pod trên cluster...\n";
}

function buildLogPlaceholder(item) {
  const eff = buildStatusEffective(item);
  if (eff === "success") {
    return "✓ Build GitHub hoàn tất. Mở link bên dưới để xem log đầy đủ.\n";
  }
  if (deployIsLive(item)) return "▶ Đang chờ log từ GitHub Actions...\n";
  if (item.github_run_url) {
    return "Log build không tải được từ GitHub. Mở link bên dưới để xem đầy đủ.\n";
  }
  return "Chưa có log.";
}

function buildpackBuildActive(item) {
  return (item.build_steps || []).some(function (s) {
    const n = (s.name || "").toLowerCase();
    return (
      (n.indexOf("buildpack") >= 0 || n.indexOf("pack") >= 0) &&
      (s.status === "running" || s.status === "pending")
    );
  });
}

function extractBuildErrors(log) {
  if (!log) return "";
  const hits = log
    .split("\n")
    .filter(function (line) {
      return (
        line.indexOf("##[error]") >= 0 ||
        /(^|\s)(error|failed|failure):/i.test(line) ||
        line.indexOf("Error:") >= 0
      );
    });
  return hits.slice(-40).join("\n");
}

function harborSevClass(sev) {
  const s = (sev || "").toLowerCase();
  if (s === "critical") return "harbor-sev-critical";
  if (s === "high") return "harbor-sev-high";
  if (s === "medium") return "harbor-sev-medium";
  if (s === "low") return "harbor-sev-low";
  return "harbor-sev-unknown";
}

function harborSevLabel(sev) {
  const s = (sev || "").toLowerCase();
  if (s === "critical") return "Nghiêm trọng";
  if (s === "high") return "Cao";
  if (s === "medium") return "Trung bình";
  if (s === "low") return "Thấp";
  return sev || "Khác";
}

function harborPackageHint(items) {
  if (!items || !items.length) return "thư viện hệ thống (base image)";
  let goN = 0;
  let alpineN = 0;
  let otherN = 0;
  items.forEach(function (v) {
    const pkg = (v.package || "").toLowerCase();
    if (pkg === "stdlib" || pkg.indexOf("go") >= 0) goN++;
    else if (/^lib|musl|apk|alpine|busybox/.test(pkg)) alpineN++;
    else otherN++;
  });
  const parts = [];
  if (goN > 0) parts.push("Go");
  if (alpineN > 0) parts.push("Alpine/OS");
  if (otherN > 0 && parts.length === 0) parts.push("thư viện hệ thống");
  else if (otherN > 0) parts.push("package khác");
  return parts.join(", ") || "thư viện hệ thống";
}

function harborPlainVerdict(s, env, items) {
  if (!s) return "";
  const sev = s.severity || {};
  const critical = sev.Critical || 0;
  const high = sev.High || 0;
  const medium = sev.Medium || 0;
  const low = sev.Low || 0;
  const isProd = (env || "dev") === "prod";
  if (s.status === "running" || s.status === "pending") {
    return "Đang quét lỗ hổng bảo mật trên image…";
  }
  if (s.status === "failed") {
    return "Quét bảo mật lỗi — thử push lại hoặc kiểm tra Harbor Trivy.";
  }
  if (s.total === 0) {
    return "Image sạch — Trivy không phát hiện lỗ hổng trên bản build này.";
  }
  const src = harborPackageHint(items);
  let msg =
    "Trivy phát hiện " +
    s.total +
    " lỗ hổng từ " +
    src +
    " — thường do base image/runtime, không phải lỗi logic app.";
  if (critical + high > 0) {
    msg += " Có " + critical + " nghiêm trọng, " + high + " cao";
    if (medium + low > 0) msg += ", " + (medium + low) + " trung bình/thấp";
    msg += ".";
  } else {
    msg += " Toàn bộ ở mức trung bình/thấp (" + (medium + low) + ").";
  }
  if (critical > 0) {
    msg += isProd
      ? " Prod có CVE nghiêm trọng — nên cập nhật base image / Go trước khi coi bản này ổn định."
      : " Dev vẫn test được, nhưng nên lên kế hoạch vá base image / Go sớm.";
  } else if (high > 0) {
    msg += isProd
      ? " Prod: có thể chạy tạm, nên lên lịch cập nhật base image khi rảnh."
      : " Dev/test deploy bình thường — vá dần khi có thời gian.";
  } else {
    msg += isProd
      ? " Mức thấp/trung bình — thường không ảnh hưởng vận hành ngắn hạn."
      : " Dev/test — có thể bỏ qua tạm, không cần chặn deploy.";
  }
  return msg;
}

function summarizeClusterLog(log) {
  if (!log || !log.trim()) return ["Chưa có sự kiện cluster."];
  const bullets = [];
  if (/ImagePullBackOff|ErrImagePull/i.test(log)) {
    bullets.push("Không tải được image — kiểm tra Harbor login hoặc tag image.");
  }
  if (/CrashLoopBackOff/i.test(log)) {
    bullets.push("Pod bị crash liên tục — xem log ứng dụng bên dưới.");
  }
  if (/Started: Container started|Pulled:.*already present|Running/i.test(log)) {
    bullets.push("Container app đã khởi động trên cluster.");
  }
  if (/CertificateIssued|certificate has been successfully issued|Order completed successfully/i.test(log)) {
    bullets.push("Chứng chỉ HTTPS (Let's Encrypt) đã sẵn sàng.");
  }
  if (/Scaled up replica set/i.test(log)) {
    bullets.push("Kubernetes đã tạo pod mới cho bản deploy này.");
  }
  if (!bullets.length) {
    bullets.push("Cluster đang xử lý: tạo pod, cấu hình ingress, SSL…");
  }
  return bullets;
}

function runtimeSignalIcon(st) {
  if (st === "success") return "✓";
  if (st === "failed") return "✕";
  if (st === "running") return "…";
  if (st === "info") return "ℹ";
  if (st === "skipped") return "—";
  return "○";
}

function renderRuntimeSignalExpand(sig, itemId) {
  const items = (sig && sig.items) || [];
  if (!items.length) return "";
  const logId = "runtime-events-" + (itemId || "x") + "-" + esc(sig.id || "ev");
  const body = items.map(function (line) {
    return esc(line);
  }).join("\n");
  return (
    '<details class="runtime-signal-expand">' +
    '<summary class="runtime-signal-expand-summary">' +
    esc(items.length + " events · xem chi tiết") +
    '</summary><pre class="runtime-signal-events-log" id="' +
    logId +
    '">' +
    body +
    "</pre></details>"
  );
}

function renderRuntimeSignalLegend() {
  const rows = [
    { icon: "✓", cls: "success", label: "OK" },
    { icon: "✕", cls: "failed", label: "Lỗi" },
    { icon: "…", cls: "running", label: "Đang chạy / chờ" },
    { icon: "ℹ", cls: "info", label: "Thông tin — gợi ý từ log, không quyết định pass/fail" },
    { icon: "—", cls: "skipped", label: "Bỏ qua" },
    { icon: "○", cls: "pending", label: "Chưa có dữ liệu" },
  ];
  return (
    '<div class="runtime-signals-legend" role="note" aria-label="Chú giải icon kiểm tra runtime">' +
    '<p class="runtime-signals-legend-heading">Chú giải icon</p>' +
    '<table class="runtime-signals-legend-table"><tbody>' +
    rows
      .map(function (r) {
        return (
          "<tr><td><span class=\"runtime-signal runtime-signal-" +
          esc(r.cls) +
          '"><span class="runtime-signal-icon">' +
          esc(r.icon) +
          "</span></span></td><td>" +
          esc(r.label) +
          "</td></tr>"
        );
      })
      .join("") +
    "</tbody></table></div>"
  );
}

function renderRuntimeSignalsPanel(item) {
  const signals = (item && item.runtime_signals) || [];
  if (!signals.length) return "";
  const itemId = item && item.id != null ? String(item.id) : "";
  const rows = signals
    .map(function (sig) {
      const cls = "runtime-signal runtime-signal-" + esc(sig.status || "pending");
      const expand =
        sig.id === "events" && sig.items && sig.items.length
          ? renderRuntimeSignalExpand(sig, itemId)
          : "";
      return (
        '<li class="' +
        cls +
        '"><span class="runtime-signal-icon" aria-hidden="true">' +
        esc(runtimeSignalIcon(sig.status)) +
        "</span><div><strong>" +
        esc(sig.label || sig.id || "Signal") +
        "</strong>" +
        (sig.detail ? '<div class="muted runtime-signal-detail">' + esc(sig.detail) + "</div>" : "") +
        expand +
        "</div></li>"
      );
    })
    .join("");
  return (
    '<div class="runtime-signals-wrap">' +
    '<p class="runtime-signals-title"><strong>Kiểm tra runtime</strong> <span class="muted">(' +
    signals.length +
    " tầng · Rancher)</span></p>" +
    renderRuntimeSignalLegend() +
    '<ul class="runtime-signals-list">' +
    rows +
    "</ul></div>"
  );
}

function logDetailsBlock(opts) {
  opts = opts || {};
  const open = opts.open ? " open" : "";
  const id = opts.id || "log-view";
  const text = opts.text || "";
  const summary = opts.summary || "Log chi tiết";
  const copyLabel = opts.copyLabel || "Copy log";
  return (
    '<details class="deploy-raw-details"' +
    open +
    ">" +
    '<summary class="deploy-log-summary-row">' +
    "<span>" +
    esc(summary) +
    '</span><button type="button" class="btn-ghost btn-sm btn-copy-log" data-copy-log="' +
    esc(id) +
    '" onclick="event.preventDefault();event.stopPropagation();">' +
    esc(copyLabel) +
    "</button></summary>" +
    '<pre class="build-log" id="' +
    esc(id) +
    '">' +
    esc(text) +
    "</pre></details>"
  );
}

function bindDeployLogCopyButtons(root) {
  (root || document).querySelectorAll("[data-copy-log]").forEach(function (btn) {
    btn.onclick = function (ev) {
      if (ev) {
        ev.preventDefault();
        ev.stopPropagation();
      }
      const id = btn.getAttribute("data-copy-log");
      const el = id ? document.getElementById(id) : null;
      copyText(el ? el.textContent : "", "Đã copy log");
    };
  });
}

function summarizeRuntimeLog(item) {
  const signals = (item && item.runtime_signals) || [];
  const k8s = signals.find(function (s) {
    return s.id === "k8s";
  });
  const smoke = signals.find(function (s) {
    return s.id === "smoke";
  });
  const logSig = signals.find(function (s) {
    return s.id === "log";
  });
  const events = signals.find(function (s) {
    return s.id === "events";
  });
  if (item && (item.runtime_status === "failed" || item.status === "failed")) {
    if (item.error_message) return item.error_message;
    if (k8s && k8s.detail) return k8s.detail;
    if (events && events.detail) return events.detail;
    if (smoke && smoke.status === "failed" && smoke.detail) return "HTTPS: " + smoke.detail;
    if (item.runtime_detail) return item.runtime_detail;
    return "Runtime thất bại — xem các tầng kiểm tra bên dưới.";
  }
  if (item && item.runtime_status === "success") {
    let msg = "Deploy runtime OK";
    if (k8s && k8s.detail) msg += " · " + k8s.detail;
    if (smoke && smoke.status === "success" && smoke.detail) msg += " · " + smoke.detail;
    return msg;
  }
  if (k8s && k8s.status === "running") {
    return k8s.detail || "Đang chờ pod Ready (readiness probe /health)…";
  }
  if (logSig && logSig.status === "info" && logSig.detail) {
    return "Log gợi ý: " + logSig.detail;
  }
  const log = (item && item.runtime_log) || "";
  const detail = (item && item.runtime_detail) || "";
  if (/server listening/i.test(log)) {
    const m = log.match(/server listening[^\n]*/i);
    if (m) return "App đã lên: " + m[0].trim();
  }
  if (item && item.runtime_status === "running") {
    return detail || "Pod đang khởi động…";
  }
  return detail || "Đang chờ pod trên cluster.";
}

function deployStagePlainLine(stage) {
  if (!stage) return "";
  const st = stage.status || "pending";
  const skip = st === "skipped" ? " (bỏ qua)" : "";
  if (stage.id === "build") {
    if (st === "success") return "Build GitHub: xong — image đã được tạo.";
    if (st === "failed") return "Build GitHub: lỗi — xem log bên dưới.";
    if (st === "skipped") return "Build GitHub: bỏ qua.";
    if (st === "running") return "Build GitHub: đang chạy trên GitHub Actions…";
    return "Build GitHub: chờ chạy.";
  }
  if (stage.id === "registry") {
    if (st === "success") return "Harbor: image đã push và quét bảo mật xong.";
    if (st === "failed") return "Harbor: lỗi push hoặc quét image.";
    if (st === "skipped") return "Harbor: bỏ qua" + skip + ".";
    if (st === "running") return "Harbor: đang push / quét CVE…";
    return "Harbor: chờ push image.";
  }
  if (stage.id === "deploy") {
    if (st === "success") return "Deploy cluster: manifest đã apply lên Kubernetes.";
    if (st === "failed") return "Deploy cluster: lỗi — không apply được lên K8s.";
    if (st === "skipped") return "Deploy cluster: bỏ qua" + skip + ".";
    if (st === "running") return "Deploy cluster: đang apply lên namespace…";
    return "Deploy cluster: chờ.";
  }
  if (stage.id === "runtime") {
    if (st === "success") return "Pod runtime: container đang chạy, site phục vụ được.";
    if (st === "failed") return "Pod runtime: lỗi — pod crash hoặc không ready.";
    if (st === "skipped") return "Pod runtime: bỏ qua" + skip + ".";
    if (st === "running") return "Pod runtime: đang chờ container ready…";
    return "Pod runtime: chờ.";
  }
  return (stage.label || stage.id) + ": " + st;
}

function deployProfileLabel(item) {
  if (!item) return "";
  if (item.deploy_profile) return item.deploy_profile;
  if (!item.deploy_layout) return "";
  var names = (item.deploy_services || []).map(function (s) { return s.name; }).filter(Boolean);
  if (item.deploy_layout === "multi") return "Web + API · " + (names.join("+") || "api+web");
  return "Một website · " + (names[0] || "app");
}

function rollbackLayoutAllowed(item, clusterProfile) {
  if (!item || !item.deploy_layout) return true;
  var target = item.deploy_layout;
  var current = (clusterProfile && clusterProfile.layout) || "";
  if (!current) return true;
  return target === current;
}

function renderDeployProfileBadge(item) {
  var label = deployProfileLabel(item);
  if (!label) return "";
  return (
    '<span class="badge neutral deploy-profile-badge" title="Profile khi deploy">' + esc(label) + "</span>"
  );
}

function renderDeployProfileContext(activity) {
  if (!activity) return "";
  var cp = activity.console_profile;
  var kp = activity.cluster_profile;
  if (!cp && !kp) return "";
  var parts = [];
  if (cp && cp.profile_label) {
    parts.push(
      "<span><strong>Console</strong>: " +
        esc(cp.profile_label) +
        (cp.branch ? ' · branch <code class="inline-code">' + esc(cp.branch) + "</code>" : "") +
        "</span>"
    );
  }
  if (kp && kp.profile_label) {
    parts.push(
      "<span><strong>Cluster</strong>: " +
        esc(kp.profile_label) +
        (kp.image_tag ? ' · tag <code class="inline-code">' + esc(String(kp.image_tag).slice(0, 7)) + "</code>" : "") +
        "</span>"
    );
  }
  if (!parts.length) return "";
  var mismatch =
    cp && kp && cp.profile_label && kp.profile_label && cp.profile_label !== kp.profile_label;
  return (
    '<div class="deploy-profile-context' +
    (mismatch ? " warn" : "") +
    '">' +
    parts.join('<span class="deploy-profile-sep"> · </span>') +
    (mismatch ? ' <em class="deploy-profile-mismatch">Console ≠ cluster</em>' : "") +
    "</div>"
  );
}

function renderDeployHumanSummary(item) {
  if (!item) return "";
  const env = (item.environment || "dev") === "prod" ? "Production" : "Dev";
  const st = item.status || "in_progress";
  const failed = st === "failed" || item.build_status === "failed";
  const staged = st === "success" && !failed && item.serving === false;
  let headline = "";
  let headlineCls = "deploy-summary-neutral";
  if (staged) {
    headline = "Đã deploy manifest — chưa live trên cluster";
    headlineCls = "deploy-summary-warn";
  } else if (st === "success" && !failed) {
    headline = "Deploy " + env + " thành công";
    headlineCls = "deploy-summary-ok";
  } else if (st === "failed" || failed) {
    headline = "Deploy " + env + " thất bại";
    headlineCls = "deploy-summary-bad";
  } else {
    headline = "Đang deploy lên " + env + "…";
    headlineCls = "deploy-summary-run";
  }
  const bullets = (item.stages || []).map(deployStagePlainLine).filter(Boolean);
  if (item.harbor_scan) {
    bullets.push(harborPlainVerdict(item.harbor_scan, item.environment, item.harbor_scan.items));
  }
  if (item.image) {
    bullets.push("Image: " + item.image);
  }
  if (item.smoke_status === "success") {
    bullets.push("Smoke check: " + (item.smoke_detail || "OK"));
  } else if (item.smoke_status === "failed") {
    bullets.push("Smoke check thất bại: " + (item.smoke_detail || "không phản hồi"));
  } else if (item.smoke_status === "skipped") {
    bullets.push("Smoke check: bỏ qua (chưa có domain HTTPS)");
  }
  const bulletsHtml = bullets
    .map(function (b) {
      return "<li>" + esc(b) + "</li>";
    })
    .join("");
  return (
    '<div class="deploy-plain-summary ' + headlineCls + '">' +
    "<h4>" + esc(headline) + "</h4>" +
    (bulletsHtml ? '<ul class="deploy-plain-list">' + bulletsHtml + "</ul>" : "") +
    (item.error_message
      ? '<p class="error-text deploy-plain-error"><strong>Lỗi:</strong> ' + esc(item.error_message) + "</p>"
      : "") +
    "</div>"
  );
}

function setupSyncFailedStep(errMsg) {
  const m = (errMsg || "").toLowerCase();
  if (m.includes("chưa kết nối github") || m.includes("owner") || m.includes("repo bắt buộc")) return 0;
  if (m.includes("workflow") || m.includes("push workflow")) return 1;
  if (m.includes("secret") || m.includes("harbor") || m.includes("robot") || m.includes("ghcr")) return 2;
  return 1;
}

function setupSyncErrorHint(errMsg) {
  const m = (errMsg || "").toLowerCase();
  if (m.includes("chưa kết nối github")) return "Bấm「Kết nối GitHub」trước khi chọn repo.";
  if (m.includes("workflow")) return "Cần quyền ghi repo trên GitHub (OAuth scope workflow).";
  if (m.includes("harbor") || m.includes("robot")) return "Harbor có thể đang lỗi — kiểm tra registry trên VPS rồi thử lại.";
  if (m.includes("secret")) return "Tài khoản GitHub cần quyền admin/maintain repo để tạo Actions secrets.";
  if (m.includes("thời gian chờ") || m.includes("timeout")) return "Mạng hoặc GitHub chậm — thử lại sau vài giây.";
  if (m.includes("403") || m.includes("forbidden")) return "Không đủ quyền trên repo GitHub — kiểm tra quyền collaborator.";
  return "Xem chi tiết lỗi phía trên, sửa rồi bấm đồng bộ lại.";
}

function renderSetupSyncError(progress, errMsg, steps) {
  if (!progress) return;
  const failedIdx = setupSyncFailedStep(errMsg);
  const stepsHtml = steps
    .map(function (s, i) {
      let cls = "setup-step-pending";
      if (i < failedIdx) cls = "setup-step-done";
      else if (i === failedIdx) cls = "setup-step-fail";
      return '<div class="setup-step ' + cls + '">' + esc(s) + "</div>";
    })
    .join("");
  progress.hidden = false;
  progress.innerHTML =
    '<div class="setup-progress-title setup-progress-fail">✕ Đồng bộ GitHub thất bại</div>' +
    '<p class="setup-progress-error">' + esc(errMsg || "Lỗi không xác định") + "</p>" +
    '<p class="muted setup-progress-hint">' + esc(setupSyncErrorHint(errMsg)) + "</p>" +
    stepsHtml +
    '<p class="setup-retry-note">Sửa xong bấm <strong>Kết nối repo & bật auto-deploy</strong> để thử lại.</p>';
}

function renderHarborScanPanel(item) {
  if (!item || !item.harbor_scan) return "";
  const s = item.harbor_scan;
  const sev = s.severity || {};
  const badge =
    s.status === "success"
      ? s.total > 0
        ? '<span class="badge warn" style="margin-left:8px">CẦN XEM</span>'
        : '<span class="badge ok" style="margin-left:8px">SẠCH</span>'
      : s.status === "running" || s.status === "pending"
        ? '<span class="badge warn" style="margin-left:8px">ĐANG QUÉT</span>'
        : s.status === "failed"
          ? '<span class="badge bad" style="margin-left:8px">LỖI</span>'
          : "";
  const chips = ["Critical", "High", "Medium", "Low"]
    .filter(function (k) {
      return (sev[k] || 0) > 0;
    })
    .map(function (k) {
      return (
        '<span class="harbor-sev-chip ' +
        harborSevClass(k) +
        '">' +
        esc(harborSevLabel(k)) +
        ": " +
        sev[k] +
        "</span>"
      );
    })
    .join("");
  const items = s.items || [];
  const itemsTotal = s.items_total || s.total || items.length;
  const tableHtml =
    items.length > 0
      ? '<table class="harbor-vuln-table">' +
        "<thead><tr><th>Mã lỗi</th><th>Nguy hiểm</th><th>Thư viện</th><th>Đang dùng</th><th>Bản vá</th></tr></thead><tbody>" +
        items
          .map(function (v) {
            return (
              "<tr>" +
              "<td><code>" + esc(v.id || "—") + "</code></td>" +
              '<td><span class="harbor-sev ' + harborSevClass(v.severity) + '">' + esc(harborSevLabel(v.severity)) + "</span></td>" +
              "<td>" + esc(v.package || "—") + "</td>" +
              "<td><code>" + esc(v.version || "—") + "</code></td>" +
              "<td>" + esc(v.fix_version || "Chưa có") + "</td>" +
              "</tr>"
            );
          })
          .join("") +
        "</tbody></table>"
      : "";
  const detailsBlock =
    tableHtml
      ? '<details class="deploy-raw-details harbor-vuln-details">' +
        '<summary>Danh sách chi tiết (' +
        itemsTotal +
        " lỗ hổng) — chỉ mở khi cần xem từng CVE</summary>" +
        '<div class="harbor-vuln-wrap">' +
        tableHtml +
        "</div></details>"
      : s.status === "success" && s.total > 0
        ? '<p class="muted deploy-panel-lead">Đang tải danh sách chi tiết…</p>'
        : "";
  return (
    '<div class="build-live-panel harbor-scan-panel" id="deploy-harbor-scan">' +
    '<div class="build-live-head"><strong>Bảo mật image (Trivy)</strong>' +
    badge +
    "</div>" +
    '<div class="deploy-panel-body">' +
    '<p class="deploy-panel-lead">' + esc(harborPlainVerdict(s, item.environment, items)) + "</p>" +
    (chips ? '<div class="harbor-sev-chips">' + chips + "</div>" : "") +
    detailsBlock +
    "</div></div>"
  );
}

function renderDeployClusterLogPanel(item) {
  if (!item || !item.deploy_log || !item.deploy_log.trim()) return "";
  const bullets = summarizeClusterLog(item.deploy_log);
  const bulletsHtml = bullets
    .map(function (b) {
      return "<li>" + esc(b) + "</li>";
    })
    .join("");
  const openRaw = item.status === "failed" || item.deploy_status === "failed";
  return (
    '<div class="build-live-panel deploy-cluster-panel" id="deploy-cluster-live">' +
    '<div class="build-live-head"><strong>Deploy lên cluster</strong></div>' +
    '<div class="deploy-panel-body">' +
    '<p class="deploy-panel-lead">Kubernetes đã làm những việc sau:</p>' +
    '<ul class="deploy-plain-list deploy-inline-list">' + bulletsHtml + "</ul>" +
    logDetailsBlock({
      open: openRaw,
      id: "deploy-cluster-log-view",
      text: item.deploy_log,
      summary: "Log kỹ thuật K8s (tuỳ chọn)",
      copyLabel: "Copy log K8s",
    }) +
    "</div></div>"
  );
}

function renderRuntimeLogPanel(item) {
  if (!item) return "";
  const hasLog = !!(item.runtime_log && item.runtime_log.trim());
  const failed = item.runtime_status === "failed" || item.status === "failed";
  const waiting =
    !hasLog &&
    !failed &&
    (item.runtime_status === "running" || item.runtime_status === "pending") &&
    (item.deploy_status === "success" || buildStatusEffective(item) === "success");
  if (!hasLog && !waiting && !item.pod_name && !failed) return "";

  const liveBadge =
    item.runtime_status === "running" && !deployIsTerminal(item)
      ? '<span class="badge warn" style="margin-left:8px">LIVE</span>'
      : failed
        ? '<span class="badge bad" style="margin-left:8px">FAILED</span>'
        : "";
  const logText = item.runtime_log || runtimeLogPlaceholder(item);

  const runtimeErr =
    failed && item.error_message
      ? '<div class="build-log-errors"><strong>Lỗi runtime:</strong><pre>' + esc(item.error_message) + "</pre></div>"
      : "";
  const truncNote = item.runtime_log_truncated
    ? '<p class="muted build-log-note">Pod log dài — chỉ hiển thị một phần (tối đa ~2000 dòng).</p>'
    : "";

  const runtimeSummary = summarizeRuntimeLog(item);
  const openRaw = item.runtime_status === "failed" || item.status === "failed" || deployIsLive(item);

  return (
    '<div class="build-live-panel runtime-live-panel" id="deploy-runtime-live">' +
    '<div class="build-live-head"><strong>App đang chạy</strong>' +
    liveBadge +
    (item.pod_name
      ? '<span class="muted" style="margin-left:8px;font-size:11px">Pod <code>' + esc(item.pod_name) + "</code></span>"
      : "") +
    "</div>" +
    '<div class="deploy-panel-body">' +
    runtimeErr +
    renderRuntimeSignalsPanel(item) +
    '<p class="deploy-panel-lead">' + esc(runtimeSummary) + "</p>" +
    truncNote +
    logDetailsBlock({
      open: openRaw,
      id: "runtime-log-view",
      text: logText,
      summary: "Log ứng dụng chi tiết",
      copyLabel: "Copy log app",
    }) +
    "</div></div>"
  );
}

function renderPromoteSkipBuildPanel(item) {
  if (!item || item.github_run_id) return "";
  if (item.build_status !== "success") return "";
  return (
    '<div class="build-live-panel build-skip-panel" id="deploy-build-live">' +
    '<div class="build-live-head"><strong>Build GitHub</strong>' +
    '<span class="badge ok" style="margin-left:8px">BỎ QUA</span>' +
    '<span class="muted" style="margin-left:auto;font-size:11px">Promote / deploy image có sẵn — không chạy Actions</span></div>' +
    '<p class="muted" style="margin:10px 0 0">Image <code>' +
    esc((item.image_tag || "").slice(0, 12)) +
    "</code> đã build ở Dev. Xem log cluster và pod bên dưới.</p></div>"
  );
}

function renderBuildLogPanel(item) {
  if (!item) return "";
  const promoteSkip = renderPromoteSkipBuildPanel(item);
  if (promoteSkip) return promoteSkip;
  const hasLog = !!(item.build_log && item.build_log.trim());
  const hasSteps = !!(item.build_steps && item.build_steps.length);
  if (!hasLog && !hasSteps && !deployIsLive(item) && !item.github_run_id) return "";

  const liveBadge = deployIsLive(item)
    ? '<span class="badge warn" style="margin-left:8px">LIVE</span>'
    : "";
  const logText = item.build_log || buildLogPlaceholder(item);
  const errBlock =
    item.status === "failed" || item.build_status === "failed" || item.runtime_status === "failed"
      ? extractBuildErrors(logText)
      : "";
  const errHtml = errBlock
    ? '<div class="build-log-errors"><strong>Lỗi:</strong><pre>' + esc(errBlock) + "</pre></div>"
    : "";

  const truncNote = item.build_log_truncated
    ? '<p class="muted build-log-note">Log build rất dài — hiển thị đầu+cuối. <a class="pipe-link" href="' +
      esc(item.github_run_url || "#") +
      '" target="_blank" rel="noopener">Xem đủ trên GitHub Actions</a></p>'
    : "";

  const buildEff = buildStatusEffective(item);
  const buildLead =
    buildEff === "success"
      ? (item.build_steps || []).some(function (s) {
          return isMainBuildStep(s.name);
        }) &&
        (item.build_steps || []).some(function (s) {
          return (s.name || "").toLowerCase().indexOf("buildpack") >= 0;
        })
        ? "Buildpack đã build và push image xong (pack build)."
        : "GitHub Actions đã build và push image xong."
      : buildEff === "failed"
        ? "Build GitHub thất bại — xem lỗi bên dưới."
        : deployIsLive(item) && buildpackBuildActive(item)
          ? "Buildpack đang build image (pack build) — thường mất 1–3 phút…"
          : deployIsLive(item)
            ? "GitHub Actions đang build image…"
            : buildStepsAllSuccess(item.build_steps)
              ? "GitHub Actions đã build và push image xong."
              : "Chờ GitHub Actions chạy workflow.";
  const openRaw = buildEff === "failed" || deployIsLive(item);

  return (
    '<div class="build-live-panel" id="deploy-build-live">' +
    '<div class="build-live-head"><strong>Build trên GitHub</strong>' +
    liveBadge +
    "</div>" +
    '<div class="deploy-panel-body">' +
    errHtml +
    '<p class="deploy-panel-lead">' + esc(buildLead) + "</p>" +
    renderBuildSteps(item.build_steps) +
    truncNote +
    logDetailsBlock({
      open: openRaw,
      id: "build-log-view",
      text: logText,
      summary: "Log kỹ thuật GitHub Actions (tuỳ chọn)",
      copyLabel: "Copy log build",
    }) +
    (item.github_run_url
      ? '<div class="build-log-foot muted"><a class="pipe-link" href="' +
        esc(item.github_run_url) +
        '" target="_blank" rel="noopener">Mở trên GitHub Actions</a></div>'
      : "") +
    "</div></div>"
  );
}

function renderDeployPipelineItem(item, withLog, actions) {
  actions = actions || {};
  const stages = item.stages || [];
  const stagesHtml = stages
    .map(function (s) {
      const showStageLink =
        s.url &&
        !(s.id === "build" && withLog && item.build_log) &&
        !(s.id === "registry" && item.harbor_scan);
      const stageLinkLabel =
        s.id === "registry" && (item.harbor_scan || /harbor/i.test(s.url || ""))
          ? "Xem trên Harbor →"
          : s.id === "build"
            ? "Xem log GitHub →"
            : "Xem chi tiết →";
      return (
        '<div class="pipe-stage pipe-' + esc(s.status || "pending") + '">' +
        deployStageIcon(s.status) +
        '<div class="pipe-body"><strong>' + esc(s.label) + "</strong>" +
        (s.detail ? '<div class="muted pipe-detail">' + esc(s.detail) + "</div>" : "") +
        (s.error ? '<div class="error-text pipe-detail">' + esc(s.error) + "</div>" : "") +
        (showStageLink
          ? '<a class="pipe-link" href="' + esc(s.url) + '" target="_blank" rel="noopener">' + stageLinkLabel + "</a>"
          : s.id === "build" && item.build_log
            ? '<div class="muted pipe-detail">Chi tiết build ở khung <strong>Build trên GitHub</strong> phía trên ↑</div>'
            : s.id === "registry" && item.harbor_scan
              ? '<div class="muted pipe-detail">Kết quả quét bảo mật ở khung phía trên ↑</div>'
            : s.id === "deploy" && item.deploy_log
              ? '<div class="muted pipe-detail">K8s events ở khung <strong>Cluster log</strong> phía trên ↑</div>'
            : s.id === "runtime" && (item.runtime_log || item.pod_name)
              ? '<div class="muted pipe-detail">Pod log ở khung <strong>ngay phía trên</strong> bước này ↑ (cuộn lên nếu không thấy)</div>'
              : "") +
        "</div></div>"
      );
    })
    .join("");
  const head =
    '<div class="pipe-head">' +
    '<code class="pipe-sha">' + esc((item.image_tag || "").slice(0, 7)) + "</code>" +
    '<span class="badge ' + (item.status === "success" ? "ok" : item.status === "failed" ? "bad" : "warn") + '">' +
    esc(item.status || "in_progress") +
    "</span>" +
    (item.serving
      ? '<span class="badge ok deploy-serving-badge">ĐANG PHỤC VỤ</span>'
      : item.status === "success" && !item.serving
        ? '<span class="badge neutral deploy-staged-badge">ĐÃ DEPLOY</span>'
        : "") +
    '<span class="muted">' + esc(item.environment || "dev") + " · " + esc(fmtTime(item.created_at)) + "</span>" +
    (item.git_branch
      ? '<span class="badge neutral deploy-branch-badge" title="Branch lúc deploy">' + esc(item.git_branch) + "</span>"
      : "") +
    renderDeployProfileBadge(item) +
    (actions.rollback && canWriteK8s()
      ? rollbackLayoutAllowed(item, actions.clusterProfile)
        ? '<button type="button" class="btn-ghost btn-sm pipe-rollback-btn" data-tag="' +
          esc(item.image_tag || "") +
          '" data-env="' +
          esc(item.environment || actions.env || "dev") +
          '" data-deploy-profile="' +
          esc(deployProfileLabel(item)) +
          '" data-deploy-layout="' +
          esc(item.deploy_layout || "") +
          '" data-git-branch="' +
          esc(item.git_branch || "") +
          '" title="Deploy lại image tag này (chỉ cùng kiểu chạy)">Deploy lại</button>'
        : '<span class="muted" style="font-size:11px" title="Bản này khác kiểu chạy với site hiện tại — dùng wizard Đổi kiểu chạy, không Deploy lại">Khác kiểu chạy</span>'
      : "") +
    "</div>";
  const summaryPanel = withLog ? renderDeployHumanSummary(item) : "";
  const logPanel =
    (withLog ? renderBuildLogPanel(item) : "") +
    renderHarborScanPanel(item) +
    (withLog ? renderDeployClusterLogPanel(item) + renderRuntimeLogPanel(item) : "");
  const stagesBlock =
    '<details class="pipe-stages-wrap"' +
    (withLog ? " open" : "") +
    '><summary><strong>4 bước deploy</strong></summary><div class="pipe-stages">' +
    stagesHtml +
    "</div></details>";
  return '<div class="pipe-item">' + head + summaryPanel + logPanel + stagesBlock + "</div>";
}

function renderPromotePrepItem(it, slug) {
  const isWarn = it.level === "warn";
  const icon = it.ok ? "✓" : isWarn ? "!" : "○";
  const cls = it.ok ? "promote-prep-ok" : isWarn ? "promote-prep-warn" : "promote-prep-miss";
  const prepEnv = it.id === "build_contract_dev" ? "dev" : it.tab === "env" ? "prod" : "";
  const tabLink =
    !it.ok && !isWarn && it.tab
      ? ' <a class="pipe-link promote-prep-link" href="#/project/' +
        esc(slug) +
        "/" +
        esc(it.tab) +
        '" data-promote-tab="' +
        esc(it.tab) +
        '" data-promote-env="' +
        esc(prepEnv) +
        '">Cấu hình →</a>'
      : "";
  return (
    '<li class="promote-prep-item ' + cls + '">' +
    '<span class="promote-prep-icon">' + icon + "</span>" +
    "<span><strong>" + esc(it.label) + "</strong>" +
    (it.detail ? '<span class="muted"> — ' + esc(it.detail) + "</span>" : "") +
    tabLink +
    "</span></li>"
  );
}

function renderDeployPromotePrep(readiness, slug) {
  if (!readiness) {
    return (
      '<div class="deploy-promote-prep" id="deploy-promote-prep">' +
      '<p class="muted">Đang kiểm tra checklist Promote…</p></div>'
    );
  }
  const items = readiness.items || [];
  if (!items.length) return "";
  const devItems = items.filter(function (it) { return it.group === "dev_image"; });
  const prodItems = items.filter(function (it) { return it.group === "prod" || !it.group; });
  const buildItem = items.find(function (it) { return it.id === "build_contract_dev"; });
  const runtimeItem = items.find(function (it) { return it.id === "runtime_contract"; });
  let detailPanels = "";
  if (buildItem && !buildItem.ok && readiness.build_readiness) {
    detailPanels +=
      '<div class="promote-prep-detail env-readiness-card">' +
      renderEnvReadinessPanel(readiness.build_readiness, slug, "dev", "build") +
      "</div>";
  }
  if (runtimeItem && !runtimeItem.ok && readiness.runtime_readiness) {
    detailPanels +=
      '<div class="promote-prep-detail env-readiness-card">' +
      renderEnvReadinessPanel(readiness.runtime_readiness, slug, "prod", "runtime") +
      "</div>";
  }
  return (
    '<div class="deploy-promote-prep" id="deploy-promote-prep">' +
    '<p class="promote-prep-title"><strong>Checklist Promote lên Prod</strong>' +
    (readiness.ready
      ? ' <span class="badge ok" style="margin-left:6px">Sẵn sàng</span>'
      : ' <span class="badge warn" style="margin-left:6px">Chưa đủ</span>') +
    "</p>" +
    '<p class="muted promote-prep-intro">Promote <strong>không build lại</strong> — đưa cùng image dev lên prod. Checklist đảm bảo image đã build đúng contract và prod sẵn sàng nhận traffic.</p>' +
    (devItems.length
      ? '<p class="promote-prep-group-title"><strong>① Image dev</strong> <span class="muted">(tag sẽ promote)</span></p><ul class="promote-prep-list">' +
        devItems.map(function (it) { return renderPromotePrepItem(it, slug); }).join("") +
        "</ul>"
      : "") +
    (prodItems.length
      ? '<p class="promote-prep-group-title"><strong>② Prod</strong> <span class="muted">(cluster + runtime)</span></p><ul class="promote-prep-list">' +
        prodItems.map(function (it) { return renderPromotePrepItem(it, slug); }).join("") +
        "</ul>"
      : "") +
    detailPanels +
    '<p class="muted promote-prep-note">Mục <strong>!</strong> là cảnh báo (không chặn promote). Mục <strong>○</strong> phải xử lý trước.</p>' +
    "</div>"
  );
}

function bindPromotePrepLinks(slug) {
  document.querySelectorAll(".promote-prep-link").forEach(function (a) {
    a.onclick = function () {
      const tab = a.dataset.promoteTab;
      const prepEnv = a.dataset.promoteEnv;
      if (tab === "env") {
        state.projectEnv = prepEnv || "prod";
        localStorage.setItem("project-env", prepEnv || "prod");
      }
    };
  });
}

function deployHistoryPageKey(slug, env) {
  return slug + ":" + (env || "dev");
}

const DEPLOY_HISTORY_PAGE_SIZE = 5;

function deployHistoryItems(activity) {
  const cur = activity && activity.current;
  return ((activity && activity.items) || []).filter(function (it) {
    if (cur && it.id === cur.id) return false;
    if (cur && it.image_tag && cur.image_tag && it.image_tag === cur.image_tag && it.environment === cur.environment) {
      return false;
    }
    return true;
  });
}

function renderDeployHistoryContent(activity, opts) {
  opts = opts || {};
  const envLabel = deployActivityEnv(activity, opts.expectedEnv).toUpperCase();
  const history = deployHistoryItems(activity);
  const key = deployHistoryPageKey(opts.slug, opts.expectedEnv);
  const pageSize = DEPLOY_HISTORY_PAGE_SIZE;
  const totalPages = Math.max(1, Math.ceil(history.length / pageSize) || 1);
  let page = state.deployHistoryPage[key] || 1;
  if (page > totalPages) page = totalPages;
  if (page < 1) page = 1;
  state.deployHistoryPage[key] = page;
  const start = (page - 1) * pageSize;
  const slice = history.slice(start, start + pageSize);
  const clusterProfile = activity.cluster_profile || null;
  const itemsHtml = history.length
    ? slice
        .map(function (it) {
          return renderDeployPipelineItem(it, false, {
            rollback: true,
            env: deployActivityEnv(activity, opts.expectedEnv),
            clusterProfile: clusterProfile,
          });
        })
        .join("")
    : '<p class="muted">Mỗi lần push GitHub sẽ thêm bản mới. Bản cũ hiện ở đây kèm nút <strong>Deploy lại</strong>.</p>';
  let pagerHtml = "";
  if (history.length > pageSize) {
    pagerHtml =
      '<div class="deploy-history-pager" id="deploy-history-pager" data-total="' +
      history.length +
      '" data-page-size="' +
      pageSize +
      '">' +
      '<button type="button" class="btn-ghost btn-sm" data-history-page="' +
      (page - 1) +
      '" ' +
      (page <= 1 ? "disabled" : "") +
      ">← Trước</button>" +
      '<span class="muted deploy-history-pager-meta">Trang ' +
      page +
      " / " +
      totalPages +
      " · " +
      history.length +
      " bản</span>" +
      '<button type="button" class="btn-ghost btn-sm" data-history-page="' +
      (page + 1) +
      '" ' +
      (page >= totalPages ? "disabled" : "") +
      ">Sau →</button>" +
      "</div>";
  }
  return { itemsHtml: itemsHtml, pagerHtml: pagerHtml, count: history.length, envLabel: envLabel };
}

function bindDeployHistoryPagination(slug, env) {
  const pager = document.getElementById("deploy-history-pager");
  if (!pager) return;
  const key = deployHistoryPageKey(slug, env);
  pager.querySelectorAll("[data-history-page]").forEach(function (btn) {
    btn.onclick = function () {
      const next = parseInt(btn.getAttribute("data-history-page"), 10);
      const total = parseInt(pager.dataset.total || "0", 10);
      const pageSize = parseInt(pager.dataset.pageSize || String(DEPLOY_HISTORY_PAGE_SIZE), 10);
      const totalPages = Math.max(1, Math.ceil(total / pageSize));
      if (next < 1 || next > totalPages || btn.disabled) return;
      state.deployHistoryPage[key] = next;
      refreshDeployHistoryList(slug, env);
    };
  });
}

function refreshDeployHistoryList(slug, env) {
  const key = deployHistoryPageKey(slug, env);
  const activity = state.deployActivityCache[key];
  if (!activity) return;
  const hist = renderDeployHistoryContent(activity, { slug: slug, expectedEnv: env });
  const listEl = document.getElementById("deploy-history-list");
  const summaryEl = document.querySelector(".deploy-history-wrap > summary");
  if (listEl) listEl.innerHTML = hist.itemsHtml;
  const bodyEl = listEl && listEl.parentElement;
  let pagerEl = document.getElementById("deploy-history-pager");
  if (bodyEl) {
    if (hist.pagerHtml) {
      if (pagerEl) pagerEl.outerHTML = hist.pagerHtml;
      else bodyEl.insertAdjacentHTML("beforeend", hist.pagerHtml);
    } else if (pagerEl) {
      pagerEl.remove();
    }
  }
  if (summaryEl) {
    summaryEl.innerHTML =
      "<strong>Lịch sử " +
      esc(hist.envLabel) +
      "</strong>" +
      (hist.count ? " (" + hist.count + " bản cũ)" : " — chưa có bản cũ");
  }
  bindDeployHistoryPagination(slug, env);
  bindDeployActivityActions(slug, env);
}

function renderDeployActivityCard(activity, opts) {
  opts = opts || {};
  const showHistory = opts.showHistory === true;
  const showPromotePrep = opts.showPromotePrep === true;
  const showPromoteBar = opts.showPromoteBar === true;
  if (activity && activity.loading) {
    return (
      '<div class="card" style="margin-bottom:16px" id="deploy-activity-card"><h3>Tiến trình deploy</h3>' +
      '<p class="loading">Đang tải tiến trình…</p></div>'
    );
  }
  if (!activity || (!activity.current && !(activity.items || []).length)) {
    const emptyEnv = deployActivityEnv(activity, opts.expectedEnv);
    const emptyEnvLabel = emptyEnv.toUpperCase();
    let extra = "";
    if (emptyEnv === "dev" && opts.slug && canManagePlatformProjects() && showPromotePrep) {
      extra += renderDeployPromotePrep(opts.promoteReadiness || null, opts.slug);
    }
    const emptyBody =
      emptyEnv === "prod"
        ? '<p class="muted">Chưa có deploy <strong>Prod</strong>. Dùng tab <a href="' +
          esc(projectRoute(opts.slug, "promote")) +
          '"><strong>Promote Prod</strong></a> sau khi dev ổn.</p>'
        : '<p class="muted">Chưa thấy deploy. <strong>Kết nối GitHub</strong> ở trên, rồi push code — tiến trình cập nhật tại đây.</p>' +
          (opts.slug
            ? ' <a class="pipe-link" href="' + esc(projectRoute(opts.slug, "deploy-history")) + '">Xem lịch sử deploy →</a>'
            : "");
    return (
      '<div class="card" style="margin-bottom:16px" id="deploy-activity-card" data-deploy-env="' +
      esc(emptyEnv) +
      '"><h3>Tiến trình deploy · ' +
      esc(emptyEnvLabel) +
      "</h3>" +
      extra +
      emptyBody +
      "</div>"
    );
  }
  const envLabel = deployActivityEnv(activity, opts.expectedEnv).toUpperCase();
  const profileCtx = renderDeployProfileContext(activity);
  const cur = activity.current;
  const servingTag = activity.serving_image_tag || state.deployServingTag || "";
  let body = "";
  if (servingTag && cur && cur.status === "success" && cur.image_tag && cur.image_tag !== servingTag) {
    body +=
      '<p class="muted deploy-serving-note"><span class="live-dot" style="background:#22c55e"></span> ' +
      "Cluster đang phục vụ <code>" + esc(servingTag.slice(0, 7)) + "</code> · " +
      "Bản mới <code>" + esc((cur.image_tag || "").slice(0, 7)) + "</code>" +
      (cur.serving ? " (đã live)" : " chưa thay traffic") +
      "</p>";
  }
  const promoteReady = opts.promoteReadiness || null;
  const promoteTag = promotableDevImageTag(activity, promoteReady);
  const showPromote =
    showPromoteBar &&
    opts.slug &&
    envLabel === "DEV" &&
    promoteTag &&
    canManagePlatformProjects();
  const canPromote = !promoteReady || promoteReady.ready;
  if (showPromotePrep) {
    body += renderDeployPromotePrep(promoteReady || { ready: false, items: [] }, opts.slug);
  }
  if (showPromote) {
    body +=
      '<div class="deploy-promote-bar" id="deploy-promote-bar">' +
      '<div class="deploy-promote-bar-inner">' +
      '<span class="muted">Bản dev <code>' +
      esc(promoteTag.slice(0, 7)) +
      "</code> đã chạy ổn — đưa lên prod <strong>không build lại</strong>.</span>" +
      '<button type="button" class="btn-primary btn-sm" id="deploy-promote-btn" data-tag="' +
      esc(promoteTag) +
      '"' +
      (canPromote
        ? ""
        : ' disabled title="Hoàn tất checklist Promote phía trên"') +
      ">Promote lên Prod →</button></div></div>";
  }
  if (cur) {
    const liveNote = deployIsLive(cur)
      ? '<p class="muted deploy-live-note" style="margin-bottom:8px"><span class="live-dot"></span> Đang build · ' + esc(envLabel) + " — cập nhật mỗi " + (activity.poll_interval_sec || 2) + "s</p>"
      : "";
    body +=
      '<details class="deploy-current-wrap" open id="deploy-current-details">' +
      '<summary class="deploy-current-summary"><strong>Deploy mới nhất</strong> <span class="badge neutral">' + esc(envLabel) + "</span></summary>" +
      liveNote +
      '<div id="deploy-pipeline-current">' + renderDeployPipelineItem(cur, true, { env: activity.environment }) + "</div></details>";
  }
  if (showHistory) {
    const hist = renderDeployHistoryContent(activity, { slug: opts.slug, expectedEnv: opts.expectedEnv });
    body +=
      '<details class="deploy-history-wrap" style="margin-top:14px" open>' +
      '<summary class="muted" style="cursor:pointer">' +
      "<strong>Lịch sử " +
      esc(hist.envLabel) +
      "</strong>" +
      (hist.count ? " (" + hist.count + " bản)" : "") +
      '</summary><div class="deploy-history-body">' +
      '<p class="muted deploy-history-note" style="margin:0 0 10px;font-size:11px">Mỗi commit = 1 tag. <strong>Deploy lại</strong> = đổi tag, <em>cùng kiểu chạy</em>. Đổi single ↔ multi → dùng <strong>Đổi kiểu chạy</strong>, không rollback.</p>' +
      '<div id="deploy-history-list">' +
      hist.itemsHtml +
      "</div>" +
      hist.pagerHtml +
      "</div></details>";
  } else if (opts.slug) {
    body +=
      '<p class="muted" style="margin-top:12px;font-size:12px">' +
      '<a class="pipe-link" href="' +
      esc(projectRoute(opts.slug, "deploy-history")) +
      '">Lịch sử deploy · ' +
      esc(envLabel) +
      " →</a>" +
      (cur && cur.status === "success" && envLabel === "DEV" && canManagePlatformProjects()
        ? ' · <a class="pipe-link" href="' + esc(projectRoute(opts.slug, "promote")) + '">Promote Prod →</a>'
        : "") +
      "</p>";
  }
  return (
    '<div class="card" style="margin-bottom:16px" id="deploy-activity-card" data-deploy-env="' +
    esc(deployActivityEnv(activity, opts.expectedEnv)) +
    '" data-deploy-live="' +
    (cur && deployIsLive(cur) ? "1" : "0") +
    '"><h3>Tiến trình deploy · ' +
    esc(envLabel) +
    "</h3>" +
    profileCtx +
    body +
    "</div>"
  );
}

function rememberPromoteReadiness(slug, readiness) {
  state.deployPromoteReadiness[slug] = readiness || null;
}

function promotableDevImageTag(activity, readiness) {
  if (readiness && readiness.latest_success_tag) {
    return readiness.latest_success_tag;
  }
  const cur = activity && activity.current;
  if (cur && cur.status === "success" && cur.image_tag) {
    return cur.image_tag;
  }
  if (activity && activity.serving_image_tag) {
    return activity.serving_image_tag;
  }
  return "";
}

function imageTagMatches(a, b) {
  a = String(a || "").trim();
  b = String(b || "").trim();
  if (!a || !b) return false;
  return a === b || a.startsWith(b) || b.startsWith(a);
}

function scrollToDeployProgress(force) {
  const pf = state.promoteFollow;
  if (!force && pf && pf.scrolled) return;
  requestAnimationFrame(function () {
    const card = document.getElementById("deploy-activity-card");
    const details = document.getElementById("deploy-current-details");
    if (details) details.open = true;
    if (card) card.scrollIntoView({ behavior: "smooth", block: "start" });
    if (pf) pf.scrolled = true;
  });
}

function promoteFollowActive(slug, env) {
  const pf = state.promoteFollow;
  return !!(pf && pf.slug === slug && (env || "dev").toLowerCase() === "prod");
}

function handlePromoteFollowTerminal(activity, slug) {
  const pf = state.promoteFollow;
  if (!pf || pf.slug !== slug) return;
  const cur = activity && activity.current;
  if (!cur || !imageTagMatches(cur.image_tag, pf.tag)) return;
  if (!deployIsTerminal(cur)) return;
  scrollToDeployProgress(true);
  if (cur.status === "failed") {
    const detail = cur.error_message || cur.runtime_detail || "Deploy prod thất bại";
    toastError("Promote prod thất bại · " + detail);
  } else if (cur.status === "success") {
    toastSuccess("Promote prod thành công · tag " + String(pf.tag).slice(0, 7));
  }
  state.promoteFollow = null;
}

function navigateAfterPromote(slug, imageTag) {
  state.projectEnv = "prod";
  localStorage.setItem("project-env", "prod");
  state.promoteFollow = { slug: slug, tag: String(imageTag || "").trim() };
  toastSuccess("Đã promote — chuyển sang Deploy / Git (Prod), theo dõi tiến trình bên dưới…");
  const target = "#/project/" + slug + "/deploy";
  if (location.hash === target) {
    navigate();
  } else {
    location.hash = target;
  }
}

function bindDeployActivityActions(slug, env, promoteReadiness) {
  if (promoteReadiness !== undefined) {
    rememberPromoteReadiness(slug, promoteReadiness);
  }
  bindPromotePrepLinks(slug);
  bindDeployHistoryPagination(slug, env);
  const main = document.getElementById("main");
  if (main && env === "dev") bindEnvSuggestButtons(main, slug, "dev");
  const promoteBtn = document.getElementById("deploy-promote-btn");
  if (promoteBtn) {
    promoteBtn.onclick = async function () {
      let readiness = state.deployPromoteReadiness[slug];
      if (env === "dev" && canManagePlatformProjects()) {
        try {
          readiness = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote-readiness");
          rememberPromoteReadiness(slug, readiness);
        } catch (_e) {
          readiness = readiness || null;
        }
      }
      if (readiness && !readiness.ready) {
        toastError("Hoàn tất checklist Promote (xem mục ○ phía trên)");
        return;
      }
      const tag = promoteBtn.dataset.tag;
      if (!tag) return;
      const ok = await uiConfirm({
        title: "Promote lên Prod",
        message: "Deploy image " + tag.slice(0, 7) + " từ DEV lên PROD?",
        details: [
          "Cùng image tag — không build lại trên GitHub",
          "Dùng env vars và domain prod đã cấu hình",
        ],
        confirmText: "Promote lên Prod",
      });
      if (!ok) return;
      setButtonLoading(promoteBtn, true, "Đang promote…");
      try {
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote", {
          method: "POST",
          body: { image_tag: tag },
        });
        navigateAfterPromote(slug, tag);
      } catch (err) {
        toastError(err.message || "Promote prod thất bại");
      } finally {
        setButtonLoading(promoteBtn, false, "Promote lên Prod →");
      }
    };
  }
  document.querySelectorAll(".pipe-rollback-btn").forEach(function (btn) {
    btn.onclick = async function () {
      const tag = btn.dataset.tag;
      const itemEnv = btn.dataset.env || env;
      if (!tag) return;
      const profile = btn.dataset.deployProfile || "";
      const branch = btn.dataset.gitBranch || "";
      const cacheKey = deployHistoryPageKey(slug, itemEnv);
      const cached = state.deployActivityCache[cacheKey] || {};
      const clusterP = cached.cluster_profile;
      if (clusterP && !rollbackLayoutAllowed({ deploy_layout: btn.dataset.deployLayout || "" }, clusterP)) {
        toastError("Không thể Deploy lại: bản này khác kiểu chạy. Dùng 「Đổi kiểu chạy…」 rồi deploy bản mới — rollback chỉ cùng kiểu.");
        return;
      }
      const details = [
        "Khôi phục image đã build — không build lại trên GitHub",
        "Chỉ hoạt động khi bản này cùng kiểu chạy với site hiện tại",
      ];
      if (profile) details.push("Profile bản này: " + profile);
      if (branch) details.push("Branch lúc deploy: " + branch);
      if (clusterP && clusterP.profile_label && profile && clusterP.profile_label !== profile) {
        details.push("Cluster hiện: " + clusterP.profile_label + " → sau rollback: " + profile);
      }
      const ok = await uiConfirm({
        title: "Deploy lại (cùng kiểu chạy)",
        message: "Khôi phục image " + tag.slice(0, 7) + " lên " + itemEnv.toUpperCase() + "?",
        details: details,
        confirmText: "Deploy lại",
      });
      if (!ok) return;
      setButtonLoading(btn, true, "…");
      try {
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/rollback", {
          method: "POST",
          body: { image_tag: tag, environment: itemEnv },
        });
        toastSuccess("Đã gửi rollback — theo dõi tiến trình bên dưới");
        const scope = document.getElementById("deploy-history-page") ? "history" : "current";
        const activity = await api(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: itemEnv, scope: scope })
        );
        if (document.getElementById("deploy-history-page")) {
          const hist = renderDeployHistoryContent(activity, { slug: slug, expectedEnv: itemEnv });
          const cur = activity.current;
          let body = "";
          if (cur) {
            body +=
              '<p class="muted" style="margin-bottom:12px">Bản mới nhất (cũng xem tại tab <a href="' +
              esc(projectRoute(slug, "deploy")) +
              '">Deploy / Git</a>):</p>' +
              renderDeployPipelineItem(cur, true, { env: itemEnv });
          }
          const envLabel = itemEnv.toUpperCase();
          body +=
            '<h4 style="margin:20px 0 10px">Các bản trước (' +
            hist.count +
            ")</h4>" +
            '<p class="muted deploy-history-note" style="margin:0 0 10px;font-size:12px">Mỗi commit = 1 tag. <strong>Deploy lại</strong> chỉ với bản cùng kiểu chạy hiện tại.</p>' +
            '<div id="deploy-history-list">' +
            hist.itemsHtml +
            "</div>" +
            hist.pagerHtml;
          document.getElementById("deploy-history-page").innerHTML =
            "<h3>Lịch sử · " + esc(envLabel) + "</h3>" + renderDeployProfileContext(activity) + body;
          bindDeployHistoryPagination(slug, itemEnv);
          bindDeployActivityActions(slug, itemEnv);
          state.deployActivityCache[deployHistoryPageKey(slug, itemEnv)] = activity;
          if (activity.serving_image_tag) state.deployServingTag = activity.serving_image_tag;
        } else {
          updateDeployActivityDOM(activity, slug, promoteReadiness, itemEnv);
        }
      } catch (err) {
        toastError(err.message);
      } finally {
        setButtonLoading(btn, false, "Deploy lại");
      }
    };
  });
  bindDeployLogCopyButtons(document.getElementById("deploy-activity-card"));
}

function updateDeployActivityDOM(activity, slug, promoteReadiness, expectedEnv, renderOpts) {
  expectedEnv = deployActivityEnv(null, expectedEnv);
  renderOpts = renderOpts || {};
  if (activity && !activity.loading) {
    state.deployActivityCache[deployHistoryPageKey(slug, expectedEnv)] = activity;
  }
  if (activity && activity.serving_image_tag) {
    state.deployServingTag = activity.serving_image_tag;
  }
  if (promoteReadiness !== undefined) {
    rememberPromoteReadiness(slug, promoteReadiness);
  }
  const card = document.getElementById("deploy-activity-card");
  if (!card) return;
  if (activity && !activity.loading && !deployActivityEnvMatches(activity, expectedEnv)) {
    return;
  }
  const detailsOpen = document.getElementById("deploy-current-details");
  const wasOpen = detailsOpen ? detailsOpen.open : true;
  const historyWrap = document.querySelector(".deploy-history-wrap");
  const wasHistoryOpen = historyWrap ? historyWrap.open : false;
  const logEl = document.getElementById("build-log-view");
  const atBottom = logEl && logEl.scrollHeight - logEl.scrollTop - logEl.clientHeight < 80;
  const cur = activity.current;
  const prevLive = card.dataset.deployLive === "1";
  const nowLive = deployIsLive(cur);
  const curWrap = document.getElementById("deploy-pipeline-current");

  const mustFullRender =
    !curWrap ||
    !cur ||
    prevLive !== nowLive ||
    card.dataset.deployEnv !== expectedEnv;
  if (mustFullRender) {
    const tmp = document.createElement("div");
    tmp.innerHTML = renderDeployActivityCard(activity, {
      slug: slug,
      promoteReadiness: promoteReadiness,
      expectedEnv: expectedEnv,
      showHistory: renderOpts.showHistory === true,
      showPromotePrep: renderOpts.showPromotePrep === true,
      showPromoteBar: renderOpts.showPromoteBar === true,
    });
    const fresh = tmp.firstElementChild;
    if (fresh) {
      fresh.dataset.deployLive = nowLive ? "1" : "0";
      fresh.dataset.deployEnv = expectedEnv;
      const newDetails = fresh.querySelector("#deploy-current-details");
      if (newDetails && !wasOpen) newDetails.open = false;
      const newHistory = fresh.querySelector(".deploy-history-wrap");
      if (newHistory && wasHistoryOpen) newHistory.open = true;
      card.replaceWith(fresh);
      bindDeployActivityActions(slug, expectedEnv, promoteReadiness);
    }
    return;
  }

  const liveNoteEl = card.querySelector(".deploy-live-note");
  if (liveNoteEl) {
    if (nowLive) {
      liveNoteEl.style.display = "";
    } else {
      liveNoteEl.remove();
    }
  } else if (nowLive && detailsOpen) {
    const note = document.createElement("p");
    note.className = "muted deploy-live-note";
    note.style.marginBottom = "8px";
    note.innerHTML =
      '<span class="live-dot"></span> Đang build · ' +
      esc((activity.environment || "dev").toUpperCase()) +
      " — cập nhật mỗi " +
      (activity.poll_interval_sec || 2) +
      "s";
    detailsOpen.insertBefore(note, curWrap);
  }

  curWrap.innerHTML = renderDeployPipelineItem(cur, true, { env: expectedEnv });
  bindDeployActivityActions(slug, expectedEnv, promoteReadiness);
  const newDetails = document.getElementById("deploy-current-details");
  if (newDetails) newDetails.open = wasOpen;
  const newLog = document.getElementById("build-log-view");
  const newRuntime = document.getElementById("runtime-log-view");
  if (newLog) {
    if (cur.status === "failed") {
      newLog.scrollTop = newLog.scrollHeight;
    } else if (atBottom) {
      newLog.scrollTop = newLog.scrollHeight;
    }
  }
  if (newRuntime) {
    const rtBottom = newRuntime.scrollHeight - newRuntime.scrollTop - newRuntime.clientHeight < 80;
    if (rtBottom || (cur.runtime_status === "running" && !deployIsTerminal(cur))) {
      newRuntime.scrollTop = newRuntime.scrollHeight;
    }
  }
}

function bindDeployActivityPoll(slug, env, navToken) {
  stopDeployPoll();
  let pollSec = 5;
  async function refresh() {
    if (!isNavTokenActive(navToken)) return;
    try {
      const activity = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env, scope: "current" })
      );
      if (!isNavTokenActive(navToken)) return;
      const terminal = activity.current && deployIsTerminal(activity.current);
      const hadLive = state.deployWasLive;
      const nowLive = activity.current && deployIsLive(activity.current);
      state.deployWasLive = !!nowLive;
      pollSec = terminal ? 0 : activity.poll_interval_sec || (deployIsLive(activity.current) ? 2 : 5);
      updateDeployActivityDOM(activity, slug, undefined, env, { showHistory: false, showPromotePrep: false, showPromoteBar: false });
      if (promoteFollowActive(slug, env)) {
        scrollToDeployProgress();
        if (terminal) handlePromoteFollowTerminal(activity, slug);
      }
      if (terminal) {
        if (hadLive) {
          setTimeout(function () {
            if (isNavTokenActive(navToken)) refresh();
          }, 2500);
        }
        stopDeployPoll();
        return;
      }
      if (state.deployPoll) {
        clearInterval(state.deployPoll);
        state.deployPoll = setInterval(refresh, pollSec * 1000);
      }
    } catch (_e) {
      /* ignore poll errors */
    }
  }
  refresh();
  state.deployPoll = setInterval(refresh, pollSec * 1000);
}

function platformHomeHash() {
  return canManagePlatformProjects() ? "#/platform-projects" : "#/my-projects";
}

function projectRoute(slug, tab) {
  return "#/project/" + slug + (tab && tab !== "overview" ? "/" + tab : "");
}

const PROJECT_NAV = [
  { tab: "overview", label: "Tổng quan", icon: "◉" },
  { tab: "monitoring", label: "Monitoring", icon: "📈" },
  { tab: "runtime", label: "Runtime", icon: "▶" },
  { tab: "deploy", label: "Deploy / Git", icon: "⎇" },
  { tab: "promote", label: "Promote Prod", icon: "↑" },
  { tab: "deploy-history", label: "Lịch sử deploy", icon: "🕘" },
  { tab: "domains", label: "Domains", icon: "🌐" },
  { tab: "env", label: "Cấu hình app", icon: "🔑" },
  { tab: "settings", label: "Cài đặt", icon: "⚙" },
];

const PROJECT_WORKLOADS = [
  { tab: "pods", label: "Pods", icon: "●" },
  { tab: "deployments", label: "Deployments", icon: "▶" },
  { tab: "services", label: "Services", icon: "🔗" },
  { tab: "ingresses", label: "Ingresses", icon: "🌐" },
];

const GRAFANA_NS_DASH_UID = "85a562078cdf77779eaa1add43ccec1e";

function grafanaNamespaceDashboardUrl(baseUrl, namespace) {
  if (!baseUrl || !namespace) return "";
  const base = String(baseUrl).replace(/\/+$/, "");
  const q = new URLSearchParams({
    "var-namespace": namespace,
    orgId: "1",
    from: "now-6h",
    to: "now",
    timezone: "browser",
  });
  return base + "/d/" + GRAFANA_NS_DASH_UID + "/kubernetes-compute-resources-namespace-pods?" + q.toString();
}

function updateSidebarBrand(ctx) {
  const h1 = document.querySelector(".sidebar-brand h1");
  const sub = document.querySelector(".sidebar-brand p");
  const layout = document.querySelector(".layout");
  if (!h1 || !sub) return;
  if (ctx.mode === "project") {
    layout?.classList.add("sidebar-project-mode");
    h1.textContent = ctx.name || ctx.slug;
    sub.innerHTML = "<code>" + esc(ctx.slug) + "</code> · Project";
  } else {
    layout?.classList.remove("sidebar-project-mode");
    h1.textContent = "Platform Console";
    sub.textContent = "K8s Explorer";
  }
}

function projectHeader(p, subtitle, opts) {
  opts = opts || {};
  const helpBtn = opts.help === "deploy" ? renderDeployHelpButton("steps", "btn-help-header") : "";
  return (
    '<div class="page-header project-header page-header-row">' +
    '<div class="page-header-text">' +
    "<h2 class=\"page-title\">" + esc(p.name) + "</h2>" +
    '<p class="page-subtitle">' + esc(subtitle || p.slug) + "</p></div>" +
    helpBtn +
    "</div>"
  );
}

function copyText(text, okMsg) {
  navigator.clipboard.writeText(text || "").then(
    function () { toastSuccess(okMsg || "Đã copy"); },
    function () { toastError("Không copy được — chọn thủ công"); }
  );
}

function snippetBlock(id, title, content, copyLabel) {
  return (
    '<div class="snippet-block" style="margin-top:12px">' +
    '<div class="snippet-head">' +
    "<h4>" + esc(title) + "</h4>" +
    '<button type="button" class="btn-sm btn-copy-snippet" data-snippet-id="' + esc(id) + '">' +
    esc(copyLabel || "Copy") +
    "</button></div>" +
    '<pre class="yaml-box" id="' + esc(id) + '">' + esc(content) + "</pre></div>"
  );
}

function bindSnippetCopyButtons(root) {
  (root || document).querySelectorAll(".btn-copy-snippet").forEach(function (btn) {
    btn.onclick = function () {
      const el = document.getElementById(btn.dataset.snippetId);
      copyText(el ? el.textContent : "", "Đã copy");
    };
  });
}

function domainKindBadge(kind) {
  return kind === "auto"
    ? '<span class="badge ok">Tự động</span>'
    : '<span class="badge">Custom</span>';
}

function domainSyncBadge(status) {
  const s = status || "pending";
  if (s === "synced") return '<span class="badge ok">Ingress OK</span>';
  if (s === "error") return '<span class="badge warn">Lỗi sync</span>';
  return '<span class="badge">Chờ sync</span>';
}

function domainCertBadge(status, tls) {
  if (!tls) return '<span class="muted">—</span>';
  const s = status || "pending";
  if (s === "ready") return '<span class="badge ok">TLS Ready</span>';
  if (s === "failed") return '<span class="badge warn">TLS lỗi</span>';
  if (s === "n/a") return '<span class="muted">—</span>';
  return '<span class="badge">TLS pending</span>';
}

function renderDNSHint(dns) {
  if (!dns) return "";
  if (dns.mode === "auto") {
    return '<p class="muted dns-hint">' + esc(dns.message || "") + "</p>";
  }
  let html = '<div class="dns-hint-box"><p class="muted"><strong>Cấu hình DNS</strong></p>';
  if (dns.record_type && dns.record_value) {
    html +=
      "<p><code>" +
      esc(dns.record_type) +
      "</code> · <code>" +
      esc(dns.record_name || "@") +
      "</code> → <code>" +
      esc(dns.record_value) +
      "</code></p>";
  }
  if (dns.alt_type && dns.alt_value) {
    html +=
      '<p class="muted">Hoặc <code>' +
      esc(dns.alt_type) +
      "</code> → <code>" +
      esc(dns.alt_value) +
      "</code></p>";
  }
  if (dns.note) html += '<p class="muted">' + esc(dns.note) + "</p>";
  return html + "</div>";
}

function selectWrapHtml(id, optionsHtml, opts) {
  opts = opts || {};
  const cls = "select-wrap" + (opts.compact ? " select-wrap-compact" : "");
  const attrs = [];
  if (id) attrs.push('id="' + esc(id) + '"');
  if (opts.name) attrs.push('name="' + esc(opts.name) + '"');
  if (opts.required) attrs.push("required");
  return (
    '<div class="' + cls + '">' +
    "<select " + attrs.join(" ") + ">" +
    optionsHtml +
    '</select><span class="select-chev" aria-hidden="true">▾</span></div>'
  );
}

function setButtonLoading(btn, loading, label) {
  if (!btn) return;
  if (loading) {
    if (!btn.dataset.idleLabel) btn.dataset.idleLabel = btn.textContent;
    btn.disabled = true;
    btn.classList.add("is-loading");
    btn.innerHTML = '<span class="btn-spinner" aria-hidden="true"></span> ' + esc(label || "Đang xử lý…");
  } else {
    btn.disabled = false;
    btn.classList.remove("is-loading");
    btn.textContent = btn.dataset.idleLabel || label || "OK";
    delete btn.dataset.idleLabel;
  }
}

function bindEnvSegment(onChange) {
  setTimeout(function () {
    document.querySelectorAll(".env-segment .env-seg-btn").forEach(function (btn) {
      btn.onclick = function () {
        const env = btn.dataset.env;
        if (!env || state.projectEnv === env) return;
        state.projectEnv = env;
        localStorage.setItem("project-env", env);
        nextNavToken();
        document.querySelectorAll(".env-segment .env-seg-btn").forEach(function (b) {
          b.classList.toggle("active", b.dataset.env === env);
        });
        if (onChange) onChange();
        else location.reload();
      };
    });
  }, 0);
}

function deployActivityEnv(activity, fallback) {
  return (fallback || state.projectEnv || activity?.environment || "dev").toLowerCase();
}

function deployActivityEnvMatches(activity, expectedEnv) {
  if (!activity || activity.loading) return true;
  const want = deployActivityEnv(null, expectedEnv);
  const got = deployActivityEnv(activity, null);
  return want === got;
}

function projectEnvToolbar(slug, p, onChange) {
  const env = state.projectEnv || "dev";
  const ns = env === "prod" ? p.namespace_prod : p.namespace_dev;
  bindEnvSegment(onChange);
  return (
    '<div class="toolbar project-env-bar">' +
    '<div class="env-segment" role="group" aria-label="Môi trường">' +
    '<span class="env-segment-label">Môi trường</span>' +
    '<button type="button" class="env-seg-btn' + (env === "dev" ? " active" : "") + '" data-env="dev">Dev</button>' +
    '<button type="button" class="env-seg-btn' + (env === "prod" ? " active" : "") + '" data-env="prod">Prod</button>' +
    "</div>" +
    '<span class="muted project-env-ns">Namespace <code>' + esc(ns) + "</code></span></div>"
  );
}

function canManagePlatformProjects() {
  const r = state.user && state.user.role;
  return r === "admin" || r === "tech_lead";
}

function canDeleteProject() {
  return canManagePlatformProjects();
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

function renderEnvReadinessPanel(readiness, slug, env, scope) {
  if (!readiness) return "";
  const c = readiness.contract || {};
  const issues = (c.issues || []).filter(function (i) { return i.severity === "error"; });
  const warnIssues = (c.issues || []).filter(function (i) { return i.severity === "warning"; });
  const warnings = c.warnings || [];
  const suggestions = readiness.suggestions || [];
  let html = "";

  function renderWarnBlock(title, lines) {
    if (!lines.length) return "";
    return (
      '<div class="env-readiness env-readiness-warn">' +
      "<strong>" + esc(title) + "</strong><ul>" +
      lines.map(function (w) { return "<li>" + esc(w) + "</li>"; }).join("") +
      "</ul></div>"
    );
  }

  if (!readiness.ready && issues.length) {
    html +=
      '<div class="env-readiness env-readiness-error">' +
      "<strong>Chưa sẵn sàng" + (scope === "build" ? " build" : "") + "</strong>" +
      "<ul>" +
      issues.map(function (i) {
        let line =
          '<li class="env-readiness-issue"><span class="env-readiness-issue-msg">' + esc(i.message) + "</span>";
        if (i.description) {
          line += '<span class="env-readiness-issue-desc muted">' + esc(i.description) + "</span>";
        }
        return line + "</li>";
      }).join("") +
      "</ul>";
    if (suggestions.length) {
      html +=
        '<p class="muted" style="margin:8px 0 4px">Từ <code>.platform/' +
        (scope === "build" ? "build.yaml" : "runtime.yaml") +
        '</code> — thêm trên Console hoặc bấm <strong>Lấy key từ contract</strong>:</p><div class="env-suggest-row">';
      suggestions.forEach(function (s) {
        html +=
          '<button type="button" class="btn-sm btn-ghost env-suggest-add" data-key="' + esc(s.key) + '" data-scope="' + esc(scope) + '"' +
          (s.required ? ' title="Bắt buộc"' : "") +
          ">+ " + esc(s.key) + (s.required ? " *" : "") + "</button> ";
      });
      html += "</div>";
    }
    html += "</div>";
  } else if (c.contract_found && readiness.ready) {
    html =
      '<div class="env-readiness env-readiness-ok"><span class="badge ok">Đã đủ cấu hình</span>' +
      ' <span class="muted">Contract <code>' + esc(c.contract_path || "") + "</code></span></div>";
  } else if (c.contract_found && warnings.length && !issues.length) {
    html = renderWarnBlock("Cảnh báo", warnings);
  }

  const driftLines = warnIssues.map(function (i) { return i.message || i.description || ""; }).filter(Boolean);
  const extraWarn = warnings.filter(function (w) {
    return driftLines.indexOf(w) < 0;
  });
  const allWarn = driftLines.concat(extraWarn);
  if (readiness.ready && allWarn.length) {
    html += renderWarnBlock("Cảnh báo contract / Dockerfile", allWarn);
  } else if (!readiness.ready && allWarn.length && issues.length) {
    html += renderWarnBlock("Cảnh báo thêm", allWarn);
  }

  return html;
}

function renderMissingContractRows(suggestions, scope) {
  return (suggestions || [])
    .map(function (s) {
      return (
        '<tr class="env-row-missing"><td><code>' +
        esc(s.key) +
        '</code> <span class="badge warn">contract</span></td><td class="muted">' +
        esc(s.description || "Chưa có trên Console") +
        '</td><td><button type="button" class="btn-sm btn-primary env-suggest-add" data-key="' +
        esc(s.key) +
        '" data-scope="' +
        esc(scope) +
        '">Thêm</button></td></tr>'
      );
    })
    .join("");
}

function renderEnvSyncNote(syncStatus) {
  if (!syncStatus) return "";
  const synced = !!syncStatus.synced;
  const badge = synced
    ? '<span class="badge ok">Đã khớp cluster</span>'
    : '<span class="badge warn">Chưa khớp cluster</span>';
  return (
    '<p class="env-sync-note' +
    (synced ? " env-sync-ok" : " env-sync-pending") +
    '">' +
    badge +
    ' <span class="muted">' +
    esc(syncStatus.detail || "") +
    "</span></p>"
  );
}

async function promptContractKeys(slug, env, suggestions, scope) {
  const list = suggestions || [];
  if (!list.length) {
    toastSuccess("Đã đủ key từ contract");
    return;
  }
  for (let i = 0; i < list.length; i++) {
    const s = list[i];
    const result = await openEnvVarDialog(slug, env, { key: s.key, scope: scope || "build" });
    if (!result) {
      if (i === 0) return;
      break;
    }
  }
  const main = document.getElementById("main");
  if (main) pageProjectHub(main, slug, "env");
}

function bindEnvSuggestButtons(main, slug, env) {
  main.querySelectorAll(".env-suggest-add").forEach(function (btn) {
    btn.onclick = function () {
      openEnvVarDialog(slug, env, { key: btn.dataset.key, scope: btn.dataset.scope || "build" });
    };
  });
}

function envVarIsBuildScope(v) {
  return String((v && v.scope) || "").toLowerCase() === "build";
}

function envVarIsRuntimeScope(v) {
  const s = String((v && v.scope) || "").toLowerCase();
  return s === "" || s === "runtime";
}

function renderPlatformBuildArgRows() {
  return [
    { key: "GIT_SHA", desc: "Tự động mỗi lần build (commit SHA)" },
    { key: "GIT_REF", desc: "Tự động mỗi lần build (branch/tag)" },
  ]
    .map(function (r) {
      return (
        '<tr class="env-row-platform"><td><code>' +
        esc(r.key) +
        '</code> <span class="badge neutral">platform</span></td><td class="muted">' +
        esc(r.desc) +
        "</td><td></td></tr>"
      );
    })
    .join("");
}

function renderEnvVarTable(rows, slug, env, canEditEnv, scope) {
  return (rows || [])
    .map(function (v) {
      const valCell = v.is_secret
        ? '<span class="env-secret-val">' + esc(v.value || "—") + ' <span class="badge neutral">secret</span></span>'
        : "<code>" + esc(v.value || "—") + "</code>";
      return (
        "<tr><td><code>" + esc(v.key) + "</code></td><td>" + valCell + "</td><td>" +
        (canEditEnv
          ? '<button type="button" class="btn-sm env-edit" data-id="' + v.id + '" data-key="' + esc(v.key) + '" data-secret="' + (v.is_secret ? "1" : "0") + '" data-scope="' + esc(scope) + '">Sửa</button> ' +
            '<button type="button" class="btn-sm btn-danger env-del" data-id="' + v.id + '" data-key="' + esc(v.key) + '">Xóa</button>'
          : "") +
        "</td></tr>"
      );
    })
    .join("");
}

function bindEnvVarTableActions(main, slug, env) {
  main.querySelectorAll(".env-del").forEach(function (btn) {
    btn.onclick = async function () {
      if (!(await uiConfirm('Xóa biến "' + btn.dataset.key + '"?', { danger: true, title: "Xóa cấu hình" }))) return;
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/" + btn.dataset.id, {
          method: "DELETE",
        });
        if (res.sync_warning) toastError(res.sync_warning);
        else toastSuccess("Đã xóa");
        pageProjectHub(main, slug, "env");
      } catch (err) {
        toastError(err.message);
      }
    };
  });
  main.querySelectorAll(".env-edit").forEach(function (btn) {
    btn.onclick = function () {
      openEnvVarDialog(slug, env, {
        id: parseInt(btn.dataset.id, 10),
        key: btn.dataset.key,
        is_secret: btn.dataset.secret === "1",
        scope: btn.dataset.scope || "runtime",
      });
    };
  });
}

function openEnvVarDialog(slug, environment, existing) {
  const initScope = (existing && existing.scope) || "runtime";
  const scopeLabel =
    initScope === "build" ? "Khi build image (Dockerfile ARG)" : "Khi app chạy (Pod)";
  return new Promise(function (resolve) {
    const isEdit = !!(existing && existing.id);
    const overlay = document.createElement("div");
    overlay.className = "ui-overlay";
    overlay.innerHTML =
      '<div class="ui-dialog" role="dialog" aria-modal="true">' +
      '<div class="ui-dialog-glow"></div>' +
      "<h3 class=\"ui-dialog-title\">" + (isEdit ? "Sửa cấu hình" : "Thêm cấu hình") + "</h3>" +
      '<form id="env-var-dialog-form" class="login-form dialog-form">' +
      (isEdit
        ? '<p class="muted">Key: <code>' + esc(existing.key) + "</code> · " +
          (initScope === "build" ? "Khi build image" : "Khi app chạy") + "</p>"
        : '<p class="muted">Loại: <strong>' + esc(scopeLabel) + "</strong></p>" +
          '<input type="hidden" name="scope" value="' + esc(initScope) + '" />' +
          '<label>Key<input name="key" required placeholder="APP_VERSION" pattern="[^\\s]+" value="' + esc((existing && existing.key) || "") + '" /></label>') +
      '<label>Value<textarea name="value" rows="3" placeholder="giá trị…" required></textarea></label>' +
      '<label class="checkbox-row"><input type="checkbox" name="is_secret"' + (existing && existing.is_secret ? " checked" : "") + " /> Secret (ẩn trên UI" + (initScope === "build" ? " + đẩy GitHub Secrets" : "") + ")</label>" +
      '<p class="muted">Môi trường: <strong>' + esc(environment) + "</strong></p>" +
      '<div class="ui-dialog-actions" style="margin-top:16px;padding-top:0;border:0">' +
      '<button type="button" class="btn-ghost ui-dialog-cancel">Huỷ</button>' +
      '<button type="submit" class="btn-primary">' + (isEdit ? "Lưu" : "Thêm") + "</button></div></form></div>";

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

    const form = overlay.querySelector("#env-var-dialog-form");
    form.onsubmit = async function (e) {
      e.preventDefault();
      const fd = new FormData(form);
      const submitBtn = form.querySelector('button[type="submit"]');
      submitBtn.disabled = true;
      try {
        let res;
        if (isEdit) {
          res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/" + existing.id, {
            method: "PATCH",
            body: {
              value: fd.get("value"),
              is_secret: !!form.querySelector('input[name="is_secret"]').checked,
            },
          });
        } else {
          res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env", {
            method: "POST",
            body: {
              key: fd.get("key"),
              value: fd.get("value"),
              is_secret: !!form.querySelector('input[name="is_secret"]').checked,
              environment: environment,
              scope: (form.querySelector('[name="scope"]') && form.querySelector('[name="scope"]').value) || initScope,
            },
          });
        }
        if (res.sync_warning) {
          toastError(res.sync_warning);
        } else {
          toastSuccess(isEdit ? "Đã lưu" : "Đã thêm biến");
        }
        close(res);
        const main = document.getElementById("main");
        if (main) pageProjectHub(main, slug, "env");
      } catch (err) {
        toastError(err.message);
        submitBtn.disabled = false;
      }
    };
    const firstInput = form.querySelector('input[name="key"], textarea[name="value"]');
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
    return (
      '<div class="card gitops-project-card" style="margin-bottom:16px">' +
      '<h3>GitOps <span class="badge muted">Chưa bật</span></h3>' +
      '<p class="muted">Admin chưa cấu hình repo GitOps — vẫn deploy bình thường qua Rancher. ' +
      (canManageGitOps() ? '<a href="#/gitops">Cấu hình GitOps →</a>' : "Hỏi admin nếu cần Argo CD.") +
      "</p></div>"
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
  return (
    '<div class="card gitops-project-card" style="margin-bottom:16px">' +
    '<div class="gitops-card-head">' +
    "<h3>GitOps " +
    (pub.configured ? '<span class="badge ok">Đã cấu hình</span>' : '<span class="badge warn">Thiếu PAT</span>') +
    "</h3>" +
    scaffoldBtn +
    "</div>" +
    '<p class="muted">Repo: <code>' + esc(pub.repo_url || status.repo_url || "—") + "</code> · branch <code>" + esc(pub.repo_branch || "main") + "</code></p>" +
    '<div class="meta-chips" style="margin-top:8px">' + devBadge + prodBadge + "</div>" +
    argoHtml +
    '<p class="muted" style="margin-top:8px;font-size:12px">Scaffold tạo <code>apps/' + esc(slug) + '/overlays/dev|prod</code> trên repo GitOps + đăng ký Argo CD (nếu bật).</p>' +
    "</div>"
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
    '<label>Registry<select name="registry_provider" id="registry-provider-select">' + opts + "</select></label>" +
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

function githubRepoOptionsHtml(repo, ghRepos) {
  if (ghRepos && ghRepos.items === null) {
    return '<option value="" disabled>Đang tải danh sách repo…</option>';
  }
  const items = (ghRepos && ghRepos.items) || [];
  return items
    .map(function (r) {
      const sel = repo.github_owner === r.owner && repo.github_repo === r.name ? " selected" : "";
      return (
        '<option value="' +
        esc(r.owner + "/" + r.name) +
        '" data-branch="' +
        esc(r.default_branch || "main") +
        '"' +
        sel +
        ">" +
        esc(r.full_name || r.name) +
        (r.private ? " 🔒" : "") +
        "</option>"
      );
    })
    .join("");
}

function githubBranchOptionsHtml(branches, selected) {
  selected = (selected || "main").toString();
  const items = branches && branches.length ? branches : [{ name: selected, is_default: true }];
  return items
    .map(function (b) {
      const name = (b.name || b).toString();
      const sel = name === selected ? " selected" : "";
      const label = name + (b.is_default ? " (default)" : "");
      return '<option value="' + esc(name) + '"' + sel + ">" + esc(label) + "</option>";
    })
    .join("");
}

function parseGitHubRepoValue(value) {
  const full = (value || "").toString().trim();
  const parts = full.split("/");
  if (parts.length < 2) return null;
  return { owner: parts[0], repo: parts.slice(1).join("/") };
}

async function loadGitHubBranchSelect(selectEl, owner, repo, selectedBranch) {
  if (!selectEl) return;
  const keep = selectedBranch || selectEl.value || "main";
  selectEl.innerHTML = '<option value="">Đang tải branch…</option>';
  selectEl.disabled = true;
  try {
    const data = await api(
      "/api/v1/github/repos/" + encodeURIComponent(owner) + "/" + encodeURIComponent(repo) + "/branches"
    );
    const branches = data.items || [];
    let sel = keep;
    if (!branches.some(function (b) { return b.name === sel; })) {
      const def = branches.find(function (b) { return b.is_default; });
      sel = def ? def.name : branches[0] ? branches[0].name : "main";
    }
    selectEl.innerHTML = githubBranchOptionsHtml(branches, sel);
  } catch (err) {
    selectEl.innerHTML = githubBranchOptionsHtml([{ name: keep }], keep);
    toastError(err.message || "Không tải được branch từ GitHub");
  } finally {
    selectEl.disabled = false;
  }
}

function buildModeAutoHintHtml(repo) {
  const mode = ((repo && repo.build_mode) || "").toLowerCase();
  const path = repo && repo.build_mode_detected_path;
  if (mode === "buildpack") {
    return (
      '<p class="muted github-setup-hint">Platform tự quét repo: không thấy <code>Dockerfile</code> → build bằng <strong>Buildpack</strong> (tự nhận stack Node, Go, Python…). App listen <code>8080</code>.</p>'
    );
  }
  if (mode === "dockerfile" && path) {
    return (
      '<p class="muted github-setup-hint">Platform tự quét repo: thấy <code>' +
      esc(path) +
      "</code> → build <strong>Docker</strong>. App listen <code>8080</code>.</p>"
    );
  }
  return (
    '<p class="muted github-setup-hint">Platform tự quét repo khi kết nối: có <code>Dockerfile</code> → Docker; không → Buildpack. App listen <code>8080</code>.</p>'
  );
}

function renderBackFrontConventionBanner(conv, canEdit) {
  if (!conv || !conv.enabled) return "";
  const apiBase = (conv.recommended_build && conv.recommended_build.VITE_API_BASE) || "/api";
  return (
    '<div class="convention-banner">' +
    "<strong>Chuẩn Backend + Frontend</strong>" +
    '<p class="muted" style="margin:6px 0">Prod: 1 domain · web <code>/</code> · API <code>/api</code> · frontend gọi <code>' +
    esc(apiBase) +
    "</code></p>" +
    '<p class="muted" style="margin:0;font-size:12px">' +
    esc(conv.dev_local_hint || "Dev máy: proxy /api → backend — không hardcode localhost trong code prod.") +
    "</p>" +
    (canEdit
      ? '<button type="button" class="btn-ghost btn-sm" id="apply-conventions-btn" style="margin-top:8px">Áp dụng env mặc định</button>'
      : "") +
    '<span class="muted" style="font-size:11px;display:block;margin-top:6px">Repo mẫu: <code>templates/back-front/</code> · Prod cấm <code>localhost</code> trong env</span>' +
    "</div>"
  );
}

function bindApplyConventionsButton(main, slug, onDone) {
  const btn = document.getElementById("apply-conventions-btn");
  if (!btn) return;
  btn.onclick = async function () {
    setButtonLoading(btn, true, "Đang áp dụng…");
    try {
      const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/conventions/apply", { method: "POST" });
      const n = (res.seeds || []).filter(function (s) { return s.created; }).length;
      toastSuccess(n ? "Đã thêm " + n + " biến env mặc định" : "Env chuẩn đã có sẵn");
      if (onDone) onDone();
    } catch (err) {
      toastError(err.message);
    } finally {
      setButtonLoading(btn, false, "Áp dụng env mặc định");
    }
  };
}

function defaultMultiTemplate(svcData) {
  const template = (svcData && svcData.template) || [];
  if (template.length >= 2) return template;
  return [
    { name: "api", display_name: "API", build_context: "backend", build_mode: "dockerfile", dockerfile_path: "backend/Dockerfile", ingress_path: "/api", health_path: "/health" },
    { name: "web", display_name: "Web", build_context: "frontend", build_mode: "dockerfile", dockerfile_path: "frontend/Dockerfile", ingress_path: "/", health_path: "/" },
  ];
}

function buildModeLabel(mode) {
  return (mode || "").toLowerCase() === "buildpack" ? "Buildpack" : "Docker";
}

function serviceRowIsPublic(s) {
  return s.expose_ingress !== false && String(s.ingress_path || "").toLowerCase() !== "internal" && s.ingress_path !== "-";
}

function serviceCtxCellHtml(idx, s) {
  const val = s.build_context || ".";
  return (
    '<td class="svc-ctx-cell">' +
    '<select name="svc_ctx_sel_' + idx + '" class="svc-ctx-select" data-idx="' + idx + '">' +
    '<option value="' + esc(val) + '" selected>' + esc(val) + "</option>" +
    '<option value="__custom__">Tự nhập…</option></select>' +
    '<input name="svc_ctx_custom_' + idx + '" class="svc-ctx-custom" value="' + esc(val) + '" placeholder="vd. backend" hidden />' +
    '<input name="svc_ctx_' + idx + '" type="hidden" value="' + esc(val) + '" />' +
    "</td>"
  );
}

function stackLabel(stack, mode) {
  if ((mode || "").toLowerCase() === "buildpack" && stack) {
    return " · " + stack;
  }
  return "";
}

function defaultHealthPath(s) {
  s = s || {};
  const ingress = (s.ingress_path || "/").trim();
  if (ingress === "/" || ingress === "") return "/";
  return s.health_path || "/health";
}

function resourcesModeSelectHtml(name, mode, idx) {
  mode = mode || "platform";
  const idxAttr = idx != null && idx !== "" ? ' data-idx="' + idx + '"' : "";
  return (
    '<select name="' + name + '" class="svc-res-mode"' + idxAttr + ">" +
    '<option value="platform"' + (mode === "platform" ? " selected" : "") + ">Mặc định platform</option>" +
    '<option value="none"' + (mode === "none" ? " selected" : "") + ">Không set</option>" +
    '<option value="custom"' + (mode === "custom" ? " selected" : "") + ">Tùy chỉnh</option>" +
    "</select>"
  );
}

function serviceResourcesInputsHtml(s, fieldPrefix) {
  s = s || {};
  fieldPrefix = fieldPrefix || "svc";
  const mode = s.resources_mode || "platform";
  const custom = mode === "custom";
  const dis = custom ? "" : " disabled";
  return (
    '<div class="svc-res-grid">' +
    '<input name="' + fieldPrefix + '_cpu_req" class="svc-res-input" value="' + esc(s.cpu_request || "") + '" placeholder="CPU req" title="CPU request (vd. 100m)"' + dis + " />" +
    '<input name="' + fieldPrefix + '_mem_req" class="svc-res-input" value="' + esc(s.memory_request || "") + '" placeholder="RAM req" title="Memory request (vd. 128Mi)"' + dis + " />" +
    '<input name="' + fieldPrefix + '_cpu_lim" class="svc-res-input" value="' + esc(s.cpu_limit || "") + '" placeholder="CPU lim" title="CPU limit (vd. 500m)"' + dis + " />" +
    '<input name="' + fieldPrefix + '_mem_lim" class="svc-res-input" value="' + esc(s.memory_limit || "") + '" placeholder="RAM lim" title="Memory limit (vd. 512Mi)"' + dis + " />" +
    "</div>"
  );
}

function readServiceResourcesFromForm(form, idx) {
  if (!form) return { resources_mode: "platform", cpu_request: "", memory_request: "", cpu_limit: "", memory_limit: "" };
  if (idx === "app") {
    return {
      resources_mode: (form.querySelector('[name="app_res_mode"]') || {}).value || "platform",
      cpu_request: (form.querySelector('[name="app_cpu_req"]') || {}).value || "",
      memory_request: (form.querySelector('[name="app_mem_req"]') || {}).value || "",
      cpu_limit: (form.querySelector('[name="app_cpu_lim"]') || {}).value || "",
      memory_limit: (form.querySelector('[name="app_mem_lim"]') || {}).value || "",
    };
  }
  return {
    resources_mode: (form.querySelector('[name="svc_res_mode_' + idx + '"]') || {}).value || "platform",
    cpu_request: (form.querySelector('[name="svc_cpu_req_' + idx + '"]') || {}).value || "",
    memory_request: (form.querySelector('[name="svc_mem_req_' + idx + '"]') || {}).value || "",
    cpu_limit: (form.querySelector('[name="svc_cpu_lim_' + idx + '"]') || {}).value || "",
    memory_limit: (form.querySelector('[name="svc_mem_lim_' + idx + '"]') || {}).value || "",
  };
}

function toggleServiceResourceInputs(form, idx) {
  if (!form) return;
  if (idx === "app") {
    const mode = (form.querySelector('[name="app_res_mode"]') || {}).value || "platform";
    const custom = mode === "custom";
    form.querySelectorAll('[name="app_cpu_req"],[name="app_mem_req"],[name="app_cpu_lim"],[name="app_mem_lim"]').forEach(function (el) {
      el.disabled = !custom;
    });
    return;
  }
  const modeEl = form.querySelector('[name="svc_res_mode_' + idx + '"]');
  const custom = modeEl && modeEl.value === "custom";
  ["svc_cpu_req_", "svc_mem_req_", "svc_cpu_lim_", "svc_mem_lim_"].forEach(function (prefix) {
    const el = form.querySelector('[name="' + prefix + idx + '"]');
    if (el) el.disabled = !custom;
  });
}

function renderSingleResourcesPanel(s) {
  s = s || {};
  const mode = s.resources_mode || "platform";
  return (
    '<div class="single-resources-panel" style="margin-top:12px">' +
    "<strong>CPU / RAM</strong>" +
    '<p class="muted" style="margin:6px 0 8px;font-size:12px">Mặc định platform = preset an toàn · Không set = không inject limits (Grafana % có thể trống)</p>' +
    '<div class="svc-res-row">' +
    resourcesModeSelectHtml("app_res_mode", mode, "app") +
    serviceResourcesInputsHtml(s, "app") +
    "</div></div>"
  );
}

function renderServiceResourcesCard(s, idx) {
  const mode = (s && s.resources_mode) || "platform";
  const custom = mode === "custom";
  const dis = custom ? "" : " disabled";
  return (
    '<div class="service-resources-card" data-svc-res-idx="' + idx + '">' +
    '<div class="service-resources-card-head"><strong>' + esc((s && (s.display_name || s.name)) || ("Service " + idx)) + "</strong></div>" +
    '<div class="svc-res-row">' +
    resourcesModeSelectHtml("svc_res_mode_" + idx, mode, idx) +
    '<div class="svc-res-grid">' +
    '<input name="svc_cpu_req_' + idx + '" class="svc-res-input" value="' + esc((s && s.cpu_request) || "") + '" placeholder="CPU req" title="CPU request (vd. 100m)"' + dis + " />" +
    '<input name="svc_mem_req_' + idx + '" class="svc-res-input" value="' + esc((s && s.memory_request) || "") + '" placeholder="RAM req" title="Memory request (vd. 128Mi)"' + dis + " />" +
    '<input name="svc_cpu_lim_' + idx + '" class="svc-res-input" value="' + esc((s && s.cpu_limit) || "") + '" placeholder="CPU lim" title="CPU limit (vd. 500m)"' + dis + " />" +
    '<input name="svc_mem_lim_' + idx + '" class="svc-res-input" value="' + esc((s && s.memory_limit) || "") + '" placeholder="RAM lim" title="Memory limit (vd. 768Mi)"' + dis + " />" +
    "</div></div></div>"
  );
}

function buildServiceTableRowHtml(s, idx) {
  const pub = serviceRowIsPublic(s);
  const stack = s.stack || "";
  return (
    '<tr data-svc-idx="' + idx + '">' +
    '<td><input name="svc_name_' + idx + '" value="' + esc(s.name || "") + '" /></td>' +
    '<td><select name="svc_mode_' + idx + '">' +
    '<option value="dockerfile"' + ((s.build_mode || "dockerfile") !== "buildpack" ? " selected" : "") + ">Docker</option>" +
    '<option value="buildpack"' + ((s.build_mode || "") === "buildpack" ? " selected" : "") + ">Buildpack</option>" +
    "</select></td>" +
    '<td><select name="svc_stack_' + idx + '">' +
    '<option value=""' + (!stack ? " selected" : "") + ">auto</option>" +
    '<option value="python"' + (stack === "python" ? " selected" : "") + ">python</option>" +
    '<option value="node"' + (stack === "node" ? " selected" : "") + ">node</option>" +
    '<option value="go"' + (stack === "go" ? " selected" : "") + ">go</option>" +
    '<option value="dotnet"' + (stack === "dotnet" ? " selected" : "") + ">dotnet</option>" +
    "</select></td>" +
    serviceCtxCellHtml(idx, s) +
    '<td><input name="svc_df_' + idx + '" value="' + esc(s.dockerfile_path || "Dockerfile") + '" placeholder="Dockerfile" /></td>' +
    '<td><label class="svc-public-label"><input type="checkbox" name="svc_public_' + idx + '"' + (pub ? " checked" : "") + ' /> Public</label></td>' +
    '<td><input name="svc_ingress_' + idx + '" value="' + esc(pub ? (s.ingress_path || "/") : "-") + '" placeholder="/ hoặc /api" /></td>' +
    '<td><input type="hidden" name="svc_health_' + idx + '" value="' + esc(s.health_path || defaultHealthPath(s)) + '" /></td>' +
    '<td><button type="button" class="btn-ghost btn-sm svc-remove-row" data-idx="' + idx + '">×</button></td>' +
    "</tr>"
  );
}

function renderServicePreviewCard(s) {
  const mode = buildModeLabel(s.build_mode);
  const pub = serviceRowIsPublic(s);
  const access = pub
    ? "URL công khai: <code>" + esc(s.ingress_path || "/") + "*</code>"
    : '<span class="badge warn">Internal</span> · cluster <code>http://' + esc(s.name || "?") + ":80</code>";
  return (
    '<div class="service-preview-card">' +
    "<h4>" + esc(s.display_name || s.name || "?") + ' <span class="badge">' + esc(mode) + esc(stackLabel(s.stack, s.build_mode)) + "</span></h4>" +
    "<p>Thư mục <code>" + esc(s.build_context || ".") + "</code> → image <code>" + esc(s.name) + "</code><br>" +
    access + "</p></div>"
  );
}

function renderServicesContractBanner(contract, canEdit) {
  contract = contract || {};
  if (!contract.found) {
    return (
      '<p class="muted repo-detect-hint" style="font-size:12px;margin:0 0 10px">Chưa thấy <code>.platform/services.yaml</code> — Console quét Dockerfile để gợi ý build (Docker / Buildpack).</p>'
    );
  }
  if (contract.parse_error) {
    return (
      '<div class="banner warn" style="margin-bottom:10px"><strong>services.yaml</strong> — ' +
      esc(contract.parse_error) +
      "</div>"
    );
  }
  const svcs = contract.services || [];
  const names = (contract.service_names || svcs.map(function (s) { return s.name; })).filter(Boolean);
  const synced = !!contract.in_sync;
  const layoutLabel = layoutKindLabel(contract.suggested_layout || contract.layout || "single");
  let html =
    '<div class="banner repo-detect-banner' +
    (synced ? "" : " warn") +
    '" style="margin-bottom:8px"><strong>Repo gợi ý</strong> · ' +
    esc(layoutLabel);
  if (names.length) {
    html += " · <code>" + esc(names.join(" + ")) + "</code>";
  }
  html +=
    ' · branch <code>' +
    esc(contract.branch || "?") +
    "</code>";
  html += synced
    ? ' — <span style="color:#15803d">khớp Console</span>'
    : ' — <span style="color:#b45309">chưa khớp Console</span>';
  const gitSub = (contract.git_submodules || "").trim();
  if (gitSub || contract.has_gitmodules) {
    const subLabel = gitSub || "recursive";
    const subSync = contract.git_submodules_in_sync !== false;
    html +=
      ' · Submodule <code>' +
      esc(subLabel) +
      "</code>" +
      (subSync ? "" : ' <span style="color:#b45309">(chưa sync workflow)</span>');
  }
  html += "</div>";
  if (canEdit && !synced && !contract.parse_error) {
    const isMulti = (contract.suggested_layout || contract.layout) === "multi";
    const svcCount = (contract.service_names || names).length;
    const btnClass = isMulti ? "btn-primary btn-sm" : "btn-ghost btn-sm";
    const btnLabel = isMulti && svcCount > 2
      ? "Bước 2: Áp dụng fleet từ repo (" + svcCount + " service)"
      : isMulti
        ? "Bước 2: Áp dụng api + web từ repo"
        : "Áp dụng cấu hình từ repo";
    html +=
      '<p class="muted repo-detect-action-hint" style="font-size:12px;margin:0 0 8px">' +
      (isMulti
        ? "Repo đã có <code>services.yaml</code> (api + web) — bấm nút dưới, rồi <strong>Lưu &amp; đồng bộ GitHub</strong> trước khi push."
        : "Repo có cấu hình platform — áp dụng vào Console trước khi sync.") +
      "</p>" +
      '<button type="button" class="' +
      btnClass +
      '" id="sync-services-contract" style="margin-bottom:10px">' +
      esc(btnLabel) +
      "</button>";
  }
  return html;
}

function parseRepoFromForm(form) {
  form = form || document.getElementById("pipeline-setup-form");
  if (!form) return null;
  const repoEl = form.querySelector('[name="repo"]');
  return parseGitHubRepoValue(repoEl && repoEl.value);
}

function layoutKindLabel(layout) {
  return layout === "multi" ? "Web + API riêng" : "Một website";
}

function contractServiceToFormRow(s) {
  s = s || {};
  return {
    name: s.name || "",
    display_name: s.display_name || s.name || "",
    build_mode: s.build_mode || "dockerfile",
    stack: s.stack || "",
    build_context: s.build_context || ".",
    dockerfile_path: s.dockerfile_path || "Dockerfile",
    ingress_path: s.ingress_path || "/",
    expose_ingress: s.expose_ingress !== false,
    health_path: s.health_path || "/health",
    container_port: s.container_port || 8080,
  };
}

function updatePipelineBuildHint(contract) {
  const el = document.getElementById("pipeline-build-hint");
  if (!el || !contract) return;
  el.innerHTML = buildModeAutoHintHtml({
    build_mode: contract.build_mode,
    build_mode_detected_path: contract.build_mode_detected_path,
  });
}

function applyRepoLayoutSuggestion(form, contract) {
  if (!form || !contract || contract.parse_error || !contract.found) return false;
  if (state.pipelineLayoutUserTouched) return false;
  const suggested = contract.suggested_layout || contract.layout || "single";
  const radio = form.querySelector('input[name="layout"][value="' + suggested + '"]');
  if (!radio || radio.checked) return false;
  radio.checked = true;
  radio.dispatchEvent(new Event("change", { bubbles: true }));
  toastSuccess("Repo gợi ý: " + layoutKindLabel(suggested) + " — đã chọn sẵn");
  return true;
}

function applyRepoServicesSuggestion(form, contract) {
  if (!form || !contract || contract.parse_error || !contract.found) return false;
  if (state.pipelineServicesUserTouched) return false;
  if ((contract.suggested_layout || contract.layout) !== "multi") return false;
  const services = contract.services || [];
  if (services.length < 2) return false;
  const tbody = document.getElementById("project-services-tbody");
  if (!tbody) return false;
  const rows = services.map(contractServiceToFormRow);
  tbody.innerHTML = rows
    .map(function (s, idx) {
      return buildServiceTableRowHtml(s, idx);
    })
    .join("");
  toastSuccess("Đã điền " + rows.length + " service từ repo");
  return true;
}

function applyRepoContractSuggestion(form, contract, opts) {
  opts = opts || {};
  const layoutChanged = applyRepoLayoutSuggestion(form, contract);
  const servicesFilled = applyRepoServicesSuggestion(form, contract);
  updatePipelineBuildHint(contract);
  if ((layoutChanged || servicesFilled) && opts.onPrefilled) {
    opts.onPrefilled({ layoutChanged: layoutChanged, servicesFilled: servicesFilled });
  }
  return layoutChanged || servicesFilled;
}

function refreshRepoDetectBanner(contract, canEdit) {
  const panel = document.getElementById("repo-detect-banner-slot");
  if (!panel) return;
  panel.innerHTML = renderServicesContractBanner(contract, canEdit);
  const syncBtn = document.getElementById("sync-services-contract");
  if (syncBtn && state._repoDetectSyncHandler) {
    syncBtn.onclick = state._repoDetectSyncHandler;
  }
}

function renderPipelineCrosscheckHtml(repo, svcData, contract) {
  repo = repo || {};
  svcData = svcData || {};
  contract = contract || svcData.repo_contract || {};
  const layout = (svcData.layout || "single");
  const lines = [];
  let cls = "ok";
  if (repo.workflow_stale && repo.workflow_stale_reason) {
    lines.push(esc(repo.workflow_stale_reason));
    cls = "warn";
  } else if (!repo.workflow_synced_at) {
    lines.push("Workflow chưa khớp Console — cần bấm 「Lưu & đồng bộ GitHub」.");
    cls = "warn";
  }
  if (contract.found && !contract.parse_error) {
    if (contract.layout === "multi" && layout !== "multi") {
      lines.push("Repo cần <strong>Web + API riêng</strong> — chọn đúng kiểu hoặc bấm 「Áp dụng từ repo」.");
      cls = "warn";
    }
    if (contract.layout !== "multi" && layout === "multi") {
      lines.push("Console chọn Web + API nhưng repo chưa có cấu hình multi — cần 「Lưu & đồng bộ GitHub」.");
      cls = "warn";
    }
    if (layout === "multi" && !contract.in_sync && contract.layout === "multi") {
      lines.push("Cấu hình Console khác file <code>services.yaml</code> trên branch <code>" + esc(contract.branch || "?") + "</code>.");
      cls = "warn";
    }
    if (contract.git_submodules && contract.git_submodules_in_sync === false) {
      lines.push("Submodule <code>" + esc(contract.git_submodules) + "</code> chưa sync workflow.");
      cls = "warn";
    }
  }
  if (lines.length === 0) {
    return '<div id="pipeline-crosscheck" class="pipeline-crosscheck ok">✓ Repo và kiểu chạy nhất quán — sẵn sàng đồng bộ workflow.</div>';
  }
  return '<div id="pipeline-crosscheck" class="pipeline-crosscheck ' + cls + '">' + lines.map(function (l) { return "• " + l; }).join("<br>") + "</div>";
}

async function refreshPipelineCrosscheck(slug, form, svcData, repo, opts) {
  opts = opts || {};
  const el = document.getElementById("pipeline-crosscheck");
  if (!el || !form) return null;
  const parsed = parseRepoFromForm(form);
  const branch = selectedGitHubBranch(repo && repo.branch);
  if (!parsed) {
    el.className = "pipeline-crosscheck warn";
    el.innerHTML = "• Chọn repository và branch — Console sẽ quét repo và gợi ý kiểu chạy.";
    return null;
  }
  el.className = "pipeline-crosscheck";
  el.innerHTML = '<span class="btn-spinner"></span> Đang quét branch <code>' + esc(branch) + "</code>…";
  try {
    const contract = await api(
      "/api/v1/projects/" +
        encodeURIComponent(slug) +
        "/services/detect" +
        qs({ branch: branch, owner: parsed.owner, repo: parsed.repo })
    );
    applyRepoContractSuggestion(form, contract, opts);
    refreshRepoDetectBanner(contract, true);
    const merged = Object.assign({}, svcData || {}, {
      repo_contract: contract,
      layout: collectProjectLayoutPayload(form).layout,
    });
    const html = renderPipelineCrosscheckHtml(repo, merged, contract);
    const newEl = document.getElementById("pipeline-crosscheck");
    if (newEl) {
      newEl.outerHTML = html;
    }
    return contract;
  } catch (err) {
    el.className = "pipeline-crosscheck warn";
    el.innerHTML = "• Không quét được repo: " + esc(errorMessage(err));
    return null;
  }
}

var PIPELINE_SETUP_STEPS = [
  "Lưu kiểu chạy",
  "Lưu repo & branch",
  "Push workflow GitHub",
  "Inject secrets (Harbor + deploy token)",
];

function renderPipelinePolicyCallout() {
  return (
    '<div class="pipeline-policy-callout">' +
    '<div class="pipeline-policy-head">' +
    "<strong>Quy tắc</strong>" +
    renderDeployHelpButton("rules", "btn-help-inline") +
    "</div>" +
    "<ul>" +
    "<li><strong>Deploy lại</strong> — đổi tag/image, <em>cùng</em> kiểu chạy (Một website hoặc Web + API).</li>" +
    "<li><strong>Đổi kiểu chạy</strong> — đổi topology (single ↔ multi), deploy bản mới; <em>không</em> dùng rollback.</li>" +
    "<li>Kiểu chạy chốt ở <strong>bước 2</strong> — Console gợi ý từ repo sau khi chọn branch.</li>" +
    "</ul></div>"
  );
}

function pipelineCardTitleHtml() {
  return (
    '<div class="card-title-row">' +
    '<h3 style="margin:0">Pipeline · GitHub &amp; Kiểu chạy</h3>' +
    renderDeployHelpButton("steps") +
    "</div>"
  );
}

function collectProjectLayoutPayload(form) {
  if (!form) return { layout: "single", services: [] };
  const checked = form.querySelector('input[name="layout"]:checked');
  const layout = checked ? checked.value : "single";
  const services = [];
  if (layout === "single") {
    const res = readServiceResourcesFromForm(form, "app");
    services.push(Object.assign({ name: "app", container_port: 8080, health_path: "/health", ingress_path: "/" }, res));
  }
  if (layout === "multi") {
    const tbody = document.getElementById("project-services-tbody");
    if (tbody) {
      Array.prototype.slice.call(tbody.querySelectorAll("tr")).forEach(function (tr) {
        const idx = tr.getAttribute("data-svc-idx");
        const modeEl = form.querySelector('[name="svc_mode_' + idx + '"]');
        const pubEl = form.querySelector('[name="svc_public_' + idx + '"]');
        const expose = pubEl ? pubEl.checked : true;
        const ingressEl = form.querySelector('[name="svc_ingress_' + idx + '"]');
        let ingress = ingressEl ? ingressEl.value : "/";
        if (!expose) ingress = "-";
        const stackEl = form.querySelector('[name="svc_stack_' + idx + '"]');
        const ctxSel = form.querySelector('[name="svc_ctx_sel_' + idx + '"]');
        const ctxHidden = form.querySelector('[name="svc_ctx_' + idx + '"]');
        const ctxCustom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
        let buildContext = ".";
        if (ctxSel && ctxSel.value === "__custom__" && ctxCustom) {
          buildContext = ctxCustom.value || ".";
        } else if (ctxHidden && ctxHidden.value) {
          buildContext = ctxHidden.value;
        } else if (ctxSel && ctxSel.value && ctxSel.value !== "__custom__") {
          buildContext = ctxSel.value;
        }
        const healthEl = form.querySelector('[name="svc_health_' + idx + '"]');
        const ingressVal = expose ? (ingressEl ? ingressEl.value : "/") : "-";
        let healthPath = healthEl ? healthEl.value : "/health";
        if (expose && (ingressVal === "/" || ingressVal === "") && !healthEl) healthPath = "/";
        services.push({
          name: (form.querySelector('[name="svc_name_' + idx + '"]') || {}).value || "",
          build_mode: modeEl ? modeEl.value : "dockerfile",
          stack: stackEl ? stackEl.value : "",
          build_context: buildContext,
          dockerfile_path: (form.querySelector('[name="svc_df_' + idx + '"]') || {}).value || "Dockerfile",
          ingress_path: ingressVal,
          expose_ingress: expose,
          container_port: 8080,
          health_path: healthPath,
          sort_order: parseInt(idx, 10) || 0,
        });
        const res = readServiceResourcesFromForm(form, idx);
        Object.assign(services[services.length - 1], res);
      });
    }
  }
  return { layout: layout, services: services };
}

async function runGitHubPipelineSetup(slug, opts) {
  opts = opts || {};
  const body = {
    owner: opts.owner,
    repo: opts.repo,
    branch: opts.branch || "main",
    environment: opts.environment || "dev",
  };
  if (opts.apply_repo_contract) {
    body.apply_repo_contract = true;
  } else {
    const layoutPayload = opts.layoutPayload || { layout: "single", services: [] };
    body.layout = layoutPayload.layout;
    body.services = layoutPayload.services;
  }
  const progress = opts.progressEl;
  const submitBtn = opts.submitBtn;
  const formRoot = opts.formRoot;
  const steps = PIPELINE_SETUP_STEPS;
  if (progress) {
    progress.hidden = false;
    progress.innerHTML =
      '<div class="setup-progress-title"><span class="btn-spinner"></span> Đang đồng bộ pipeline…</div>' +
      steps
        .map(function (s) {
          return '<div class="setup-step setup-step-pending">' + esc(s) + "</div>";
        })
        .join("");
  }
  if (submitBtn) setButtonLoading(submitBtn, true, "Đang đồng bộ…");
  if (formRoot) {
    formRoot.querySelectorAll("input, select, button").forEach(function (el) {
      if (el !== submitBtn) el.disabled = true;
    });
  }
  let stepIdx = 0;
  const stepTimer = setInterval(function () {
    if (!progress) return;
    progress.querySelectorAll(".setup-step").forEach(function (el, i) {
      el.className = "setup-step " + (i < stepIdx ? "setup-step-done" : i === stepIdx ? "setup-step-run" : "setup-step-pending");
    });
    if (stepIdx < steps.length - 1) stepIdx++;
  }, 900);
  try {
    const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/github/setup", {
      method: "POST",
      body: body,
    });
    clearInterval(stepTimer);
    if (progress) {
      progress.querySelectorAll(".setup-step").forEach(function (el) {
        el.className = "setup-step setup-step-done";
      });
      progress.innerHTML =
        '<div class="setup-progress-title setup-progress-ok">✓ Pipeline sẵn sàng — layout + workflow đồng bộ</div>' +
        steps
          .map(function (s) {
            return '<div class="setup-step setup-step-done">' + esc(s) + "</div>";
          })
          .join("");
      setTimeout(function () {
        progress.hidden = true;
      }, 4000);
    }
    return res;
  } catch (err) {
    clearInterval(stepTimer);
    if (progress) {
      renderSetupSyncError(progress, errorMessage(err, "Đồng bộ pipeline thất bại"), steps);
    }
    throw err;
  } finally {
    if (submitBtn) setButtonLoading(submitBtn, false, "Lưu & đồng bộ GitHub");
    if (formRoot) {
      formRoot.querySelectorAll("input, select, button").forEach(function (el) {
        el.disabled = false;
      });
    }
  }
}

function renderPipelineSetupCard(slug, svcData, repo, ghStatus, ghRepos, canEdit) {
  repo = repo || {};
  ghStatus = ghStatus || {};
  ghRepos = ghRepos || { items: [] };
  svcData = svcData || {};
  const layout = svcData.layout || "single";
  const items = svcData.items || [];
  const isMulti = layout === "multi";
  const tpl = defaultMultiTemplate(svcData);
  const multiItems = isMulti && items.length >= 2 ? items : tpl;
  const conv = svcData.conventions || null;
  const repoContract = svcData.repo_contract || null;
  const conventionBanner = isMulti ? renderBackFrontConventionBanner(conv, canEdit) : "";
  const contractBanner = renderServicesContractBanner(repoContract, canEdit);
  const ghRepoOpts = githubRepoOptionsHtml(repo, ghRepos);

  const singleHint =
    '<div id="layout-single-hint" class="layout-hint-panel"' + (isMulti ? ' hidden' : "") + ">" +
    "<strong>Một website</strong> — một link, một app. Platform tự quét Dockerfile trên branch đã chọn.<br>" +
    (repo.build_mode
      ? "Gần nhất: <strong>" + esc(buildModeLabel(repo.build_mode)) + "</strong>" +
        (repo.build_mode_detected_path ? " · <code>" + esc(repo.build_mode_detected_path) + "</code>" : "") +
        " · listen <code>8080</code>"
      : "Chọn branch → kiểm tra tự động bên trên.") +
    renderSingleResourcesPanel((items[0] || {})) +
    "</div>";

  const multiPanel =
    '<div id="layout-multi-panel"' + (!isMulti ? ' hidden' : "") + ">" +
    conventionBanner +
    '<p class="muted" style="margin:0 0 10px">N service · public (Ingress) hoặc internal. Env discovery <code>SVC_&lt;TÊN&gt;_URL</code>.</p>' +
    '<div class="service-preview-grid" id="service-preview-grid">' +
    multiItems.map(renderServicePreviewCard).join("") +
    "</div>" +
    '<div class="service-resources-panel" id="service-resources-panel">' +
    "<h4>CPU / RAM từng service</h4>" +
    '<p class="muted" style="margin:0 0 10px;font-size:12px">Chọn <strong>Mặc định platform</strong> (an toàn) · <strong>Không set</strong> (không limit) · <strong>Tùy chỉnh</strong> để nhập CPU/RAM. Lưu nháp rồi deploy lại để áp dụng.</p>' +
    '<div class="service-resources-grid" id="service-resources-grid">' +
    multiItems.map(function (s, idx) { return renderServiceResourcesCard(s, idx); }).join("") +
    "</div></div>" +
    '<details class="layout-advanced-details"><summary>Tùy chỉnh service (dev) — build path, ingress</summary>' +
    '<p class="muted" id="github-dir-hint" style="margin:8px 0">Thư mục build theo branch đã chọn ở bước 1.</p>' +
    '<button type="button" class="btn-ghost btn-sm" id="refresh-github-dirs" style="margin-bottom:8px">Quét thư mục từ GitHub</button>' +
    '<table class="data-table"><thead><tr><th>Tên</th><th>Build</th><th>Stack</th><th>Thư mục</th><th>Dockerfile</th><th>Public</th><th>Ingress</th><th></th></tr></thead>' +
    '<tbody id="project-services-tbody">' +
    multiItems.map(function (s, idx) { return buildServiceTableRowHtml(s, idx); }).join("") +
    "</tbody></table>" +
    '<button type="button" class="btn-ghost btn-sm" id="project-services-add-row" style="margin-top:8px">+ Thêm service</button>' +
    "</details></div>";

  const statusChips =
    '<div class="pipeline-status-chips">' +
    chip("Kiểu chạy", isMulti ? "Web + API riêng" : "Một website") +
    (ghStatus.connected ? chip("GitHub", "@" + (ghStatus.login || "?")) : "") +
    (repo.workflow_stale
      ? '<span class="badge warn" title="' + esc(repo.workflow_stale_reason || "") + '">Cần đồng bộ</span>'
      : repo.workflow_synced_at && repo.auto_deploy_enabled
        ? '<span class="badge ok">Workflow OK</span>'
        : repo.workflow_synced_at
          ? '<span class="badge warn">Workflow cũ</span>'
          : '<span class="badge warn">Chưa đồng bộ</span>') +
    "</div>";

  const crosscheck = renderPipelineCrosscheckHtml(repo, svcData, repoContract);

  if (!ghStatus.enabled) {
    return (
      '<div class="card" style="margin-bottom:16px" id="pipeline-setup-card">' + pipelineCardTitleHtml() +
      renderDeployHelpInlineCard(slug) +
      '<p class="muted">GitHub OAuth chưa cấu hình trên VPS.</p></div>'
    );
  }

  if (!ghStatus.connected) {
    return (
      '<div class="card" style="margin-bottom:16px" id="pipeline-setup-card">' + pipelineCardTitleHtml() +
      renderDeployHelpInlineCard(slug) +
      renderPipelinePolicyCallout() +
      '<p class="muted">Kết nối GitHub → chọn repo/branch → chốt kiểu chạy → đồng bộ workflow.</p>' +
      (canEdit ? '<button type="button" class="btn-primary" id="github-connect-btn">① Kết nối GitHub</button>' : "") +
      "</div>"
    );
  }

  if (!canEdit) {
    return (
      '<div class="card" style="margin-bottom:16px" id="pipeline-setup-card">' + pipelineCardTitleHtml() +
      renderDeployHelpInlineCard(slug) +
      renderPipelinePolicyCallout() +
      statusChips +
      crosscheck +
      '<div class="meta-chips">' + chip("Repo", (repo.github_owner || "") + "/" + (repo.github_repo || "")) + chip("Branch", repo.branch || "main") + "</div>" +
      (isMulti ? '<div class="service-preview-grid" style="margin-top:12px">' + multiItems.map(renderServicePreviewCard).join("") + "</div>" : singleHint) +
      "</div>"
    );
  }

  return (
    '<div class="card" style="margin-bottom:16px" id="pipeline-setup-card">' + pipelineCardTitleHtml() +
    renderDeployHelpInlineCard(slug) +
    renderPipelinePolicyCallout() +
    statusChips +
    crosscheck +
    '<form id="pipeline-setup-form" class="login-form pipeline-wizard">' +
    '<div class="pipeline-step">' +
    '<div class="pipeline-step-head"><span class="pipeline-step-num">1</span> Nguồn GitHub</div>' +
    '<p class="muted" style="margin:0 0 10px">Đã kết nối <strong>@' + esc(ghStatus.login || "") + "</strong></p>" +
    '<label>Repository' +
    selectWrapHtml("github-repo-select", '<option value="">— chọn repo —</option>' + ghRepoOpts, { name: "repo", required: true }) +
    "</label>" +
    '<div class="form-row">' +
    '<label>Branch' +
    selectWrapHtml("github-branch-select", githubBranchOptionsHtml([], repo.branch || "main"), { name: "branch", required: true }) +
    "</label>" +
    '<label>Deploy env' +
    selectWrapHtml(
      "",
      '<option value="dev"' + (repo.deploy_environment !== "prod" ? " selected" : "") + ">dev</option>" +
        '<option value="prod"' + (repo.deploy_environment === "prod" ? " selected" : "") + ">prod (push → deploy thẳng)</option>",
      { name: "environment" }
    ) +
    "</label></div>" +
    (repo.deploy_environment === "prod"
      ? '<p class="muted pipeline-prod-warn">⚠ Deploy env = <strong>prod</strong> — mỗi push lên branch này build và deploy <em>trực tiếp</em> lên production (không qua dev).</p>'
      : "") +
    '<div id="pipeline-build-hint">' + buildModeAutoHintHtml(repo) + "</div>" +
    (repo.workflow_synced_at
      ? '<label class="auto-deploy-toggle"><input type="checkbox" id="auto-deploy-toggle" ' +
        (repo.auto_deploy_enabled ? "checked" : "") +
        " /> Tự deploy lên cluster khi build xong</label>"
      : "") +
    "</div>" +
    '<div class="pipeline-step">' +
    '<div class="pipeline-step-head"><span class="pipeline-step-num">2</span> Chốt kiểu chạy</div>' +
    '<div id="repo-detect-banner-slot">' + (isMulti ? contractBanner : renderServicesContractBanner(repoContract, canEdit)) + "</div>" +
    '<div class="layout-change-row">' +
    '<p class="muted pipeline-layout-hint">Chọn <em>một lần</em> trước khi sync workflow. Đã deploy mà muốn đổi topology?</p>' +
    '<div class="layout-change-actions">' +
    '<button type="button" class="btn-ghost btn-sm layout-change-btn" id="open-change-layout-wizard">↔ Đổi kiểu chạy</button>' +
    '<span class="muted layout-change-note">Không phải rollback — cần sync workflow rồi deploy bản mới</span>' +
    "</div></div>" +
    '<div class="layout-picker">' +
    '<label class="layout-option">' +
    '<input type="radio" name="layout" value="single"' + (!isMulti ? " checked" : "") + " />" +
    '<div class="layout-option-body"><span class="layout-option-icon" aria-hidden="true">🌐</span><strong>Một website</strong><span>Một link duy nhất</span></div></label>' +
    '<label class="layout-option">' +
    '<input type="radio" name="layout" value="multi"' + (isMulti ? " checked" : "") + " />" +
    '<div class="layout-option-body"><span class="layout-option-icon" aria-hidden="true">⚡</span><strong>Web + API riêng</strong><span>Giao diện + API tách path</span></div></label></div>' +
    singleHint +
    multiPanel +
    "</div>" +
    '<div class="pipeline-step">' +
    '<div class="pipeline-step-head"><span class="pipeline-step-num">3</span> Đồng bộ workflow</div>' +
    (repo.workflow_stale
      ? '<div class="banner warn" style="margin-bottom:10px">' + esc(repo.workflow_stale_reason || "Cần đồng bộ workflow trước khi push.") + "</div>"
      : "") +
    '<div id="github-setup-progress" class="setup-progress" hidden></div>' +
    '<div class="form-actions" style="flex-wrap:wrap;gap:8px">' +
    '<button type="submit" class="btn-primary" id="github-setup-submit">Lưu &amp; đồng bộ GitHub</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="pipeline-save-draft">Chỉ lưu Console</button>' +
    '<button type="button" class="btn-ghost btn-sm" id="github-disconnect-btn">Ngắt GitHub</button>' +
    "</div></div>" +
    "</form></div>"
  );
}

function selectedGitHubBranch(fallback) {
  const sel = document.getElementById("github-branch-select");
  if (sel && sel.value && String(sel.value).trim()) {
    return String(sel.value).trim();
  }
  return String(fallback || "main").trim() || "main";
}

function bindPipelineSetupForm(main, slug, svcData, repo, ghStatus, env, navToken) {
  const form = document.getElementById("pipeline-setup-form");
  state.pipelineLayoutUserTouched = false;
  state.pipelineServicesUserTouched = false;
  const singleHint = document.getElementById("layout-single-hint");
  const multiPanel = document.getElementById("layout-multi-panel");
  const previewGrid = document.getElementById("service-preview-grid");
  const resGrid = document.getElementById("service-resources-grid");
  const tbody = document.getElementById("project-services-tbody");
  const dirHint = document.getElementById("github-dir-hint");
  const refreshDirsBtn = document.getElementById("refresh-github-dirs");
  const template = defaultMultiTemplate(svcData);
  let nextSvcIdx = Math.max((template.length || 2) - 1, 0);
  repo = repo || {};
  ghStatus = ghStatus || {};
  env = env || state.projectEnv || "dev";
  if (!form) return;

  function readCtxValue(idx) {
    const sel = form.querySelector('[name="svc_ctx_sel_' + idx + '"]');
    const hidden = form.querySelector('[name="svc_ctx_' + idx + '"]');
    const custom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
    if (sel && sel.value === "__custom__" && custom) {
      return (custom.value || ".").trim() || ".";
    }
    if (sel && sel.value && sel.value !== "__custom__") {
      return sel.value;
    }
    return hidden ? hidden.value : ".";
  }

  function syncCtxHidden(idx) {
    const val = readCtxValue(idx);
    const hidden = form.querySelector('[name="svc_ctx_' + idx + '"]');
    if (hidden) hidden.value = val;
    const modeEl = form.querySelector('[name="svc_mode_' + idx + '"]');
    const dfEl = form.querySelector('[name="svc_df_' + idx + '"]');
    if (modeEl && modeEl.value === "dockerfile" && dfEl && val && val !== ".") {
      const guess = val.replace(/\/$/, "") + "/Dockerfile";
      if (!dfEl.value || dfEl.value === "Dockerfile" || dfEl.value.endsWith("/Dockerfile")) {
        dfEl.value = guess;
      }
    }
    refreshPreview();
  }

  function bindCtxSelect(idx) {
    const sel = form.querySelector('[name="svc_ctx_sel_' + idx + '"]');
    const custom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
    if (!sel) return;
    sel.onchange = function () {
      if (custom) custom.hidden = sel.value !== "__custom__";
      syncCtxHidden(idx);
    };
    if (custom) {
      custom.oninput = function () {
        syncCtxHidden(idx);
      };
    }
  }

  function bindResModeSelect(idx) {
    if (idx === "app") {
      const sel = form.querySelector('[name="app_res_mode"]');
      if (!sel) return;
      sel.onchange = function () {
        toggleServiceResourceInputs(form, "app");
      };
      toggleServiceResourceInputs(form, "app");
      return;
    }
    const sel = form.querySelector('[name="svc_res_mode_' + idx + '"]');
    if (!sel) return;
    sel.onchange = function () {
      toggleServiceResourceInputs(form, idx);
    };
    toggleServiceResourceInputs(form, idx);
  }

  async function loadGitHubDirs() {
    const parsed = parseRepoFromForm(form);
    const owner = parsed ? parsed.owner : (repo.github_owner || "").trim();
    const ghRepo = parsed ? parsed.repo : (repo.github_repo || "").trim();
    const branch = selectedGitHubBranch(repo.branch || "main");
    if (!ghStatus.connected || !owner || !ghRepo) {
      if (dirHint) {
        dirHint.textContent = "Chọn repo và branch ở bước 1 — rồi bấm Quét thư mục.";
      }
      return [];
    }
    if (dirHint) {
      dirHint.innerHTML =
        "Thư mục từ branch <code>" + esc(branch) + "</code> · repo <code>" + esc(owner + "/" + ghRepo) + "</code>";
    }
    const data = await api(
      "/api/v1/github/repos/" + encodeURIComponent(owner) + "/" + encodeURIComponent(ghRepo) + "/contents" +
        qs({ ref: branch, path: "" })
    );
    return (data.items || []).filter(function (e) {
      return e.type === "dir";
    });
  }

  function populateCtxSelects(dirs) {
    form.querySelectorAll(".svc-ctx-select").forEach(function (sel) {
      const idx = sel.getAttribute("data-idx");
      const cur = readCtxValue(idx);
      let html = '<option value="."' + (cur === "." ? " selected" : "") + ">. (root repo)</option>";
      dirs.forEach(function (d) {
        html += '<option value="' + esc(d.path) + '"' + (d.path === cur ? " selected" : "") + ">" + esc(d.path) + "/</option>";
      });
      const known = cur === "." || dirs.some(function (d) { return d.path === cur; });
      html += '<option value="__custom__"' + (!known ? " selected" : "") + ">Tự nhập…</option>";
      sel.innerHTML = html;
      const custom = form.querySelector('[name="svc_ctx_custom_' + idx + '"]');
      if (custom) {
        custom.hidden = known;
        if (!known) custom.value = cur;
      }
      bindCtxSelect(idx);
      syncCtxHidden(idx);
    });
  }

  async function refreshGitHubDirs(opts) {
    opts = opts || {};
    const silent = opts.silent === true;
    if (refreshDirsBtn) {
      refreshDirsBtn.disabled = true;
      refreshDirsBtn.textContent = "Đang quét…";
    }
    try {
      const dirs = await loadGitHubDirs();
      populateCtxSelects(dirs);
      if (!silent && dirs.length === 0 && ghStatus.connected) {
        toastError("Không thấy thư mục con — kiểm tra branch hoặc dùng Tự nhập");
      }
    } catch (err) {
      if (!silent) {
        toastError(err.message || "Không quét được GitHub");
      }
    } finally {
      if (refreshDirsBtn) {
        refreshDirsBtn.disabled = false;
        refreshDirsBtn.textContent = "Quét thư mục từ GitHub";
      }
    }
  }

  function currentLayout() {
    const checked = form.querySelector('input[name="layout"]:checked');
    return checked ? checked.value : "single";
  }

  function serviceFromRow(idx, defaults) {
    defaults = defaults || {};
    const modeEl = form.querySelector('[name="svc_mode_' + idx + '"]');
    const pubEl = form.querySelector('[name="svc_public_' + idx + '"]');
    const expose = pubEl ? pubEl.checked : defaults.expose_ingress !== false;
    const ingressEl = form.querySelector('[name="svc_ingress_' + idx + '"]');
    let ingress = ingressEl ? ingressEl.value : defaults.ingress_path || "/";
    if (!expose) ingress = "-";
    const stackEl = form.querySelector('[name="svc_stack_' + idx + '"]');
    const row = {
      name: (form.querySelector('[name="svc_name_' + idx + '"]') || {}).value || defaults.name || "",
      build_mode: modeEl ? modeEl.value : defaults.build_mode || "dockerfile",
      stack: stackEl ? stackEl.value : defaults.stack || "",
      build_context: readCtxValue(idx) || defaults.build_context || ".",
      dockerfile_path: (form.querySelector('[name="svc_df_' + idx + '"]') || {}).value || defaults.dockerfile_path || "Dockerfile",
      ingress_path: ingress,
      expose_ingress: expose,
      container_port: defaults.container_port || 8080,
      health_path: defaults.health_path || "/health",
      sort_order: parseInt(idx, 10) || 0,
      display_name: defaults.display_name || "",
    };
    return Object.assign(row, readServiceResourcesFromForm(form, idx));
  }

  function listServiceRows() {
    if (!tbody) return [];
    return Array.prototype.slice.call(tbody.querySelectorAll("tr"));
  }

  function servicesFromForm() {
    return listServiceRows().map(function (tr) {
      const idx = tr.getAttribute("data-svc-idx");
      return serviceFromRow(idx, {});
    });
  }

  function refreshPreview() {
    if (currentLayout() !== "multi") return;
    const rows = listServiceRows();
    const list = rows.map(function (tr) {
      return serviceFromRow(tr.getAttribute("data-svc-idx"), {});
    });
    if (previewGrid) {
      previewGrid.innerHTML = list
        .map(function (s) {
          const pub = s.expose_ingress !== false && s.ingress_path !== "-";
          return (
            '<div class="service-preview-card"><h4>' +
            esc(s.display_name || s.name) +
            ' <span class="badge">' +
            esc(buildModeLabel(s.build_mode)) +
            "</span></h4><p>Thư mục <code>" +
            esc(s.build_context) +
            "</code> → image <code>" +
            esc(s.name) +
            "</code><br>" +
            (pub
              ? "URL: <code>" + esc(s.ingress_path) + "*</code>"
              : '<span class="badge warn">Internal</span>') +
            "</p></div>"
          );
        })
        .join("");
    }
    if (resGrid) {
      resGrid.innerHTML = rows
        .map(function (tr, i) {
          const idx = tr.getAttribute("data-svc-idx");
          return renderServiceResourcesCard(list[i], idx);
        })
        .join("");
      rows.forEach(function (tr) {
        bindResModeSelect(tr.getAttribute("data-svc-idx"));
      });
    }
  }

  function appendServiceRow(s) {
    if (!tbody) return;
    nextSvcIdx += 1;
    const idx = String(nextSvcIdx);
    tbody.insertAdjacentHTML("beforeend", buildServiceTableRowHtml(s || { name: "worker", build_context: ".", expose_ingress: false, ingress_path: "-" }, idx));
    bindCtxSelect(idx);
    bindResModeSelect(idx);
    syncCtxHidden(idx);
    bindRemoveRow(idx);
    refreshPreview();
  }

  function bindRemoveRow(idx) {
    const btn = form.querySelector('.svc-remove-row[data-idx="' + idx + '"]');
    if (!btn) return;
    btn.onclick = function () {
      const rows = listServiceRows();
      if (rows.length <= 2) {
        toastError("Multi-service cần ít nhất 2 service");
        return;
      }
      const tr = btn.closest("tr");
      if (tr) tr.remove();
      refreshPreview();
    };
  }

  function rebindServiceRows() {
    listServiceRows().forEach(function (tr) {
      bindRemoveRow(tr.getAttribute("data-svc-idx"));
      bindCtxSelect(tr.getAttribute("data-svc-idx"));
      bindResModeSelect(tr.getAttribute("data-svc-idx"));
    });
    bindResModeSelect("app");
    refreshPreview();
    if (currentLayout() === "multi") {
      refreshGitHubDirs({ silent: true });
    }
  }

  function scheduleCrosscheck() {
    refreshPipelineCrosscheck(slug, form, svcData, repo, { onPrefilled: rebindServiceRows });
  }

  function togglePanels() {
    const multi = currentLayout() === "multi";
    if (singleHint) singleHint.hidden = multi;
    if (multiPanel) multiPanel.hidden = !multi;
    if (multi && tbody && !tbody.querySelector("tr")) {
      tbody.innerHTML = template
        .map(function (s, idx) {
          return buildServiceTableRowHtml(s, idx);
        })
        .join("");
    }
    if (multi) {
      template.forEach(function (_s, idx) {
        bindCtxSelect(idx);
        bindResModeSelect(idx);
      });
      refreshGitHubDirs({ silent: true });
    }
    bindResModeSelect("app");
    refreshPreview();
    scheduleCrosscheck();
  }

  form.querySelectorAll('input[name="layout"]').forEach(function (el) {
    el.onchange = function () {
      state.pipelineLayoutUserTouched = true;
      togglePanels();
    };
  });
  if (refreshDirsBtn) {
    refreshDirsBtn.onclick = function () {
      refreshGitHubDirs({ silent: false });
    };
  }
  if (tbody) {
    tbody.addEventListener("change", function () {
      state.pipelineServicesUserTouched = true;
      refreshPreview();
    });
    tbody.addEventListener("input", function () {
      state.pipelineServicesUserTouched = true;
      refreshPreview();
    });
    listServiceRows().forEach(function (tr) {
      bindRemoveRow(tr.getAttribute("data-svc-idx"));
    });
  }
  const addRowBtn = document.getElementById("project-services-add-row");
  if (addRowBtn) {
    addRowBtn.onclick = function () {
      appendServiceRow({ name: "worker", build_context: ".", expose_ingress: false, ingress_path: "-" });
    };
  }
  template.forEach(function (_s, idx) {
    bindCtxSelect(String(idx));
    bindRemoveRow(String(idx));
  });
  rebindServiceRows();
  if (currentLayout() === "multi") {
    refreshGitHubDirs({ silent: true });
  }
  scheduleCrosscheck();

  const repoSel = form.querySelector('[name="repo"]');
  const branchSel = form.querySelector('[name="branch"]');
  if (repoSel) {
    repoSel.onchange = function () {
      const parsed = parseGitHubRepoValue(repoSel.value);
      if (branchSel) {
        if (!parsed) {
          branchSel.innerHTML = '<option value="main" selected>main</option>';
        } else {
          const opt = repoSel.options[repoSel.selectedIndex];
          const defBranch = (opt && opt.dataset.branch) || "main";
          loadGitHubBranchSelect(branchSel, parsed.owner, parsed.repo, defBranch).then(scheduleCrosscheck);
          return;
        }
      }
      scheduleCrosscheck();
    };
  }
  if (branchSel) {
    branchSel.onchange = scheduleCrosscheck;
  }
  if (tbody) {
    tbody.addEventListener("change", scheduleCrosscheck);
  }

  bindApplyConventionsButton(main, slug, function () {
    pageProjectHub(main, slug, "deploy");
  });

  const syncContractBtn = document.getElementById("sync-services-contract");
  state._repoDetectSyncHandler = async function () {
    const btn = document.getElementById("sync-services-contract");
    if (!btn) return;
    btn.disabled = true;
    try {
      const parsed = parseRepoFromForm(form);
      if (ghStatus.connected && parsed) {
        const fd = new FormData(form);
        await runGitHubPipelineSetup(slug, {
          owner: parsed.owner,
          repo: parsed.repo,
          branch: fd.get("branch") || selectedGitHubBranch(repo.branch || "main"),
          environment: (fd.get("environment") || "dev").toString(),
          apply_repo_contract: true,
          progressEl: document.getElementById("github-setup-progress"),
          submitBtn: document.getElementById("github-setup-submit"),
          formRoot: form,
        });
        toastSuccess("Đã áp dụng services.yaml + đồng bộ workflow");
        pageProjectHub(main, slug, "deploy");
      } else {
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services/sync-from-repo", {
          method: "POST",
          body: { branch: selectedGitHubBranch(repo.branch || "main") },
        });
        toastSuccess("Đã áp dụng services.yaml — chọn repo và bấm Lưu & đồng bộ GitHub");
        pageProjectHub(main, slug, "deploy");
      }
    } catch (err) {
      toastError(errorMessage(err));
    } finally {
      btn.disabled = false;
    }
  };
  if (syncContractBtn) {
    syncContractBtn.onclick = state._repoDetectSyncHandler;
  }

  const changeLayoutBtn = document.getElementById("open-change-layout-wizard");
  if (changeLayoutBtn) {
    changeLayoutBtn.onclick = async function () {
      const current = currentLayout();
      const targetLabel = current === "multi" ? "Một website" : "Web + API riêng";
      const ok = await uiConfirm({
        title: "Đổi kiểu chạy",
        message: "Đổi topology trên cluster — không phải rollback về bản cũ.",
        details: [
          "Hiện tại: " + layoutKindLabel(current),
          "Sẽ chuyển sang: " + targetLabel,
          "Bước tiếp: Lưu & đồng bộ GitHub → deploy bản mới",
          "Deploy lại trong lịch sử chỉ hoạt động cùng kiểu chạy",
        ],
        confirmText: "Chuyển sang " + targetLabel,
      });
      if (!ok) return;
      const newLayout = current === "multi" ? "single" : "multi";
      const radio = form.querySelector('input[name="layout"][value="' + newLayout + '"]');
      if (radio) {
        radio.checked = true;
        state.pipelineLayoutUserTouched = true;
        togglePanels();
      }
      let payload = collectProjectLayoutPayload(form);
      if (newLayout === "multi" && (!payload.services || payload.services.length < 2)) {
        payload = { layout: "multi", services: defaultMultiTemplate(svcData) };
      } else if (newLayout === "single") {
        const cur = collectProjectLayoutPayload(form);
        payload = { layout: "single", services: cur.services && cur.services.length ? cur.services : [{ name: "app", resources_mode: "platform" }] };
      }
      try {
        changeLayoutBtn.disabled = true;
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services", {
          method: "PUT",
          body: Object.assign({ branch: selectedGitHubBranch(repo.branch || "main") }, payload),
        });
        toastSuccess("Đã đổi kiểu — bắt buộc bấm 「Lưu & đồng bộ GitHub」 rồi push/build lại (workflow cũ vẫn build kiểu cũ)");
        pageProjectHub(main, slug, "deploy");
      } catch (err) {
        toastError(errorMessage(err));
      } finally {
        changeLayoutBtn.disabled = false;
      }
    };
  }

  const draftBtn = document.getElementById("pipeline-save-draft");
  if (draftBtn) {
    draftBtn.onclick = async function () {
      const layoutPayload = collectProjectLayoutPayload(form);
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/services", {
          method: "PUT",
          body: Object.assign({ branch: selectedGitHubBranch(repo.branch || "main") }, layoutPayload),
        });
        if (res.convention_seeds && res.convention_seeds.length) {
          toastSuccess("Đã lưu nháp — đã gợi ý env · bấm Lưu & đồng bộ GitHub");
        } else {
          toastSuccess(res.hint || "Đã lưu Console — bấm Lưu & đồng bộ GitHub để push workflow");
        }
        scheduleCrosscheck();
      } catch (err) {
        toastError(errorMessage(err));
      }
    };
  }

  form.onsubmit = async function (e) {
    e.preventDefault();
    const fd = new FormData(form);
    const full = (fd.get("repo") || "").toString();
    const parts = full.split("/");
    if (parts.length < 2) {
      toastError("Chọn repository");
      return;
    }
    const submitBtn = document.getElementById("github-setup-submit");
    const progress = document.getElementById("github-setup-progress");
    try {
      await runGitHubPipelineSetup(slug, {
        owner: parts[0],
        repo: parts[1],
        branch: fd.get("branch"),
        environment: fd.get("environment"),
        layoutPayload: collectProjectLayoutPayload(form),
        progressEl: progress,
        submitBtn: submitBtn,
        formRoot: form,
      });
      toastSuccess("Pipeline sẵn sàng — theo dõi build bên dưới");
      const activity = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env })
      ).catch(function () { return { items: [] }; });
      let readiness = null;
      if (env === "dev" && canManagePlatformProjects()) {
        readiness = await api(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote-readiness"
        ).catch(function () { return null; });
      }
      updateDeployActivityDOM(activity, slug, readiness, env);
      bindDeployActivityPoll(slug, env, navToken);
      const actCard = document.getElementById("deploy-activity-card");
      if (actCard) actCard.scrollIntoView({ behavior: "smooth", block: "start" });
      scheduleCrosscheck();
    } catch (err) {
      toastError(errorMessage(err, "Đồng bộ pipeline thất bại"));
    }
  };
}

async function pageProjectHub(main, slug, tab) {
  tab = tab || "overview";
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

  if (tab === "overview") {
    const ov = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/overview" + projectQs());
    const dev = ov.dev || {};
    const prod = ov.prod || {};
    const mon = ov.monitoring || {};
    let grafanaBase = mon.grafana_url || "";
    if (!grafanaBase) {
      const infra = await api("/api/v1/infra/links").catch(function () { return { items: [] }; });
      const g = (infra.items || []).find(function (x) { return x && x.key === "grafana" && x.url; });
      grafanaBase = g && g.url ? g.url : "";
    }
    const monDevUrl = mon.dev_dashboard_url || grafanaNamespaceDashboardUrl(grafanaBase, p.namespace_dev);
    const monProdUrl = mon.prod_dashboard_url || grafanaNamespaceDashboardUrl(grafanaBase, p.namespace_prod);
    let monHtml = "";
    if (grafanaBase && (monDevUrl || monProdUrl)) {
      monHtml =
        '<div class="card detail-card"><h3>Monitoring</h3><p class="muted">Metric theo namespace trên Grafana (đăng nhập qua card Hạ tầng nếu cần).</p><div class="meta-chips">' +
        (monDevUrl
          ? '<a class="chip chip-link" href="' + esc(monDevUrl) + '" target="_blank" rel="noopener">Grafana · Dev</a>'
          : "") +
        (monProdUrl
          ? '<a class="chip chip-link" href="' + esc(monProdUrl) + '" target="_blank" rel="noopener">Grafana · Prod</a>'
          : "") +
        "</div></div>";
    }
    main.innerHTML =
      projectHeader(p, p.description || "Tổng quan project") +
      '<div class="stat-grid">' +
      statBox(dev.pods ?? "—", "Pods (dev)", "g1") +
      statBox(dev.deployments ?? "—", "Deployments (dev)", "g2") +
      statBox(prod.pods ?? "—", "Pods (prod)", "g3") +
      statBox(prod.deployments ?? "—", "Deployments (prod)", "g4") +
      "</div>" +
      '<div class="card detail-card"><h3>Namespaces</h3><div class="meta-chips">' +
      chip("Dev", p.namespace_dev) +
      chip("Prod", p.namespace_prod) +
      (p.registry && p.registry.image_prefix ? chip(p.registry.label || "Registry", p.registry.image_prefix) : "") +
      "</div></div>" +
      monHtml +
      '<div class="card detail-card"><p class="muted">Pipeline Git → image → deploy: cấu hình tại tab <strong>Deploy / Git</strong>, theo dõi workload tại <strong>Runtime</strong>.</p></div>';
    return;
  }

  if (tab === "monitoring") {
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
      projectHeader(p, "Monitoring theo namespace (Dev/Prod)") +
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
      '<div class="card detail-card"><h3>Namespace map</h3><div class="meta-chips">' +
      chip("Dev", p.namespace_dev) +
      chip("Prod", p.namespace_prod) +
      "</div></div>";
    bindInteractiveCharts(main);
    return;
  }

  if (tab === "runtime") {
    state.namespace = ns;
    localStorage.setItem("filter-ns", ns);
    const pods = await api("/api/v1/k8s/pods" + qs({ namespace: ns, limit: 50 }));
    const deps = await api("/api/v1/k8s/deployments" + qs({ namespace: ns, limit: 50 }));
    main.innerHTML =
      projectHeader(p, "Runtime · workload đang chạy") +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "runtime"); }) +
      '<div class="card"><h3>Deployments</h3>' +
      renderTable(
        [
          { key: "name", label: "Name" },
          { key: "replicas", label: "Replicas" },
          { key: "status", label: "Status" },
        ],
        deps.items || [],
        "deployments"
      ) +
      "</div>" +
      '<div class="card" style="margin-top:16px"><h3>Pods</h3>' +
      renderTable(
        [
          { key: "name", label: "Name" },
          { key: "status", label: "Status" },
          { key: "restarts", label: "Restarts" },
          { key: "node", label: "Node" },
        ],
        pods.items || [],
        "pods"
      ) +
      "</div>";
    return;
  }

  if (PROJECT_WORKLOADS.some(function (w) { return w.tab === tab; })) {
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
    return;
  }

  if (tab === "deploy-history") {
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
    return;
  }

  if (tab === "promote") {
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
    return;
  }

  if (tab === "deploy") {
    try {
    const navToken = state.navToken;
    const repo = data.repo || {};
    const reg = p.registry || {};
    const env = state.projectEnv || "dev";

    const hashQ = (location.hash.split("?")[1] || "");
    if (hashQ.indexOf("github=connected") >= 0) {
      toastSuccess("Đã kết nối GitHub");
      location.hash = "#/project/" + slug + "/deploy";
    }

    const ghStatusP = api("/api/v1/github/status").catch(function () {
      return { enabled: false, connected: false };
    });
    const planP = api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/plan" + projectQs({ environment: env })
    ).catch(function (err) {
      return { error: err.message };
    });
    const svcP = api("/api/v1/projects/" + encodeURIComponent(slug) + "/services").catch(function () {
      return { layout: "single", items: [] };
    });
    const gitopsPubP = api("/api/v1/gitops/public").catch(function () { return {}; });
    const gitopsStatusP = api("/api/v1/projects/" + encodeURIComponent(slug) + "/gitops/status").catch(function () { return {}; });
    const [ghStatus, plan, svcData, gitopsPub, gitopsStatus] = await Promise.all([ghStatusP, planP, svcP, gitopsPubP, gitopsStatusP]);

    const stepsHtml = (plan.steps || [])
      .map(function (s) {
        return "<li>" + esc(s) + "</li>";
      })
      .join("");
    const secretsHtml = (plan.workflow && plan.workflow.secrets_hint || [])
      .map(function (s) {
        return "<li><code>" + esc(s) + "</code></li>";
      })
      .join("");

    const ghReposPlaceholder = ghStatus.connected ? { items: null } : { items: [] };

    main.innerHTML =
      projectHeader(p, "Deploy / Git", { help: "deploy" }) +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "deploy"); }) +
      renderGitOpsProjectCard(slug, gitopsPub, gitopsStatus, canWriteK8s()) +
      renderPipelineSetupCard(slug, svcData, repo, ghStatus, ghReposPlaceholder, canWriteK8s()) +
      '<div class="card" style="margin-bottom:16px"><h3>Tóm tắt</h3>' +
      '<div class="meta-chips">' +
      chip(reg.label || p.registry_provider || "GHCR", reg.provider || p.registry_provider) +
      chip("Môi trường", env) +
      chip("Namespace", plan.namespace || ns) +
      (reg.ready || plan.registry_ready ? '<span class="badge ok">Registry OK</span>' : '<span class="badge warn">Registry chưa sẵn sàng</span>') +
      "</div>" +
      (plan.image ? '<p class="muted" style="margin-top:8px">Image: <code>' + esc(plan.image) + "</code></p>" : "") +
      (repo.auto_deploy_enabled
        ? '<p class="muted" style="margin-top:6px">Auto-deploy · branch <code>' + esc(repo.branch || "main") + "</code></p>"
        : "") +
      "</div>" +
      renderDeployActivityCard({ loading: true }) +
      '<div class="card" style="margin-bottom:16px"><h3>Deploy thủ công (fallback)</h3>' +
      '<p class="muted">Dùng khi cần hotfix nhanh hoặc muốn deploy lại tag cụ thể.</p>' +
      '<form id="deploy-apply-form" class="login-form" style="max-width:420px">' +
      '<label>Image tag<input name="image_tag" value="latest" placeholder="latest hoặc git sha" /></label>' +
      (plan.can_apply && canWriteK8s()
        ? '<button type="submit" class="btn-primary" style="margin-top:12px">Deploy ngay</button>'
        : '<p class="muted" style="margin-top:12px">' +
          (plan.rancher_ready
            ? "Bạn không có quyền deploy."
            : "Rancher chưa sẵn sàng — bật addon Rancher và cài cluster trước.") +
          "</p>") +
      "</form></div>" +
      (plan.error
        ? '<div class="card"><p class="error-text">' + esc(plan.error) + "</p></div>"
        : '<details class="card" style="margin-bottom:16px"><summary style="cursor:pointer"><strong>Nâng cao (Git config, workflow, manifest)</strong></summary>' +
          '<div style="margin-top:12px">' +
          '<form id="project-repo-form" class="login-form" style="max-width:560px">' +
          '<label>Git URL<input name="git_url" type="url" value="' + esc(repo.git_url || "") + '" placeholder="https://github.com/org/repo" /></label>' +
          '<div class="form-row"><label>Branch<input name="branch" value="' + esc(repo.branch || "main") + '" /></label></div>' +
          '<label>Dockerfile (để quét)<input name="dockerfile_path" value="' +
          esc(repo.dockerfile_path || "Dockerfile") +
          '" placeholder="Dockerfile" /><span class="muted" style="font-size:12px;display:block;margin-top:4px">Platform ưu tiên file này, rồi <code>Dockerfile</code>, <code>docker/Dockerfile</code>.</span></label>' +
          '<label>Build context<input name="build_context" value="' + esc(repo.build_context || ".") + '" /></label>' +
          (canWriteK8s()
            ? '<button type="submit" class="btn-primary">Lưu cấu hình</button>'
            : '<p class="muted">Read-only — không chỉnh sửa được.</p>') +
          "</form>" +
          '<ol class="deploy-steps">' + (stepsHtml || "<li>Chưa có bước</li>") + "</ol>" +
          (secretsHtml ? '<p class="muted">Secrets GitHub Actions:</p><ul>' + secretsHtml + "</ul>" : "") +
          (plan.workflow && plan.workflow.content
            ? snippetBlock(
                "deploy-wf-" + slug,
                "GitHub Actions — " + (plan.workflow.filename || "workflow.yml"),
                plan.workflow.content,
                "Copy workflow"
              )
            : "") +
          (plan.manifest && plan.manifest.yaml
            ? snippetBlock(
                "deploy-manifest-" + slug,
                "Kubernetes — " + (plan.manifest.filename || "manifest.yaml"),
                plan.manifest.yaml,
                "Copy manifest"
              )
            : "") +
          ((plan.manifests || []).length > 1
            ? plan.manifests
                .slice(1)
                .map(function (m, i) {
                  return m && m.yaml
                    ? snippetBlock(
                        "deploy-manifest-" + slug + "-" + i,
                        "Kubernetes — " + (m.filename || "manifest-" + i),
                        m.yaml,
                        "Copy manifest"
                      )
                    : "";
                })
                .join("")
            : "") +
          "</div></details>");

    bindSnippetCopyButtons(main);
    bindPipelineSetupForm(main, slug, svcData, repo, ghStatus, env, navToken);
    bindGitOpsProjectCard(main, slug);
    bindDeployHelpTriggers(main);

    Promise.all([
      ghStatus.connected
        ? api("/api/v1/github/repos").catch(function () { return { items: [] }; })
        : Promise.resolve({ items: [] }),
      api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/activity" + projectQs({ environment: env, scope: "current" })
      ).catch(function () {
        return { items: [] };
      }),
    ]).then(function (results) {
      if (!isNavTokenActive(navToken)) return;
      const ghRepos = results[0];
      const activity = results[1];
      const repoSel = document.getElementById("github-repo-select");
      const branchSel = document.getElementById("github-branch-select");
      if (repoSel) {
        repoSel.innerHTML =
          '<option value="">— chọn repo —</option>' + githubRepoOptionsHtml(repo, ghRepos);
      }
      const linked =
        repo.github_owner && repo.github_repo
          ? { owner: repo.github_owner, repo: repo.github_repo }
          : parseGitHubRepoValue(repoSel && repoSel.value);
      if (branchSel && linked) {
        loadGitHubBranchSelect(branchSel, linked.owner, linked.repo, repo.branch || "main").then(function () {
          const pipelineForm = document.getElementById("pipeline-setup-form");
          if (pipelineForm) refreshPipelineCrosscheck(slug, pipelineForm, svcData, repo);
        });
      }
      updateDeployActivityDOM(activity, slug, undefined, env, {
        showHistory: false,
        showPromotePrep: false,
        showPromoteBar: false,
      });
      if (promoteFollowActive(slug, env)) {
        scrollToDeployProgress();
        handlePromoteFollowTerminal(activity, slug);
      }
      bindDeployActivityPoll(slug, env, navToken);
    });

    const ghConnect = document.getElementById("github-connect-btn");
    if (ghConnect) {
      ghConnect.onclick = function () {
        const oauthURL =
          "/api/v1/github/oauth/start?popup=1&return=" +
          encodeURIComponent("#/project/" + slug + "/deploy");
        const pop = window.open(
          oauthURL,
          "github-oauth",
          "width=560,height=740,menubar=no,toolbar=no,location=yes,status=no"
        );
        if (!pop) {
          toastError("Trình duyệt chặn popup — cho phép popup rồi thử lại");
          return;
        }
        const onMsg = function (ev) {
          if (ev.origin !== window.location.origin) return;
          const data = ev.data || {};
          if (data.type !== "github_oauth") return;
          window.removeEventListener("message", onMsg);
          if (data.status === "connected") {
            toastSuccess("Đã kết nối GitHub");
          } else if (data.status === "login_required") {
            toastError("Phiên đăng nhập hết hạn — đăng nhập lại");
          } else {
            toastError("Kết nối GitHub thất bại");
          }
          pageProjectHub(main, slug, "deploy");
        };
        window.addEventListener("message", onMsg);
      };
    }
    const ghRepoSel = document.getElementById("github-repo-select");
    const ghBranchSel = document.getElementById("github-branch-select");
    if (ghRepoSel && ghBranchSel && !document.getElementById("pipeline-setup-form")) {
      ghRepoSel.onchange = function () {
        const parsed = parseGitHubRepoValue(ghRepoSel.value);
        if (!parsed) {
          ghBranchSel.innerHTML = '<option value="main" selected>main</option>';
          return;
        }
        const opt = ghRepoSel.options[ghRepoSel.selectedIndex];
        const defBranch = (opt && opt.dataset.branch) || "main";
        loadGitHubBranchSelect(ghBranchSel, parsed.owner, parsed.repo, defBranch);
      };
      if (!ghBranchSel.options.length || ghBranchSel.options[0].textContent.indexOf("Đang tải") >= 0) {
        const parsed =
          parseGitHubRepoValue(ghRepoSel.value) ||
          (repo.github_owner && repo.github_repo
            ? { owner: repo.github_owner, repo: repo.github_repo }
            : null);
        if (parsed) {
          loadGitHubBranchSelect(ghBranchSel, parsed.owner, parsed.repo, repo.branch || "main");
        }
      }
    }
    const ghDisc = document.getElementById("github-disconnect-btn");
    if (ghDisc) {
      ghDisc.onclick = async function () {
        if (!(await uiConfirm("Ngắt kết nối GitHub?", { title: "GitHub" }))) return;
        await api("/api/v1/github/disconnect", { method: "DELETE" });
        pageProjectHub(main, slug, "deploy");
      };
    }
    const autoToggle = document.getElementById("auto-deploy-toggle");
    if (autoToggle) {
      autoToggle.onchange = async function () {
        const enabled = autoToggle.checked;
        autoToggle.disabled = true;
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/repo/auto-deploy", {
            method: "PATCH",
            body: { enabled: enabled },
          });
          const badge = document.getElementById("auto-deploy-badge");
          if (badge) {
            badge.textContent = enabled ? "Auto-deploy bật" : "Auto-deploy tắt";
            badge.className = "badge " + (enabled ? "ok" : "warn");
          }
          toastSuccess(enabled ? "Đã bật auto-deploy" : "Đã tắt auto-deploy — build vẫn chạy, không deploy cluster");
        } catch (err) {
          autoToggle.checked = !enabled;
          toastError(err.message);
        } finally {
          autoToggle.disabled = false;
        }
      };
    }

    const repoFormEl = document.getElementById("project-repo-form");
    if (repoFormEl && canWriteK8s()) {
      repoFormEl.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(repoFormEl);
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/repo", {
          method: "PATCH",
          body: {
            git_url: fd.get("git_url"),
            branch: fd.get("branch"),
            dockerfile_path: fd.get("dockerfile_path"),
            build_context: fd.get("build_context"),
          },
        });
        toastSuccess("Đã lưu cấu hình Git");
        pageProjectHub(main, slug, "deploy");
      };
    }

    const applyForm = document.getElementById("deploy-apply-form");
    if (applyForm && plan.can_apply && canWriteK8s()) {
      applyForm.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(applyForm);
        const tag = (fd.get("image_tag") || "latest").toString().trim() || "latest";
        if (!(await uiConfirm("Deploy image vào " + (plan.namespace || ns) + "?", { title: "Deploy workload" }))) {
          return;
        }
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/apply", {
            method: "POST",
            body: { environment: env, image_tag: tag },
          });
          toastSuccess("Đã deploy — xem tab Runtime");
          pageProjectHub(main, slug, "runtime");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    bindEnvSuggestButtons(main, slug, env);
    } catch (err) {
      main.innerHTML =
        projectHeader(p, "Deploy / Git", { help: "deploy" }) +
        '<div class="card"><p class="error-text">Lỗi: ' +
        esc(errorMessage(err, "Không tải được trang Deploy")) +
        '</p><button type="button" class="btn-ghost btn-sm" onclick="location.reload()">Tải lại</button></div>';
    }
    return;
  }

  if (tab === "env") {
    const envRes = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/env" + qs({ environment: env })
    ).catch(function () { return { items: [] }; });
    const convP = api("/api/v1/projects/" + encodeURIComponent(slug) + "/conventions").catch(function () {
      return { enabled: false };
    });
    const buildReadyP =
      env === "prod"
        ? Promise.resolve(null)
        : api(
            "/api/v1/projects/" + encodeURIComponent(slug) + "/env/readiness" + qs({ environment: env, scope: "build" })
          ).catch(function () { return null; });
    const runtimeReadyP = api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/env/readiness" + qs({ environment: env, scope: "runtime" })
    ).catch(function () { return null; });
    const envSyncP = api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/env/sync-status" + qs({ environment: env })
    ).catch(function () { return null; });
    const buildReady = await buildReadyP;
    const runtimeReady = await runtimeReadyP;
    const envSyncStatus = await envSyncP;
    const conventions = await convP;
    const envItems = envRes.items || [];
    const runtimeItems = envItems.filter(envVarIsRuntimeScope);
    const buildItems = envItems.filter(envVarIsBuildScope);
    const buildSuggestions = (buildReady && buildReady.suggestions) || [];
    const runtimeSuggestions = (runtimeReady && runtimeReady.suggestions) || [];
    const canEditEnv = canWriteK8s() && (env !== "prod" || state.user.role === "admin" || state.user.role === "tech_lead");
    const runtimeRows = renderEnvVarTable(runtimeItems, slug, env, canEditEnv, "runtime");
    const buildRows = renderEnvVarTable(buildItems, slug, env, canEditEnv, "build");
    const runtimeTableBody =
      renderMissingContractRows(runtimeSuggestions, "runtime") +
      (runtimeRows ||
        (runtimeSuggestions.length
          ? ""
          : '<tr><td colspan="3" class="muted">Chưa có — ví dụ <code>APP_GREETING</code></td></tr>'));
    const buildTableBody =
      renderMissingContractRows(buildSuggestions, "build") +
      (buildRows ||
        (buildSuggestions.length
          ? ""
          : '<tr><td colspan="3" class="muted">Chưa có biến trên Console — bấm <strong>+ Thêm</strong> hoặc <strong>Lấy key từ contract</strong></td></tr>')) +
      renderPlatformBuildArgRows();
    const runtimeHeadBtns =
      (canEditEnv && runtimeSuggestions.length
        ? '<button type="button" class="btn-ghost btn-sm" id="env-contract-runtime-keys">Lấy key từ contract</button> '
        : "") +
      (canEditEnv ? '<button type="button" class="btn-primary btn-sm" id="open-add-runtime-env">+ Thêm</button>' : "");
    const buildHeadBtns =
      env === "prod"
        ? '<a class="btn-primary btn-sm" href="#/project/' + esc(slug) + '/env" id="env-go-dev-build">Cấu hình build tại Dev →</a>'
        : (canEditEnv && buildSuggestions.length
            ? '<button type="button" class="btn-ghost btn-sm" id="env-contract-build-keys">Lấy key từ contract</button> '
            : "") +
          (canEditEnv ? '<button type="button" class="btn-primary btn-sm" id="open-add-build-env">+ Thêm</button>' : "");
    const buildCardHtml =
      env === "prod"
        ? '<div class="card env-vars-card" style="margin-top:16px">' +
          '<div class="env-vars-head"><div><h3>Khi build image</h3>' +
          '<p class="muted">Biến build (<code>ARG</code> Dockerfile) chỉ cấu hình trên <strong>Dev</strong>. Promote prod tái sử dụng <strong>cùng image</strong> đã build từ dev — không cần khai báo lại trên prod.</p></div>' +
          '<div class="env-vars-head-actions">' + buildHeadBtns + "</div></div></div>"
        : '<div class="card env-vars-card" style="margin-top:16px">' +
          renderEnvReadinessPanel(buildReady, slug, env, "build") +
          '<div class="env-vars-head"><div><h3>Khi build image</h3>' +
          '<p class="muted">Truyền vào Dockerfile (<code>ARG</code>) lúc GitHub Actions build. Đổi phải <strong>push lại</strong> để image mới. Promote prod dùng cùng image.</p></div>' +
          '<div class="env-vars-head-actions">' + buildHeadBtns + "</div>" +
          "</div>" +
          (canEditEnv
            ? '<div class="toolbar" style="margin:12px 0"><button type="button" class="btn-ghost btn-sm" id="env-sync-workflow-btn">Đồng bộ workflow GitHub</button></div>'
            : "") +
          '<div class="table-wrap"><table><thead><tr><th>Key</th><th>Value</th><th></th></tr></thead><tbody>' +
          buildTableBody +
          "</tbody></table></div>" +
          '<p class="muted env-hint">Biến <code>platform</code> do hệ thống inject — không cần thêm trên Console. Contract bắt buộc: <code>.platform/build.yaml</code></p></div>';
    main.innerHTML =
      projectHeader(p, "Cấu hình app") +
      projectEnvToolbar(slug, p, function () { pageProjectHub(main, slug, "env"); }) +
      (conventions.enabled ? renderBackFrontConventionBanner(conventions, canEditEnv) : "") +
      '<div class="card env-vars-card">' +
      renderEnvReadinessPanel(runtimeReady, slug, env, "runtime") +
      '<div class="env-vars-head"><div><h3>Khi app chạy (Pod)</h3>' +
      '<p class="muted">Thay file <code>.env</code> — inject Secret <code>app-env</code>, đổi là restart pod (không build lại).</p></div>' +
      '<div class="env-vars-head-actions">' + runtimeHeadBtns + "</div>" +
      "</div>" +
      (canEditEnv
        ? '<div class="toolbar" style="margin:12px 0"><button type="button" class="btn-ghost btn-sm" id="env-sync-btn">Đồng bộ cluster &amp; restart pod</button></div>'
        : "") +
      renderEnvSyncNote(envSyncStatus) +
      '<div class="table-wrap"><table><thead><tr><th>Key</th><th>Value</th><th></th></tr></thead><tbody>' +
      runtimeTableBody +
      "</tbody></table></div></div>" +
      buildCardHtml +
      '<p class="muted env-hint" style="margin-top:8px">Runtime bắt buộc: <code>.platform/runtime.yaml</code></p>';

    const addRuntime = document.getElementById("open-add-runtime-env");
    if (addRuntime) addRuntime.onclick = function () { openEnvVarDialog(slug, env, { scope: "runtime" }); };
    const addBuild = document.getElementById("open-add-build-env");
    if (addBuild) addBuild.onclick = function () { openEnvVarDialog(slug, env, { scope: "build" }); };
    const goDevBuild = document.getElementById("env-go-dev-build");
    if (goDevBuild) {
      goDevBuild.onclick = function () {
        state.projectEnv = "dev";
        localStorage.setItem("project-env", "dev");
      };
    }
    const contractBuildBtn = document.getElementById("env-contract-build-keys");
    if (contractBuildBtn) {
      contractBuildBtn.onclick = function () { promptContractKeys(slug, env, buildSuggestions, "build"); };
    }
    const contractRuntimeBtn = document.getElementById("env-contract-runtime-keys");
    if (contractRuntimeBtn) {
      contractRuntimeBtn.onclick = function () { promptContractKeys(slug, env, runtimeSuggestions, "runtime"); };
    }
    const syncBtn = document.getElementById("env-sync-btn");
    if (syncBtn) {
      syncBtn.onclick = async function () {
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/sync" + qs({ environment: env }), { method: "POST" });
          toastSuccess("Đã đồng bộ lên cluster");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    const syncWf = document.getElementById("env-sync-workflow-btn");
    if (syncWf) {
      syncWf.onclick = async function () {
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/env/sync-workflow", { method: "POST" });
          toastSuccess("Đã cập nhật workflow + build-args trên GitHub");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    bindEnvVarTableActions(main, slug, env);
    bindEnvSuggestButtons(main, slug, env);
    bindApplyConventionsButton(main, slug, function () {
      pageProjectHub(main, slug, "env");
    });
    return;
  }

  if (tab === "domains") {
    const domRes = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/domains" + qs()).catch(function () {
      return { items: data.domains || [] };
    });
    const domains = domRes.items || [];
    let drows = domains
      .map(function (d) {
        const urlCell = d.url
          ? '<a href="' + esc(d.url) + '" target="_blank" rel="noopener">' + esc(d.hostname) + "</a>"
          : esc(d.hostname);
        return (
          "<tr><td>" +
          urlCell +
          "</td><td>" +
          esc(d.environment) +
          "</td><td>" +
          domainKindBadge(d.kind) +
          "</td><td>" +
          domainSyncBadge(d.sync_status) +
          (d.sync_error ? '<br><span class="muted">' + esc(d.sync_error) + "</span>" : "") +
          "</td><td>" +
          domainCertBadge(d.cert_status, d.tls_enabled) +
          "</td><td>" +
          (canManage
            ? '<button type="button" class="btn-sm domain-sync" data-id="' +
              d.id +
              '">Sync</button> ' +
              (d.kind !== "auto"
                ? '<button type="button" class="btn-sm btn-danger domain-del" data-id="' + d.id + '">Xóa</button>'
                : "")
            : "") +
          "</td></tr>" +
          (d.kind === "custom" && d.dns
            ? '<tr class="dns-row"><td colspan="6">' + renderDNSHint(d.dns) + "</td></tr>"
            : d.kind === "auto" && d.dns
              ? '<tr class="dns-row"><td colspan="6">' + renderDNSHint(d.dns) + "</td></tr>"
              : "")
        );
      })
      .join("");
    main.innerHTML =
      projectHeader(p, "Domains · URL & Ingress") +
      '<div class="card"><h3>URL truy cập app</h3>' +
      '<p class="muted">Domain <strong>Tự động</strong> dùng ngay (sslip / subdomain platform). <strong>Custom</strong> cần cấu hình DNS trỏ về cluster.</p>' +
      '<p class="muted deploy-help-note-optional" style="margin-top:8px;font-size:12px">Mỗi hostname chỉ gắn <strong>một project</strong> trên toàn platform — project khác dùng cùng tên sẽ bị chặn. Cùng project có thể thêm <strong>nhiều domain khác tên</strong> trên một env (dev/prod).</p>' +
      (canManage
        ? '<form id="add-domain-form" class="login-form" style="max-width:520px;margin-bottom:16px">' +
          '<div class="form-row"><label>Custom hostname<input name="hostname" required placeholder="api.congty.com" /></label>' +
          '<label>Env<select name="environment"><option value="dev">dev</option><option value="prod">prod</option></select></label></div>' +
          '<button type="submit" class="btn-primary">Thêm & đồng bộ Ingress</button></form>'
        : "") +
      '<div class="table-wrap"><table><thead><tr><th>Hostname</th><th>Env</th><th>Loại</th><th>Ingress</th><th>TLS</th><th></th></tr></thead><tbody>' +
      (drows || '<tr><td colspan="6" class="muted">Chưa có domain — tạo project sẽ có URL dev/prod tự động</td></tr>') +
      "</tbody></table></div></div>";
    const addDom = document.getElementById("add-domain-form");
    if (addDom) {
      addDom.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(addDom);
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug) + "/domains", {
            method: "POST",
            body: { hostname: fd.get("hostname"), environment: fd.get("environment") },
          });
          toastSuccess("Đã thêm domain và đồng bộ Ingress");
          pageProjectHub(main, slug, "domains");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    main.querySelectorAll(".domain-sync").forEach(function (btn) {
      btn.onclick = async function () {
        try {
          await api(
            "/api/v1/projects/" + encodeURIComponent(slug) + "/domains/" + btn.dataset.id + "/sync" + qs(),
            { method: "POST" }
          );
          toastSuccess("Đã đồng bộ Ingress");
          pageProjectHub(main, slug, "domains");
        } catch (err) {
          toastError(err.message);
        }
      };
    });
    main.querySelectorAll(".domain-del").forEach(function (btn) {
      btn.onclick = async function () {
        if (!(await uiConfirm("Xóa domain này?", { danger: true, title: "Xóa domain" }))) return;
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/domains/" + btn.dataset.id, { method: "DELETE" });
        pageProjectHub(main, slug, "domains");
      };
    });
    return;
  }

  if (tab === "settings") {
    const members = data.members || [];
    const providersRes = canManage ? await api("/api/v1/registry/providers").catch(function () { return { items: [] }; }) : { items: [] };
    const providers = providersRes.items || [];
    const users = canManage ? await api("/api/v1/team/users").catch(() => ({ items: [] })) : { items: [] };
    let mrows = members
      .map(function (m) {
        return (
          "<tr><td>" + esc(m.email) + "</td><td>" + esc(m.display_name || "—") + "</td><td>" + esc(m.role) + "</td>" +
          (canManage && m.role !== "owner"
            ? '<td><button type="button" class="btn-sm btn-danger mem-del" data-id="' + m.user_id + '">Gỡ</button></td>'
            : "<td></td>") +
          "</tr>"
        );
      })
      .join("");
    const addMemberOpts = (users.items || [])
      .filter(function (u) {
        return !members.some(function (m) { return m.user_id === u.id; });
      })
      .map(function (u) {
        return '<option value="' + u.id + '">' + esc(u.email) + "</option>";
      })
      .join("");
    main.innerHTML =
      projectHeader(p, "Cài đặt · registry & thành viên") +
      (canManage
        ? '<div class="card" style="margin-bottom:16px"><h3>Registry</h3>' +
          '<form id="registry-form" class="login-form" style="max-width:480px">' +
          registrySelectHtml(providers, p.registry_provider, "ghcr") +
          '<button type="submit" class="btn-primary" style="margin-top:12px">Lưu registry</button></form></div>'
        : '<div class="card" style="margin-bottom:16px"><h3>Registry</h3><p class="muted">' +
          esc((p.registry && p.registry.label) || p.registry_provider) +
          (p.registry && p.registry.image_prefix ? " · <code>" + esc(p.registry.image_prefix) + "</code>" : "") +
          "</p></div>") +
      '<div class="card"><h3>Thành viên project</h3>' +
      (canManage && addMemberOpts
        ? '<form id="add-member-form" class="toolbar" style="margin-bottom:12px">' +
          '<select name="user_id">' + addMemberOpts + "</select>" +
          '<select name="role"><option value="dev">dev</option><option value="readonly">readonly</option></select>' +
          '<button type="submit" class="btn-primary">Thêm</button></form>'
        : "") +
      '<div class="table-wrap"><table><thead><tr><th>Email</th><th>Tên</th><th>Role</th><th></th></tr></thead><tbody>' +
      (mrows || '<tr><td colspan="4" class="muted">Chưa có thành viên</td></tr>') +
      "</tbody></table></div></div>";
    const regForm = document.getElementById("registry-form");
    if (regForm) {
      regForm.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(regForm);
        try {
          await api("/api/v1/projects/" + encodeURIComponent(slug), {
            method: "PATCH",
            body: { registry_provider: fd.get("registry_provider") },
          });
          toastSuccess("Đã cập nhật registry");
          pageProjectHub(main, slug, "settings");
        } catch (err) {
          toastError(err.message);
        }
      };
    }
    const addMem = document.getElementById("add-member-form");
    if (addMem) {
      addMem.onsubmit = async function (e) {
        e.preventDefault();
        const fd = new FormData(addMem);
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/members", {
          method: "POST",
          body: { user_id: parseInt(fd.get("user_id"), 10), role: fd.get("role") },
        });
        pageProjectHub(main, slug, "settings");
      };
    }
    main.querySelectorAll(".mem-del").forEach(function (btn) {
      btn.onclick = async function () {
        if (!(await uiConfirm("Gỡ thành viên khỏi project?", { danger: true, title: "Gỡ thành viên" }))) return;
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/members/" + btn.dataset.id, { method: "DELETE" });
        pageProjectHub(main, slug, "settings");
      };
    });
  }
}

function bindInfraCopyButtons(root) {
  if (!root) return;
  root.querySelectorAll("[data-copy]").forEach(function (btn) {
    btn.onclick = function () {
      const text = btn.getAttribute("data-copy") || "";
      if (!text) return;
      navigator.clipboard.writeText(text).then(
        function () { toastSuccess("Đã copy"); },
        function () { toastError("Không copy được"); }
      );
    };
  });
}

function renderInfraLinksCard(links) {
  const items = (links && links.items) || [];
  if (!items.length) return "";
  const rows = items
    .map(function (it) {
      const href = esc(it.login_url || it.url || "#");
      const status = it.enabled
        ? '<span class="badge ok">Kết nối OK</span>'
        : '<span class="badge warn">Chưa cấu hình</span>';
      const cred =
        it.username || it.password
          ? '<div class="infra-creds">' +
            (it.username
              ? '<label class="infra-cred-field">User<input class="infra-cred-input" type="text" readonly value="' +
                esc(it.username) +
                '" /><button type="button" class="btn-ghost btn-sm" data-copy="' +
                esc(it.username) +
                '">Copy</button></label>'
              : "") +
            (it.password
              ? '<label class="infra-cred-field">Pass<input class="infra-cred-input" type="text" readonly value="' +
                esc(it.password) +
                '" /><button type="button" class="btn-ghost btn-sm" data-copy="' +
                esc(it.password) +
                '">Copy</button></label>'
              : '<span class="muted">Pass: chưa cấu hình trên VPS</span>') +
            "</div>"
          : "";
      return (
        '<div class="infra-link-row">' +
        '<div class="infra-link-head"><strong>' +
        esc(it.label) +
        "</strong> " +
        status +
        (it.note ? '<div class="muted" style="font-size:12px;margin-top:2px">' + esc(it.note) + "</div>" : "") +
        "</div>" +
        '<div class="infra-link-actions">' +
        '<a class="btn-primary btn-sm" href="' +
        href +
        '" target="_blank" rel="noopener">Mở ' +
        esc(it.label) +
        "</a>" +
        (it.url && it.url !== it.login_url
          ? '<a class="btn-ghost btn-sm" href="' + esc(it.url) + '" target="_blank" rel="noopener">URL gốc</a>'
          : "") +
        "</div>" +
        cred +
        "</div>"
      );
    })
    .join("");
  return (
    '<div class="card infra-links-card" style="margin-bottom:16px" id="infra-links-card">' +
    "<h3>Công cụ nội bộ</h3>" +
    '<p class="muted">Link Rancher & Harbor kèm tài khoản. <strong>Dùng nút Copy</strong> — đừng gõ tay (dễ sai ký tự).</p>' +
    '<div class="infra-links-grid">' +
    rows +
    "</div></div>"
  );
}

async function pageOverview(main) {
  const navToken = state.navToken;
  main.innerHTML = '<p class="loading">Đang tải Cluster Dashboard…</p>';
  const clusters = await api("/api/v1/rancher/clusters").catch(() => ({ items: [] }));
  const infraLinks = await api("/api/v1/infra/links").catch(function () { return { items: [] }; });
  if (!isNavTokenActive(navToken)) return;
  const clusterItems = clusters.items || [];
  if (!state.clusterId && clusterItems.length) {
    state.clusterId = clusterItems[0].id;
  }

  let d;
  try {
    const ctrl = typeof AbortController !== "undefined" ? new AbortController() : null;
    const timer = ctrl
      ? setTimeout(function () {
          ctrl.abort();
        }, 45000)
      : null;
    d = await api("/api/v1/rancher/cluster/dashboard" + qs(), {
      signal: ctrl ? ctrl.signal : undefined,
    });
    if (timer) clearTimeout(timer);
  } catch (err) {
    if (!isNavTokenActive(navToken)) return;
    const dash = await api("/api/v1/dashboard").catch(function () {
      return null;
    });
    if (!isNavTokenActive(navToken)) return;
    main.innerHTML = renderInfraLinksCard(infraLinks) + renderOverviewFallback(err.message || String(err), dash, clusterItems);
    bindInfraCopyButtons(main);
    const retryBtn = document.getElementById("overview-retry");
    if (retryBtn) retryBtn.onclick = function () { pageOverview(main); };
    return;
  }
  if (!isNavTokenActive(navToken)) return;

  const quickLinks =
    renderInfraLinksCard(infraLinks) +
    '<div class="card" style="margin-bottom:16px"><h3>Đi nhanh</h3>' +
    '<p class="muted">Deploy app nằm trong từng project — không hiển thị trên Cluster Dashboard.</p>' +
    '<div class="meta-chips">' +
    '<a class="chip-link" href="#/platform-projects">Quản lý Projects</a>' +
    '<a class="chip-link" href="#/addons">Addons</a>' +
    (clusterItems.length ? '<a class="chip-link" href="#/pods">Pods</a>' : "") +
    "</div></div>";

  const c = d.counts || {};
  const cap = d.capacity || {};
  const pods = cap.pods || {};
  const cpu = cap.cpu || {};
  const mem = cap.memory || {};
  const disk = cap.disk || {};

  let eventsHtml = "";
  if (d.recent_events && d.recent_events.length) {
    eventsHtml = renderTable(
      [
        { key: "status", label: "Type" },
        { key: "reason", label: "Reason" },
        { key: "object", label: "Object" },
        { key: "message", label: "Message" },
        { key: "created", label: "Seen", render: (r) => esc(fmtTime(r.created)) },
      ],
      d.recent_events
    );
  } else {
    eventsHtml = '<p class="muted">Không có events gần đây.</p>';
  }

  const sc = d.scaling || {};
  const barItems = [
    { label: "Pods", value: c.pods || 0 },
    { label: "Deploy", value: c.deployments || 0 },
    { label: "Svc", value: c.services || 0 },
    { label: "NS", value: c.namespaces || 0 },
    { label: "Ing", value: c.ingresses || 0 },
    { label: "Nodes", value: c.nodes || 0 },
  ];

  const comps = (d.components || [])
    .map((x) => compCard(x))
    .join("");

  main.innerHTML =
    quickLinks +
    '<div class="page-header">' +
    '<h2 class="page-title">Cluster Dashboard</h2>' +
    '<p class="page-subtitle">' +
    esc(d.name || d.cluster_id) +
    ' · <span class="pill">' +
    esc(d.state || "active") +
    "</span></p></div>" +
    (clusterItems.length > 1
      ? '<div class="list-toolbar" style="margin-top:12px">' +
        '<div class="field-group" style="min-width:240px;flex:1">' +
        '<span class="field-label">Cluster</span>' +
        '<div class="select-wrap"><select id="cluster-select">' +
        clusterItems
          .map(
            (c) =>
              '<option value="' +
              esc(c.id) +
              '"' +
              (c.id === state.clusterId ? " selected" : "") +
              ">" +
              esc(c.name || c.id) +
              "</option>"
          )
          .join("") +
        '</select><svg class="select-chev" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg></div></div></div>'
      : "") +
    '<div class="meta-chips">' +
    chip("Provider", d.provider || "RKE2") +
    chip("Kubernetes", d.k8s_version || "—") +
    chip("Cluster", d.cluster_id) +
    "</div>" +
    '<div class="stat-grid">' +
    statBox(c.resources || 0, "Total Resources", "g1") +
    statBox(c.nodes || 0, "Nodes", "g2") +
    statBox(c.deployments || 0, "Deployments", "g3") +
    statBox(c.pods || 0, "Pods", "g4") +
    statBox(c.namespaces || 0, "Namespaces", "g5") +
    statBox(c.services || 0, "Services", "g6") +
    "</div>" +
    '<div class="scale-chips">' +
    '<div class="scale-chip">HPA (Auto Scale)<strong>' + (sc.hpa_count || 0) + '</strong></div>' +
    '<div class="scale-chip">Pods đã restart<strong>' + (sc.pods_with_restart || 0) + '</strong></div>' +
    '<div class="scale-chip">Tổng lần restart<strong>' + (sc.total_restarts || 0) + '</strong></div>' +
    '<a href="#/horizontalpodautoscalers" class="scale-chip" style="text-decoration:none;color:inherit">→ Xem HPA</a>' +
    '<a href="#/pods" class="scale-chip" style="text-decoration:none;color:inherit">→ Xem Pods</a>' +
    '<a href="#/add-worker" class="scale-chip" style="text-decoration:none;color:inherit">→ Thêm worker</a>' +
    "</div>" +
    '<div class="dash-grid">' +
    '<div class="card"><h3>Capacity Overview</h3>' +
    '<div class="cap-donuts cap-donuts-4">' +
    svgDonut(pods.used_pct || 0, 100, "#8b5cf6", "Pods", (pods.used || 0) + "/" + (pods.total || 0)) +
    svgDonut(cpu.used_pct || 0, 100, "#22d3ee", "CPU Used", (cpu.used || 0) + " / " + (cpu.total || 0) + " cores") +
    svgDonut(mem.used_pct || 0, 100, "#ec4899", "Memory", (mem.used || 0) + " / " + (mem.total || 0) + " GiB") +
    svgDonut(disk.used_pct || 0, 100, "#f59e0b", "Disk", (disk.used || 0) + " / " + (disk.total || 0) + " GiB") +
    "</div>" +
    '<div class="cap-sub">' +
    meterRow("CPU Reserved", cpu.reserved, cpu.total, cpu.reserved_pct, "reserved", "cores") +
    meterRow("Memory Reserved", mem.reserved, mem.total, mem.reserved_pct, "reserved", "GiB") +
    ephemeralMeterRow(disk) +
    "</div></div>" +
    '<div class="card"><h3>Component Status</h3>' +
    '<div class="comp-grid">' +
    comps +
    "</div></div></div>" +
    '<div class="card dash-full"><h3>Resource Distribution</h3>' +
    '<p class="muted dash-hint">Số đếm tại thời điểm tải trang — refresh để cập nhật.</p>' +
    svgBarChart(barItems, 220, 900) +
    "</div>" +
    '<div class="card dash-full"><h3>Recent Events <a href="#/events" class="link-muted">Xem tất cả →</a></h3>' +
    eventsHtml +
    "</div>";

  const sel = document.getElementById("cluster-select");
  if (sel) {
    sel.onchange = () => {
      state.clusterId = sel.value;
      localStorage.setItem("cluster-id", state.clusterId);
      pageOverview(main);
    };
  }
  bindInfraCopyButtons(main);
}

function renderOverviewFallback(errMsg, dash, clusterItems) {
  const cluster = (dash && dash.cluster) || {};
  const projects = (dash && dash.projects) || [];
  const projRows = projects
    .slice(0, 8)
    .map(function (p) {
      return (
        '<tr><td><a href="#/project/' +
        esc(p.slug) +
        '/deploy">' +
        esc(p.name) +
        '</a></td><td><code>' +
        esc(p.namespace_dev || "") +
        "</code></td></tr>"
      );
    })
    .join("");

  return (
    '<div class="page-header">' +
    '<h2 class="page-title">Platform</h2>' +
    '<p class="page-subtitle">Cluster Dashboard tạm không tải được</p></div>' +
    '<div class="card" style="margin-bottom:16px;border-color:rgba(248,113,113,0.35)">' +
    '<h3>Không tải được Cluster Dashboard</h3>' +
    '<p class="muted">' +
    esc(errMsg) +
    "</p>" +
    '<p class="muted">Thường do Rancher API chậm hoặc token hết hạn. Thử refresh, hoặc mở Rancher trực tiếp. Menu <strong>Hạ tầng</strong> bên trái — bấm mũi tên để mở rộng nếu đang thu gọn.</p>' +
    '<button type="button" class="btn-primary" id="overview-retry">Thử lại</button></div>' +
    '<div class="card" style="margin-bottom:16px"><h3>Trạng thái nhanh</h3>' +
    '<div class="meta-chips">' +
    chip("Cluster", cluster.connected ? "Kết nối" : "Chưa kết nối") +
    chip("Nodes", cluster.nodes != null ? String(cluster.nodes) : "—") +
    chip("Projects", String(projects.length)) +
    "</div></div>" +
    '<div class="card" style="margin-bottom:16px"><h3>Deploy app ở đâu?</h3>' +
    '<p class="muted">Vào <strong>Quản lý Projects</strong> → chọn project → tab <strong>Deploy / Git</strong>.</p>' +
    '<a class="btn-primary" href="#/platform-projects" style="display:inline-block;margin-top:8px">Mở Quản lý Projects</a></div>' +
    (projRows
      ? '<div class="card"><h3>Projects</h3><div class="table-wrap"><table><thead><tr><th>Project</th><th>Namespace dev</th></tr></thead><tbody>' +
        projRows +
        "</tbody></table></div></div>"
      : "") +
    (clusterItems.length
      ? '<p class="muted" style="margin-top:12px">Phát hiện ' + clusterItems.length + " cluster — thử lại sau vài giây.</p>"
      : "")
  );
}

function svgDonut(pct, size, color, title, sub) {
  const stroke = 10;
  const r = (size - stroke) / 2;
  const cx = size / 2;
  const c = 2 * Math.PI * r;
  const p = Math.min(Math.max(Number(pct) || 0, 0), 100);
  const off = c * (1 - p / 100);
  return (
    '<div class="cap-donut-item">' +
    '<svg width="' + size + '" height="' + size + '" viewBox="0 0 ' + size + " " + size + '">' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="rgba(255,255,255,0.07)" stroke-width="' + stroke + '"/>' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="' + color + '" stroke-width="' + stroke + '"' +
    ' stroke-dasharray="' + c.toFixed(1) + '" stroke-dashoffset="' + off.toFixed(1) + '"' +
    ' stroke-linecap="round" transform="rotate(-90 ' + cx + " " + cx + ')"/>' +
    '<text x="' + cx + '" y="' + (cx - 2) + '" text-anchor="middle" fill="#fff" font-size="18" font-weight="700">' + Math.round(p) + "%</text>" +
    "</svg>" +
    "<h4>" + esc(title) + "</h4>" +
    (sub ? "<p>" + esc(String(sub)) + "</p>" : "") +
    "</div>"
  );
}

function svgDonutSmall(pct, size, color) {
  const stroke = 6;
  const r = (size - stroke) / 2;
  const cx = size / 2;
  const c = 2 * Math.PI * r;
  const p = Math.min(Math.max(Number(pct) || 0, 0), 100);
  const off = c * (1 - p / 100);
  return (
    '<svg width="' + size + '" height="' + size + '" viewBox="0 0 ' + size + " " + size + '">' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="rgba(255,255,255,0.08)" stroke-width="' + stroke + '"/>' +
    '<circle cx="' + cx + '" cy="' + cx + '" r="' + r + '" fill="none" stroke="' + color + '" stroke-width="' + stroke + '"' +
    ' stroke-dasharray="' + c.toFixed(1) + '" stroke-dashoffset="' + off.toFixed(1) + '"' +
    ' stroke-linecap="round" transform="rotate(-90 ' + cx + " " + cx + ')"/>' +
    "</svg>"
  );
}

function svgBarChart(items, h, w) {
  const max = Math.max.apply(null, items.map(function (i) { return i.value; }).concat([1]));
  const n = items.length;
  const barW = Math.max(24, (w - 50) / n - 10);
  let rects = "";
  items.forEach(function (it, i) {
    const bh = Math.max(4, (it.value / max) * (h - 50));
    const x = 28 + i * (barW + 10);
    const y = h - 28 - bh;
    rects +=
      '<rect x="' + x + '" y="' + y + '" width="' + barW + '" height="' + bh + '" rx="5" fill="url(#barGrad)" opacity="0.9"/>' +
      '<text x="' + (x + barW / 2) + '" y="' + (h - 8) + '" text-anchor="middle" fill="#94a3b8" font-size="10">' + esc(it.label) + "</text>" +
      '<text x="' + (x + barW / 2) + '" y="' + (y - 4) + '" text-anchor="middle" fill="#eef2ff" font-size="10">' + it.value + "</text>";
  });
  return (
    '<div class="bar-chart"><svg viewBox="0 0 ' + w + " " + h + '" preserveAspectRatio="xMidYMid meet">' +
    "<defs><linearGradient id=\"barGrad\" x1=\"0\" y1=\"1\" x2=\"0\" y2=\"0\">" +
    '<stop offset="0%" stop-color="#6d28d9"/><stop offset="100%" stop-color="#22d3ee"/>' +
    "</linearGradient></defs>" + rects + "</svg></div>"
  );
}

function compCard(comp) {
  const ok = comp.status === "ok";
  const color = ok ? "#34d399" : "#f87171";
  return (
    '<div class="comp-card ' + esc(comp.status) + '">' +
    svgDonutSmall(ok ? 100 : 40, 56, color) +
    '<div class="comp-name">' + esc(comp.name) + "</div>" +
    '<div class="comp-msg">' + esc(comp.message || comp.status) + "</div></div>"
  );
}

function chip(label, value) {
  return (
    '<div class="chip"><span class="chip-label">' +
    esc(label) +
    '</span><span class="chip-value">' +
    esc(value) +
    "</span></div>"
  );
}

function meterRow(label, used, total, pct, tone, unit) {
  const u = Number(used) || 0;
  const t = Number(total) || 0;
  let p = pct != null && !isNaN(Number(pct)) ? Number(pct) : t > 0 ? (u / t) * 100 : 0;
  if (isNaN(p)) p = 0;
  const suf = unit ? " " + unit : "";
  return (
    '<div class="meter">' +
    '<div class="meter-head"><span>' + esc(label) + "</span><span>" +
    esc(roundDisp(u) + suf) + " / " + esc(roundDisp(t) + suf) + " (" + Math.round(p) + "%)</span></div>" +
    '<div class="meter-track">' +
    (p > 0
      ? '<i class="meter-fill ' + (tone || "used") + '" style="width:' + Math.min(p, 100) + '%"></i>'
      : "") +
    "</div></div>"
  );
}

function ephemeralMeterRow(disk) {
  const reserved = Number(disk.reserved) || 0;
  const total = Number(disk.reserved_total) || 0;
  if (reserved <= 0) {
    return "";
  }
  if (total <= 0) {
    return "";
  }
  return meterRow("Ephemeral Reserved", reserved, total, disk.reserved_pct, "reserved", "GiB");
}

function roundDisp(n) {
  return Number.isInteger(n) ? n : Math.round(n * 100) / 100;
}

function statBox(n, label, grad) {
  return (
    '<div class="stat-box ' + (grad || "g1") + '">' +
    '<div class="lbl">' + esc(label) + '</div><div class="num">' + n + "</div></div>"
  );
}

function monitoringPoints(values, divisor) {
  if (!Array.isArray(values)) return [];
  return values.map(function (row) {
    if (!Array.isArray(row) || row.length < 2) return null;
    const x = Number(row[0]) || 0;
    let y = Number(row[1]) || 0;
    if (divisor) y = y / divisor;
    return [x, y];
  }).filter(Boolean);
}

const chartRegistry = new Map();
let chartSeq = 0;

function lineChartLayout(points) {
  const w = 980;
  const h = 250;
  const m = { top: 14, right: 16, bottom: 34, left: 52 };
  const cw = w - m.left - m.right;
  const ch = h - m.top - m.bottom;
  let min = points[0][1];
  let max = points[0][1];
  points.forEach(function (p) {
    if (p[1] < min) min = p[1];
    if (p[1] > max) max = p[1];
  });
  const padY = Math.max((max - min) * 0.08, 0.000001);
  min -= padY;
  max += padY;
  const span = Math.max(max - min, 0.000001);
  const step = cw / Math.max(points.length - 1, 1);
  function yPos(v) {
    return m.top + ch - ((v - min) / span) * ch;
  }
  function xPos(i) {
    return m.left + i * step;
  }
  return { w: w, h: h, m: m, cw: cw, ch: ch, min: min, max: max, span: span, step: step, yPos: yPos, xPos: xPos };
}

function lineChart(points, opts) {
  opts = opts || {};
  const color = opts.color || "#6EE7FF";
  const unit = opts.unit || "";
  const digits = opts.digits == null ? 2 : opts.digits;
  if (!points.length) {
    return '<div class="muted">Chưa có dữ liệu timeline</div>';
  }
  const layout = lineChartLayout(points);
  const w = layout.w;
  const h = layout.h;
  const m = layout.m;
  const cw = layout.cw;
  const ch = layout.ch;
  const yPos = layout.yPos;
  const xPos = layout.xPos;
  const path = points.map(function (p, i) {
    const x = xPos(i);
    const y = yPos(p[1]);
    return (i === 0 ? "M" : "L") + x.toFixed(2) + " " + y.toFixed(2);
  }).join(" ");
  const area =
    path +
    " L " + xPos(points.length - 1).toFixed(2) + " " + (m.top + ch).toFixed(2) +
    " L " + xPos(0).toFixed(2) + " " + (m.top + ch).toFixed(2) +
    " Z";
  const yTicks = 4;
  const yGrid = [];
  for (let i = 0; i <= yTicks; i++) {
    const ratio = i / yTicks;
    const val = layout.max - ratio * (layout.max - layout.min);
    const y = m.top + ratio * ch;
    yGrid.push({ y: y, val: val });
  }
  const tickCount = 5;
  const xTicks = [];
  for (let i = 0; i < tickCount; i++) {
    const idx = Math.round((points.length - 1) * (i / (tickCount - 1)));
    const ts = Number(points[idx][0]) || 0;
    xTicks.push({ x: xPos(idx), label: fmtClockFromUnix(ts) });
  }
  const last = points[points.length - 1];
  const lastX = xPos(points.length - 1);
  const lastY = yPos(last[1]);
  const chartId = "chart-" + (++chartSeq);
  chartRegistry.set(chartId, { points: points, color: color, unit: unit, digits: digits, layout: layout });

  return (
    '<div class="chart-interactive" data-chart-id="' + chartId + '">' +
    '<svg viewBox="0 0 ' + w + " " + h + '" class="chart-svg" aria-hidden="true">' +
    yGrid.map(function (g) {
      return (
        '<line x1="' + m.left + '" y1="' + g.y.toFixed(2) + '" x2="' + (m.left + cw) + '" y2="' + g.y.toFixed(2) + '" stroke="rgba(255,255,255,0.08)" stroke-width="1"></line>' +
        '<text x="' + (m.left - 8) + '" y="' + (g.y + 4).toFixed(2) + '" text-anchor="end" fill="rgba(200,210,230,0.75)" font-size="11">' +
        esc(Number(g.val).toFixed(digits) + (unit ? " " + unit : "")) +
        "</text>"
      );
    }).join("") +
    xTicks.map(function (t) {
      return '<text x="' + t.x.toFixed(2) + '" y="' + (h - 10) + '" text-anchor="middle" fill="rgba(200,210,230,0.65)" font-size="11">' + esc(t.label) + "</text>";
    }).join("") +
    '<path d="' + area + '" fill="' + color + '" opacity="0.12"></path>' +
    '<path d="' + path + '" fill="none" stroke="' + color + '" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"></path>' +
    '<circle class="chart-last-dot" cx="' + lastX.toFixed(2) + '" cy="' + lastY.toFixed(2) + '" r="3.5" fill="' + color + '"></circle>' +
    "</svg>" +
    '<div class="chart-overlay">' +
    '<div class="chart-crosshair-v"></div>' +
    '<div class="chart-crosshair-h"></div>' +
    '<div class="chart-focus-dot" style="background:' + color + ';box-shadow:0 0 0 2px #fff"></div>' +
    '<div class="chart-tooltip" role="tooltip"><div class="tt-time"></div><div class="tt-val"></div></div>' +
    "</div>" +
    "</div>"
  );
}

function fmtChartDateTime(tsSec) {
  const d = new Date((Number(tsSec) || 0) * 1000);
  if (isNaN(d.getTime())) return "--:--";
  const pad = function (n) {
    return n < 10 ? "0" + n : String(n);
  };
  return (
    pad(d.getDate()) + "/" + pad(d.getMonth() + 1) + " " +
    pad(d.getHours()) + ":" + pad(d.getMinutes()) + ":" + pad(d.getSeconds())
  );
}

function bindInteractiveCharts(root) {
  (root || document).querySelectorAll(".chart-interactive").forEach(function (wrap) {
    if (wrap.dataset.chartBound === "1") return;
    const chartId = wrap.getAttribute("data-chart-id");
    const meta = chartRegistry.get(chartId);
    if (!meta) return;
    wrap.dataset.chartBound = "1";

    const overlay = wrap.querySelector(".chart-overlay");
    const crosshairV = wrap.querySelector(".chart-crosshair-v");
    const crosshairH = wrap.querySelector(".chart-crosshair-h");
    const dot = wrap.querySelector(".chart-focus-dot");
    const lastDot = wrap.querySelector(".chart-last-dot");
    const tooltip = wrap.querySelector(".chart-tooltip");
    const ttTime = wrap.querySelector(".tt-time");
    const ttVal = wrap.querySelector(".tt-val");
    if (!overlay || !crosshairV || !crosshairH || !dot || !tooltip || !ttTime || !ttVal) return;

    const points = meta.points;
    const layout = meta.layout;
    const m = layout.m;
    const cw = layout.cw;
    const ch = layout.ch;
    const unitSuffix = meta.unit ? " " + meta.unit : "";
    let geom = null;
    let pxCache = null;
    let lastIdx = -1;

    function rebuildGeom() {
      const ow = overlay.clientWidth;
      const oh = overlay.clientHeight;
      if (!ow || !oh) return;
      geom = {
        left: (m.left / layout.w) * ow,
        top: (m.top / layout.h) * oh,
        width: (cw / layout.w) * ow,
        height: (ch / layout.h) * oh,
      };
      crosshairV.style.top = geom.top.toFixed(1) + "px";
      crosshairV.style.height = geom.height.toFixed(1) + "px";
      crosshairH.style.left = "0px";
      crosshairH.style.width = ow.toFixed(1) + "px";
      pxCache = points.map(function (p, i) {
        const xNorm = i / Math.max(points.length - 1, 1);
        const yNorm = (p[1] - layout.min) / layout.span;
        return {
          px: geom.left + xNorm * geom.width,
          py: geom.top + geom.height * (1 - yNorm),
          time: fmtChartDateTime(p[0]),
          val: Number(p[1]).toFixed(meta.digits) + unitSuffix,
        };
      });
    }

    function hideHover() {
      lastIdx = -1;
      wrap.classList.remove("is-hovering");
      if (lastDot) lastDot.style.opacity = "1";
    }

    function pointIndexFromEvent(evt) {
      if (!geom) rebuildGeom();
      if (!geom || !geom.width) return 0;
      const rect = overlay.getBoundingClientRect();
      const mouseX = evt.clientX - rect.left - geom.left;
      let idx = Math.round((mouseX / geom.width) * (points.length - 1));
      if (idx < 0) idx = 0;
      if (idx >= points.length) idx = points.length - 1;
      return idx;
    }

    function showHover(idx) {
      if (!pxCache) rebuildGeom();
      if (!pxCache || !pxCache[idx]) return;
      if (idx === lastIdx && wrap.classList.contains("is-hovering")) return;

      const c = pxCache[idx];
      lastIdx = idx;
      wrap.classList.add("is-hovering");
      crosshairV.style.transform = "translate3d(" + c.px.toFixed(1) + "px,0,0)";
      crosshairH.style.transform = "translate3d(0," + c.py.toFixed(1) + "px,0)";
      dot.style.transform = "translate3d(" + c.px.toFixed(1) + "px," + c.py.toFixed(1) + "px,0)";
      tooltip.style.left = c.px.toFixed(1) + "px";
      tooltip.style.top = (c.py - 8).toFixed(1) + "px";
      ttTime.textContent = c.time;
      ttVal.textContent = c.val;
      if (lastDot) lastDot.style.opacity = "0.35";
    }

    function onMove(evt) {
      showHover(pointIndexFromEvent(evt));
    }

    rebuildGeom();
    overlay.addEventListener("pointerenter", onMove);
    overlay.addEventListener("pointermove", onMove);
    overlay.addEventListener("pointerleave", hideHover);
    window.addEventListener("resize", function () {
      geom = null;
      pxCache = null;
      rebuildGeom();
    }, { passive: true });
  });
}

function timelineStats(points) {
  if (!points.length) {
    return { current: 0, min: 0, max: 0, avg: 0, minTs: 0, maxTs: 0 };
  }
  let min = points[0][1];
  let max = points[0][1];
  let minTs = points[0][0];
  let maxTs = points[0][0];
  let sum = 0;
  points.forEach(function (p) {
    const v = Number(p[1]) || 0;
    if (v < min) {
      min = v;
      minTs = Number(p[0]) || 0;
    }
    if (v > max) {
      max = v;
      maxTs = Number(p[0]) || 0;
    }
    sum += v;
  });
  return {
    current: Number(points[points.length - 1][1]) || 0,
    min: min,
    max: max,
    avg: sum / points.length,
    minTs: minTs,
    maxTs: maxTs,
  };
}

function timelineStatsChips(stats, unit, digits) {
  const d = digits == null ? 2 : digits;
  return (
    '<div class="meta-chips monitor-stats">' +
    chip("Current", Number(stats.current).toFixed(d) + " " + unit) +
    chip("Avg", Number(stats.avg).toFixed(d) + " " + unit) +
    chip("Đáy", Number(stats.min).toFixed(d) + " " + unit) +
    chip("Đỉnh", Number(stats.max).toFixed(d) + " " + unit) +
    "</div>"
  );
}

function fmtClockFromUnix(tsSec) {
  const d = new Date((Number(tsSec) || 0) * 1000);
  if (isNaN(d.getTime())) return "--:--";
  return d.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", hour12: false });
}

function timelinePeakText(stats, unit, digits) {
  const d = digits == null ? 2 : digits;
  return (
    '<p class="muted">Đỉnh: <strong>' + esc(Number(stats.max).toFixed(d) + " " + unit) + "</strong> lúc " +
    esc(fmtClockFromUnix(stats.maxTs)) +
    ' · Đáy: <strong>' + esc(Number(stats.min).toFixed(d) + " " + unit) + "</strong> lúc " +
    esc(fmtClockFromUnix(stats.minTs)) +
    "</p>"
  );
}

function monitoringWindowToolbar(active, slug, env) {
  const opts = [
    { key: "15m", label: "15m" },
    { key: "1h", label: "1h" },
    { key: "6h", label: "6h" },
    { key: "24h", label: "24h" },
  ];
  return (
    '<div class="monitor-toolbar">' +
    '<span class="monitor-toolbar-label">Khung thời gian</span>' +
    '<div class="monitor-segmented">' +
    opts.map(function (o) {
      const cls = o.key === active ? "seg-btn active" : "seg-btn";
      return '<a class="' + cls + '" href="' + esc("#/project/" + slug + "/monitoring?env=" + env + "&window=" + o.key) + '">' + esc(o.label) + "</a>";
    }).join("") +
    "</div>" +
    "</div>"
  );
}

function monitoringInsight(cpu, memMiB, restarts) {
  if (restarts > 0) return { tone: "warn", text: "Có restart trong khung thời gian này, nên kiểm tra pod logs." };
  if (cpu > 0.8 || memMiB > 1024) return { tone: "warn", text: "Tải tài nguyên khá cao, cân nhắc scale hoặc tối ưu." };
  if (cpu > 0.25 || memMiB > 512) return { tone: "ok", text: "Hệ thống đang ổn định, tải ở mức trung bình." };
  return { tone: "ok", text: "Hệ thống ổn định, tải thấp." };
}

function listPage(title, total, body, toolbar) {
  return (
    '<div class="list-page">' +
    '<div class="list-header"><h2 class="page-title">' + esc(title) +
    ' <span class="muted count">(' + total + ")</span></h2>" +
    (toolbar || "") +
    "</div>" +
    '<div class="card list-card">' + body + "</div></div>"
  );
}

async function pageRancherList(main, title, path, columns) {
  main.innerHTML = '<p class="loading">Đang tải ' + esc(title) + "…</p>";
  const data = await api(path);
  const rows = data.items || data;
  const total = data.total != null ? data.total : rows.length;
  main.innerHTML = listPage(title, total, renderTable(columns, Array.isArray(rows) ? rows : []));
}

function badgeStatus(val) {
  if (!val) return "—";
  const v = String(val).toLowerCase();
  const ok = v.includes("active") || v.includes("running") || v.includes("ready=true") || v === "bound" || v === "succeeded";
  const warn = v.includes("pending") || v.includes("warning") || v.includes("progress");
  const cls = warn ? "badge-warn" : ok ? "badge-ok" : "badge-neutral";
  return '<span class="badge ' + cls + '">' + esc(val) + "</span>";
}

function k8sColumns(resource, data) {
  const age = { key: "created", label: "Age", render: (r) => esc(fmtTime(r.created)) };
  const ns = { key: "namespace", label: "Namespace" };

  if (resource === "events") {
    return [
      { key: "status", label: "Type", render: (r) => badgeStatus(r.status) },
      { key: "reason", label: "Reason" },
      { key: "object", label: "Object" },
      ns,
      { key: "message", label: "Message" },
      age,
    ];
  }
  if (resource === "nodes") {
    return [
      { key: "name", label: "Name" },
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "node_ip", label: "Internal IP" },
      { key: "cpu_cores", label: "CPU", render: (r) => esc(r.cpu_cores ? r.cpu_cores + " cores" : "—") },
      { key: "mem_gib", label: "Memory", render: (r) => esc(r.mem_gib ? r.mem_gib.toFixed(1) + " GiB" : "—") },
      { key: "pods_max", label: "Pod Capacity" },
      age,
    ];
  }
  if (resource === "pods") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "status", label: "Phase", render: (r) => badgeStatus(r.status) },
      { key: "node", label: "Node" },
      { key: "pod_ip", label: "Pod IP" },
      { key: "restarts", label: "Restarts", render: (r) => r.restarts > 0 ? '<span class="badge badge-warn">' + r.restarts + "</span>" : esc(r.restarts || 0) },
      { key: "restart_policy", label: "Restart Policy" },
      { key: "images", label: "Image" },
      age,
    ];
  }
  if (resource === "deployments" || resource === "statefulsets" || resource === "daemonsets") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "replicas", label: "Replicas" },
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "selector", label: "Selector" },
      age,
    ];
  }
  if (resource === "jobs") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "completions", label: "Completions" },
      age,
    ];
  }
  if (resource === "cronjobs") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "schedule", label: "Schedule" },
      { key: "suspend", label: "Suspended" },
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      age,
    ];
  }
  if (resource === "horizontalpodautoscalers") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "scale", label: "Min–Max → Current" },
      age,
    ];
  }
  if (resource === "services") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "service_type", label: "Type" },
      { key: "cluster_ip", label: "Cluster IP" },
      { key: "ports", label: "Ports" },
      age,
    ];
  }
  if (resource === "ingresses") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "host", label: "Hosts" },
      { key: "status", label: "Class" },
      age,
    ];
  }
  if (resource === "persistentvolumeclaims" || resource === "persistentvolumes") {
    return [
      { key: "name", label: "Name" },
      ns,
      { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
      { key: "capacity", label: "Capacity" },
      { key: "access_modes", label: "Access Modes" },
      { key: "storage_class", label: "Storage Class" },
      age,
    ].filter(function (c) { return resource !== "persistentvolumes" || c.key !== "namespace"; });
  }
  const cols = [
    { key: "name", label: "Name" },
    ns,
    { key: "status", label: "Status", render: (r) => badgeStatus(r.status) },
    age,
  ];
  if (!data.items || !data.items.some((i) => i.namespace)) {
    cols.splice(1, 1);
  }
  return cols;
}

function listToolbar(resource, namespaced) {
  let html =
    '<div class="list-toolbar">' +
    '<div class="field-search">' +
    '<svg class="search-icon" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
    '<circle cx="11" cy="11" r="7"/><path d="M20 20l-3-3"/></svg>' +
    '<input type="search" id="list-search" placeholder="Tìm theo tên…" value="' +
    esc(state.search) +
    '" autocomplete="off">' +
    "</div>";
  if (namespaced) {
    html +=
      '<div class="field-group">' +
      '<span class="field-label">Namespace</span>' +
      '<div class="select-wrap">' +
      '<select id="ns-filter"><option value="">Tất cả</option></select>' +
      '<svg class="select-chev" width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M6 9l6 6 6-6"/></svg>' +
      "</div></div>";
  }
  html += "</div>";
  return html;
}

async function bindListToolbar(main, resource, namespaced, reload) {
  const search = document.getElementById("list-search");
  if (search) {
    search.oninput = () => {
      state.search = search.value.trim().toLowerCase();
      reload();
    };
  }
  if (namespaced) {
    const sel = document.getElementById("ns-filter");
    const data = await api(
      "/api/v1/rancher/namespaces" + (state.clusterId ? "?cluster_id=" + encodeURIComponent(state.clusterId) : "")
    );
    const items = data.items || [];
    sel.innerHTML =
      '<option value="">Tất cả</option>' +
      items.map((n) => '<option value="' + esc(n) + '">' + esc(n) + "</option>").join("");
    sel.value = state.namespace;
    sel.onchange = () => {
      state.namespace = sel.value;
      localStorage.setItem("filter-ns", state.namespace);
      state.page[resource] = 1;
      reload();
    };
  }
}

function filterRows(rows) {
  if (!state.search) return rows;
  return rows.filter((r) => (r.name || "").toLowerCase().includes(state.search));
}

async function pageK8s(main, resource, label, page, limit) {
  const route = resource;
  page = page || state.page[route] || 1;
  limit = limit || state.limit;
  state.page[route] = page;
  state.limit = limit;

  const resMeta = await api("/api/v1/explorer/menu");
  const item = resMeta.find((m) => m.key === resource);
  const namespaced = item && item.type === "k8s" && resource !== "namespaces" && resource !== "nodes" && resource !== "events" && resource !== "persistentvolumes" && resource !== "storageclasses";

  main.innerHTML = '<p class="loading">Đang tải ' + esc(label) + "…</p>";
  const data = await api(
    "/api/v1/k8s/" + resource + qs({ page: page, limit: limit })
  );
  const cols = k8sColumns(resource, data);
  const onPage = (p, l) => pageK8s(main, resource, label, p, l);
  const rows = filterRows(data.items || []);

  main.innerHTML = listPage(
    label,
    data.total,
    renderTable(cols, rows, resource) +
      renderPagination(route, data.total, data.page, data.limit, onPage),
    listToolbar(resource, namespaced)
  );
  await bindListToolbar(main, resource, namespaced, () => pageK8s(main, resource, label, page, limit));
}

async function pageAddWorker(main) {
  main.innerHTML = '<p class="loading">Đang tải…</p>';
  const info = await api("/api/v1/cluster/join-info" + qs());
  let gate = getJoinGate();
  let scriptHtml = '<p class="muted">Nhập PIN join (xem trên VPS: config/join-gate.env) rồi bấm lấy script.</p>';
  let script = "";

  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">Thêm worker (RKE2)</h2>' +
    '<p class="page-subtitle">Copy script chạy trên VPS mới — token không hiện công khai.</p></div>' +
    '<div class="card detail-card">' +
    '<div class="meta-chips">' +
    chip("Server IP", info.server_ip || "—") +
    chip("Supervisor URL", info.server_url || "—") +
    chip("Nodes hiện tại", String(info.node_count || 0)) +
    chip("Join ready", info.join_configured ? "yes" : "no") +
    "</div>" +
    "<h3>Ports cần mở (VPS mới → server)</h3><ul class='port-list'>" +
    (info.required_ports || []).map((p) => "<li>" + esc(p) + "</li>").join("") +
    "</ul>" +
    '<div class="toolbar"><label>PIN join <input type="password" id="join-gate-in" placeholder="JOIN_GATE_SECRET" /></label>' +
    '<button type="button" id="join-fetch-btn">Lấy script join</button>' +
    '<button type="button" id="join-poll-btn">Refresh nodes</button></div>' +
    '<pre id="join-script" class="yaml-box">' +
    esc(scriptHtml) +
    "</pre>" +
    '<p class="muted">Bảo mật: PIN lưu session trình duyệt; script chứa token — không share. Xóa sau khi join.</p>' +
    "</div>";

  const gateIn = document.getElementById("join-gate-in");
  if (gate) gateIn.value = gate;

  document.getElementById("join-fetch-btn").onclick = async () => {
    gate = gateIn.value.trim();
    if (!gate) {
      toastWarn("Nhập PIN join trước");
      return;
    }
    setJoinGate(gate);
    try {
      const resp = await api("/api/v1/cluster/join-script", {
        method: "POST",
        headers: { "X-Join-Gate": gate },
        body: { gate: gate },
      });
      script = resp.script || "";
      document.getElementById("join-script").textContent = script;
    } catch (e) {
      document.getElementById("join-script").textContent = "Lỗi: " + e.message;
    }
  };

  document.getElementById("join-poll-btn").onclick = async () => {
    const i = await api("/api/v1/cluster/join-info" + qs());
    toastInfo("Nodes hiện tại: " + (i.node_count || 0));
    location.hash = "#/nodes";
  };
}

async function pageResourceDetail(main, resource, ns, name) {
  main.innerHTML = '<p class="loading">Đang tải chi tiết…</p>';
  const nsQ = ns && ns !== "_" ? ns : "";
  const base = "/api/v1/k8s/" + resource + "/" + encodeURIComponent(name) + qs({ namespace: nsQ });

  let detail = {};
  try {
    detail = await api(base);
  } catch (e) {
    main.innerHTML = '<p class="error">Lỗi: ' + esc(e.message) + "</p>";
    return;
  }

  const title = name + (nsQ ? " · " + nsQ : "");
  let actions =
    '<div class="action-bar">' +
    '<a href="#/' + esc(resource) + '" class="btn-ghost">← Quay lại</a>';

  if (resource === "deployments" && nsQ && canWriteK8s()) {
    actions +=
      ' <label>Replicas <input type="number" id="scale-replicas" min="0" max="100" style="width:4rem" />' +
      ' <button type="button" id="scale-btn">Scale</button></label>';
  }
  if (resource === "pods" && nsQ && canWriteK8s()) {
    actions += ' <button type="button" id="restart-btn">Restart (delete)</button>';
  }
  actions += ' <button type="button" id="yaml-btn">YAML</button>';
  if (resource === "pods" && nsQ) {
    actions += ' <button type="button" id="logs-btn">Logs</button>';
  }
  if (canWriteK8s()) {
    actions += ' <button type="button" id="delete-btn" class="btn-danger">Xóa</button>';
  }
  actions += "</div>";

  main.innerHTML =
    '<div class="page-header"><h2 class="page-title">' +
    esc(title) +
    '</h2><p class="page-subtitle">' +
    esc(resource) +
    "</p></div>" +
    actions +
    '<div class="card detail-card"><h3>Overview</h3><pre class="yaml-box">' +
    esc(JSON.stringify(detail, null, 2)) +
    '</pre></div><div id="extra-panel"></div>';

  document.getElementById("yaml-btn").onclick = async () => {
    const y = await api(base + "/yaml");
    document.getElementById("extra-panel").innerHTML =
      '<div class="card detail-card"><h3>YAML</h3><pre class="yaml-box">' + esc(y.yaml || "") + "</pre></div>";
  };

  if (resource === "pods" && nsQ) {
    document.getElementById("logs-btn").onclick = async () => {
      const l = await api(
        "/api/v1/k8s/pods/" + encodeURIComponent(name) + "/logs" + qs({ namespace: nsQ, tail: 300 })
      );
      document.getElementById("extra-panel").innerHTML =
        '<div class="card detail-card"><h3>Logs</h3><pre class="log-box">' + esc(l.logs || "") + "</pre></div>";
    };
  }

  if (resource === "deployments" && nsQ && canWriteK8s()) {
    document.getElementById("scale-btn").onclick = async () => {
      const n = parseInt(document.getElementById("scale-replicas").value, 10);
      if (isNaN(n)) return;
      await api(
        "/api/v1/k8s/deployments/" + encodeURIComponent(name) + "/scale" + qs({ namespace: nsQ }),
        { method: "PATCH", body: { replicas: n } }
      );
      toastSuccess("Đã scale — đang refresh");
      pageResourceDetail(main, resource, ns, name);
    };
  }

  if (resource === "pods" && nsQ && canWriteK8s()) {
    document.getElementById("restart-btn").onclick = async () => {
      if (!(await uiConfirm("Restart pod " + name + "?", { title: "Restart pod", confirmText: "Restart" }))) return;
      await api(base, { method: "DELETE" });
      toastInfo("Pod đang recreate…");
      location.hash = "#/pods";
    };
  }

  const delBtn = document.getElementById("delete-btn");
  if (delBtn) {
    delBtn.onclick = async () => {
      if (!(await uiConfirm("Xóa " + resource + "/" + name + "?", { danger: true, title: "Xóa resource" }))) return;
      await api(base, { method: "DELETE" });
      toastSuccess("Đã xóa");
      location.hash = "#/" + resource;
    };
  }
}

const routes = {
  overview: (main) => pageOverview(main),
  "my-projects": (main) => pageMyProjects(main),
  "platform-projects": (main) => pagePlatformProjects(main),
  addons: (main) => pageAddons(main),
  gitops: (main) => pageGitOps(main),
  "add-worker": (main) => pageAddWorker(main),
  audit: (main) => pageAudit(main),
  users: (main) => pageUsers(main),
  clusters: (main) =>
    pageRancherList(main, "Clusters", "/api/v1/rancher/clusters", [
      { key: "name", label: "Name" },
      { key: "id", label: "ID" },
      { key: "state", label: "State", render: (r) => badgeStatus(r.state) },
      { key: "provider", label: "Provider" },
      { key: "k8s_version", label: "Kubernetes" },
      { key: "nodes", label: "Nodes" },
      { key: "driver", label: "Driver" },
      { key: "created", label: "Age", render: (r) => esc(fmtTime(r.created)) },
    ]),
  projects: (main) =>
    pageRancherList(main, "Projects", "/api/v1/rancher/projects", [
      { key: "name", label: "Name" },
      { key: "id", label: "ID" },
      { key: "cluster_id", label: "Cluster" },
      { key: "state", label: "State", render: (r) => badgeStatus(r.state) },
      { key: "description", label: "Description" },
    ]),
};

function roleLabel(role) {
  const m = { admin: "Admin", tech_lead: "Tech Lead", dev: "Developer", readonly: "Read-only" };
  return m[role] || role;
}

const ROLE_HELP = [
  {
    role: "admin",
    title: "Admin",
    desc: "Toàn quyền platform",
    perms: ["Quản lý user (tạo, đổi role, vô hiệu)", "Xem audit log", "Thêm worker node", "Xem & thao tác mọi project/namespace", "Xóa project (sau này)"],
  },
  {
    role: "tech_lead",
    title: "Tech Lead",
    desc: "Giám sát team, không quản lý user",
    perms: ["Quản lý Projects (wizard Harbor + namespace)", "Xem mọi project & cluster", "Xem audit log", "Thêm worker node", "Deploy/restart prod (theo policy)"],
  },
  {
    role: "dev",
    title: "Developer",
    desc: "Chỉ project được gán",
    perms: ["Menu Dự án + Pods/Deployments/Services/Ingress trong namespace dev", "Không thấy Hạ tầng (nodes, events cluster-wide…)", "Không thêm worker / quản lý user", "Không thao tác namespace prod"],
  },
  {
    role: "readonly",
    title: "Read-only",
    desc: "Chỉ xem",
    perms: ["Xem pod, deployment, log trong namespace dev + prod được gán", "Không scale, restart, xóa", "Không menu Hạ tầng cluster"],
  },
];

function roleHelpHtml() {
  return (
    '<div class="role-help-grid">' +
    ROLE_HELP.map(function (r) {
      return (
        '<div class="role-help-card">' +
        '<h4>' + esc(r.title) + ' <span class="muted">(' + esc(r.role) + ")</span></h4>" +
        "<p>" + esc(r.desc) + "</p><ul>" +
        r.perms.map(function (p) { return "<li>" + esc(p) + "</li>"; }).join("") +
        "</ul></div>"
      );
    }).join("") +
    "</div>"
  );
}

function showLoginPage(msg) {
  state.onLoginPage = true;
  stopDeployPoll();
  document.querySelector(".sidebar")?.classList.add("hidden");
  const nav = $("#sidebar-nav");
  if (nav) nav.innerHTML = "";
  const main = $("#main");
  if (!main) return;
  const existingForm = document.getElementById("login-form");
  if (existingForm) {
    if (msg) setLoginError(msg);
    else clearLoginError();
    return;
  }
  main.innerHTML =
    '<div class="login-wrap">' +
    '<div class="login-card">' +
    "<h2>Platform Console</h2>" +
    "<p class=\"muted\">Đăng nhập để quản lý cluster</p>" +
    '<p class="login-session-error" role="alert" aria-live="polite" hidden></p>' +
    '<form id="login-form" class="login-form">' +
    '<label>Email<input type="email" name="email" autocomplete="username" required /></label>' +
    '<label>Mật khẩu<input type="password" name="password" autocomplete="current-password" required minlength="12" /></label>' +
    '<button type="submit" class="btn-primary" id="login-submit-btn">Đăng nhập</button>' +
    "</form>" +
    '<div id="quick-login-box" class="quick-login-box quick-login-loading">' +
    '<p class="muted quick-login-placeholder">Đang tải đăng nhập nhanh…</p></div>' +
    '<p class="login-hint muted">Mật khẩu ≥ 12 ký tự, có chữ và số</p>' +
    "</div></div>";
  if (msg) setLoginError(msg);
  $("#login-form").onsubmit = async (e) => {
    e.preventDefault();
    clearLoginError();
    const fd = new FormData(e.target);
    const btn = document.getElementById("login-submit-btn");
    if (btn) {
      btn.disabled = true;
      btn.dataset.label = btn.textContent;
      btn.innerHTML = '<span class="btn-spinner"></span> Đang đăng nhập…';
    }
    try {
      await performLogin(String(fd.get("email") || ""), String(fd.get("password") || ""));
    } catch (err) {
      setLoginError(err.message || "Đăng nhập thất bại");
      toastError(err.message || "Đăng nhập thất bại");
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.textContent = btn.dataset.label || "Đăng nhập";
      }
    }
  };
  bindQuickLoginBox();
}

function setLoginError(msg) {
  msg = String(msg || "").trim();
  if (!msg) return;
  const card = document.querySelector(".login-card");
  if (!card) return;
  let errEl = card.querySelector(".login-session-error");
  if (!errEl) {
    errEl = document.createElement("p");
    errEl.className = "login-session-error";
    errEl.setAttribute("role", "alert");
    errEl.setAttribute("aria-live", "polite");
    const form = document.getElementById("login-form");
    if (form) card.insertBefore(errEl, form);
    else card.appendChild(errEl);
  }
  errEl.hidden = false;
  errEl.removeAttribute("hidden");
  errEl.classList.add("login-session-error--visible");
  errEl.textContent = msg;
  errEl.scrollIntoView({ block: "nearest", behavior: "smooth" });
}

function clearLoginError() {
  const errEl = document.querySelector(".login-card .login-session-error");
  if (!errEl) return;
  errEl.textContent = "";
  errEl.hidden = true;
  errEl.classList.remove("login-session-error--visible");
}

async function performLogin(email, password) {
  const data = await api("/api/v1/auth/login", {
    method: "POST",
    body: { email: email, password: password },
  });
  state.user = data.user;
  state.onLoginPage = false;
  document.querySelector(".sidebar")?.classList.remove("hidden");
  const keepHash = location.hash && location.hash.length > 2;
  if (!keepHash) {
    location.hash = "#/" + defaultHomeRoute();
  }
  await navigate();
}

async function bindQuickLoginBox() {
  const box = document.getElementById("quick-login-box");
  if (!box) return;
  try {
    const hint = await api("/api/v1/auth/quick-login", { silent401: true });
    if (!hint || !hint.enabled || !hint.email || !hint.password) {
      box.className = "quick-login-box quick-login-empty";
      box.innerHTML = "";
      return;
    }
    box.className = "quick-login-box";
    box.innerHTML =
      '<p class="quick-login-title muted">' + esc(hint.label || "Đăng nhập nhanh") + " <span class=\"quick-login-temp\">(tạm)</span></p>" +
      '<button type="button" class="quick-login-btn" id="quick-login-btn">' +
      '<span class="quick-login-role">Admin</span>' +
      '<span class="quick-login-cred"><code>' + esc(hint.email) + "</code> · <code>" + esc(hint.password) + "</code></span>" +
      '<span class="quick-login-action">Bấm để đăng nhập →</span>' +
      "</button>";
    document.getElementById("quick-login-btn").onclick = async function () {
      const form = document.getElementById("login-form");
      if (form) {
        const emailEl = form.querySelector('[name="email"]');
        const passEl = form.querySelector('[name="password"]');
        if (emailEl) emailEl.value = hint.email;
        if (passEl) passEl.value = hint.password;
      }
      clearLoginError();
      try {
        await performLogin(hint.email, hint.password);
      } catch (err) {
        setLoginError(err.message || "Đăng nhập thất bại");
        toastError(err.message || "Đăng nhập thất bại");
      }
    };
  } catch (_e) {
    box.className = "quick-login-box quick-login-empty";
    box.innerHTML = "";
  }
}

async function logout() {
  try {
    await api("/api/v1/auth/logout", { method: "POST" });
  } catch (_) {}
  state.user = null;
  showLoginPage();
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

async function ensureAuth() {
  const ctrl = new AbortController();
  const timer = setTimeout(function () { ctrl.abort(); }, 8000);
  try {
    state.user = await api("/api/v1/auth/me", { signal: ctrl.signal, silent401: true });
    state.onLoginPage = false;
    return true;
  } catch (_) {
    showLoginPage();
    return false;
  } finally {
    clearTimeout(timer);
  }
}

function getRoute() {
  return location.hash.replace(/^#\/?/, "") || defaultHomeRoute();
}

function parseRoute() {
  const raw = getRoute();
  const pathOnly = raw.split("?")[0];
  if (pathOnly.startsWith("view/")) {
    const parts = pathOnly.split("/");
    return { type: "view", resource: parts[1], ns: parts[2] || "_", name: parts.slice(3).join("/") };
  }
  const parts = pathOnly.split("/");
  if (parts[0] === "project" && parts[1]) {
    return { type: "project", slug: parts[1], tab: parts[2] || "overview", key: "project/" + parts[1] };
  }
  return { type: "page", key: pathOnly };
}

async function navigate() {
  const navToken = nextNavToken();
  const parsed = parseRoute();
  await buildSidebarForRoute(parsed);
  if (!isNavTokenActive(navToken)) return;
  const main = $("#main");
  try {
    if (parsed.type === "view") {
      await pageResourceDetail(main, parsed.resource, parsed.ns, parsed.name);
      return;
    }
    if (parsed.type === "project") {
      await pageProjectHub(main, parsed.slug, parsed.tab);
      return;
    }
    if (routes[parsed.key]) {
      await routes[parsed.key](main);
      return;
    }
    const menu = await api("/api/v1/explorer/menu");
    const item = menu.find((m) => m.key === parsed.key);
    if (item && item.type === "k8s") {
      await pageK8s(main, item.key, item.label);
      return;
    }
    if (!isNavTokenActive(navToken)) return;
    main.innerHTML = '<p class="error">Không tìm thấy trang: ' + esc(parsed.key) + "</p>";
  } catch (e) {
    if (!isNavTokenActive(navToken)) return;
    main.innerHTML =
      '<p class="error">Lỗi: ' +
      esc(errorMessage(e)) +
      '</p><p class="muted" style="margin-top:8px"><button type="button" class="btn-ghost btn-sm" onclick="location.reload()">Tải lại</button></p>';
  }
}

function markActiveNav(parsed) {
  document.querySelectorAll(".nav-link").forEach(function (el) {
    let active = false;
    if (parsed.type === "project") {
      const route = el.dataset.route || "";
      active = route === "project/" + parsed.slug + "/" + (parsed.tab || "overview") ||
        route === "project/" + parsed.slug;
    } else if (parsed.type === "view") {
      active = el.dataset.route === parsed.resource;
    } else {
      active = el.dataset.route === parsed.key;
    }
    el.classList.toggle("active", active);
  });
}

function renderSidebarUser() {
  let userBar = document.getElementById("sidebar-user");
  const sidebar = document.querySelector(".sidebar");
  if (!userBar && sidebar) {
    userBar = document.createElement("div");
    userBar.id = "sidebar-user";
    userBar.className = "sidebar-user";
    const footer = sidebar.querySelector(".sidebar-footer");
    sidebar.insertBefore(userBar, footer);
  }
  if (userBar) {
    userBar.innerHTML =
      '<div class="sidebar-user-info"><strong>' + esc(state.user?.email || "") + "</strong>" +
      '<span class="muted">' + esc(roleLabel(state.user?.role || "")) + "</span></div>" +
      '<button type="button" class="btn-logout" id="btn-logout">Đăng xuất</button>';
    const logoutBtn = document.getElementById("btn-logout");
    if (logoutBtn) logoutBtn.onclick = logout;
  }
}

function bindSidebarEvents(nav) {
  nav.querySelectorAll(".nav-group-toggle").forEach(function (btn) {
    btn.onclick = function () {
      const g = btn.dataset.group;
      const items = btn.nextElementSibling;
      const nowCollapsed = !items.classList.contains("collapsed");
      items.classList.toggle("collapsed", nowCollapsed);
      btn.querySelector(".chev").textContent = nowCollapsed ? "▸" : "▾";
      setGroupCollapsed(g, nowCollapsed);
    };
  });
  nav.querySelectorAll(".nav-section-toggle").forEach(function (btn) {
    btn.onclick = function () {
      const sec = btn.dataset.section;
      const body = btn.nextElementSibling;
      const nowCollapsed = !body.classList.contains("collapsed");
      body.classList.toggle("collapsed", nowCollapsed);
      btn.querySelector(".chev").textContent = nowCollapsed ? "▸" : "▾";
      setSectionCollapsed(sec, nowCollapsed);
    };
  });
}

function buildProjectSidebarHtml(nav, slug, tab, p) {
  let html =
    '<a class="nav-back" href="' + platformHomeHash() + '" title="Về Platform">' +
    '<span class="ico">←</span><span class="nav-label">Platform</span></a>';

  html += '<div class="nav-section"><div class="nav-section-label">Project</div><div class="nav-section-body">';
  PROJECT_NAV.forEach(function (item) {
    if (item.tab === "promote" && !canManagePlatformProjects()) {
      return;
    }
    const routeKey = "project/" + slug + "/" + item.tab;
    const href = projectRoute(slug, item.tab);
    html +=
      '<a class="nav-link" data-route="' + esc(routeKey) + '" href="' + href + '" title="' + esc(item.label) + '">' +
      '<span class="ico">' + item.icon + '</span><span class="nav-label">' + esc(item.label) + "</span></a>";
  });
  html += "</div></div>";

  html += '<div class="nav-section"><div class="nav-section-label">Workloads</div><div class="nav-section-body">';
  PROJECT_WORKLOADS.forEach(function (item) {
    const routeKey = "project/" + slug + "/" + item.tab;
    html +=
      '<a class="nav-link" data-route="' + esc(routeKey) + '" href="' + projectRoute(slug, item.tab) + '" title="' + esc(item.label) + '">' +
      '<span class="ico">' + item.icon + '</span><span class="nav-label">' + esc(item.label) + "</span></a>";
  });
  html += "</div></div>";

  html +=
    '<div class="project-sidebar-meta">' +
    chip("Dev", p.namespace_dev) +
    chip("Prod", p.namespace_prod) +
    "</div>";

  nav.innerHTML = html;
  nav.classList.remove("loading");
}

async function buildPlatformSidebar(nav) {
  const menu = await api("/api/v1/explorer/menu");
  const sections = { platform: [], workspace: [], infra: [] };
  menu.forEach(function (item) {
    const sec = item.section || (item.group === "Platform" ? "platform" : "infra");
    if (!sections[sec]) sections[sec] = [];
    sections[sec].push(item);
  });

  const infraGroupOrder = ["Cluster", "Workloads", "Networking", "Storage", "Config"];
  const workspaceGroupOrder = ["Workloads", "Networking"];
  let html = "";

  function renderGroup(group, items) {
    const collapsed = groupCollapsed(group);
    let g =
      '<div class="nav-group">' +
      '<button type="button" class="nav-group-toggle" data-group="' + esc(group) + '">' +
      '<span class="chev">' + (collapsed ? "▸" : "▾") + "</span>" + esc(group) +
      "</button>" +
      '<div class="nav-group-items' + (collapsed ? " collapsed" : "") + '">';
    items.forEach(function (item) {
      const ico = NAV_ICONS[item.key] || "·";
      g +=
        '<a class="nav-link" data-route="' + esc(item.key) + '" href="#/' + esc(item.key) + '" title="' + esc(item.label) + '">' +
        '<span class="ico">' + ico + '</span><span class="nav-label">' + esc(item.label) + "</span></a>";
    });
    return g + "</div></div>";
  }

  if (sections.platform.length) {
    const secCollapsed = sectionCollapsed("platform");
    html +=
      '<div class="nav-section">' +
      '<button type="button" class="nav-section-toggle" data-section="platform">' +
      '<span class="chev">' + (secCollapsed ? "▸" : "▾") + "</span>" +
      esc(SECTION_LABELS.platform) +
      "</button>" +
      '<div class="nav-section-body' + (secCollapsed ? " collapsed" : "") + '">';
    sections.platform.forEach(function (item) {
      const ico = NAV_ICONS[item.key] || "·";
      html +=
        '<a class="nav-link" data-route="' + esc(item.key) + '" href="#/' + esc(item.key) + '" title="' + esc(item.label) + '">' +
        '<span class="ico">' + ico + '</span><span class="nav-label">' + esc(item.label) + "</span></a>";
    });
    html += "</div></div>";
  }

  if (sections.workspace.length) {
    const secCollapsed = sectionCollapsed("workspace");
    html +=
      '<div class="nav-section nav-section-workspace">' +
      '<button type="button" class="nav-section-toggle" data-section="workspace">' +
      '<span class="chev">' + (secCollapsed ? "▸" : "▾") + "</span>" +
      esc(SECTION_LABELS.workspace) +
      "</button>" +
      '<div class="nav-section-body' + (secCollapsed ? " collapsed" : "") + '">';
    const groups = {};
    sections.workspace.forEach(function (item) {
      if (!groups[item.group]) groups[item.group] = [];
      groups[item.group].push(item);
    });
    for (const group of workspaceGroupOrder) {
      if (groups[group]) html += renderGroup(group, groups[group]);
    }
    Object.keys(groups).forEach(function (group) {
      if (!workspaceGroupOrder.includes(group)) html += renderGroup(group, groups[group]);
    });
    html += "</div></div>";
  }

  if (sections.infra.length) {
    const secCollapsed = sectionCollapsed("infra");
    html +=
      '<div class="nav-section nav-section-infra">' +
      '<button type="button" class="nav-section-toggle" data-section="infra">' +
      '<span class="chev">' + (secCollapsed ? "▸" : "▾") + "</span>" +
      esc(SECTION_LABELS.infra) +
      "</button>" +
      '<div class="nav-section-body' + (secCollapsed ? " collapsed" : "") + '">';
    const groups = {};
    sections.infra.forEach(function (item) {
      if (!groups[item.group]) groups[item.group] = [];
      groups[item.group].push(item);
    });
    for (const group of infraGroupOrder) {
      if (groups[group]) html += renderGroup(group, groups[group]);
    }
    html += "</div></div>";
  }

  nav.innerHTML = html;
  nav.classList.remove("loading");
}

async function buildSidebarForRoute(parsed) {
  const nav = $("#sidebar-nav");
  if (!nav) return;

  if (parsed.type === "project") {
    let p = state.projectCtx;
    if (!p || p.slug !== parsed.slug) {
      try {
        const data = await api("/api/v1/projects/" + encodeURIComponent(parsed.slug));
        p = data.project;
        state.projectCtx = p;
      } catch (_) {
        p = { slug: parsed.slug, name: parsed.slug, namespace_dev: "—", namespace_prod: "—" };
      }
    }
    updateSidebarBrand({ mode: "project", name: p.name, slug: p.slug });
    buildProjectSidebarHtml(nav, parsed.slug, parsed.tab || "overview", p);
  } else {
    state.projectCtx = null;
    updateSidebarBrand({ mode: "platform" });
    await buildPlatformSidebar(nav);
  }

  renderSidebarUser();
  bindSidebarEvents(nav);
  markActiveNav(parsed);
}

async function buildSidebar() {
  await buildSidebarForRoute(parseRoute());
}

function groupCollapsed(group) {
  const key = "nav-collapsed-" + group;
  return localStorage.getItem(key) === "1";
}

function setGroupCollapsed(group, collapsed) {
  localStorage.setItem("nav-collapsed-" + group, collapsed ? "1" : "0");
}

const NAV_ICONS = {
  overview: "◉", "my-projects": "▣", "platform-projects": "＋", addons: "🧩", gitops: "⎇", audit: "📋", users: "👤", "add-worker": "⊕", clusters: "◎", projects: "▣", namespaces: "▤", nodes: "⬡",
  events: "⚡", deployments: "▶", statefulsets: "▧", daemonsets: "▨",
  jobs: "⏱", cronjobs: "↻", pods: "●", services: "🔗", ingresses: "🌐",
  horizontalpodautoscalers: "📈", persistentvolumeclaims: "💾",
  persistentvolumes: "🗄", storageclasses: "📂", configmaps: "⚙",
  secrets: "🔒",
};

function sectionCollapsed(section) {
  return localStorage.getItem("nav-section-" + section) === "1";
}

function setSectionCollapsed(section, collapsed) {
  localStorage.setItem("nav-section-" + section, collapsed ? "1" : "0");
}

const SECTION_LABELS = { platform: "Platform", workspace: "Dự án", infra: "Hạ tầng" };

window.addEventListener("hashchange", () => { if (state.user) navigate(); });

function sidebarMode() {
  return localStorage.getItem("sidebar-mode") || "expanded";
}

function setSidebarMode(mode) {
  localStorage.setItem("sidebar-mode", mode);
  applySidebarMode();
}

function applySidebarMode() {
  const layout = document.querySelector(".layout");
  const btn = document.getElementById("sidebar-toggle");
  const fab = document.getElementById("sidebar-fab");
  if (!layout) return;
  layout.classList.remove("sidebar-collapsed", "sidebar-hidden");
  const mode = sidebarMode();
  if (mode === "collapsed") {
    layout.classList.add("sidebar-collapsed");
    if (btn) btn.textContent = "›";
    if (btn) btn.title = "Mở rộng sidebar";
  } else if (mode === "hidden") {
    layout.classList.add("sidebar-hidden");
    if (fab) fab.style.display = "block";
    if (btn) btn.textContent = "‹";
  } else {
    if (btn) btn.textContent = "‹";
    if (btn) btn.title = "Thu gọn sidebar";
    if (fab) fab.style.display = "none";
  }
}

function initSidebarToggle() {
  const btn = document.getElementById("sidebar-toggle");
  const fab = document.getElementById("sidebar-fab");
  if (btn) {
    btn.onclick = () => {
      const mode = sidebarMode();
      if (mode === "expanded") setSidebarMode("collapsed");
      else if (mode === "collapsed") setSidebarMode("hidden");
      else setSidebarMode("expanded");
    };
  }
  if (fab) {
    fab.onclick = () => setSidebarMode("expanded");
  }
  applySidebarMode();
}

(async function init() {
  initSidebarToggle();
  document.querySelector(".sidebar")?.classList.add("hidden");
  const main = $("#main");
  if (main) {
    main.innerHTML =
      '<div class="login-wrap"><div class="login-card auth-check-card">' +
      '<p class="muted" style="margin:0;text-align:center"><span class="btn-spinner"></span> Đang kiểm tra phiên…</p></div></div>';
  }
  if (await ensureAuth()) {
    document.querySelector(".sidebar")?.classList.remove("hidden");
    await navigate();
  }
})();
