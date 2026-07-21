package deploy

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest gồm Deployment + Service JSON (apply qua Rancher).
type Manifest struct {
	ServiceName string          `json:"service_name,omitempty"`
	Filename    string          `json:"filename"`
	Deployment  json.RawMessage `json:"deployment"`
	Service     json.RawMessage `json:"service"`
	YAML        string          `json:"yaml"`
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
			// Project ResourceQuota thường được cấp sát tổng limit của workload.
			// Không surge thêm pod vì admission sẽ từ chối khi pod cũ còn chiếm quota.
			"maxUnavailable": 1,
			"maxSurge":       0,
		},
	}
}

func K8sManifests(p Params) ([]Manifest, error) {
	svcs := p.EffectiveServices()
	out := make([]Manifest, 0, len(svcs))
	for _, svc := range svcs {
		m, err := K8sManifestForService(p, svc)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// K8sManifest — backward compat: manifest service đầu tiên.
func K8sManifest(p Params) (Manifest, error) {
	svcs := p.EffectiveServices()
	if len(svcs) == 0 {
		return Manifest{}, fmt.Errorf("không có service để deploy")
	}
	return K8sManifestForService(p, svcs[0])
}

func K8sManifestForService(p Params, svc ServiceDef) (Manifest, error) {
	svc = normalizeServiceDef(svc)
	image := p.imageRefFor(svc)
	ns := p.Namespace
	name := svc.Name
	replicas := deploymentReplicas(p)
	port := svc.ContainerPort
	health := ContainerProbePath(svc)
	allSvcs := p.EffectiveServices()
	discoveryEnv := ServiceDiscoveryEnvVars(allSvcs, name)

	dep := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
			"labels": map[string]string{
				"app.kubernetes.io/name":       name,
				"app.kubernetes.io/part-of":    p.ProjectSlug,
				"app.kubernetes.io/component":  name,
				"platform/environment":         p.Environment,
			},
		},
		"spec": map[string]any{
			"replicas":        replicas,
			"strategy":        rollingUpdateStrategy(),
			"minReadySeconds": 5,
			"selector": map[string]any{
				"matchLabels": map[string]string{
					"app": name,
				},
			},
			"template": map[string]any{
				"metadata": func() map[string]any {
					meta := map[string]any{
						"labels": map[string]string{
							"app":         name,
							LabelImageTag: ImageTagLabelValue(p.ImageTag),
						},
					}
					if p.ForceRolloutRestart {
						ann := map[string]string{
							"kubectl.kubernetes.io/restartedAt": time.Now().UTC().Format(time.RFC3339Nano),
						}
						meta["annotations"] = ann
					}
					return meta
				}(),
				"spec": func() map[string]any {
					spec := map[string]any{
						"containers": []map[string]any{
							func() map[string]any {
								c := map[string]any{
									"name":  name,
									"image": image,
									"ports": []map[string]any{
										{"containerPort": port, "name": "http"},
									},
									"env": append(append([]map[string]any{
										{"name": "PORT", "value": fmt.Sprintf("%d", port)},
									}, ServiceExtraEnv(svc, port)...), discoveryEnv...),
									"readinessProbe": map[string]any{
										"httpGet": map[string]any{
											"path": health,
											"port": port,
										},
										"initialDelaySeconds": 3,
										"periodSeconds":       5,
										"failureThreshold":    6,
									},
									"livenessProbe": map[string]any{
										"httpGet": map[string]any{
											"path": health,
											"port": port,
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
								if res := ContainerResources(svc); res != nil {
									c["resources"] = res
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

	svcObj := map[string]any{
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
					"targetPort": port,
					"protocol":   "TCP",
				},
			},
		},
	}

	depJSON, err := json.Marshal(dep)
	if err != nil {
		return Manifest{}, err
	}
	svcJSON, err := json.Marshal(svcObj)
	if err != nil {
		return Manifest{}, err
	}

	yamlText, err := marshalManifestYAML(p, name, image, dep, svcObj)
	if err != nil {
		return Manifest{}, err
	}

	return Manifest{
		ServiceName: name,
		Filename:    fmt.Sprintf("k8s/%s-%s-%s.yaml", p.ProjectSlug, name, p.Environment),
		Deployment:  depJSON,
		Service:     svcJSON,
		YAML:        yamlText,
	}, nil
}

func marshalManifestYAML(p Params, name, image string, dep, svcObj map[string]any) (string, error) {
	depBytes, err := yaml.Marshal(dep)
	if err != nil {
		return "", err
	}
	svcBytes, err := yaml.Marshal(svcObj)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("# Deployment + Service cho %s / %s (%s)\n# Image: %s\n---\n%s---\n%s",
		p.ProjectName, name, p.Environment, image, depBytes, svcBytes), nil
}

// IngressRoutesFromServices sinh route Ingress — path dài hơn (/api) trước /; bỏ qua internal.
func IngressRoutesFromServices(svcs []ServiceDef) []IngressRoute {
	type pair struct {
		svc  ServiceDef
		path string
	}
	pairs := make([]pair, 0, len(svcs))
	for _, s := range svcs {
		s = normalizeServiceDef(s)
		if !s.ExposeIngress {
			continue
		}
		pairs = append(pairs, pair{svc: s, path: s.IngressPath})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return len(pairs[i].path) > len(pairs[j].path)
	})
	routes := make([]IngressRoute, 0, len(pairs))
	for _, p := range pairs {
		routes = append(routes, IngressRoute{
			Path:        p.path,
			PathType:    "Prefix",
			ServiceName: p.svc.Name,
			ServicePort: 80,
		})
	}
	return routes
}
