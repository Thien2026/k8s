package handler

import "net/url"

const grafanaNamespacePodsUID = "85a562078cdf77779eaa1add43ccec1e"

// grafanaNamespaceDashboardURL — dashboard mặc định kube-prometheus-stack (Compute Resources / Namespace Pods).
func grafanaNamespaceDashboardURL(base, namespace string) string {
	base = trimURL(base)
	if base == "" || namespace == "" {
		return ""
	}
	u := base + "/d/" + grafanaNamespacePodsUID + "/kubernetes-compute-resources-namespace-pods"
	q := url.Values{}
	q.Set("var-namespace", namespace)
	q.Set("orgId", "1")
	q.Set("from", "now-6h")
	q.Set("to", "now")
	q.Set("timezone", "browser")
	return u + "?" + q.Encode()
}

func trimURL(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (h *Handler) monitoringConfigured() bool {
	return trimURL(h.cfg.GrafanaURL) != ""
}
