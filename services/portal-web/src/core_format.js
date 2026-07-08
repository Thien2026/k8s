function errorMessage(err, fallback) {
  fallback = fallback || "Không tải được dữ liệu — thử tải lại trang";
  if (!err) return fallback;
  const msg = String(err.message || err.error || err).trim();
  return msg || fallback;
}

function esc(s) {
  if (s == null) return "";
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function fmtTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const diffMs = Date.now() - d.getTime();
  if (diffMs < 0) {
    return d.toLocaleString("vi-VN", {
      day: "2-digit",
      month: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
  }
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
