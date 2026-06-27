package domains

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Thien2026/k8s/services/portal-api/internal/deploy"
)

const (
	LegacyServiceName = "app"
	ServicePort       = 80
)

func IngressName(domainID int64) string {
	return fmt.Sprintf("app-%d", domainID)
}

func TLSSecretName(domainID int64) string {
	return fmt.Sprintf("tls-app-%d", domainID)
}

// IngressManifest JSON cho networking.k8s.io/v1 Ingress.
// routes rỗng → single app (backward compat).
func IngressManifest(hostname, namespace string, domainID int64, tlsEnabled bool, routes []deploy.IngressRoute) ([]byte, error) {
	name := IngressName(domainID)
	if len(routes) == 0 {
		routes = []deploy.IngressRoute{{
			Path:        "/",
			PathType:    "Prefix",
			ServiceName: LegacyServiceName,
			ServicePort: ServicePort,
		}}
	}
	// Path dài hơn trước (/api trước /).
	sorted := append([]deploy.IngressRoute(nil), routes...)
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Path) > len(sorted[j].Path)
	})
	paths := make([]map[string]any, 0, len(sorted))
	for _, r := range sorted {
		pt := r.PathType
		if pt == "" {
			pt = "Prefix"
		}
		port := r.ServicePort
		if port <= 0 {
			port = ServicePort
		}
		paths = append(paths, map[string]any{
			"path":     r.Path,
			"pathType": pt,
			"backend": map[string]any{
				"service": map[string]any{
					"name": r.ServiceName,
					"port": map[string]any{"number": port},
				},
			},
		})
	}
	obj := map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "Ingress",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels": map[string]string{
				"app.kubernetes.io/managed-by": "platform-console",
			},
		},
		"spec": map[string]any{
			"ingressClassName": "nginx",
			"rules": []map[string]any{
				{
					"host": hostname,
					"http": map[string]any{
						"paths": paths,
					},
				},
			},
		},
	}
	if tlsEnabled {
		secret := TLSSecretName(domainID)
		meta := obj["metadata"].(map[string]any)
		meta["annotations"] = map[string]string{
			"cert-manager.io/cluster-issuer": "letsencrypt-prod",
		}
		spec := obj["spec"].(map[string]any)
		spec["tls"] = []map[string]any{
			{
				"hosts":      []string{hostname},
				"secretName": secret,
			},
		}
	}
	return json.Marshal(obj)
}
