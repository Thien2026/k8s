package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/go-chi/chi/v5"
)

const (
	minioAddonImage            = "minio/minio:RELEASE.2024-12-18T13-15-44Z"
	minioMCImage               = "minio/mc:latest"
	minioAPIPort               = 9000
	minioConsolePort           = 9001
	minioDefaultStorageGB      = 5
	minioDefaultMemoryMB       = 256
	minioDefaultBucket         = "app"
	minioDistributedReplicas   = 4
	minioDistributedStorageSC  = "longhorn"
)

type minioAddonAPIView struct {
	projectAddonView
	ConnectionSecret      string         `json:"connection_secret,omitempty"`
	EndpointMasked        string         `json:"endpoint_masked,omitempty"`
	Endpoint              string         `json:"endpoint,omitempty"`
	EndpointExternal      string         `json:"endpoint_external,omitempty"`
	EndpointExternalMasked string        `json:"endpoint_external_masked,omitempty"`
	ConsoleURLExternal    string         `json:"console_url_external,omitempty"`
	Bucket                string         `json:"bucket,omitempty"`
	AccessKeyMasked       string         `json:"access_key_masked,omitempty"`
	AccessKey             string         `json:"access_key,omitempty"`
	SecretKeyMasked       string         `json:"secret_key_masked,omitempty"`
	SecretKey             string         `json:"secret_key,omitempty"`
	ExternalHostname      string         `json:"external_hostname,omitempty"`
	ExternalAPIPort       int            `json:"external_api_port,omitempty"`
	ExternalConsolePort   int            `json:"external_console_port,omitempty"`
	HACapable             bool           `json:"ha_capable"`
	HACapability          map[string]any `json:"ha_capability,omitempty"`
	TopologyNote          string         `json:"topology_note,omitempty"`
	UpgradeAvailable      bool           `json:"upgrade_available"`
}

func minioAddonRelease(slug, env string) string {
	return slug + "-minio-" + env
}

func minioAddonConnectionSecretName(release string) string {
	return release + "-connection"
}

func minioAddonNodePort(slug, env, kind string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug + "|minio|" + kind + "|" + env))
	return 30000 + int(h.Sum32()%2768)
}

func (h *Handler) minioZoneDomain() string {
	if z := strings.TrimSpace(h.cfg.MinioZone); z != "" {
		return z
	}
	if d := strings.TrimSpace(h.cfg.ApexDomain); d != "" {
		return "minio." + d
	}
	pd := strings.TrimSpace(h.cfg.PlatformDomain)
	parts := strings.Split(pd, ".")
	if len(parts) >= 2 {
		return "minio." + strings.Join(parts[len(parts)-2:], ".")
	}
	return ""
}

func minioAddonExternalHostname(slug, env, minioZone string) string {
	if minioZone == "" {
		return ""
	}
	return slug + "-minio-" + env + "." + minioZone
}

func (h *Handler) minioAddonExternalEndpoints(slug, env string) (hostname string, apiPort, consolePort int, apiURL, consoleURL string) {
	if env != "dev" {
		return "", 0, 0, "", ""
	}
	zone := h.minioZoneDomain()
	hostname = minioAddonExternalHostname(slug, env, zone)
	if hostname == "" {
		return "", 0, 0, "", ""
	}
	apiPort = minioAddonNodePort(slug, env, "api")
	consolePort = minioAddonNodePort(slug, env, "console")
	if consolePort == apiPort {
		consolePort = 30000 + ((apiPort - 30000 + 1) % 2768)
	}
	apiURL = fmt.Sprintf("http://%s:%d", hostname, apiPort)
	consoleURL = fmt.Sprintf("http://%s:%d", hostname, consolePort)
	return hostname, apiPort, consolePort, apiURL, consoleURL
}

func normalizeMinioTopology(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "distributed":
		return "distributed"
	default:
		return "standalone"
	}
}

func normalizeMinioStorageGB(n int) int {
	if n < 1 {
		return minioDefaultStorageGB
	}
	if n > 100 {
		return 100
	}
	return n
}

func randomMinioAccessKey() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "AKIA" + strings.ToUpper(base64.RawURLEncoding.EncodeToString(b)[:16]), nil
}

func randomMinioSecretKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func maskSecretTail(raw string, keep int) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if keep < 0 {
		keep = 0
	}
	if len(raw) <= keep {
		return strings.Repeat("•", len(raw))
	}
	return strings.Repeat("•", len(raw)-keep) + raw[len(raw)-keep:]
}

