/* Deploy history + promote prep UI. */

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
        (it.id === "redis_prod" ? "/redis" : "") +
        '" data-promote-tab="' +
        esc(it.tab) +
        '" data-promote-env="' +
        esc(prepEnv) +
        '">Cấu hình →</a>'
      : "";
  const actionBtn =
    it.id === "redis_prod" && !it.ok && !isWarn
      ? ' <button type="button" class="btn-ghost btn-sm promote-redis-prod-btn" data-slug="' +
        esc(slug) +
        '">Provision Redis prod</button>'
      : "";
  return (
    '<li class="promote-prep-item ' + cls + '">' +
    '<span class="promote-prep-icon">' + icon + "</span>" +
    "<span><strong>" + esc(it.label) + "</strong>" +
    (it.detail ? '<span class="muted"> — ' + esc(it.detail) + "</span>" : "") +
    tabLink +
    actionBtn +
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
      if (tab === "addons") {
        state.projectEnv = "prod";
        localStorage.setItem("project-env", "prod");
      }
    };
  });
  document.querySelectorAll(".promote-redis-prod-btn").forEach(function (btn) {
    btn.onclick = async function () {
      const s = btn.dataset.slug;
      if (!s) return;
      btn.disabled = true;
      try {
        const res = await api("/api/v1/projects/" + encodeURIComponent(s) + "/addons/redis/promote-prod", {
          method: "POST",
          body: {},
        });
        toastSuccess(res.message || "Đã provision Redis prod");
        const main = document.getElementById("main-content");
        const p = state.currentProject;
        if (main && p && state.projectTab === "promote") {
          await loadProjectPromote(main, s, p);
        }
      } catch (err) {
        toastError(err.message || "Provision Redis prod thất bại");
        btn.disabled = false;
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
