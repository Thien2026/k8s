package handler

import "net/url"

// grafanaNamespaceDashboardURL — dashboard mặc định kube-prometheus-stack (Compute Resources / Namespace).
func grafanaNamespaceDashboardURL(base, namespace string) string {
	base = trimURL(base)
	if base == "" || namespace == "" {
		return ""
	}
	u := base + "/d/kubernetes-compute-resources-namespace/kubernetes-compute-resources-namespace"
	q := url.Values{}
	q.Set("var-namespace", namespace)
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