func (h *Handler) minioEndpoint(release, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", release, namespace, minioAPIPort)
}

// minioHACapability — điều kiện bật/upgrade distributed (không auto-migrate object).
func (h *Handler) minioHACapability(ctx context.Context) map[string]any {
	out := map[string]any{
		"capable":          false,
		"topology_default": "standalone",
		"min_volumes":      minioDistributedReplicas,
		"reasons":          []string{},
		"longhorn_ready":   false,
		"node_count":       0,
		"note":             "Distributed: instance mới hoặc Upgrade HA có xác nhận — không auto-migrate object từ PVC standalone.",
	}
	reasons := []string{}
	if h.rancher == nil || !h.rancher.Enabled() || !h.pluginEnabled(ctx, plugins.Rancher) {
		reasons = append(reasons, "Rancher chưa sẵn sàng")
		out["reasons"] = reasons
		return out
	}
	nodes, err := h.rancher.ListK8s(ctx, "", "nodes", "", 1, 50)
	nodeCount := 0
	if err == nil {
		nodeCount = len(nodes.Items)
	}
	out["node_count"] = nodeCount

	longhorn := false
	scs, err := h.rancher.ListK8s(ctx, "", "storageclasses", "", 1, 50)
	if err == nil {
		for _, it := range scs.Items {
			name := strings.ToLower(strings.TrimSpace(it.Name))
			if name == "longhorn" || strings.Contains(name, "longhorn") {
				longhorn = true
				break
			}
		}
	}
	out["longhorn_ready"] = longhorn
	if !longhorn {
		reasons = append(reasons, "Chưa có StorageClass Longhorn (PVC replicated)")
	}
	if nodeCount < 2 {
		reasons = append(reasons, fmt.Sprintf("Cần ≥2 node (hiện %d) để HA thực sự", nodeCount))
	}
	if longhorn && nodeCount >= 2 {
		out["capable"] = true
		reasons = append(reasons, "Đủ điều kiện — có thể tạo distributed hoặc Upgrade HA (có xác nhận)")
	}
	out["reasons"] = reasons
	return out
}

func (h *Handler) minioHACapable(ctx context.Context) bool {
	cap := h.minioHACapability(ctx)
	c, _ := cap["capable"].(bool)
	return c
}

