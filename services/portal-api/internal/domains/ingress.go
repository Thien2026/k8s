package domains

import (
	"encoding/json"
	"fmt"
)

const (
	ServiceName = "app"
	ServicePort = 80
)

func IngressName(domainID int64) string {
	return fmt.Sprintf("app-%d", domainID)
}

func TLSSecretName(domainID int64) string {
	return fmt.Sprintf("tls-app-%d", domainID)
}

// IngressManifest JSON cho networking.k8s.io/v1 Ingress.
func IngressManifest(hostname, namespace string, domainID int64, tlsEnabled bool) ([]byte, error) {
	name := IngressName(domainID)
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
						"paths": []map[string]any{
							{
								"path":     "/",
								"pathType": "Prefix",
								"backend": map[string]any{
									"service": map[string]any{
										"name": ServiceName,
										"port": map[string]any{"number": ServicePort},
									},
								},
							},
						},
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
