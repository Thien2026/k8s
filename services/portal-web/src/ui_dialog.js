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