func (h *Handler) setProjectAddonTopology(ctx context.Context, projectID int64, engine, env, topology string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_data_addons SET topology=$1, updated_at=now()
		WHERE project_id=$2 AND engine=$3 AND environment=$4`,
		normalizeMinioTopology(topology), projectID, engine, env)
}

func minioDistributedArgs(release, headless, ns string) []string {
	// MinIO erasure: 4 drives qua StatefulSet + headless DNS.
	peer := fmt.Sprintf("http://%s-{0...%d}.%s.%s.svc.cluster.local/data",
		release, minioDistributedReplicas-1, headless, ns)
	return []string{"server", peer, "--console-address", fmt.Sprintf(":%d", minioConsolePort)}
}

func (h *Handler) applyMinioAddonObjects(ctx context.Context, p projectRow, env string, addon *projectAddonView) (string, error) {
	if h.rancher == nil || !h.rancher.Enabled() || !h.pluginEnabled(ctx, plugins.Rancher) {
		return "", fmt.Errorf("Rancher chưa sẵn sàng")
	}
	topology := normalizeMinioTopology(addon.Topology)
	if topology == "distributed" && !h.minioHACapable(ctx) {
		cap := h.minioHACapability(ctx)
		reasons, _ := cap["reasons"].([]string)
		msg := "topology distributed cần ha_capable (Longhorn + ≥2 node)"
		if len(reasons) > 0 {
			msg += ": " + strings.Join(reasons, "; ")
		}
		return "", fmt.Errorf("%s", msg)
	}
	ns := h.projectNamespace(p, env)
	release := minioAddonRelease(p.Slug, env)
	if err := h.rancher.EnsureNamespace(ctx, "", ns); err != nil {
		return "", err
	}

	accessKey := ""
	secretKey := ""
	authSecretName := release + "-auth"
	if data, ok, err := h.rancher.GetOpaqueSecretData(ctx, "", ns, authSecretName); err == nil && ok {
		accessKey = strings.TrimSpace(data["rootUser"])
		secretKey = strings.TrimSpace(data["rootPassword"])
	}
	if accessKey == "" || secretKey == "" {
		var err error
		accessKey, err = randomMinioAccessKey()
		if err != nil {
			return "", err
		}
		secretKey, err = randomMinioSecretKey()
		if err != nil {
			return "", err
		}
	}
	bucket := minioDefaultBucket
	storageGB := normalizeMinioStorageGB(addon.StorageGB)
	limitMi := addon.MaxMemoryMB
	if limitMi < 128 {
		limitMi = minioDefaultMemoryMB
	}
	requestMi := limitMi / 2
	if requestMi < 64 {
		requestMi = 64
	}

	authSecret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": authSecretName,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "minio",
				"app.kubernetes.io/instance": release,
			},
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"rootUser":     accessKey,
			"rootPassword": secretKey,
		},
	}
	authJSON, _ := json.Marshal(authSecret)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/secrets", ns, authJSON); err != nil {
		return "", fmt.Errorf("secret auth: %w", err)
	}

	svc := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name": release,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "minio",
				"app.kubernetes.io/instance": release,
			},
		},
		"spec": map[string]any{
			"type": "ClusterIP",
			"ports": []map[string]any{
				{"name": "api", "port": minioAPIPort, "targetPort": minioAPIPort},
				{"name": "console", "port": minioConsolePort, "targetPort": minioConsolePort},
			},
			"selector": map[string]string{
				"app.kubernetes.io/name":     "minio",
				"app.kubernetes.io/instance": release,
			},
		},
	}
	if env == "dev" {
		apiNP := minioAddonNodePort(p.Slug, env, "api")
		consoleNP := minioAddonNodePort(p.Slug, env, "console")
		if consoleNP == apiNP {
			consoleNP = 30000 + ((apiNP - 30000 + 1) % 2768)
		}
		svc["spec"].(map[string]any)["type"] = "NodePort"
		svc["spec"].(map[string]any)["ports"] = []map[string]any{
			{"name": "api", "port": minioAPIPort, "targetPort": minioAPIPort, "nodePort": apiNP},
			{"name": "console", "port": minioConsolePort, "targetPort": minioConsolePort, "nodePort": consoleNP},
		}
	}
	svcJSON, _ := json.Marshal(svc)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/services", ns, svcJSON); err != nil {
		return "", fmt.Errorf("service minio: %w", err)
	}

	replicas := 1
	serviceName := release
	minioArgs := []string{"server", "/data", "--console-address", fmt.Sprintf(":%d", minioConsolePort)}
	var pvcSpec map[string]any
	pvcSpec = map[string]any{
		"accessModes": []string{"ReadWriteOnce"},
		"resources": map[string]any{
			"requests": map[string]string{
				"storage": fmt.Sprintf("%dGi", storageGB),
			},
		},
	}
	if topology == "distributed" {
		replicas = minioDistributedReplicas
		hlName := release + "-hl"
		serviceName = hlName
		minioArgs = minioDistributedArgs(release, hlName, ns)
		pvcSpec["storageClassName"] = minioDistributedStorageSC
		hl := map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]any{
				"name": hlName,
				"labels": map[string]string{
					"app.kubernetes.io/name":     "minio",
					"app.kubernetes.io/instance": release,
					"platform/minio-role":        "headless",
				},
			},
			"spec": map[string]any{
				"clusterIP":                "None",
				"publishNotReadyAddresses": true,
				"ports": []map[string]any{
					{"name": "api", "port": minioAPIPort, "targetPort": minioAPIPort},
					{"name": "console", "port": minioConsolePort, "targetPort": minioConsolePort},
				},
				"selector": map[string]string{
					"app.kubernetes.io/name":     "minio",
					"app.kubernetes.io/instance": release,
				},
			},
		}
		hlJSON, _ := json.Marshal(hl)
		if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/services", ns, hlJSON); err != nil {
			return "", fmt.Errorf("service minio headless: %w", err)
		}
	}

	sts := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]any{
			"name": release,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "minio",
				"app.kubernetes.io/instance": release,
				"app.kubernetes.io/part-of":  p.Slug,
				"platform/addon":             "minio",
				"platform/environment":       env,
				"platform/minio-topology":    topology,
			},
		},
		"spec": map[string]any{
			"serviceName": serviceName,
			"replicas":    replicas,
			"selector": map[string]any{
				"matchLabels": map[string]string{
					"app.kubernetes.io/name":     "minio",
					"app.kubernetes.io/instance": release,
				},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{
						"app":                        release,
						"app.kubernetes.io/name":     "minio",
						"app.kubernetes.io/instance": release,
					},
				},
				"spec": map[string]any{
					"containers": []map[string]any{
						{
							"name":  "minio",
							"image": minioAddonImage,
							"args":  minioArgs,
							"ports": []map[string]any{
								{"name": "api", "containerPort": minioAPIPort},
								{"name": "console", "containerPort": minioConsolePort},
							},
							"env": []map[string]any{
								{
									"name": "MINIO_ROOT_USER",
									"valueFrom": map[string]any{
										"secretKeyRef": map[string]string{"name": authSecretName, "key": "rootUser"},
									},
								},
								{
									"name": "MINIO_ROOT_PASSWORD",
									"valueFrom": map[string]any{
										"secretKeyRef": map[string]string{"name": authSecretName, "key": "rootPassword"},
									},
								},
							},
							"resources": map[string]any{
								"requests": map[string]string{
									"cpu":    "50m",
									"memory": fmt.Sprintf("%dMi", requestMi),
								},
								"limits": map[string]string{
									"cpu":    "500m",
									"memory": fmt.Sprintf("%dMi", limitMi),
								},
							},
							"volumeMounts": []map[string]any{
								{"name": "data", "mountPath": "/data"},
							},
							"livenessProbe": map[string]any{
								"httpGet":             map[string]any{"path": "/minio/health/live", "port": minioAPIPort},
								"initialDelaySeconds": 30,
								"periodSeconds":       15,
								"timeoutSeconds":      5,
							},
							"readinessProbe": map[string]any{
								"httpGet":             map[string]any{"path": "/minio/health/ready", "port": minioAPIPort},
								"initialDelaySeconds": 15,
								"periodSeconds":       5,
								"timeoutSeconds":      3,
							},
						},
					},
				},
			},
			"volumeClaimTemplates": []map[string]any{
				{
					"metadata": map[string]any{"name": "data"},
					"spec":     pvcSpec,
				},
			},
		},
	}
	stsJSON, _ := json.Marshal(sts)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/apps/v1/statefulsets", ns, stsJSON); err != nil {
		return "", fmt.Errorf("statefulset minio: %w", err)
	}

	endpoint := h.minioEndpoint(release, ns)
	jobName := release + "-init-bucket"
	job := map[string]any{
		"apiVersion": "batch/v1",
		"kind":       "Job",
		"metadata": map[string]any{
			"name": jobName,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "minio-init",
				"app.kubernetes.io/instance": release,
			},
		},
		"spec": map[string]any{
			"ttlSecondsAfterFinished": 300,
			"backoffLimit":            6,
			"template": map[string]any{
				"spec": map[string]any{
					"restartPolicy": "OnFailure",
					"containers": []map[string]any{
						{
							"name":  "mc",
							"image": minioMCImage,
							"env": []map[string]any{
								{
									"name": "MINIO_ROOT_USER",
									"valueFrom": map[string]any{
										"secretKeyRef": map[string]string{"name": authSecretName, "key": "rootUser"},
									},
								},
								{
									"name": "MINIO_ROOT_PASSWORD",
									"valueFrom": map[string]any{
										"secretKeyRef": map[string]string{"name": authSecretName, "key": "rootPassword"},
									},
								},
								{"name": "MINIO_ENDPOINT", "value": endpoint},
								{"name": "MINIO_BUCKET", "value": bucket},
							},
							"command": []string{"/bin/sh", "-c"},
							"args": []string{
								`set -e
i=0
until mc alias set local "$MINIO_ENDPOINT" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"; do
  i=$((i+1)); [ "$i" -gt 30 ] && exit 1; sleep 2
done
mc mb -p "local/$MINIO_BUCKET" || true
mc anonymous set none "local/$MINIO_BUCKET" || true
echo "bucket ready: $MINIO_BUCKET"`,
							},
						},
					},
				},
			},
		},
	}
	// Xóa job cũ nếu có (re-provision) rồi tạo lại.
	_ = h.rancher.DeleteNamespacedObject(ctx, "", "/apis/batch/v1/jobs", ns, jobName)
	jobJSON, _ := json.Marshal(job)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/batch/v1/jobs", ns, jobJSON); err != nil {
		return "", fmt.Errorf("job init bucket: %w", err)
	}

	if env == "prod" {
		if err := h.applyMinioNetworkPolicy(ctx, release, ns, p.Slug); err != nil {
			return "", fmt.Errorf("networkpolicy minio: %w", err)
		}
	}

	connSecretName := minioAddonConnectionSecretName(release)
	connSecret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": connSecretName,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "minio",
				"app.kubernetes.io/instance": release,
			},
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"S3_ENDPOINT":   endpoint,
			"S3_ACCESS_KEY": accessKey,
			"S3_SECRET_KEY": secretKey,
			"S3_BUCKET":     bucket,
			"S3_REGION":     "us-east-1",
			"S3_USE_SSL":    "false",
		},
	}
	connJSON, _ := json.Marshal(connSecret)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/secrets", ns, connJSON); err != nil {
		return "", fmt.Errorf("secret connection: %w", err)
	}

	envPairs := []struct {
		key, val string
		secret   bool
	}{
		{"S3_ENDPOINT", endpoint, false},
		{"S3_ACCESS_KEY", accessKey, true},
		{"S3_SECRET_KEY", secretKey, true},
		{"S3_BUCKET", bucket, false},
		{"S3_REGION", "us-east-1", false},
		{"S3_USE_SSL", "false", false},
	}
	for _, e := range envPairs {
		if err := h.upsertRuntimeEnvVar(ctx, p.ID, env, e.key, e.val, e.secret); err != nil {
			return "", fmt.Errorf("save env %s: %w", e.key, err)
		}
	}
	if err := h.syncAppEnvSecret(ctx, p, env, "", true); err != nil {
		return "", fmt.Errorf("sync app-env: %w", err)
	}
	if err := h.rancher.RolloutRestartStatefulSet(ctx, "", ns, release); err != nil {
		return "", fmt.Errorf("restart minio statefulset: %w", err)
	}
	return connSecretName, nil
}

func (h *Handler) applyMinioNetworkPolicy(ctx context.Context, release, ns, slug string) error {
	np := map[string]any{
		"apiVersion": "networking.k8s.io/v1",
		"kind":       "NetworkPolicy",
		"metadata": map[string]any{
			"name": release + "-allow-app",
		},
		"spec": map[string]any{
			"podSelector": map[string]any{
				"matchLabels": map[string]string{
					"app.kubernetes.io/name":     "minio",
					"app.kubernetes.io/instance": release,
				},
			},
			"policyTypes": []string{"Ingress"},
			"ingress": []map[string]any{
				{
					"from": []map[string]any{
						{
							"podSelector": map[string]any{
								"matchExpressions": []map[string]any{
									{
										"key":      "app",
										"operator": "Exists",
									},
								},
							},
						},
					},
					"ports": []map[string]any{
						{"protocol": "TCP", "port": minioAPIPort},
					},
				},
			},
		},
	}
	_ = slug
	raw, _ := json.Marshal(np)
	return h.rancher.ApplyNamespacedObject(ctx, "", "/apis/networking.k8s.io/v1/networkpolicies", ns, raw)
}

func (h *Handler) provisionMinioAddon(ctx context.Context, p projectRow, engine, env string, addon *projectAddonView) error {
	h.setProjectAddonStatus(ctx, p.ID, engine, env, "provisioning")
	runCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	connSecret, err := h.applyMinioAddonObjects(runCtx, p, env, addon)
	if err != nil {
		h.setProjectAddonStatus(ctx, p.ID, engine, env, "failed")
		return err
	}
	h.setProjectAddonConnectionSecret(ctx, p.ID, engine, env, connSecret)
	h.setProjectAddonStatus(ctx, p.ID, engine, env, "running")
	addon.HasConnection = true
	addon.Status = "running"
	addon.Topology = normalizeMinioTopology(addon.Topology)
	return nil
}

func (h *Handler) reconcileMinioAddonStatus(ctx context.Context, p projectRow, addon *projectAddonView) projectAddonView {
	out := *addon
	if strings.TrimSpace(out.Topology) == "" {
		out.Topology = "standalone"
	}
	ns := h.projectNamespace(p, addon.Environment)
	release := out.K8sRelease
	if release == "" {
		release = minioAddonRelease(p.Slug, addon.Environment)
	}
	if h.rancher == nil || !h.rancher.Enabled() || !h.pluginEnabled(ctx, plugins.Rancher) {
		if out.Status == "" {
			out.Status = "pending"
		}
		return out
	}
	list, err := h.rancher.ListK8s(ctx, "", "statefulsets", ns, 1, 50)
	if err != nil {
		return out
	}
	found := false
	for _, it := range list.Items {
		if it.Name != release {
			continue
		}
		found = true
		status := strings.ToLower(strings.TrimSpace(it.Status))
		if redisStatefulSetReady(it) {
			out.Status = "running"
		} else if strings.Contains(status, "err") || strings.Contains(status, "crash") || strings.Contains(status, "fail") {
			out.Status = "failed"
		} else {
			out.Status = "provisioning"
		}
		break
	}
	if !found {
		if out.HasConnection {
			out.Status = "provisioning"
		} else {
			out.Status = "pending"
		}
	}
	if out.Status != addon.Status {
		h.setProjectAddonStatus(ctx, p.ID, addon.Engine, addon.Environment, out.Status)
	}
	if !out.HasConnection {
		connSecretName := minioAddonConnectionSecretName(release)
		if data, ok, err := h.rancher.GetOpaqueSecretData(ctx, "", ns, connSecretName); err == nil && ok && strings.TrimSpace(data["S3_ENDPOINT"]) != "" {
			out.HasConnection = true
			h.setProjectAddonConnectionSecret(ctx, p.ID, addon.Engine, addon.Environment, connSecretName)
		}
	}
	return out
}

func (h *Handler) enrichMinioAddonAPIView(ctx context.Context, p projectRow, v projectAddonView, exposeFull bool) minioAddonAPIView {
	out := minioAddonAPIView{projectAddonView: v}
	if strings.TrimSpace(out.Topology) == "" {
		out.Topology = "standalone"
	}
	if out.StorageGB <= 0 {
		out.StorageGB = minioDefaultStorageGB
	}
	cap := h.minioHACapability(ctx)
	out.HACapability = cap
	if c, _ := cap["capable"].(bool); c {
		out.HACapable = true
	}
	out.UpgradeAvailable = out.HACapable && out.Topology == "standalone"
	if out.Topology == "distributed" {
		out.TopologyNote = "Distributed (4 pods + Longhorn). Object không tự migrate từ standalone cũ."
	} else if out.HACapable {
		out.TopologyNote = "Standalone. Cluster đủ điều kiện HA — có thể Upgrade HA (xác nhận, không auto-migrate object)."
	} else {
		out.TopologyNote = "Standalone. Distributed khi ha_capable (Longhorn + ≥2 node) + upgrade có xác nhận."
	}
	if !v.HasConnection {
		return out
	}
	sec := h.getProjectAddonConnectionSecretName(ctx, p.ID, v.Engine, v.Environment)
	if sec != "" {
		out.ConnectionSecret = sec
	}
	vars, err := h.envVarsMap(ctx, p.ID, v.Environment)
	if err != nil {
		return out
	}
	out.Bucket = strings.TrimSpace(vars["S3_BUCKET"])
	if out.Bucket == "" {
		out.Bucket = minioDefaultBucket
	}
	ep := strings.TrimSpace(vars["S3_ENDPOINT"])
	ak := strings.TrimSpace(vars["S3_ACCESS_KEY"])
	sk := strings.TrimSpace(vars["S3_SECRET_KEY"])
	out.EndpointMasked = ep
	out.AccessKeyMasked = maskSecretTail(ak, 4)
	out.SecretKeyMasked = maskSecretTail(sk, 4)
	if exposeFull {
		out.Endpoint = ep
		out.AccessKey = ak
		out.SecretKey = sk
	}
	host, apiPort, consolePort, apiURL, consoleURL := h.minioAddonExternalEndpoints(p.Slug, v.Environment)
	if apiURL != "" {
		out.ExternalHostname = host
		out.ExternalAPIPort = apiPort
		out.ExternalConsolePort = consolePort
		out.EndpointExternalMasked = apiURL
		out.ConsoleURLExternal = consoleURL
		if exposeFull {
			out.EndpointExternal = apiURL
		}
	}
	return out
}

func (h *Handler) minioDevAddonReady(ctx context.Context, projectID int64) bool {
	addon, err := h.getProjectAddon(ctx, projectID, "minio", "dev")
	if err == nil && addon != nil && addon.HasConnection && addon.Status == "running" {
		return true
	}
	vars, _ := h.envVarsMap(ctx, projectID, "dev")
	return strings.TrimSpace(vars["S3_ENDPOINT"]) != "" && strings.TrimSpace(vars["S3_ACCESS_KEY"]) != ""
}

func (h *Handler) minioProdAddonReady(ctx context.Context, projectID int64) bool {
	addon, err := h.getProjectAddon(ctx, projectID, "minio", "prod")
	if err != nil || addon == nil {
		return false
	}
	return addon.HasConnection && addon.Status == "running"
}

func (h *Handler) minioAddonPromoteReadiness(ctx context.Context, p projectRow) promoteReadinessItem {
	item := promoteReadinessItem{
		ID:    "minio_prod",
		Label: "MinIO addon (prod)",
		Group: "prod",
		Tab:   "addons",
	}
	if !h.minioDevAddonReady(ctx, p.ID) {
		item.OK = true
		item.Detail = "Dev không dùng MinIO — bỏ qua"
		return item
	}
	if h.minioProdAddonReady(ctx, p.ID) {
		item.OK = true
		item.Detail = "MinIO prod running · ClusterIP"
		return item
	}
	item.OK = false
	item.Detail = "Dev đã có MinIO — cần provision prod (instance riêng, không copy object)"
	return item
}

func (h *Handler) promoteMinioAddonFromDev(ctx context.Context, p projectRow) error {
	if !h.minioDevAddonReady(ctx, p.ID) {
		return nil
	}
	if h.minioProdAddonReady(ctx, p.ID) {
		return nil
	}
	dev, err := h.getProjectAddon(ctx, p.ID, "minio", "dev")
	storageGB := minioDefaultStorageGB
	mem := minioDefaultMemoryMB
	topology := "standalone"
	if err == nil && dev != nil {
		storageGB = normalizeMinioStorageGB(dev.StorageGB)
		if dev.MaxMemoryMB >= 128 {
			mem = dev.MaxMemoryMB
		}
		topology = normalizeMinioTopology(dev.Topology)
		if topology == "distributed" && !h.minioHACapable(ctx) {
			topology = "standalone"
		}
	}
	release := minioAddonRelease(p.Slug, "prod")
	_, err = h.db.Exec(ctx, `
		INSERT INTO project_data_addons (project_id, engine, environment, status, k8s_release, max_memory_mb, max_clients, topology, storage_gb)
		VALUES ($1, 'minio', 'prod', 'pending', $2, $3, 100, $4, $5)
		ON CONFLICT (project_id, engine, environment) DO UPDATE SET
			max_memory_mb = EXCLUDED.max_memory_mb,
			topology = EXCLUDED.topology,
			storage_gb = EXCLUDED.storage_gb,
			status = CASE WHEN project_data_addons.status IN ('stopped', 'failed') THEN 'pending' ELSE project_data_addons.status END,
			updated_at = now()`,
		p.ID, release, mem, topology, storageGB)
	if err != nil {
		return err
	}
	addon, err := h.getProjectAddon(ctx, p.ID, "minio", "prod")
	if err != nil {
		return err
	}
	return h.provisionMinioAddon(ctx, p, "minio", "prod", addon)
}

// PromoteMinioAddon POST /projects/{slug}/addons/minio/promote-prod
func (h *Handler) PromoteMinioAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	if !auth.CanWriteProd(u.Role) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được provision MinIO prod"})
		return
	}
	if !h.minioDevAddonReady(r.Context(), p.ID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Dev chưa bật MinIO addon"})
		return
	}
	if err := h.promoteMinioAddonFromDev(r.Context(), p); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Provision MinIO prod thất bại: " + err.Error()})
		return
	}
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", "prod")
	if err != nil || addon == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Không đọc được addon prod sau provision"})
		return
	}
	reconciled := h.reconcileMinioAddonStatus(r.Context(), p, addon)
	auditAction(r.Context(), h, r, "addon.minio.promote_prod", slug, map[string]any{"by": u.Email})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Đã provision MinIO prod — S3_* đã sync vào app env",
		"addon":   h.enrichMinioAddonAPIView(r.Context(), p, reconciled, true),
	})
}

// GetMinioHACapability GET /projects/{slug}/addons/minio/ha-capability
func (h *Handler) GetMinioHACapability(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if _, ok := h.requireProjectAccess(w, r, slug); !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.minioHACapability(r.Context()))
}

// UpgradeMinioAddonHA POST /projects/{slug}/addons/minio/upgrade-ha
// standalone → distributed. Không auto-migrate object; cần confirm: true.
func (h *Handler) UpgradeMinioAddonHA(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	var body struct {
		Environment string `json:"environment"`
		Confirm     bool   `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	env := strings.TrimSpace(body.Environment)
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	if env == "prod" && !auth.CanWriteProd(u.Role) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Chỉ admin/tech_lead được upgrade MinIO prod"})
		return
	}
	if !body.Confirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Cần confirm: true — Upgrade HA không copy object từ PVC standalone",
		})
		return
	}
	if !h.minioHACapable(r.Context()) {
		cap := h.minioHACapability(r.Context())
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":         "Cluster chưa ha_capable",
			"ha_capability": cap,
		})
		return
	}
	addon, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil || addon == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MinIO chưa được bật"})
		return
	}
	if normalizeMinioTopology(addon.Topology) == "distributed" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Đã là topology distributed"})
		return
	}
	ns := h.projectNamespace(p, env)
	release := addon.K8sRelease
	if release == "" {
		release = minioAddonRelease(p.Slug, env)
	}
	if _, ok := h.guardK8sWrite(w, r, ns); !ok {
		return
	}
	// Xóa STS cũ (PVC standalone giữ orphan để cứu nếu cần). volumeClaimTemplates không đổi in-place.
	_ = h.rancher.DeleteNamespacedObject(r.Context(), "", "/apis/apps/v1/statefulsets", ns, release)
	h.setProjectAddonTopology(r.Context(), p.ID, "minio", env, "distributed")
	addon.Topology = "distributed"
	if err := h.provisionMinioAddon(r.Context(), p, "minio", env, addon); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Upgrade HA thất bại: " + err.Error()})
		return
	}
	fresh, _ := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if fresh == nil {
		fresh = addon
	}
	reconciled := h.reconcileMinioAddonStatus(r.Context(), p, fresh)
	auditAction(r.Context(), h, r, "addon.minio.upgrade_ha", slug, map[string]any{
		"environment": env, "by": u.Email,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"message": "Đã upgrade MinIO sang distributed — PVC standalone cũ không tự migrate",
		"addon":   h.enrichMinioAddonAPIView(r.Context(), p, reconciled, true),
	})
}

