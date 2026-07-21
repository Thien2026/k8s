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

function renderAddonMinioMonitor() {
  return (
    '<div class="addon-minio-monitor">' +
    '<p class="muted">Dung lượng volume (Tổng / Đã dùng / Còn lại) + object data từ Prometheus.</p>' +
    '<div class="addon-redis-monitor-actions">' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-minio-stats-refresh">Làm mới</button>' +
    '<div class="monitor-segmented" id="addon-minio-window-segment">' +
    '<button type="button" class="seg-btn" data-win="15m">15m</button>' +
    '<button type="button" class="seg-btn" data-win="1h">1h</button>' +
    '<button type="button" class="seg-btn active" data-win="6h">6h</button>' +
    '<button type="button" class="seg-btn" data-win="24h">24h</button>' +
    "</div>" +
    '<a class="btn-ghost btn-sm hidden" id="addon-minio-grafana-link" href="#" target="_blank" rel="noopener">Mở Grafana</a>' +
    '<span class="muted" id="addon-minio-stats-ts"></span>' +
    "</div>" +
    '<div id="addon-minio-stats-grid">' +
    '<p class="muted">Đang tải metrics…</p>' +
    "</div>" +
    '<div class="addon-redis-sparklines hidden" id="addon-minio-sparklines"></div>' +
    '<details class="deploy-raw-details"><summary class="muted">JSON thô (debug)</summary>' +
    '<pre class="addon-redis-stats-raw" id="addon-minio-stats-raw"></pre></details>' +
    "</div>"
  );
}

function renderAddonMinioOverview(addon, haBadge, reasons) {
  return (
    "<p>Trạng thái: " + addonStatusBadge(addon.status) +
    " · Topology: <code>" + esc(addon.topology || "standalone") + "</code> · " + haBadge + "</p>" +
    '<p class="muted">Release: <code>' + esc(addon.k8s_release || "—") + "</code> · Storage: <code>" +
    esc(String(addon.storage_gb || 5)) + "Gi</code> · Max object: <code>" +
    esc(String(addon.max_object_mb || 100)) + "Mi</code> · RAM: <code>" +
    esc(String(addon.max_memory_mb || 256)) + "Mi</code></p>" +
    '<p><button type="button" class="btn-ghost btn-sm" id="addon-minio-open-quota">Chỉnh Storage / Max object / RAM (Quota) →</button></p>' +
    (addon.topology_note ? '<p class="muted">' + esc(addon.topology_note) + "</p>" : "") +
    (reasons.length
      ? '<details class="deploy-raw-details"><summary class="muted">Điều kiện HA / distributed</summary><ul class="muted">' +
        reasons.map(function (r) { return "<li>" + esc(r) + "</li>"; }).join("") +
        "</ul></details>"
      : "")
  );
}

function renderAddonMinioQuota(addon, canManage, ceilings) {
  ceilings = ceilings || {};
  const storageMax = ceilings.minio_max_storage_gb || 100;
  const memMax = ceilings.minio_max_memory_mb || 1024;
  const objectMax = ceilings.minio_max_object_mb || 5120;
  const storage = addon.storage_gb || 5;
  const mem = addon.max_memory_mb || 256;
  const maxObject = addon.max_object_mb || 100;
  if (!canManage) {
    return (
      "<p>Storage: <strong>" +
      esc(String(storage)) +
      " Gi</strong> · Max object: <strong>" +
      esc(String(maxObject)) +
      " Mi</strong> · RAM pod: <strong>" +
      esc(String(mem)) +
      " Mi</strong></p>" +
      '<p class="muted">Chỉ owner/admin chỉnh quota. Trần platform: storage ≤ ' +
      esc(String(storageMax)) +
      " Gi · object ≤ " +
      esc(String(objectMax)) +
      " Mi · RAM ≤ " +
      esc(String(memMax)) +
      " Mi</p>"
    );
  }
  return (
    '<p class="muted">Trần admin (Platform Policy): storage ≤ <code id="addon-minio-storage-max">' +
    esc(String(storageMax)) +
    "</code> Gi · object ≤ <code id=\"addon-minio-object-max\">" +
    esc(String(objectMax)) +
    "</code> Mi · RAM ≤ <code id=\"addon-minio-mem-max\">" +
    esc(String(memMax)) +
    "</code> Mi — vượt trần sẽ <strong>không lưu được</strong>.</p>" +
    '<div class="addon-quota-form">' +
    '<label class="addon-quota-field" id="addon-minio-storage-wrap">Storage (GB)' +
    '<input type="number" id="addon-minio-storage" min="1" max="' +
    esc(String(storageMax)) +
    '" step="1" value="' +
    esc(String(storage)) +
    '">' +
    '<span class="addon-quota-hint muted" id="addon-minio-storage-hint"></span></label>' +
    '<label class="addon-quota-field" id="addon-minio-object-wrap">Max object (MiB)' +
    '<input type="number" id="addon-minio-object" min="1" max="' +
    esc(String(objectMax)) +
    '" step="1" value="' +
    esc(String(maxObject)) +
    '">' +
    '<span class="addon-quota-hint muted" id="addon-minio-object-hint"></span></label>' +
    '<label class="addon-quota-field" id="addon-minio-mem-wrap">RAM pod (MB)' +
    '<input type="number" id="addon-minio-mem" min="128" max="' +
    esc(String(memMax)) +
    '" step="1" value="' +
    esc(String(mem)) +
    '">' +
    '<span class="addon-quota-hint muted" id="addon-minio-mem-hint"></span></label>' +
    '<button type="button" class="btn-primary btn-sm" id="addon-minio-quota-apply">Lưu & apply</button>' +
    "</div>" +
    '<p class="error-text hidden" id="addon-minio-quota-error"></p>' +
    '<p class="muted">Apply = re-provision (PVC + bucket hard quota + <code>S3_MAX_OBJECT_MB</code>). App path: env gợi ý — chặn cứng S3 trực tiếp Phase 2.</p>'
  );
}

function bindAddonMinioQuotaValidation(storageMax, memMax, objectMax) {
  const storageEl = document.getElementById("addon-minio-storage");
  const memEl = document.getElementById("addon-minio-mem");
  const objectEl = document.getElementById("addon-minio-object");
  const applyBtn = document.getElementById("addon-minio-quota-apply");
  const errEl = document.getElementById("addon-minio-quota-error");
  if (!storageEl || !memEl || !applyBtn) return;
  objectMax = objectMax || 5120;

  function paint(el, wrapId, hintId, ok, msg) {
    const wrap = document.getElementById(wrapId);
    const hint = document.getElementById(hintId);
    if (wrap) wrap.classList.toggle("addon-quota-invalid", !ok);
    if (el) el.classList.toggle("input-invalid", !ok);
    if (hint) {
      hint.textContent = msg || "";
      hint.className = "addon-quota-hint " + (ok ? "muted" : "error-text");
    }
  }

  function validate() {
    const storageGB = parseInt(storageEl.value, 10);
    const memMB = parseInt(memEl.value, 10);
    const objectMB = objectEl ? parseInt(objectEl.value, 10) : 100;
    let ok = true;
    let err = "";
    if (!(storageGB >= 1)) {
      ok = false;
      paint(storageEl, "addon-minio-storage-wrap", "addon-minio-storage-hint", false, "Tối thiểu 1 Gi");
    } else if (storageGB > storageMax) {
      ok = false;
      paint(
        storageEl,
        "addon-minio-storage-wrap",
        "addon-minio-storage-hint",
        false,
        "Vượt trần platform (" + storageMax + " Gi) — không được lưu"
      );
      err = "Storage vượt trần admin (" + storageMax + " Gi).";
    } else {
      paint(
        storageEl,
        "addon-minio-storage-wrap",
        "addon-minio-storage-hint",
        true,
        "OK · trần " + storageMax + " Gi · hard quota bucket"
      );
    }
    if (objectEl) {
      if (!(objectMB >= 1)) {
        ok = false;
        paint(objectEl, "addon-minio-object-wrap", "addon-minio-object-hint", false, "Tối thiểu 1 Mi");
      } else if (objectMB > objectMax) {
        ok = false;
        paint(
          objectEl,
          "addon-minio-object-wrap",
          "addon-minio-object-hint",
          false,
          "Vượt trần platform (" + objectMax + " Mi) — không được lưu"
        );
        err = (err ? err + " " : "") + "Max object vượt trần admin (" + objectMax + " Mi).";
      } else {
        paint(
          objectEl,
          "addon-minio-object-wrap",
          "addon-minio-object-hint",
          true,
          "OK · trần " + objectMax + " Mi · inject S3_MAX_OBJECT_MB"
        );
      }
    }
    if (!(memMB >= 128)) {
      ok = false;
      paint(memEl, "addon-minio-mem-wrap", "addon-minio-mem-hint", false, "Tối thiểu 128 Mi");
    } else if (memMB > memMax) {
      ok = false;
      paint(
        memEl,
        "addon-minio-mem-wrap",
        "addon-minio-mem-hint",
        false,
        "Vượt trần platform (" + memMax + " Mi) — không được lưu"
      );
      err = (err ? err + " " : "") + "RAM vượt trần admin (" + memMax + " Mi).";
    } else {
      paint(memEl, "addon-minio-mem-wrap", "addon-minio-mem-hint", true, "OK · trần " + memMax + " Mi");
    }
    applyBtn.disabled = !ok;
    if (errEl) {
      errEl.textContent = err;
      errEl.classList.toggle("hidden", !err);
    }
    return ok;
  }

  storageEl.addEventListener("input", validate);
  memEl.addEventListener("input", validate);
  if (objectEl) objectEl.addEventListener("input", validate);
  validate();
  return validate;
}

