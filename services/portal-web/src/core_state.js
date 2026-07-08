/* Shared app state. */

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