// RestartMinioAddon POST /projects/{slug}/addons/minio/restart
func (h *Handler) RestartMinioAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	if !h.rancherRequired(w, r) || !h.pluginEnabled(r.Context(), plugins.Rancher) {
		return
	}
	var body struct {
		Environment string `json:"environment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	env := strings.TrimSpace(body.Environment)
	if env == "" {
		env = strings.TrimSpace(r.URL.Query().Get("environment"))
	}
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	item, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MinIO chưa được bật"})
		return
	}
	release := item.K8sRelease
	if release == "" {
		release = minioAddonRelease(p.Slug, env)
	}
	ns := h.projectNamespace(p, env)
	if _, ok := h.guardK8sWrite(w, r, ns); !ok {
		return
	}
	if err := h.rancher.RolloutRestartStatefulSet(r.Context(), clusterQuery(r), ns, release); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	auditAction(r.Context(), h, r, "addon.minio.restart", slug, map[string]any{
		"environment": env, "release": release, "by": u.Email,
	})
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"message": "Đang restart MinIO — pod mới sẽ lên trong vài giây",
	})
}

// GetMinioAddonLogs GET /projects/{slug}/addons/minio/logs
func (h *Handler) GetMinioAddonLogs(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	if !h.rancherRequired(w, r) {
		return
	}
	env := strings.TrimSpace(r.URL.Query().Get("environment"))
	if env == "" {
		env = "dev"
	}
	if !validAddonEnv(env) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "environment phải là dev hoặc prod"})
		return
	}
	item, err := h.getProjectAddon(r.Context(), p.ID, "minio", env)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "MinIO chưa được bật"})
		return
	}
	release := item.K8sRelease
	if release == "" {
		release = minioAddonRelease(p.Slug, env)
	}
	ns := h.projectNamespace(p, env)
	if _, ok := h.guardK8sRead(w, r, "pods", ns); !ok {
		return
	}
	pod := release + "-0"
	if v := strings.TrimSpace(r.URL.Query().Get("pod")); v != "" {
		pod = v
	}
	tail := 200
	if v := strings.TrimSpace(r.URL.Query().Get("tail")); v != "" {
		if n, err := parsePositiveInt(v); err == nil && n > 0 && n <= 2000 {
			tail = n
		}
	}
	logs, err := h.rancher.GetPodLogs(r.Context(), clusterQuery(r), ns, pod, "minio", tail)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"pod":  pod,
		"logs": logs,
	})
}