function renderAddonMinioConnection(addon, canManage) {
  if (!addon.has_connection) {
    return '<p class="muted">Chưa có connection — bấm Re-provision.</p>';
  }
  return (
    '<div class="addon-connection-box">' +
    '<p class="muted">S3 endpoint · trong cluster</p>' +
    '<code class="addon-connection-url">' + esc(addon.endpoint_masked || addon.endpoint || "—") + "</code>" +
    (addon.endpoint_external_masked
      ? '<p class="muted" style="margin-top:12px">S3 endpoint · local (NodePort + DNS <code>*.minio.{domain}</code>)</p>' +
        '<div class="addon-connection-row">' +
        '<code class="addon-connection-url">' + esc(addon.endpoint_external_masked) + "</code>" +
        (canManage && addon.endpoint_external
          ? ' <button type="button" class="btn-ghost btn-sm" id="addon-minio-copy-ext">Copy</button>'
          : "") +
        "</div>" +
        (addon.console_url_external
          ? '<p class="muted" style="margin-top:8px">Console: <a href="' + esc(addon.console_url_external) +
            '" target="_blank" rel="noopener"><code>' + esc(addon.console_url_external) + "</code></a></p>"
          : "") +
        (addon.external_hostname
          ? '<p class="muted" style="font-size:12px">Host <code>' + esc(addon.external_hostname) +
            "</code> · API :" + esc(String(addon.external_api_port || "")) +
            " · Console :" + esc(String(addon.external_console_port || "")) +
            " · Cloudflare DNS only</p>"
          : "")
      : "") +
    '<p class="muted" style="margin-top:10px">Bucket · Access key</p>' +
    "<p><code>" + esc(addon.bucket || "app") + "</code> · <code>" +
    esc(addon.access_key_masked || "••••") + "</code></p>" +
    '<p class="muted" style="margin-top:8px">Env inject (cluster): <code>S3_ENDPOINT</code> <code>S3_ACCESS_KEY</code> <code>S3_SECRET_KEY</code> <code>S3_BUCKET</code></p>' +
    (canManage && addon.endpoint
      ? '<button type="button" class="btn-ghost btn-sm" id="addon-minio-copy-endpoint">Copy cluster endpoint</button> ' +
        (addon.endpoint_external
          ? '<button type="button" class="btn-ghost btn-sm" id="addon-minio-copy-ext-json">Copy external JSON</button> '
          : "") +
        '<button type="button" class="btn-ghost btn-sm" id="addon-minio-copy-keys">Copy keys JSON</button>'
      : "") +
    "</div>"
  );
}

function renderAddonMinioOps(canManage) {
  if (!canManage) return '<p class="muted">Chỉ owner/admin thao tác restart/logs.</p>';
  return (
    '<div class="addon-minio-ops">' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-minio-restart">Restart pod</button> ' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-minio-load-logs">Xem logs</button>' +
    '</div><pre class="addon-minio-logs hidden" id="addon-minio-logs"></pre>'
  );
}

function renderAddonMinioFiles(canManage, bucket, buckets) {
  const options = (buckets || []).filter(function (b) { return b.status === "running"; }).map(function (b) {
    return '<option value="' + esc(b.name) + '"' + (b.name === bucket ? " selected" : "") + ">" + esc(b.name) + "</option>";
  }).join("");
  return (
    '<div class="addon-minio-files">' +
    '<p class="muted">Duyệt object theo bucket · upload tối đa 10 MiB.</p>' +
    '<div class="addon-redis-keys-toolbar addon-minio-files-toolbar">' +
    '<label>Bucket <select id="addon-minio-files-bucket">' + options + "</select></label>" +
    '<label>Prefix <input type="text" id="addon-minio-files-prefix" placeholder="(trống = tất cả)" value=""></label>' +
    '<button type="button" class="btn-primary btn-sm" id="addon-minio-files-refresh">Làm mới</button>' +
    (canManage
      ? ' <label class="btn-ghost btn-sm addon-minio-file-pick">' +
        '<input type="file" id="addon-minio-files-input" class="addon-minio-files-input-native" />' +
        "<span>Chọn tệp</span></label>" +
        '<span class="muted addon-minio-files-name" id="addon-minio-files-name">Chưa chọn file</span>' +
        '<input type="text" id="addon-minio-files-key" placeholder="key (tuỳ chọn)" class="addon-minio-files-key" />' +
        '<button type="button" class="btn-ghost btn-sm" id="addon-minio-files-upload">Upload</button>'
      : "") +
    "</div>" +
    '<div class="addon-redis-keys-table-wrap">' +
    '<table class="addon-redis-keys-table" id="addon-minio-files-table">' +
    "<thead><tr><th>Key</th><th>Size</th><th>Modified</th><th></th></tr></thead>" +
    '<tbody id="addon-minio-files-body"><tr><td colspan="4" class="muted">Đang tải…</td></tr></tbody>' +
    "</table></div>" +
    '<div class="addon-redis-keys-footer"><span class="muted" id="addon-minio-files-meta"></span></div>' +
    "</div>"
  );
}

function renderAddonMinioBuckets(data, canManage) {
  const items = data.items || [];
  const rows = items.map(function (b) {
    const badge = b.is_default ? ' <span class="badge neutral">default</span>' : "";
    const state = b.status === "running" ? '<span class="badge ok">ready</span>' : '<span class="badge warn">' + esc(b.status || "unknown") + "</span>";
    const scan = b.scan_mode === "required"
      ? '<span class="badge warn">scan required</span>'
      : '<span class="badge muted">scan tắt</span>';
    return "<tr><td><code>" + esc(b.name) + "</code>" + badge + "</td><td>" + esc(String(b.storage_gb)) + " Gi</td><td>" +
      esc(String(b.max_object_mb)) + " Mi</td><td>" + state + "<br>" + scan + "</td><td>" +
      (canManage ? '<button class="btn-ghost btn-sm addon-minio-bucket-edit" data-bucket="' + esc(b.name) + '" data-quota="' + esc(String(b.storage_gb)) + '">Chỉnh quota</button>' +
        (b.scan_mode === "disabled" ? ' <button class="btn-ghost btn-sm addon-minio-scan-enable" data-bucket="' + esc(b.name) + '">Bật scan</button>' : "") : "—") +
      "</td></tr>";
  }).join("") || '<tr><td colspan="5" class="muted">Chưa có bucket.</td></tr>';
  return '<div class="addon-minio-buckets"><p class="muted">Ngân sách instance: <strong>' + esc(String(data.instance_storage_gb || 0)) +
    ' Gi</strong> · đã phân bổ <strong>' + esc(String(data.allocated_storage_gb || 0)) + ' Gi</strong> · còn <strong>' +
    esc(String(data.free_storage_gb || 0)) + " Gi</strong>. Giảm quota bucket <code>app</code> trước khi tạo bucket mới.</p>" +
    '<table class="addon-redis-keys-table"><thead><tr><th>Bucket</th><th>Quota</th><th>Max object</th><th>Trạng thái</th><th></th></tr></thead><tbody>' + rows + "</tbody></table>" +
    (canManage ? '<div class="addon-quota-form" style="margin-top:12px"><label class="addon-quota-field">Tên bucket<input id="addon-minio-bucket-name" placeholder="media-private"></label>' +
      '<label class="addon-quota-field">Quota (Gi)<input id="addon-minio-bucket-storage" type="number" min="1" value="1"></label>' +
      '<label class="addon-quota-field">Max object (MiB)<input id="addon-minio-bucket-object" type="number" min="1" value="100"></label>' +
      '<button class="btn-primary btn-sm" id="addon-minio-bucket-create">+ Tạo bucket</button></div>' : "") + "</div>";
}

function bindAddonMinioBuckets(slug, env, canManage, reload) {
  if (!canManage) return;
  document.querySelectorAll(".addon-minio-bucket-edit").forEach(function (btn) {
    btn.onclick = async function () {
      const next = window.prompt("Quota mới (GiB) cho bucket " + btn.dataset.bucket + ":", btn.dataset.quota);
      if (next === null) return;
      try {
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/buckets/" + encodeURIComponent(btn.dataset.bucket) +
          projectQs({ environment: env }), { method: "PATCH", body: { storage_gb: Number(next) } });
        toastSuccess("Đã cập nhật quota bucket");
        reload();
      } catch (err) { toastError(err.message || "Không cập nhật được quota"); }
    };
  });
  document.querySelectorAll(".addon-minio-scan-enable").forEach(function (btn) {
    btn.onclick = async function () {
      const ok = await uiConfirm(
        "Bật scan bắt buộc cho " + btn.dataset.bucket + "? App key sẽ bị thu hồi quyền ghi trực tiếp bucket clean. Upload Console sẽ chờ scan trước khi file xuất hiện.",
        { title: "Bật antivirus quarantine" }
      );
      if (!ok) return;
      btn.disabled = true;
      try {
        await withAppLoading(function () {
          return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/buckets/" + encodeURIComponent(btn.dataset.bucket) +
            "/scan/enable" + projectQs({ environment: env }), { method: "POST", timeout: 180000 });
        }, { title: "Đang tạo quarantine và khóa direct upload…" });
        toastSuccess("Đã bật scan required. Upload mới sẽ vào quarantine.");
        reload();
      } catch (err) { toastError(err.message || "Không bật được scan"); btn.disabled = false; }
    };
  });
  const create = document.getElementById("addon-minio-bucket-create");
  if (create) create.onclick = async function () {
    const name = (document.getElementById("addon-minio-bucket-name") || {}).value || "";
    const storage = Number((document.getElementById("addon-minio-bucket-storage") || {}).value);
    const object = Number((document.getElementById("addon-minio-bucket-object") || {}).value);
    create.disabled = true;
    try {
      const result = await withAppLoading(function () {
        return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/buckets", { method: "POST", body: {
          environment: env, name: name, storage_gb: storage, max_object_mb: object
        }, timeout: 180000 });
      }, { title: "Đang tạo bucket và service account…" });
      window.prompt(
        "Đã tạo bucket " + result.name + ". Sao chép JSON credential ngay; sau khi đóng sẽ không được API hiển thị lại.",
        JSON.stringify(result.connection || {}, null, 2)
      );
      toastSuccess("Đã tạo bucket " + result.name + ".");
      reload();
    } catch (err) { toastError(err.message || "Không tạo được bucket"); create.disabled = false; }
  };
}

function fmtMinioObjectSize(n) {
  n = Number(n) || 0;
  if (n < 1024) return n + " B";
  if (n < 1048576) return (n / 1024).toFixed(1) + " KiB";
  return (n / 1048576).toFixed(1) + " MiB";
}

