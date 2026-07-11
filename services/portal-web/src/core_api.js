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

async function api(path, opts) {
  opts = opts || {};
  const isAuth = path.indexOf("/auth/") >= 0;
  const method = String(opts.method || "GET").toUpperCase();
  const isWrite = method !== "GET" && method !== "HEAD" && method !== "OPTIONS";
  // Loading mặc định cho mọi thao tác ghi; GET chỉ khi opts.loading === true.
  // Tắt bằng opts.noLoading / opts.loading === false (poll, silent, auth refresh).
  const wantLoading =
    opts.loading === true ||
    (isWrite && opts.loading !== false && !opts.noLoading && !opts.silent401 && path !== "/api/v1/auth/refresh");
  const loadingTitle = opts.loadingTitle || (isWrite ? "Đang xử lý…" : "Đang tải…");
  const loadingDetail = opts.loadingDetail || "";
  if (wantLoading) showAppLoading(loadingTitle, loadingDetail);

  const timeoutMs = opts.timeout != null ? opts.timeout : 60000;
  const ctrl = new AbortController();
  const timer = timeoutMs > 0 ? setTimeout(function () { ctrl.abort(); }, timeoutMs) : null;
  async function doFetch() {
    return fetch(path, {
      method: method,
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
    if (wantLoading) hideAppLoading();
  }
}
