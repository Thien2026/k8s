package deploy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Manifest gồm Deployment + Service JSON (apply qua Rancher).
type Manifest struct {
	Filename   string          `json:"filename"`
	Deployment json.RawMessage `json:"deployment"`
	Service    json.RawMessage `json:"service"`
	YAML       string          `json:"yaml"`
}

// deploymentReplicas — prod 2 bản để rolling không mất traffic; dev giữ 1.
func deploymentReplicas(p Params) int {
	if strings.EqualFold(strings.TrimSpace(p.Environment), "prod") {
		return 2
	}
	return 1
}

func rollingUpdateStrategy() map[string]any {
	return map[string]any{
		"type": "RollingUpdate",
		"rollingUpdate": map[string]any{
			"maxUnavailable": 0,
			"maxSurge":       1,
		},
	}
}

func K8sManifest(p Params) (Manifest, error) {
	image := p.imageRef()
	ns := p.Namespace
	name := p.deploymentName()
	replicas := deploymentReplicas(p)

	dep := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels": map[string]string{
				"app.kubernetes.io/name":       name,
				"app.kubernetes.io/part-of":    p.ProjectSlug,
				"platform/environment":         p.Environment,
			},
		},
		"spec": map[string]any{
			"replicas": replicas,
			"strategy": rollingUpdateStrategy(),
			"minReadySeconds": 5,
			"selector": map[string]any{
				"matchLabels": map[string]string{
					"app": name,
				},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{
						"app":         name,
						LabelImageTag: ImageTagLabelValue(p.ImageTag),
					},
				},
				"spec": func() map[string]any {
					spec := map[string]any{
						"containers": []map[string]any{
							func() map[string]any {
								c := map[string]any{
									"name":  name,
									"image": image,
									"ports": []map[string]any{
										{"containerPort": 8080, "name": "http"},
									},
									// PORT cố định — tránh envFrom ghi đè bằng chuỗi rỗng (buildpack Node/Python crash).
									"env": []map[string]any{
										{"name": "PORT", "value": "8080"},
									},
									"readinessProbe": map[string]any{
										"httpGet": map[string]any{
											"path": "/health",
											"port": 8080,
										},
										"initialDelaySeconds": 3,
										"periodSeconds":       5,
										"failureThreshold":    6,
									},
									"livenessProbe": map[string]any{
										"httpGet": map[string]any{
											"path": "/health",
											"port": 8080,
										},
										"initialDelaySeconds": 20,
										"periodSeconds":       10,
										"failureThreshold":    3,
									},
								}
								if secretName := strings.TrimSpace(p.AppEnvFromSecret); secretName != "" {
									c["envFrom"] = []map[string]any{
										{"secretRef": map[string]string{"name": secretName}},
									}
								}
								return c
							}(),
						},
					}
					if secret := strings.TrimSpace(p.ImagePullSecret); secret != "" {
						spec["imagePullSecrets"] = []map[string]string{{"name": secret}}
					}
					return spec
				}(),
			},
		},
	}

	svc := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels": map[string]string{
				"app.kubernetes.io/name":    name,
				"app.kubernetes.io/part-of": p.ProjectSlug,
			},
		},
		"spec": map[string]any{
			"selector": map[string]string{
				"app": name,
			},
			"ports": []map[string]any{
				{
					"name":       "http",
					"port":       80,
					"targetPort": 8080,
					"protocol":   "TCP",
				},
			},
		},
	}

	depJSON, err := json.Marshal(dep)
	if err != nil {
		return Manifest{}, err
	}
	svcJSON, err := json.Marshal(svc)
	if err != nil {
		return Manifest{}, err
	}

	yaml := fmt.Sprintf(`# Deployment + Service cho %s (%s)
# Image: %s
# Strategy: RollingUpdate (maxUnavailable=0, maxSurge=1) — pod mới ready trước khi tắt pod cũ
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
spec:
  replicas: %d
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
  minReadySeconds: 5
  selector:
    matchLabels:
      app: %s
  template:
    metadata:
      labels:
        app: %s
    spec:
      containers:
        - name: %s
          image: %s
          ports:
            - containerPort: 8080
              name: http
---
apiVersion: v1
kind: Service
metadata:
  name: %s
  namespace: %s
spec:
  selector:
    app: %s
  ports:
    - name: http
      port: 80
      targetPort: 8080
`, p.ProjectName, p.Environment, image, name, ns, replicas, name, name, name, image, name, ns, name)

	return Manifest{
		Filename:   "k8s/" + p.ProjectSlug + "-" + p.Environment + ".yaml",
		Deployment: depJSON,
		Service:    svcJSON,
		YAML:       yaml,
	}, nil
}