function bindAddonMinioFiles(slug, env, canManage, bucket, reload) {
  const body = document.getElementById("addon-minio-files-body");
  const meta = document.getElementById("addon-minio-files-meta");
  const prefixEl = document.getElementById("addon-minio-files-prefix");
  const bucketEl = document.getElementById("addon-minio-files-bucket");
  if (!body) return;

  async function loadObjects() {
    body.innerHTML = '<tr><td colspan="4" class="muted">Đang tải…</td></tr>';
    try {
      const prefix = prefixEl ? prefixEl.value.trim() : "";
      const data = await api(
        "/api/v1/projects/" +
          encodeURIComponent(slug) +
          "/addons/minio/objects" +
          projectQs({ environment: env, bucket: bucket || undefined, prefix: prefix || undefined, limit: 100 }),
        { noLoading: true }
      );
      const items = data.items || [];
      if (!items.length) {
        body.innerHTML =
          '<tr><td colspan="4" class="muted">Bucket trống' + (prefix ? " (prefix)" : "") + "</td></tr>";
      } else {
        body.innerHTML = items
          .map(function (it) {
            const key = it.key || "";
            const dl =
              "/api/v1/projects/" +
              encodeURIComponent(slug) +
              "/addons/minio/objects/download" +
              projectQs({ environment: env, bucket: bucket || undefined, key: key });
            return (
              "<tr><td><code>" +
              esc(key) +
              "</code></td><td>" +
              esc(fmtMinioObjectSize(it.size)) +
              "</td><td>" +
              (it.last_modified ? esc(new Date(it.last_modified).toLocaleString()) : "—") +
              '</td><td class="addon-minio-files-actions">' +
              '<a class="btn-ghost btn-sm" href="' +
              esc(dl) +
              '" download>Tải</a>' +
              (canManage
                ? ' <button type="button" class="btn-ghost btn-sm danger addon-minio-files-del" data-key="' +
                  esc(key) +
                  '">Xóa</button>'
                : "") +
              "</td></tr>"
            );
          })
          .join("");
        body.querySelectorAll(".addon-minio-files-del").forEach(function (btn) {
          btn.onclick = async function () {
            const k = btn.getAttribute("data-key");
            const ok = await uiConfirm("Xóa object «" + k + "»?", { title: "Xóa object MinIO" });
            if (!ok) return;
            try {
              await api(
                "/api/v1/projects/" +
                  encodeURIComponent(slug) +
                  "/addons/minio/objects" +
                  projectQs({ environment: env, bucket: bucket || undefined, key: k }),
                { method: "DELETE" }
              );
              toastSuccess("Đã xóa " + k);
              loadObjects();
            } catch (err) {
              toastError(err.message || "Xóa thất bại");
            }
          };
        });
      }
      if (meta) {
        meta.textContent =
          "Bucket " +
          (data.bucket || "app") +
          " · " +
          (data.count || 0) +
          " object" +
          (data.is_truncated ? " (còn nữa — thu hẹp prefix)" : "");
      }
    } catch (err) {
      body.innerHTML = '<tr><td colspan="4" class="muted">' + esc(err.message || "Lỗi") + "</td></tr>";
      if (meta) meta.textContent = "";
    }
  }

  const refreshBtn = document.getElementById("addon-minio-files-refresh");
  if (refreshBtn) refreshBtn.onclick = loadObjects;
  if (prefixEl) {
    prefixEl.onkeydown = function (e) {
      if (e.key === "Enter") loadObjects();
    };
  }
  if (bucketEl) bucketEl.onchange = function () {
    state.minioBucketSelection = state.minioBucketSelection || {};
    state.minioBucketSelection[slug + ":" + env] = bucketEl.value;
    reload();
  };
  const uploadBtn = document.getElementById("addon-minio-files-upload");
  const fileInput = document.getElementById("addon-minio-files-input");
  const keyInput = document.getElementById("addon-minio-files-key");
  const nameEl = document.getElementById("addon-minio-files-name");
  if (fileInput && nameEl) {
    fileInput.onchange = function () {
      const f = fileInput.files && fileInput.files[0];
      nameEl.textContent = f ? f.name : "Chưa chọn file";
    };
  }
  if (uploadBtn && fileInput && canManage) {
    uploadBtn.onclick = async function () {
      const f = fileInput.files && fileInput.files[0];
      if (!f) {
        toastError("Chọn file trước");
        return;
      }
      const fd = new FormData();
      fd.append("file", f);
      fd.append("environment", env);
      if (bucket) fd.append("bucket", bucket);
      const key = keyInput ? keyInput.value.trim() : "";
      if (key) fd.append("key", key);
      uploadBtn.disabled = true;
      showAppLoading("Đang upload…", f.name);
      try {
        const res = await fetch(
          "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/objects",
          { method: "POST", credentials: "include", body: fd }
        );
        const ct = res.headers.get("content-type") || "";
        const data = ct.includes("json") ? await res.json() : await res.text();
        if (!res.ok) {
          throw new Error((data && data.error) || res.statusText);
        }
        toastSuccess("Đã upload: " + (data.key || f.name));
        fileInput.value = "";
        if (keyInput) keyInput.value = "";
        loadObjects();
      } catch (err) {
        toastError(err.message || "Upload thất bại");
      } finally {
        hideAppLoading();
        uploadBtn.disabled = false;
      }
    };
  }
  loadObjects();
}

function formatBytesShort(n) {
  const v = Number(n) || 0;
  if (v <= 0) return "0";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let x = v;
  let i = 0;
  while (x >= 1024 && i < units.length - 1) {
    x /= 1024;
    i++;
  }
  return (Math.round(x * 10) / 10) + " " + units[i];
}

function renderAddonMinioStatsGrid(data) {
  const prom = (data && data.prometheus) || {};
  const k8s = (data && data.k8s) || {};
  if (!prom.available) {
    return (
      '<p class="muted">' +
      esc(prom.hint || "Chưa có Prometheus metrics cho MinIO.") +
      "</p>" +
      (k8s.available
        ? '<div class="addon-stat-card"><span class="muted">Pod RAM</span><strong>' +
          esc(String(k8s.memory_mib || 0)) +
          " MiB</strong></div>"
        : "")
    );
  }
  const total =
    prom.capacity_total_bytes != null ? prom.capacity_total_bytes : prom.usable_total_bytes;
  const free =
    prom.capacity_free_bytes != null ? prom.capacity_free_bytes : prom.usable_free_bytes;
  const used =
    prom.capacity_used_bytes != null
      ? prom.capacity_used_bytes
      : Math.max(0, (Number(total) || 0) - (Number(free) || 0));
  const quotaGB = data.storage_quota_gb;
  const nativeQuota = (data && data.native_quota) || {};
  const usedPct = prom.used_pct != null ? prom.used_pct : 0;
  const barW = Math.max(0, Math.min(100, Number(usedPct) || 0));

  return (
    '<div class="addon-minio-storage-block">' +
    '<p class="addon-minio-storage-title">Dung lượng ' +
    '<span class="muted">(Budget project)</span></p>' +
    '<div class="addon-minio-usage-bar" title="' +
    esc(String(usedPct)) +
    '%"><span style="width:' +
    esc(String(barW)) +
    '%"></span></div>' +
    (quotaGB
      ? '<p class="muted addon-minio-quota-hint">Budget: <strong>' +
        esc(String(quotaGB)) +
        " GiB</strong> (PVC + hard quota bucket)" +
        (data.max_object_mb
          ? " · Max object: <strong>" + esc(String(data.max_object_mb)) + " MiB</strong>"
          : "") +
        "</p>"
      : "") +
    '<p class="muted addon-minio-quota-hint">Native hard quota: <strong class="native-quota-' +
    esc(String(nativeQuota.status || "unverified")) +
    '">' +
    esc(nativeQuota.status === "verified" ? "Đã xác minh" : nativeQuota.status === "failed" ? "Lỗi xác minh" : "Chưa xác minh") +
    "</strong>" +
    (nativeQuota.verified_at ? " · " + esc(new Date(nativeQuota.verified_at).toLocaleString()) : "") +
    (nativeQuota.error ? " · " + esc(String(nativeQuota.error)) : "") +
    "</p>" +
    "</div>" +
    '<div class="addon-redis-stats-grid">' +
    '<div class="addon-stat-card"><span class="muted">Tổng</span><strong>' +
    esc(formatBytesShort(total)) +
    "</strong></div>" +
    '<div class="addon-stat-card"><span class="muted">Đã dùng</span><strong>' +
    esc(formatBytesShort(used)) +
    "</strong></div>" +
    '<div class="addon-stat-card"><span class="muted">Còn lại</span><strong>' +
    esc(formatBytesShort(free)) +
    "</strong></div>" +
    '<div class="addon-stat-card"><span class="muted">Đã dùng %</span><strong>' +
    esc(String(usedPct)) +
    "%</strong></div>" +
    '<div class="addon-stat-card"><span class="muted">Số object</span><strong>' +
    esc(String(prom.objects_count != null ? Math.round(prom.objects_count) : "—")) +
    "</strong></div>" +
    '<div class="addon-stat-card"><span class="muted">Nodes online</span><strong>' +
    esc(String(prom.nodes_online || 0)) +
    "</strong></div>" +
    '<div class="addon-stat-card"><span class="muted">S3 req/s</span><strong>' +
    esc(String(prom.s3_requests_per_sec || 0)) +
    "</strong></div>" +
    (k8s.available
      ? '<div class="addon-stat-card"><span class="muted">Pod RAM</span><strong>' +
        esc(String(k8s.memory_mib || 0)) +
        " MiB</strong></div>" +
        '<div class="addon-stat-card"><span class="muted">Pod CPU</span><strong>' +
        esc(String(k8s.cpu_cores || 0)) +
        "</strong></div>"
      : "") +
    "</div>"
  );
}

function bindAddonMinioMonitor(slug, env) {
  const grid = document.getElementById("addon-minio-stats-grid");
  const tsEl = document.getElementById("addon-minio-stats-ts");
  const rawEl = document.getElementById("addon-minio-stats-raw");
  const grafanaLink = document.getElementById("addon-minio-grafana-link");
  const sparkEl = document.getElementById("addon-minio-sparklines");
  const winSeg = document.getElementById("addon-minio-window-segment");
  if (!grid) return;
  const winKey = "addon-minio-window:" + slug + ":" + env;
  let windowValue = localStorage.getItem(winKey) || "6h";
  if (!/^(15m|1h|6h|24h)$/.test(windowValue)) windowValue = "6h";

  function paintWindowButtons() {
    if (!winSeg) return;
    winSeg.querySelectorAll(".seg-btn").forEach(function (btn) {
      btn.classList.toggle("active", btn.getAttribute("data-win") === windowValue);
    });
  }

  async function loadStats() {
    grid.innerHTML = '<p class="muted">Đang tải…</p>';
    try {
      const data = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/stats" + projectQs({ environment: env, window: windowValue })
      );
      grid.innerHTML = renderAddonMinioStatsGrid(data);
      if (tsEl) tsEl.textContent = "cập nhật " + new Date().toLocaleTimeString();
      if (rawEl) rawEl.textContent = JSON.stringify(data, null, 2);
      if (grafanaLink) {
        if (data.grafana_dashboard_url) {
          grafanaLink.href = data.grafana_dashboard_url;
          grafanaLink.classList.remove("hidden");
        } else {
          grafanaLink.classList.add("hidden");
        }
      }
      if (sparkEl && data.prometheus && data.prometheus.available) {
        sparkEl.classList.remove("hidden");
        sparkEl.innerHTML =
          renderAddonRedisSparkline("Usage " + windowValue, data.prometheus.usage_series, {
            divisor: 1024 * 1024,
            unit: "MiB",
            digits: 1,
            color: "#34d399",
            window: windowValue,
          }) +
          renderAddonRedisSparkline("S3 req/s " + windowValue, data.prometheus.s3_series, {
            unit: "req/s",
            digits: 2,
            color: "#60a5fa",
            window: windowValue,
          });
        bindInteractiveCharts(sparkEl);
      } else if (sparkEl) {
        sparkEl.classList.add("hidden");
        sparkEl.innerHTML = "";
      }
    } catch (err) {
      grid.innerHTML = '<p class="error-text">' + esc(err.message || "Lỗi metrics") + "</p>";
    }
  }

  paintWindowButtons();
  if (winSeg) {
    winSeg.querySelectorAll(".seg-btn").forEach(function (btn) {
      btn.onclick = function () {
        windowValue = btn.getAttribute("data-win") || "6h";
        localStorage.setItem(winKey, windowValue);
        paintWindowButtons();
        loadStats();
      };
    });
  }
  const refreshBtn = document.getElementById("addon-minio-stats-refresh");
  if (refreshBtn) refreshBtn.onclick = function () { loadStats(); };
  loadStats();
}

