/* App bootstrap — keep thin. */

window.addEventListener("hashchange", function () {
  if (state.user) navigate();
});

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
