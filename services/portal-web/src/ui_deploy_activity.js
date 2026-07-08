/* Deploy activity card, actions, poll. */

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