function renderAddonRedisMonitor() {
  return (
    '<div class="addon-redis-monitor">' +
    '<p class="muted">Số liệu từ Redis INFO + redis_exporter (Prometheus) nếu đã re-provision.</p>' +
    '<div class="addon-redis-monitor-actions">' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-redis-stats-refresh">Làm mới</button>' +
    '<div class="monitor-segmented addon-redis-window-segment" id="addon-redis-window-segment">' +
    '<button type="button" class="seg-btn" data-win="15m">15m</button>' +
    '<button type="button" class="seg-btn" data-win="1h">1h</button>' +
    '<button type="button" class="seg-btn active" data-win="6h">6h</button>' +
    '<button type="button" class="seg-btn" data-win="24h">24h</button>' +
    "</div>" +
    '<a class="btn-ghost btn-sm hidden" id="addon-redis-grafana-link" href="#" target="_blank" rel="noopener">Mở Grafana</a>' +
    '<span class="muted addon-redis-stats-ts" id="addon-redis-stats-ts"></span>' +
    "</div>" +
    '<div class="addon-redis-stats-grid" id="addon-redis-stats-grid">' +
    '<p class="muted">Đang tải metrics…</p>' +
    "</div>" +
    '<div class="addon-redis-sparklines hidden" id="addon-redis-sparklines"></div>' +
    '<div class="addon-redis-policy-box hidden" id="addon-redis-policy-box"></div>' +
    '<details class="deploy-raw-details addon-redis-slowlog-wrap hidden" id="addon-redis-slowlog-wrap">' +
    '<summary class="muted">Slow log (15 mới nhất)</summary>' +
    '<pre class="addon-redis-slowlog" id="addon-redis-slowlog"></pre></details>' +
    '<details class="deploy-raw-details addon-redis-stats-raw-wrap"><summary class="muted">JSON thô (debug)</summary>' +
    '<pre class="addon-redis-stats-raw" id="addon-redis-stats-raw"></pre></details>' +
    "</div>"
  );
}

function renderAddonRedisKeys(canManage) {
  return (
    '<div class="addon-redis-keys">' +
    '<p class="muted">Duyệt key bằng SCAN. Bấm một hàng để chọn — double-click xem full value (string).</p>' +
    '<div class="addon-redis-keys-warn hidden" id="addon-redis-keys-warn"></div>' +
    '<div class="addon-redis-keys-toolbar">' +
    '<label>Pattern <input type="text" id="addon-redis-keys-pattern" placeholder="*" value="console:*"></label>' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-redis-keys-filter-console">console:*</button>' +
    '<button type="button" class="btn-primary btn-sm" id="addon-redis-keys-scan">Quét</button>' +
    (canManage
      ? ' <button type="button" class="btn-ghost btn-sm" id="addon-redis-keys-del-selected" disabled>Xóa key đã chọn</button>'
      : "") +
    "</div>" +
    '<div class="addon-redis-keys-table-wrap">' +
    '<table class="addon-redis-keys-table" id="addon-redis-keys-table">' +
    "<thead><tr><th>Key</th><th>Type</th><th>TTL</th><th>Preview</th></tr></thead>" +
    '<tbody id="addon-redis-keys-body"><tr><td colspan="4" class="muted">Bấm Quét để xem key đang lưu…</td></tr></tbody>' +
    "</table></div>" +
    '<pre class="addon-redis-key-full hidden" id="addon-redis-key-full"></pre>' +
    '<div class="addon-redis-keys-footer">' +
    '<span class="muted" id="addon-redis-keys-meta"></span> ' +
    '<button type="button" class="btn-ghost btn-sm hidden" id="addon-redis-keys-more">Tải thêm</button>' +
    "</div></div>"
  );
}

function redisSeriesPoints(series, divisor) {
  if (!Array.isArray(series)) return [];
  return series
    .map(function (p) {
      if (!p) return null;
      const t = Number(p.t) || 0;
      let v = Number(p.v) || 0;
      if (divisor) v = v / divisor;
      return [t, v];
    })
    .filter(Boolean);
}

function redisWindowTickCount(windowKey) {
  if (windowKey === "15m") return 8;
  if (windowKey === "1h") return 7;
  if (windowKey === "24h") return 9;
  return 7;
}

function redisFormatXAxis(tsSec, windowKey) {
  const d = new Date((Number(tsSec) || 0) * 1000);
  if (isNaN(d.getTime())) return "--:--";
  if (windowKey === "24h") {
    return d.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", hour12: false });
  }
  return d.toLocaleTimeString("vi-VN", { minute: "2-digit", second: "2-digit", hour12: false });
}

