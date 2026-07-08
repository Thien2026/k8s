/* Deploy render helpers: stages, harbor, logs, pipeline item. */

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
