package handler

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	redisExporterImage     = "oliver006/redis_exporter:v1.67.0-alpine"
	redisExporterPort      = 9121
	redisGrafanaDashboardUID = "platform-redis-addon"
)

var redisMaxmemoryPolicies = []string{
	"allkeys-lru",
	"volatile-lru",
	"allkeys-lfu",
	"volatile-lfu",
	"allkeys-random",
	"volatile-random",
	"volatile-ttl",
	"noeviction",
}

func normalizeRedisMaxmemoryPolicy(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	for _, p := range redisMaxmemoryPolicies {
		if raw == p {
			return p
		}
	}
	return "allkeys-lru"
}

func normalizeRedisDefaultKeyTTL(sec int) int {
	if sec < 0 {
		return 0
	}
	if sec > 2592000 {
		return 2592000
	}
	return sec
}

func redisMaxmemoryPolicyHint(policy string) string {
	switch normalizeRedisMaxmemoryPolicy(policy) {
	case "allkeys-lru":
		return "Hết RAM → xóa key ít dùng nhất (kể cả key không TTL)."
	case "volatile-lru":
		return "Hết RAM → chỉ xóa key có TTL, ưu tiên LRU."
	case "volatile-ttl":
		return "Hết RAM → xóa key có TTL sắp hết hạn trước."
	case "noeviction":
		return "Hết RAM → từ chối ghi mới (error), không tự xóa key."
	default:
		return "Chính sách eviction khi đạt maxmemory."
	}
}

// grafanaRedisAddonDashboardURL — dashboard Platform Redis (import từ platform/monitoring).
func grafanaRedisAddonDashboardURL(base, namespace, release string) string {
	base = trimURL(base)
	if base == "" || namespace == "" || release == "" {
		return ""
	}
	u := base + "/d/" + redisGrafanaDashboardUID + "/platform-redis-addon"
	q := url.Values{}
	q.Set("var-namespace", namespace)
	q.Set("var-release", release)
	q.Set("orgId", "1")
	q.Set("from", "now-6h")
	q.Set("to", "now")
	q.Set("timezone", "browser")
	return u + "?" + q.Encode()
}

func redisExporterServiceMonitor(release, ns string) map[string]any {
	return map[string]any{
		"apiVersion": "monitoring.coreos.com/v1",
		"kind":       "ServiceMonitor",
		"metadata": map[string]any{
			"name":      release + "-exporter",
			"namespace": ns,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": release,
				"release":                    "kube-prometheus-stack",
			},
		},
		"spec": map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]string{
					"app.kubernetes.io/name":     "redis",
					"app.kubernetes.io/instance": release,
				},
			},
			"namespaceSelector": map[string]any{
				"matchNames": []string{ns},
			},
			"endpoints": []map[string]any{
				{
					"port":     "metrics",
					"interval": "30s",
					"path":     "/metrics",
				},
			},
		},
	}
}

func redisExporterContainer(authSecretName string) map[string]any {
	return map[string]any{
		"name":            "redis-exporter",
		"image":           redisExporterImage,
		"imagePullPolicy": "IfNotPresent",
		"ports": []map[string]any{
			{"containerPort": redisExporterPort, "name": "metrics"},
		},
		"env": []map[string]any{
			{"name": "REDIS_ADDR", "value": "redis://127.0.0.1:6379"},
			{
				"name": "REDIS_PASSWORD",
				"valueFrom": map[string]any{
					"secretKeyRef": map[string]any{
						"name": authSecretName,
						"key":  "password",
					},
				},
			},
		},
		"resources": map[string]any{
			"requests": map[string]string{"cpu": "10m", "memory": "32Mi"},
			"limits":   map[string]string{"cpu": "100m", "memory": "64Mi"},
		},
	}
}

func redisStatsPromQL(namespace, release string) map[string]string {
	ns := strings.ReplaceAll(namespace, `"`, `\"`)
	rel := strings.ReplaceAll(release, `"`, `\"`)
	pod := rel + "-.*"
	return map[string]string{
		"memory_bytes": fmt.Sprintf(`redis_memory_used_bytes{namespace="%s",pod=~"%s"}`, ns, pod),
		"clients":      fmt.Sprintf(`redis_connected_clients{namespace="%s",pod=~"%s"}`, ns, pod),
		"ops_rate":     fmt.Sprintf(`rate(redis_commands_processed_total{namespace="%s",pod=~"%s"}[5m])`, ns, pod),
	}
}