function redisFormatTooltipTime(tsSec, windowKey) {
  const d = new Date((Number(tsSec) || 0) * 1000);
  if (isNaN(d.getTime())) return "--:--";
  if (windowKey === "24h") {
    return d.toLocaleString("vi-VN", {
      day: "2-digit",
      month: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
  }
  return d.toLocaleTimeString("vi-VN", { hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false });
}

function renderAddonRedisSparkline(title, series, opts) {
  opts = opts || {};
  const points = redisSeriesPoints(series, opts.divisor || 0);
  if (!points.length) return "";
  const unit = opts.unit || "";
  const digits = opts.digits == null ? 2 : opts.digits;
  const windowKey = opts.window || "6h";
  const stats = timelineStats(points);
  return (
    '<div class="addon-redis-sparkline">' +
    '<div class="addon-redis-spark-head"><strong>' + esc(title) + "</strong>" +
    '<span class="muted">now ' + esc(Number(stats.current).toFixed(digits) + (unit ? " " + unit : "")) + "</span></div>" +
    lineChart(points, {
      color: opts.color || "#76a9fa",
      unit: unit,
      digits: digits,
      xTickCount: redisWindowTickCount(windowKey),
      xLabelFormatter: function (ts) { return redisFormatXAxis(ts, windowKey); },
      tooltipTimeFormatter: function (ts) { return redisFormatTooltipTime(ts, windowKey); },
    }) +
    '<p class="muted addon-redis-spark-meta">avg ' +
    esc(Number(stats.avg).toFixed(digits) + (unit ? " " + unit : "")) +
    " · peak " +
    esc(Number(stats.max).toFixed(digits) + (unit ? " " + unit : "")) +
    " @ " +
    esc(fmtClockFromUnix(stats.maxTs)) +
    "</p></div>"
  );
}

function renderAddonRedisStatsGrid(data) {
  if (!data || !data.ok) {
    return '<p class="error-text">' + esc((data && data.error) || "Không tải được stats") + "</p>";
  }
  const mem = data.memory || {};
  const ops = data.ops || {};
  const clients = data.clients || {};
  const keys = data.keys || {};
  const k8s = data.k8s || {};
  let k8sHtml = "";
  if (k8s.available) {
    k8sHtml =
      '<div class="addon-redis-stat"><span class="muted">Pod CPU</span><strong>' +
      esc(String(k8s.cpu_cores != null ? k8s.cpu_cores : "—")) +
      ' core</strong></div>' +
      '<div class="addon-redis-stat"><span class="muted">Pod RAM</span><strong>' +
      esc(String(k8s.memory_mib != null ? k8s.memory_mib : "—")) +
      " MiB</strong></div>";
  }
  const usedPct = mem.used_pct != null && mem.used_pct > 0 ? mem.used_pct + "%" : "—";
  const policy = data.policy || {};
  let policyHtml = "";
  if (policy.configured) {
    policyHtml =
      '<div class="addon-redis-stat addon-redis-stat-wide"><span class="muted">Eviction</span><strong>' +
      esc(policy.active || policy.configured) +
      "</strong><span class=\"muted addon-redis-stat-sub\">" +
      esc(policy.hint || "") +
      (policy.default_ttl_sec ? " · TTL mặc định app: " + esc(String(policy.default_ttl_sec)) + "s" : "") +
      "</span></div>";
  }
  const exp = data.exporter || {};
  let expHtml = "";
  if (exp.available) {
    expHtml =
      '<div class="addon-redis-stat"><span class="muted">Exporter ops/s</span><strong>' +
      esc(String(exp.ops_per_sec != null ? exp.ops_per_sec : "—")) +
      "</strong></div>";
  }
  return (
    '<div class="addon-redis-stat"><span class="muted">Redis</span><strong>' +
    esc(data.redis_version || "—") +
    "</strong></div>" +
    '<div class="addon-redis-stat"><span class="muted">Memory</span><strong>' +
    esc(mem.used_human || "—") +
    "</strong><span class=\"muted addon-redis-stat-sub\">max " +
    esc(mem.maxmemory_human || "—") +
    " · " +
    esc(usedPct) +
    "</span></div>" +
    '<div class="addon-redis-stat"><span class="muted">Fragmentation</span><strong>' +
    esc(mem.fragmentation_ratio != null ? String(mem.fragmentation_ratio) : "—") +
    "</strong></div>" +
    '<div class="addon-redis-stat"><span class="muted">Clients</span><strong>' +
    esc(String(clients.connected != null ? clients.connected : "—")) +
    "</strong></div>" +
    '<div class="addon-redis-stat"><span class="muted">Ops/s</span><strong>' +
    esc(String(ops.instantaneous_ops_per_sec != null ? ops.instantaneous_ops_per_sec : "—")) +
    "</strong></div>" +
    '<div class="addon-redis-stat"><span class="muted">Hit rate</span><strong>' +
    esc(ops.hit_rate_pct != null ? ops.hit_rate_pct + "%" : "—") +
    "</strong></div>" +
    '<div class="addon-redis-stat"><span class="muted">Tổng key</span><strong>' +
    esc(String(keys.total != null ? keys.total : "—")) +
    "</strong></div>" +
    expHtml +
    policyHtml +
    k8sHtml
  );
}

function bindAddonRedisMonitor(slug, env) {
  const grid = document.getElementById("addon-redis-stats-grid");
  const tsEl = document.getElementById("addon-redis-stats-ts");
  const rawEl = document.getElementById("addon-redis-stats-raw");
  const grafanaLink = document.getElementById("addon-redis-grafana-link");
  const sparkEl = document.getElementById("addon-redis-sparklines");
  const policyBox = document.getElementById("addon-redis-policy-box");
  const slowWrap = document.getElementById("addon-redis-slowlog-wrap");
  const slowPre = document.getElementById("addon-redis-slowlog");
  const winSeg = document.getElementById("addon-redis-window-segment");
  if (!grid) return;
  const winKey = "addon-redis-window:" + slug + ":" + env;
  let windowValue = localStorage.getItem(winKey) || "6h";
  if (!/^(15m|1h|6h|24h)$/.test(windowValue)) windowValue = "6h";

  function paintWindowButtons() {
    if (!winSeg) return;
    winSeg.querySelectorAll(".seg-btn").forEach(function (btn) {
      const w = btn.getAttribute("data-win");
      btn.classList.toggle("active", w === windowValue);
    });
  }

  async function loadStats() {
    grid.innerHTML = '<p class="muted">Đang tải…</p>';
    try {
      const data = await api(
        "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis/stats" + projectQs({ environment: env, window: windowValue })
      );
      grid.innerHTML = renderAddonRedisStatsGrid(data);
      if (tsEl) tsEl.textContent = "cập nhật " + new Date().toLocaleTimeString();
      if (rawEl && data.ok) {
        rawEl.textContent = JSON.stringify(data, null, 2);
      }
      if (grafanaLink) {
        if (data.grafana_dashboard_url) {
          grafanaLink.href = data.grafana_dashboard_url;
          grafanaLink.classList.remove("hidden");
        } else {
          grafanaLink.classList.add("hidden");
        }
      }
      if (sparkEl && data.exporter && data.exporter.available) {
        const windowLabel = windowValue;
        sparkEl.classList.remove("hidden");
        sparkEl.innerHTML =
          renderAddonRedisSparkline("Memory " + windowLabel, data.exporter.memory_series, {
            divisor: 1024 * 1024,
            unit: "MiB",
            digits: 1,
            color: "#a78bfa",
            window: windowValue,
          }) +
          renderAddonRedisSparkline("Ops/s " + windowLabel, data.exporter.ops_series, {
            unit: "ops/s",
            digits: 2,
            color: "#60a5fa",
            window: windowValue,
          });
        bindInteractiveCharts(sparkEl);
      } else if (sparkEl) {
        sparkEl.classList.add("hidden");
        sparkEl.innerHTML = "";
      }
      if (policyBox && data.memory_doctor) {
        policyBox.classList.remove("hidden");
        policyBox.innerHTML =
          '<p class="muted"><strong>MEMORY DOCTOR</strong></p><pre class="addon-redis-doctor">' +
          esc(data.memory_doctor) +
          "</pre>";
      } else if (policyBox) {
        policyBox.classList.add("hidden");
        policyBox.innerHTML = "";
      }
      if (slowWrap && slowPre) {
        const rows = data.slowlog || [];
        if (rows.length) {
          slowWrap.classList.remove("hidden");
          slowPre.textContent = rows
            .map(function (r) {
              return (
                "#" +
                r.id +
                " · " +
                (r.duration_ms != null ? r.duration_ms + "ms" : r.duration_us + "µs") +
                " · " +
                (r.command || "")
              );
            })
            .join("\n");
        } else {
          slowWrap.classList.add("hidden");
          slowPre.textContent = "";
        }
      }
    } catch (err) {
      grid.innerHTML = '<p class="error-text">' + esc(err.message || String(err)) + "</p>";
    }
  }

  const refreshBtn = document.getElementById("addon-redis-stats-refresh");
  if (refreshBtn) refreshBtn.onclick = loadStats;
  if (winSeg) {
    winSeg.querySelectorAll(".seg-btn").forEach(function (btn) {
      btn.onclick = function () {
        const w = btn.getAttribute("data-win");
        if (!w || w === windowValue) return;
        windowValue = w;
        localStorage.setItem(winKey, windowValue);
        paintWindowButtons();
        loadStats();
      };
    });
  }
  paintWindowButtons();
  loadStats();
}

function bindAddonRedisKeys(slug, env, canManage) {
  const body = document.getElementById("addon-redis-keys-body");
  const meta = document.getElementById("addon-redis-keys-meta");
  const moreBtn = document.getElementById("addon-redis-keys-more");
  const patternEl = document.getElementById("addon-redis-keys-pattern");
  const warnEl = document.getElementById("addon-redis-keys-warn");
  const fullPre = document.getElementById("addon-redis-key-full");
  if (!body) return;

  let cursor = 0;
  let scanning = false;
  let lastClickKey = "";
  let lastClickAt = 0;

  function renderRows(items, append) {
    if (!append) body.innerHTML = "";
    if (!items || !items.length) {
      if (!append) body.innerHTML = '<tr><td colspan="4" class="muted">Không có key khớp pattern.</td></tr>';
      return;
    }
    const html = items
      .map(function (row) {
        const ttlCls = row.no_ttl ? " addon-redis-ttl-infinite" : "";
        return (
          "<tr data-key=\"" +
          esc(row.key) +
          "\" data-type=\"" +
          esc(row.type) +
          "\"><td><code class=\"addon-redis-key-name\">" +
          esc(row.key) +
          "</code></td><td><span class=\"badge neutral\">" +
          esc(row.type) +
          "</span></td><td class=\"" +
          ttlCls +
          "\">" +
          esc(row.ttl) +
          (row.no_ttl ? " ⚠" : "") +
          "</td><td class=\"addon-redis-key-preview\">" +
          esc(row.preview || "") +
          "</td></tr>"
        );
      })
      .join("");
    if (append) body.insertAdjacentHTML("beforeend", html);
    else body.innerHTML = html;
    body.querySelectorAll("tr[data-key]").forEach(function (tr) {
      tr.onclick = function () {
        const key = tr.getAttribute("data-key");
        const now = Date.now();
        if (key && key === lastClickKey && now - lastClickAt < 600) {
          loadFullValue(key, tr.getAttribute("data-type"));
        }
        lastClickKey = key;
        lastClickAt = now;
        body.querySelectorAll("tr.selected").forEach(function (r) {
          r.classList.remove("selected");
        });
        tr.classList.add("selected");
        const playKey = document.getElementById("addon-redis-play-key");
        if (playKey && key) playKey.value = key;
        const delBtn = document.getElementById("addon-redis-keys-del-selected");
        if (delBtn) delBtn.disabled = !canManage;
      };
    });
  }

  async function loadFullValue(key, typ) {
    if (!fullPre || typ !== "string") return;
    fullPre.classList.remove("hidden");
    fullPre.textContent = "Đang tải full value…";
    try {
      const data = await api(
        "/api/v1/projects/" +
          encodeURIComponent(slug) +
          "/addons/redis/keys" +
          projectQs({ environment: env, pattern: key, limit: 1, full: 1 })
      );
      const item = (data.items || [])[0];
      fullPre.textContent = item && item.value != null ? item.value : item ? item.preview || "(trống)" : "Không tìm thấy key";
    } catch (err) {
      fullPre.textContent = err.message || "Lỗi";
    }
  }

  async function scanKeys(append) {
    if (scanning) return;
    scanning = true;
    const pattern = patternEl ? patternEl.value : "*";
    if (!append) cursor = 0;
    if (meta) meta.textContent = "Đang quét…";
    try {
      const data = await api(
        "/api/v1/projects/" +
          encodeURIComponent(slug) +
          "/addons/redis/keys" +
          projectQs({ environment: env, pattern: pattern, cursor: append ? cursor : 0, limit: 40 })
      );
      cursor = data.cursor || 0;
      renderRows(data.items || [], append);
      if (warnEl) {
        if (data.no_ttl_count > 0) {
          warnEl.classList.remove("hidden");
          warnEl.innerHTML =
            '<p class="badge warn">Có <strong>' +
            esc(String(data.no_ttl_count)) +
            "</strong> key không TTL (∞) trong lô này — cân nhắc SET EX hoặc dùng REDIS_KEY_TTL_SECONDS.</p>";
        } else {
          warnEl.classList.add("hidden");
          warnEl.innerHTML = "";
        }
      }
      if (meta) {
        meta.textContent =
          (data.count || 0) + " key · pattern " + (data.pattern || pattern) + (data.has_more ? " · còn thêm" : "");
      }
      if (moreBtn) moreBtn.classList.toggle("hidden", !data.has_more);
    } catch (err) {
      if (!append) body.innerHTML = '<tr><td colspan="4" class="error-text">' + esc(err.message) + "</td></tr>";
      if (meta) meta.textContent = "";
    } finally {
      scanning = false;
    }
  }

  const scanBtn = document.getElementById("addon-redis-keys-scan");
  if (scanBtn) scanBtn.onclick = function () {
    scanKeys(false);
  };
  const filterConsole = document.getElementById("addon-redis-keys-filter-console");
  if (filterConsole && patternEl) {
    filterConsole.onclick = function () {
      patternEl.value = "console:*";
      scanKeys(false);
    };
  }
  if (moreBtn) moreBtn.onclick = function () {
    scanKeys(true);
  };
  const delBtn = document.getElementById("addon-redis-keys-del-selected");
  if (delBtn && canManage) {
    delBtn.onclick = async function () {
      const sel = body.querySelector("tr.selected");
      if (!sel) return;
      const key = sel.getAttribute("data-key");
      if (!key || !(await uiConfirm("Xóa key `" + key + "`?", { title: "Xóa Redis key" }))) return;
      try {
        await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis/play", {
          method: "POST",
          body: { environment: env, action: "del", key: key },
        });
        toastSuccess("Đã xóa key");
        await scanKeys(false);
      } catch (err) {
        toastError(err.message || "Không xóa được");
      }
    };
  }
}

function renderAddonRedisPlayground(canManage, slug, env) {
  if (!canManage) {
    return '<p class="muted">Chỉ owner/admin dùng playground Redis.</p>';
  }
  const appURL = slug
    ? "https://" + esc(slug) + "-" + esc(env) + ".platform.7mlabs.com/api/redis/ping"
    : "";
  return (
    '<div class="addon-redis-play">' +
    '<p class="muted">Thử kết nối trực tiếp từ Console (cluster → Redis). Key demo nên prefix <code>console:</code>.</p>' +
    '<div class="addon-redis-play-actions">' +
    '<button type="button" class="btn-primary btn-sm" id="addon-redis-play-ping">Ping · PONG</button> ' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-redis-play-info">INFO</button>' +
    "</div>" +
    '<div class="addon-redis-play-form">' +
    '<label>Key<input type="text" id="addon-redis-play-key" placeholder="console:hello" value="console:hello"></label>' +
    '<label>Value<input type="text" id="addon-redis-play-val" placeholder="world"></label>' +
    '<div class="addon-redis-play-form-btns">' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-redis-play-get">GET</button> ' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-redis-play-set">SET</button> ' +
    '<button type="button" class="btn-ghost btn-sm" id="addon-redis-play-del">DEL</button>' +
    "</div></div>" +
  (appURL
    ? '<p class="muted addon-redis-app-probe">App probe: <a href="' + appURL + '" target="_blank" rel="noopener"><code>/api/redis/ping</code></a> (cần template backend có route Redis)</p>'
    : "") +
    '<pre class="addon-redis-play-out muted" id="addon-redis-play-out">Bấm Ping để thử…</pre>' +
    "</div>"
  );
}

function addonRedisCollapsibleSummary(title, badgeHtml, actionsHtml) {
  badgeHtml = badgeHtml || "";
  actionsHtml = actionsHtml || "";
  return (
    '<div class="deploy-collapsible-summary-inner">' +
    '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
    '<div class="deploy-collapsible-title-row">' +
    "<h3 style=\"margin:0\">" + esc(title) + "</h3>" +
    (badgeHtml ? '<span class="deploy-collapsible-no-toggle">' + badgeHtml + "</span>" : "") +
    "</div>" +
    (actionsHtml
      ? '<div class="deploy-collapsible-summary-actions deploy-collapsible-no-toggle">' + actionsHtml + "</div>"
      : "") +
    "</div>"
  );
}

function openAddonRedisHashSection(root) {
  if (!root) return;
  const raw = (location.hash || "").replace(/^#/, "");
  if (!raw || raw.indexOf("addon-sec-") !== 0) return;
  const el = document.getElementById(raw);
  if (el && el.tagName === "DETAILS") {
    el.open = true;
    setTimeout(function () {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 80);
  }
}

function bindAddonRedisPlayground(slug, env) {
  const out = document.getElementById("addon-redis-play-out");
  if (!out) return;
  async function runPlay(action, extra) {
    out.textContent = "Đang gọi Redis…";
    try {
      const body = Object.assign({ environment: env, action: action }, extra || {});
      const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis/play", {
        method: "POST",
        body: body,
      });
      const lines = [
        res.ok ? "✓ OK" : "✗ Lỗi",
        "action: " + (res.action || action),
        res.result != null ? "result: " + res.result : "",
        res.key ? "key: " + res.key : "",
        res.value != null && res.value !== "" ? "value: " + res.value : "",
        res.latency_ms != null ? "latency: " + res.latency_ms + " ms" : "",
        res.redis_url_masked ? "via: " + res.redis_url_masked : "",
        res.error ? "error: " + res.error : "",
      ].filter(Boolean);
      out.textContent = lines.join("\n");
    } catch (err) {
      out.textContent = "Lỗi: " + (err.message || String(err));
    }
  }
  const pingBtn = document.getElementById("addon-redis-play-ping");
  if (pingBtn) pingBtn.onclick = function () { runPlay("ping"); };
  const infoBtn = document.getElementById("addon-redis-play-info");
  if (infoBtn) infoBtn.onclick = function () { runPlay("info"); };
  const getBtn = document.getElementById("addon-redis-play-get");
  if (getBtn) {
    getBtn.onclick = function () {
      runPlay("get", { key: document.getElementById("addon-redis-play-key").value });
    };
  }
  const setBtn = document.getElementById("addon-redis-play-set");
  if (setBtn) {
    setBtn.onclick = function () {
      runPlay("set", {
        key: document.getElementById("addon-redis-play-key").value,
        value: document.getElementById("addon-redis-play-val").value,
        ttl_seconds: 3600,
      });
    };
  }
  const delBtn = document.getElementById("addon-redis-play-del");
  if (delBtn) {
    delBtn.onclick = function () {
      runPlay("del", { key: document.getElementById("addon-redis-play-key").value });
    };
  }
}

function addonRedisCollapsibleSummary(title, badgeHtml, actionsHtml) {
  badgeHtml = badgeHtml || "";
  actionsHtml = actionsHtml || "";
  return (
    '<div class="deploy-collapsible-summary-inner">' +
    '<span class="deploy-collapsible-chev" aria-hidden="true"></span>' +
    '<div class="deploy-collapsible-title-row">' +
    "<h3 style=\"margin:0\">" + esc(title) + "</h3>" +
    (badgeHtml ? '<span class="deploy-collapsible-no-toggle">' + badgeHtml + "</span>" : "") +
    "</div>" +
    (actionsHtml
      ? '<div class="deploy-collapsible-summary-actions deploy-collapsible-no-toggle">' + actionsHtml + "</div>"
      : "") +
    "</div>"
  );
}

function openAddonRedisHashSection(root) {
  if (!root) return;
  const raw = (location.hash || "").replace(/^#/, "");
  if (!raw || raw.indexOf("addon-sec-") !== 0) return;
  const el = document.getElementById(raw);
  if (el && el.tagName === "DETAILS") {
    el.open = true;
    setTimeout(function () {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 80);
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
  if (engine === "minio") return "📦";
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
      '<div class="addon-redis-doc-box">' +
      "<p><strong>Dùng trong app</strong></p>" +
      '<ul class="addon-redis-doc-list muted">' +
      "<li>Runtime env: <code>REDIS_URL</code> (secret) — Platform inject vào <code>app-env</code></li>" +
      (addon.default_key_ttl_sec
        ? "<li>TTL gợi ý: <code>REDIS_KEY_TTL_SECONDS=" + esc(String(addon.default_key_ttl_sec)) + "</code></li>"
        : "<li>TTL gợi ý: <code>REDIS_KEY_TTL_SECONDS</code> (nếu cấu hình ở Quota)</li>") +
      "<li>Go template: <code>GET /api/redis/ping</code> · <code>/api/redis/demo</code></li>" +
      "<li>Doc: <code>docs/REDIS-ADDON.md</code> trên repo platform</li>" +
      "</ul></div>" +
      '<p class="muted addon-connection-hint">App trong cluster dùng REDIS_URL runtime. Dev local dùng URL external (nếu có). Prod: chỉ ClusterIP + NetworkPolicy.</p>' +
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

function renderAddonRedisQuota(addon, canManage, ceilings) {
  ceilings = ceilings || {};
  const memMax = ceilings.redis_max_memory_mb || 512;
  const clientsMax = ceilings.redis_max_clients || 1000;
  const mem = addon.max_memory_mb || 128;
  const clients = addon.max_clients || 100;
  const policy = addon.maxmemory_policy || "allkeys-lru";
  const ttl = addon.default_key_ttl_sec != null ? addon.default_key_ttl_sec : 86400;
  const policies = [
    "allkeys-lru",
    "volatile-lru",
    "allkeys-lfu",
    "volatile-lfu",
    "volatile-ttl",
    "noeviction",
  ];
  if (!canManage) {
    return (
      "<p>RAM: <strong>" + esc(mem) + " MB</strong> · Max clients: <strong>" + esc(clients) + "</strong></p>" +
      "<p>Eviction: <strong>" + esc(policy) + "</strong> · TTL mặc định app: <strong>" + esc(ttl) + "s</strong></p>" +
      (addon.policy_hint ? '<p class="muted">' + esc(addon.policy_hint) + "</p>" : "") +
      '<p class="muted">Chỉ owner/admin chỉnh quota. Trần platform: RAM ≤ ' + esc(memMax) + " · clients ≤ " + esc(clientsMax) + "</p>"
    );
  }
  const policyOpts = policies
    .map(function (p) {
      return '<option value="' + esc(p) + '"' + (p === policy ? " selected" : "") + ">" + esc(p) + "</option>";
    })
    .join("");
  return (
    '<p class="muted">Trần platform: RAM ≤ <code>' + esc(memMax) + "</code> MB · clients ≤ <code>" + esc(clientsMax) + "</code></p>" +
    '<div class="addon-quota-form">' +
    '<label class="addon-quota-field">RAM (MB)<input type="number" id="addon-redis-mem" min="64" max="' + esc(memMax) + '" step="1" value="' + esc(mem) + '"></label>' +
    '<label class="addon-quota-field">Max clients<input type="number" id="addon-redis-clients" min="10" max="' + esc(clientsMax) + '" step="1" value="' + esc(clients) + '"></label>' +
    '<label class="addon-quota-field">Eviction policy<select id="addon-redis-policy">' + policyOpts + "</select></label>" +
    '<label class="addon-quota-field">TTL mặc định app (s)<input type="number" id="addon-redis-default-ttl" min="0" max="2592000" step="1" value="' + esc(ttl) + '"></label>' +
    '<button type="button" class="btn-primary btn-sm" id="addon-redis-quota-apply">Lưu & apply</button>' +
    "</div>" +
    (addon.policy_hint ? '<p class="muted">' + esc(addon.policy_hint) + "</p>" : "") +
    '<p class="muted">Apply = re-provision (redis.conf, redis_exporter, inject <code>REDIS_KEY_TTL_SECONDS</code> vào runtime env).</p>'
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
      const policyEl = document.getElementById("addon-redis-policy");
      const ttlEl = document.getElementById("addon-redis-default-ttl");
      if (!(mem >= 64 && mem <= 512)) {
        toastError("RAM phải từ 64–512 MB");
        return;
      }
      if (!(clients >= 10 && clients <= 1000)) {
        toastError("Max clients phải từ 10–1000");
        return;
      }
      const ttl = ttlEl ? parseInt(ttlEl.value, 10) : 86400;
      if (!(ttl >= 0 && ttl <= 2592000)) {
        toastError("TTL mặc định phải từ 0–2592000 giây");
        return;
      }
      runProvision({
        max_memory_mb: mem,
        max_clients: clients,
        maxmemory_policy: policyEl ? policyEl.value : "allkeys-lru",
        default_key_ttl_sec: ttl,
      });
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
          const body = { environment: env, max_memory_mb: 128, max_clients: 100 };
          if (engine === "minio") {
            body.max_memory_mb = 256;
            body.storage_gb = 5;
            body.topology = "standalone";
          }
          const res = await withAppLoading(function () {
            return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/" + encodeURIComponent(engine), {
              method: "POST",
              body: body,
              timeout: 300000,
            });
          }, {
            title: "Đang thêm " + engine + "…",
            detail: "Provision trên cluster — có thể mất 1–2 phút",
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

async function loadProjectAddonMinio(main, slug, p) {
  const env = state.projectEnv || "dev";
  main.innerHTML =
    projectHeader(p, "MinIO · addon") +
    projectEnvToolbar(slug, p, function () {
      pageProjectHub(main, slug, "addons", "minio");
    }) +
    '<div id="addon-minio-root" class="card"><p class="loading">Đang tải MinIO…</p></div>';
  try {
    const data = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio" + projectQs({ environment: env })
    );
    const bucketData = data.installed
      ? await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/buckets" + projectQs({ environment: env })).catch(function () { return { items: [] }; })
      : { items: [] };
    const ceilings = await api("/api/v1/platform/policy/ceilings").catch(function () { return {}; });
    const storageMax = ceilings.minio_max_storage_gb || 100;
    const memMax = ceilings.minio_max_memory_mb || 1024;
    const objectMax = ceilings.minio_max_object_mb || 5120;
    const root = document.getElementById("addon-minio-root");
    if (!root) return;
    const ha = data.ha_capability || {};
    const haBadge = ha.capable
      ? '<span class="badge ok">HA capable</span>'
      : '<span class="badge muted">standalone</span>';
    if (!data.installed) {
      root.innerHTML =
        "<h3>MinIO · " + esc(env.toUpperCase()) + "</h3>" +
        '<p class="muted">Object storage S3-compatible — mỗi env một instance riêng.</p>' +
        '<p class="muted">Topology mặc định: <strong>standalone</strong> · ' + haBadge + "</p>" +
        (Array.isArray(ha.reasons) && ha.reasons.length
          ? '<ul class="muted" style="margin:8px 0 12px 18px">' +
            ha.reasons.map(function (r) { return "<li>" + esc(r) + "</li>"; }).join("") +
            "</ul>"
          : "") +
        (ha.capable
          ? '<p style="margin:10px 0"><label class="muted">Topology </label>' +
            '<select id="addon-minio-topology">' +
            '<option value="standalone" selected>standalone</option>' +
            '<option value="distributed">distributed (4 pods + Longhorn)</option>' +
            "</select></p>"
          : "") +
        (data.can_manage
          ? '<div class="addon-quota-form" style="margin:12px 0">' +
            '<label class="addon-quota-field">Storage (GB)<input type="number" id="addon-minio-enable-storage" min="1" max="' +
            esc(String(storageMax)) +
            '" value="5"></label>' +
            '<label class="addon-quota-field">Max object (MiB)<input type="number" id="addon-minio-enable-object" min="1" max="' +
            esc(String(objectMax)) +
            '" value="100"></label>' +
            '<label class="addon-quota-field">RAM pod (MB)<input type="number" id="addon-minio-enable-mem" min="128" max="' +
            esc(String(memMax)) +
            '" value="256"></label>' +
            '<p class="muted" style="width:100%">Trần platform: ≤ ' +
            esc(String(storageMax)) +
            " Gi · object ≤ " +
            esc(String(objectMax)) +
            " Mi · RAM ≤ " +
            esc(String(memMax)) +
            " Mi</p></div>" +
            '<button type="button" class="btn-primary" id="addon-minio-enable">+ Bật MinIO</button> ' +
            '<a class="btn-ghost" href="' + esc(projectAddonsRoute(slug)) + '">← Catalog</a>'
          : '<p class="muted">Chỉ owner/admin được bật addon.</p>');
      const enableBtn = document.getElementById("addon-minio-enable");
      if (enableBtn) {
        enableBtn.onclick = async function () {
          const topoEl = document.getElementById("addon-minio-topology");
          const topology = topoEl && topoEl.value === "distributed" ? "distributed" : "standalone";
          let storageGB = parseInt((document.getElementById("addon-minio-enable-storage") || {}).value, 10);
          let memMB = parseInt((document.getElementById("addon-minio-enable-mem") || {}).value, 10);
          let objectMB = parseInt((document.getElementById("addon-minio-enable-object") || {}).value, 10);
          if (!(storageGB >= 1 && storageGB <= storageMax)) storageGB = 5;
          if (!(memMB >= 128 && memMB <= memMax)) memMB = 256;
          if (!(objectMB >= 1 && objectMB <= objectMax)) objectMB = 100;
          try {
            await withAppLoading(function () {
              return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio", {
                method: "POST",
                body: {
                  environment: env,
                  max_memory_mb: memMB,
                  storage_gb: storageGB,
                  max_object_mb: objectMB,
                  topology: topology,
                },
                timeout: 300000,
              });
            }, {
              title: "Đang provision MinIO…",
              detail: topology === "distributed" ? "Distributed 4 pods + bucket app" : "StatefulSet + bucket app + inject S3_*",
            });
            toastSuccess("Đã bật MinIO");
            await loadProjectAddonMinio(main, slug, p);
          } catch (err) {
            toastError(err.message || "Lỗi");
          }
        };
      }
      return;
    }
    const addon = data.addon || {};
    const canManage = !!data.can_manage;
    const buckets = bucketData.items || [];
    state.minioBucketSelection = state.minioBucketSelection || {};
    const selectedBucket = state.minioBucketSelection[slug + ":" + env] ||
      ((buckets.filter(function (b) { return b.is_default; })[0] || {}).name || addon.bucket || "app");
    const reasons = (addon.ha_capability && addon.ha_capability.reasons) || ha.reasons || [];
    const overviewBody = renderAddonMinioOverview(addon, haBadge, reasons);
    const connBody = renderAddonMinioConnection(addon, canManage);
    const quotaBody = renderAddonMinioQuota(addon, canManage, ceilings);
    const monitorBody = addon.has_connection ? renderAddonMinioMonitor() : '<p class="muted">Chưa có connection — chưa có metrics.</p>';
    const filesBody = addon.has_connection
      ? renderAddonMinioFiles(canManage, selectedBucket, buckets)
      : '<p class="muted">Bật MinIO và chờ connection để duyệt file.</p>';
    const opsBody = renderAddonMinioOps(canManage);
    const footerActions = canManage
      ? '<div style="margin-top:8px">' +
        '<button type="button" class="btn-ghost" id="addon-minio-reprovision">Re-provision</button> ' +
        (addon.upgrade_available
          ? '<button type="button" class="btn-primary" id="addon-minio-upgrade-ha">Upgrade HA</button> '
          : !ha.capable
            ? '<button type="button" class="btn-ghost" disabled title="Cần Longhorn + ≥2 node">Upgrade HA</button> '
            : "") +
        '<a class="btn-ghost" href="' + esc(projectAddonsRoute(slug)) + '">← Catalog</a></div>'
      : '<p style="margin-top:12px"><a href="' + esc(projectAddonsRoute(slug)) + '">← Catalog</a></p>';

    root.className = "addon-minio-root";
    root.innerHTML =
      '<p class="muted addon-redis-lead">MinIO · <strong>' +
      esc(env.toUpperCase()) +
      "</strong> · <code>" +
      esc(data.namespace || "") +
      "</code></p>" +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-buckets",
        addonRedisCollapsibleSummary("Buckets", '<span class="badge neutral">' + esc(String(buckets.length)) + " bucket</span>"),
        renderAddonMinioBuckets(bucketData, canManage),
        true,
        { id: "addon-sec-minio-buckets", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-overview",
        addonRedisCollapsibleSummary("Overview", addonStatusBadge(addon.status) + " " + haBadge),
        overviewBody,
        true,
        { id: "addon-sec-minio-overview", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-connection",
        addonRedisCollapsibleSummary(
          "Connection",
          addon.has_connection ? '<span class="badge ok">Đã cấu hình</span>' : '<span class="badge warn">Chưa có</span>'
        ),
        connBody,
        true,
        { id: "addon-sec-minio-connection", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-quota",
        addonRedisCollapsibleSummary(
          "Quota",
          '<span class="badge neutral">' +
            esc(String(addon.storage_gb || 5)) +
            " Gi · obj " +
            esc(String(addon.max_object_mb || 100)) +
            " Mi · " +
            esc(String(addon.max_memory_mb || 256)) +
            " Mi RAM</span>"
        ),
        quotaBody,
        true,
        { id: "addon-sec-minio-quota", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-files",
        addonRedisCollapsibleSummary("Files", '<span class="badge muted">List · Upload · Xóa</span>'),
        filesBody,
        true,
        { id: "addon-sec-minio-files", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-monitor",
        addonRedisCollapsibleSummary("Monitor", '<span class="badge muted">Prometheus · Grafana</span>'),
        monitorBody,
        false,
        { id: "addon-sec-minio-monitor", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-minio-ops",
        addonRedisCollapsibleSummary("Ops", '<span class="badge muted">Restart · Logs</span>'),
        opsBody,
        false,
        { id: "addon-sec-minio-ops", extraClass: "addon-redis-collapsible" }
      ) +
      footerActions;

    bindDeployCollapsibleCards(root, slug);
    bindAddonMinioBuckets(slug, env, canManage, function () { loadProjectAddonMinio(main, slug, p); });
    if (addon.has_connection) {
      bindAddonMinioFiles(slug, env, canManage, selectedBucket, function () { loadProjectAddonMinio(main, slug, p); });
    }
    const quotaValidate = canManage ? bindAddonMinioQuotaValidation(storageMax, memMax, objectMax) : null;
    const openQuotaBtn = document.getElementById("addon-minio-open-quota");
    if (openQuotaBtn) {
      openQuotaBtn.onclick = function () {
        const card = document.getElementById("addon-sec-minio-quota");
        if (card) {
          card.open = true;
          try {
            setDeploySectionOpen(slug, "addon-minio-quota", true);
          } catch (e) {}
          card.scrollIntoView({ behavior: "smooth", block: "start" });
        }
      };
    }

    function minioS3EnvPayload(endpoint) {
      return JSON.stringify(
        {
          S3_ENDPOINT: endpoint || addon.endpoint,
          S3_ENDPOINT_CLUSTER: addon.endpoint,
          S3_ACCESS_KEY: addon.access_key,
          S3_SECRET_KEY: addon.secret_key,
          S3_BUCKET: addon.bucket || "app",
          S3_REGION: "us-east-1",
          S3_USE_SSL: "false",
          S3_FORCE_PATH_STYLE: "true",
          S3_MAX_OBJECT_MB: String(addon.max_object_mb || 100),
        },
        null,
        2
      );
    }
    const copyEp = document.getElementById("addon-minio-copy-endpoint");
    if (copyEp && addon.endpoint) {
      copyEp.onclick = function () {
        navigator.clipboard.writeText(addon.endpoint).then(function () {
          toastSuccess("Đã copy cluster endpoint");
        });
      };
    }
    const copyExt = document.getElementById("addon-minio-copy-ext");
    if (copyExt && addon.endpoint_external) {
      copyExt.onclick = function () {
        navigator.clipboard.writeText(addon.endpoint_external).then(function () {
          toastSuccess("Đã copy external endpoint");
        });
      };
    }
    const copyExtJson = document.getElementById("addon-minio-copy-ext-json");
    if (copyExtJson && addon.access_key && addon.endpoint_external) {
      copyExtJson.onclick = function () {
        navigator.clipboard.writeText(minioS3EnvPayload(addon.endpoint_external)).then(function () {
          toastSuccess("Đã copy external S3 JSON");
        });
      };
    }
    const copyKeys = document.getElementById("addon-minio-copy-keys");
    if (copyKeys && addon.access_key) {
      copyKeys.onclick = function () {
        navigator.clipboard.writeText(minioS3EnvPayload(addon.endpoint)).then(function () {
          toastSuccess("Đã copy S3 credentials (cluster)");
        });
      };
    }
    if (addon.has_connection) {
      bindAddonMinioMonitor(slug, env);
    }
    const restartBtn = document.getElementById("addon-minio-restart");
    if (restartBtn) {
      restartBtn.onclick = async function () {
        const ok = await uiConfirm("Restart pod MinIO?", { title: "Restart MinIO" });
        if (!ok) return;
        restartBtn.disabled = true;
        try {
          const res = await api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/restart", {
            method: "POST",
            body: { environment: env },
          });
          toastSuccess(res.message || "Đang restart MinIO");
          await loadProjectAddonMinio(main, slug, p);
        } catch (err) {
          toastError(err.message || "Restart thất bại");
          restartBtn.disabled = false;
        }
      };
    }
    const logsBtn = document.getElementById("addon-minio-load-logs");
    const logsPre = document.getElementById("addon-minio-logs");
    if (logsBtn && logsPre) {
      logsBtn.onclick = async function () {
        logsBtn.disabled = true;
        logsPre.classList.remove("hidden");
        logsPre.textContent = "Đang tải logs…";
        try {
          const res = await api(
            "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/logs" + projectQs({ environment: env, tail: 300 })
          );
          logsPre.textContent = res.logs || "(trống)";
        } catch (err) {
          logsPre.textContent = err.message || "Không tải được logs";
        } finally {
          logsBtn.disabled = false;
        }
      };
    }
    const upgradeBtn = document.getElementById("addon-minio-upgrade-ha");
    if (upgradeBtn) {
      upgradeBtn.onclick = async function () {
        const ok = await uiConfirm({
          title: "Upgrade MinIO → HA / distributed?",
          message:
            "Sẽ tạo 4 pods + PVC Longhorn. Object trên PVC standalone KHÔNG tự migrate. Credentials giữ nguyên. Tiếp tục?",
          confirmText: "Upgrade HA",
          danger: true,
        });
        if (!ok) return;
        try {
          await withAppLoading(function () {
            return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/upgrade-ha", {
              method: "POST",
              body: { environment: env, confirm: true },
              timeout: 300000,
            });
          }, { title: "Đang upgrade MinIO HA…", detail: "Distributed 4 pods + Longhorn" });
          toastSuccess("Đã upgrade MinIO sang distributed");
          await loadProjectAddonMinio(main, slug, p);
        } catch (err) {
          toastError(err.message || "Upgrade thất bại");
        }
      };
    }
    const repro = document.getElementById("addon-minio-reprovision");
    if (repro) {
      repro.onclick = async function () {
        const ok = await uiConfirm({
          title: "Re-provision MinIO?",
          message: "Tạo lại credentials và sync S3_* vào app-env. PVC data giữ nguyên nếu volumeClaim còn.",
          confirmText: "Re-provision",
          danger: true,
        });
        if (!ok) return;
        try {
          await withAppLoading(function () {
            return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/provision", {
              method: "POST",
              body: {
                environment: env,
                storage_gb: addon.storage_gb || 5,
                max_memory_mb: addon.max_memory_mb || 256,
                max_object_mb: addon.max_object_mb || 100,
                topology: addon.topology || "standalone",
              },
              timeout: 300000,
            });
          }, { title: "Đang re-provision MinIO…", detail: "Cập nhật secret + env" });
          toastSuccess("Đã re-provision MinIO");
          await loadProjectAddonMinio(main, slug, p);
        } catch (err) {
          toastError(err.message || "Lỗi");
        }
      };
    }
    const quotaApply = document.getElementById("addon-minio-quota-apply");
    if (quotaApply) {
      quotaApply.onclick = async function () {
        if (quotaValidate && !quotaValidate()) {
          toastError("Giá trị vượt trần Platform Policy — không lưu được");
          return;
        }
        const storageGB = parseInt(document.getElementById("addon-minio-storage").value, 10);
        const memMB = parseInt(document.getElementById("addon-minio-mem").value, 10);
        const objectEl = document.getElementById("addon-minio-object");
        const objectMB = objectEl ? parseInt(objectEl.value, 10) : addon.max_object_mb || 100;
        if (!(storageGB >= 1 && storageGB <= storageMax)) {
          toastError("Storage phải từ 1–" + storageMax + " GB (trần platform)");
          return;
        }
        if (!(objectMB >= 1 && objectMB <= objectMax)) {
          toastError("Max object phải từ 1–" + objectMax + " MiB (trần platform)");
          return;
        }
        if (!(memMB >= 128 && memMB <= memMax)) {
          toastError("RAM phải từ 128–" + memMax + " MB (trần platform)");
          return;
        }
        const ok = await uiConfirm({
          title: "Áp dụng quota MinIO?",
          message:
            "Re-provision với storage " +
            storageGB +
            " Gi · max object " +
            objectMB +
            " Mi · RAM " +
            memMB +
            " Mi.",
          confirmText: "Lưu & apply",
        });
        if (!ok) return;
        try {
          await withAppLoading(function () {
            return api("/api/v1/projects/" + encodeURIComponent(slug) + "/addons/minio/provision", {
              method: "POST",
              body: {
                environment: env,
                storage_gb: storageGB,
                max_memory_mb: memMB,
                max_object_mb: objectMB,
                topology: addon.topology || "standalone",
              },
              timeout: 300000,
            });
          }, {
            title: "Đang apply quota MinIO…",
            detail: storageGB + " Gi · obj " + objectMB + " Mi · " + memMB + " Mi RAM",
          });
          toastSuccess("Đã cập nhật quota MinIO");
          await loadProjectAddonMinio(main, slug, p);
        } catch (err) {
          toastError(err.message || "Lỗi");
        }
      };
    }
  } catch (err) {
    const root = document.getElementById("addon-minio-root");
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
    '<div id="addon-redis-root"><p class="loading">Đang tải Redis…</p></div>';
  try {
    const data = await api(
      "/api/v1/projects/" + encodeURIComponent(slug) + "/addons/redis" + projectQs({ environment: env })
    );
    const ceilings = await api("/api/v1/platform/policy/ceilings").catch(function () { return {}; });
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
    const reprovisionBtn = canManage
      ? '<button type="button" class="btn-ghost btn-sm" id="addon-redis-reprovision-top">Re-provision</button>'
      : "";
    const dashBody =
      '<p>Trạng thái: ' + addonStatusBadge(addon.status) + "</p>" +
      '<p class="muted">Release: <code>' + esc(addon.k8s_release || "—") + "</code></p>" +
      '<p class="muted">Namespace: <code>' + esc(data.namespace || "—") + "</code></p>" +
      renderAddonRedisDashboard(addon, canManage);
    const connBody = renderAddonRedisConnection(addon, canManage);
    const quotaBody = renderAddonRedisQuota(addon, canManage, ceilings);
    const playBody = renderAddonRedisPlayground(canManage, slug, env);
    const monitorBody = renderAddonRedisMonitor();
    const keysBody = renderAddonRedisKeys(canManage);
    root.className = "addon-redis-root";
    root.innerHTML =
      '<p class="muted addon-redis-lead">Redis · <strong>' +
      esc(env.toUpperCase()) +
      "</strong> · <code>" +
      esc(data.namespace || "") +
      "</code></p>" +
      renderDeployCollapsibleCard(
        slug,
        "addon-redis-dashboard",
        addonRedisCollapsibleSummary("Dashboard", addonStatusBadge(addon.status)),
        dashBody,
        true,
        { id: "addon-sec-dashboard", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-redis-monitor",
        addonRedisCollapsibleSummary("Monitor", '<span class="badge muted">INFO · Prometheus</span>'),
        monitorBody,
        false,
        { id: "addon-sec-monitor", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-redis-keys",
        addonRedisCollapsibleSummary("Dữ liệu đang lưu", '<span class="badge muted">SCAN keys</span>'),
        keysBody,
        false,
        { id: "addon-sec-keys", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-redis-connection",
        addonRedisCollapsibleSummary(
          "Connection",
          addon.has_connection ? '<span class="badge ok">Đã cấu hình</span>' : '<span class="badge warn">Chưa có URL</span>',
          reprovisionBtn
        ),
        connBody,
        false,
        { id: "addon-sec-connection", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-redis-quota",
        addonRedisCollapsibleSummary(
          "Quota",
          '<span class="badge neutral">' + esc(String(addon.max_memory_mb || 128)) + " MB</span>"
        ),
        quotaBody,
        false,
        { id: "addon-sec-quota", extraClass: "addon-redis-collapsible" }
      ) +
      renderDeployCollapsibleCard(
        slug,
        "addon-redis-play",
        addonRedisCollapsibleSummary("Playground", canManage ? "" : '<span class="badge muted">Chỉ admin</span>'),
        playBody,
        false,
        { id: "addon-sec-play", extraClass: "addon-redis-collapsible" }
      ) +
      '<p class="addon-back-wrap"><a class="addon-back-link" href="' + esc(projectAddonsRoute(slug)) + '">← Về catalog addons</a></p>';
    bindDeployCollapsibleCards(root, slug);
    openAddonRedisHashSection(root);
    bindAddonRedisMonitor(slug, env);
    bindAddonRedisKeys(slug, env, canManage);
    bindAddonRedisActions(main, slug, p, env, canManage, addon);
    bindAddonRedisPlayground(slug, env);
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
      '<div class="card" id="promote-page"></div>';
    bindDeployHelpTriggers(main);
    showAppLoading("Đang kiểm tra checklist Promote…", "Quét image dev, contract env và addons prod");
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
          try {
            await withAppLoading(function () {
              return api("/api/v1/projects/" + encodeURIComponent(slug) + "/deploy/promote", {
                method: "POST",
                body: { image_tag: t },
                timeout: 300000,
              });
            }, {
              title: "Đang promote lên Prod…",
              detail: "Sync GitOps, ArgoCD và runtime — có thể mất 1–3 phút",
            });
            navigateAfterPromote(slug, t);
          } catch (err) {
            toastError(err.message || "Promote prod thất bại");
          }
        };
      }
    } catch (err) {
      document.getElementById("promote-page").innerHTML = '<p class="error-text">' + esc(err.message) + "</p>";
    } finally {
      hideAppLoading();
    }
}
