package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type addonCatalogItem struct {
	Engine      string `json:"engine"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Available   bool   `json:"available"`
}

type projectAddonView struct {
	Engine            string `json:"engine"`
	Environment       string `json:"environment"`
	Status            string `json:"status"`
	K8sRelease        string `json:"k8s_release,omitempty"`
	MaxMemoryMB       int    `json:"max_memory_mb"`
	MaxClients        int    `json:"max_clients"`
	MaxmemoryPolicy   string `json:"maxmemory_policy"`
	DefaultKeyTTLSec  int    `json:"default_key_ttl_sec"`
	HasConnection     bool   `json:"has_connection"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

type redisAddonAPIView struct {
	projectAddonView
	PolicyHint                    string             `json:"policy_hint,omitempty"`
	GrafanaDashboardURL           string             `json:"grafana_dashboard_url,omitempty"`
	ConnectionURLMasked           string             `json:"connection_url_masked,omitempty"`
	ConnectionURL                 string             `json:"connection_url,omitempty"`
	ConnectionURLExternalMasked   string             `json:"connection_url_external_masked,omitempty"`
	ConnectionURLExternal         string             `json:"connection_url_external,omitempty"`
	ExternalHostname              string             `json:"external_hostname,omitempty"`
	ExternalPort                  int                `json:"external_port,omitempty"`
	ConnectionSecret              string             `json:"connection_secret,omitempty"`
	Pod                           *redisAddonPodView `json:"pod,omitempty"`
}

const (
	redisAddonImage       = "redis:7.2-alpine"
	redisAddonServicePort = 6379
)

var addonCatalog = []addonCatalogItem{
	{
		Engine:      "redis",
		Label:       "Redis",
		Description: "Cache, session, queue — mỗi project một instance riêng",
		Icon:        "redis",
		Available:   true,
	},
	{
		Engine:      "postgres",
		Label:       "Postgres",
		Description: "Database qua CNPG — Phase 10b",
		Icon:        "postgres",
		Available:   false,
	},
}

func validAddonEngine(engine string) bool {
	for _, item := range addonCatalog {
		if item.Engine == engine && item.Available {
			return true
		}
	}
	return false
}

func validAddonEnv(env string) bool {
	return env == "dev" || env == "prod"
}

func (h *Handler) listProjectAddons(ctx context.Context, projectID int64) ([]projectAddonView, error) {
	rows, err := h.db.Query(ctx, `
		SELECT engine, environment, status, k8s_release, max_memory_mb, max_clients,
		       maxmemory_policy, default_key_ttl_sec,
		       (connection_secret <> ''), created_at::text, updated_at::text
		FROM project_data_addons
		WHERE project_id = $1
		ORDER BY engine, environment`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]projectAddonView, 0, 4)
	for rows.Next() {
		var v projectAddonView
		if err := rows.Scan(
			&v.Engine, &v.Environment, &v.Status, &v.K8sRelease,
			&v.MaxMemoryMB, &v.MaxClients, &v.MaxmemoryPolicy, &v.DefaultKeyTTLSec,
			&v.HasConnection,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (h *Handler) getProjectAddon(ctx context.Context, projectID int64, engine, env string) (*projectAddonView, error) {
	var v projectAddonView
	err := h.db.QueryRow(ctx, `
		SELECT engine, environment, status, k8s_release, max_memory_mb, max_clients,
		       maxmemory_policy, default_key_ttl_sec,
		       (connection_secret <> ''), created_at::text, updated_at::text
		FROM project_data_addons
		WHERE project_id = $1 AND engine = $2 AND environment = $3`,
		projectID, engine, env).Scan(
		&v.Engine, &v.Environment, &v.Status, &v.K8sRelease,
		&v.MaxMemoryMB, &v.MaxClients, &v.MaxmemoryPolicy, &v.DefaultKeyTTLSec,
		&v.HasConnection, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(v.MaxmemoryPolicy) == "" {
		v.MaxmemoryPolicy = "allkeys-lru"
	}
	return &v, nil
}

func redisAddonRelease(slug, env string) string {
	return slug + "-redis-" + env
}

func redisAddonConnectionSecretName(release string) string {
	return release + "-connection"
}

func maskRedisURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd < 0 {
		return "••••••••"
	}
	rest := raw[schemeEnd+3:]
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return raw
	}
	return raw[:schemeEnd+3] + "***@" + rest[at+1:]
}

func (h *Handler) getProjectAddonConnectionSecretName(ctx context.Context, projectID int64, engine, env string) string {
	var name string
	_ = h.db.QueryRow(ctx, `
		SELECT connection_secret FROM project_data_addons
		WHERE project_id=$1 AND engine=$2 AND environment=$3`,
		projectID, engine, env).Scan(&name)
	return strings.TrimSpace(name)
}

func randomRedisPassword() (string, error) {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func redisMemoryLimitMi(maxMemoryMB int) int {
	if maxMemoryMB < 64 {
		return 64
	}
	return maxMemoryMB
}

func redisRequestMi(limitMi int) int {
	return int(math.Max(32, float64(limitMi/2)))
}

func (h *Handler) setProjectAddonStatus(ctx context.Context, projectID int64, engine, env, status string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_data_addons
		SET status=$1, updated_at=now()
		WHERE project_id=$2 AND engine=$3 AND environment=$4`,
		status, projectID, engine, env)
}

func (h *Handler) setProjectAddonConnectionSecret(ctx context.Context, projectID int64, engine, env, secretName string) {
	_, _ = h.db.Exec(ctx, `
		UPDATE project_data_addons
		SET connection_secret=$1, updated_at=now()
		WHERE project_id=$2 AND engine=$3 AND environment=$4`,
		secretName, projectID, engine, env)
}

func (h *Handler) upsertRuntimeEnvVar(ctx context.Context, projectID int64, env, key, value string, isSecret bool) error {
	var id int64
	err := h.db.QueryRow(ctx, `
		SELECT id FROM project_env_vars
		WHERE project_id=$1 AND environment=$2 AND scope='runtime' AND key=$3
		LIMIT 1`, projectID, env, key).Scan(&id)
	if err == nil {
		_, err = h.db.Exec(ctx, `
			UPDATE project_env_vars
			SET value=$1, is_secret=$2, updated_at=now()
			WHERE id=$3`, value, isSecret, id)
		return err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	_, err = h.db.Exec(ctx, `
		INSERT INTO project_env_vars (project_id, environment, key, value, is_secret, scope)
		VALUES ($1,$2,$3,$4,$5,'runtime')`,
		projectID, env, key, value, isSecret)
	return err
}

func (h *Handler) redisAddonURL(release, namespace, password string) string {
	host := fmt.Sprintf("%s.%s.svc.cluster.local", release, namespace)
	return fmt.Sprintf("redis://:%s@%s:%d/0", password, host, redisAddonServicePort)
}

func (h *Handler) applyRedisAddonObjects(ctx context.Context, p projectRow, env string, addon *projectAddonView) (string, error) {
	if h.rancher == nil || !h.rancher.Enabled() || !h.pluginEnabled(ctx, plugins.Rancher) {
		return "", fmt.Errorf("Rancher chưa sẵn sàng")
	}
	ns := h.projectNamespace(p, env)
	release := redisAddonRelease(p.Slug, env)
	if err := h.rancher.EnsureNamespace(ctx, "", ns); err != nil {
		return "", err
	}
	pass, err := randomRedisPassword()
	if err != nil {
		return "", err
	}
	authSecretName := release + "-auth"
	authSecret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": authSecretName,
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"password": pass,
		},
	}
	authSecretJSON, _ := json.Marshal(authSecret)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/secrets", ns, authSecretJSON); err != nil {
		return "", fmt.Errorf("secret auth: %w", err)
	}

	limitMi := redisMemoryLimitMi(addon.MaxMemoryMB)
	requestMi := redisRequestMi(limitMi)
	policy := normalizeRedisMaxmemoryPolicy(addon.MaxmemoryPolicy)
	conf := fmt.Sprintf("bind 0.0.0.0\nport %d\nappendonly yes\nmaxmemory %dmb\nmaxmemory-policy %s\nmaxclients %d\n", redisAddonServicePort, limitMi, policy, addon.MaxClients)
	confMap := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name": release + "-config",
		},
		"data": map[string]string{
			"redis.conf": conf,
		},
	}
	confJSON, _ := json.Marshal(confMap)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/configmaps", ns, confJSON); err != nil {
		return "", fmt.Errorf("configmap redis: %w", err)
	}

	svc := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name": release,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": release,
			},
		},
		"spec": map[string]any{
			"type": "ClusterIP",
			"ports": []map[string]any{
				{
					"name":       "redis",
					"port":       redisAddonServicePort,
					"targetPort": redisAddonServicePort,
				},
				{
					"name":       "metrics",
					"port":       redisExporterPort,
					"targetPort": redisExporterPort,
				},
			},
			"selector": map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": release,
			},
		},
	}
	if env == "dev" {
		nodePort := redisAddonNodePort(p.Slug, env)
		svc["spec"].(map[string]any)["type"] = "NodePort"
		svc["spec"].(map[string]any)["ports"] = []map[string]any{
			{
				"name":       "redis",
				"port":       redisAddonServicePort,
				"targetPort": redisAddonServicePort,
				"nodePort":   nodePort,
			},
			{
				"name":       "metrics",
				"port":       redisExporterPort,
				"targetPort": redisExporterPort,
			},
		}
	}
	svcJSON, _ := json.Marshal(svc)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/services", ns, svcJSON); err != nil {
		return "", fmt.Errorf("service redis: %w", err)
	}

	sts := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "StatefulSet",
		"metadata": map[string]any{
			"name": release,
			"labels": map[string]string{
				"app.kubernetes.io/name":     "redis",
				"app.kubernetes.io/instance": release,
			},
		},
		"spec": map[string]any{
			"serviceName": release,
			"replicas":    1,
			"selector": map[string]any{
				"matchLabels": map[string]string{
					"app.kubernetes.io/name":     "redis",
					"app.kubernetes.io/instance": release,
				},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{
						"app.kubernetes.io/name":     "redis",
						"app.kubernetes.io/instance": release,
					},
				},
				"spec": map[string]any{
					"containers": []map[string]any{
						{
							"name":            "redis",
							"image":           redisAddonImage,
							"imagePullPolicy": "IfNotPresent",
							"command":         []string{"sh", "-c", "redis-server /etc/redis/redis.conf --requirepass \"$REDIS_PASSWORD\""},
							"ports": []map[string]any{
								{"containerPort": redisAddonServicePort, "name": "redis"},
							},
							"env": []map[string]any{
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
								{"name": "config", "mountPath": "/etc/redis"},
							},
							"livenessProbe": map[string]any{
								"exec": map[string]any{
									"command": []string{"sh", "-c", "redis-cli -a \"$REDIS_PASSWORD\" ping | grep PONG"},
								},
								"initialDelaySeconds": 15,
								"periodSeconds":       10,
								"timeoutSeconds":      5,
							},
							"readinessProbe": map[string]any{
								"exec": map[string]any{
									"command": []string{"sh", "-c", "redis-cli -a \"$REDIS_PASSWORD\" ping | grep PONG"},
								},
								"initialDelaySeconds": 5,
								"periodSeconds":       5,
								"timeoutSeconds":      3,
							},
						},
						redisExporterContainer(authSecretName),
					},
					"volumes": []map[string]any{
						{
							"name": "config",
							"configMap": map[string]any{
								"name": release + "-config",
								"items": []map[string]any{
									{"key": "redis.conf", "path": "redis.conf"},
								},
							},
						},
					},
				},
			},
			"volumeClaimTemplates": []map[string]any{
				{
					"metadata": map[string]any{
						"name": "data",
					},
					"spec": map[string]any{
						"accessModes": []string{"ReadWriteOnce"},
						"resources": map[string]any{
							"requests": map[string]string{
								"storage": "1Gi",
							},
						},
					},
				},
			},
		},
	}
	stsJSON, _ := json.Marshal(sts)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/apps/v1/statefulsets", ns, stsJSON); err != nil {
		return "", fmt.Errorf("statefulset redis: %w", err)
	}
	if env == "prod" {
		if err := h.applyRedisNetworkPolicy(ctx, release, ns, p.Slug); err != nil {
			return "", fmt.Errorf("networkpolicy redis: %w", err)
		}
	}

	connURL := h.redisAddonURL(release, ns, pass)
	connSecretName := redisAddonConnectionSecretName(release)
	connSecret := map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]any{
			"name": connSecretName,
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"REDIS_URL": connURL,
		},
	}
	connSecretJSON, _ := json.Marshal(connSecret)
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/api/v1/secrets", ns, connSecretJSON); err != nil {
		return "", fmt.Errorf("secret connection: %w", err)
	}

	if err := h.upsertRuntimeEnvVar(ctx, p.ID, env, "REDIS_URL", connURL, true); err != nil {
		return "", fmt.Errorf("save env REDIS_URL: %w", err)
	}
	ttl := normalizeRedisDefaultKeyTTL(addon.DefaultKeyTTLSec)
	if ttl > 0 {
		if err := h.upsertRuntimeEnvVar(ctx, p.ID, env, "REDIS_KEY_TTL_SECONDS", strconv.Itoa(ttl), false); err != nil {
			return "", fmt.Errorf("save env REDIS_KEY_TTL_SECONDS: %w", err)
		}
	}
	smJSON, _ := json.Marshal(redisExporterServiceMonitor(release, ns))
	if err := h.rancher.ApplyNamespacedObject(ctx, "", "/apis/monitoring.coreos.com/v1/servicemonitors", ns, smJSON); err != nil {
		return "", fmt.Errorf("servicemonitor redis-exporter: %w", err)
	}
	if err := h.syncAppEnvSecret(ctx, p, env, "", true); err != nil {
		return "", fmt.Errorf("sync app-env: %w", err)
	}
	// Secret auth đổi password nhưng pod Redis chỉ đọc REDIS_PASSWORD lúc khởi động — bắt buộc rollout restart.
	if err := h.rancher.RolloutRestartStatefulSet(ctx, "", ns, release); err != nil {
		return "", fmt.Errorf("restart redis statefulset: %w", err)
	}
	return connSecretName, nil
}

func redisStatefulSetReady(row rancher.ResourceRow) bool {
	status := strings.ToLower(strings.TrimSpace(row.Status))
	replicas := strings.ToLower(strings.TrimSpace(row.Replicas))
	if status == "available" || strings.Contains(status, "ready") {
		return true
	}
	if strings.Contains(replicas, "1/1") {
		return true
	}
	if strings.HasPrefix(replicas, "1/") && strings.Contains(replicas, "ready") {
		return true
	}
	return false
}

func (h *Handler) reconcileRedisAddonStatus(ctx context.Context, p projectRow, addon *projectAddonView) projectAddonView {
	out := *addon
	ns := h.projectNamespace(p, addon.Environment)
	release := out.K8sRelease
	if release == "" {
		release = redisAddonRelease(p.Slug, addon.Environment)
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
		} else if out.HasConnection {
			out.Status = "provisioning"
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
		connSecretName := redisAddonConnectionSecretName(release)
		if data, ok, err := h.rancher.GetOpaqueSecretData(ctx, "", ns, connSecretName); err == nil && ok && strings.TrimSpace(data["REDIS_URL"]) != "" {
			out.HasConnection = true
			h.setProjectAddonConnectionSecret(ctx, p.ID, addon.Engine, addon.Environment, connSecretName)
		}
	}
	return out
}

func (h *Handler) buildRedisAddonAPIView(ctx context.Context, p projectRow, v projectAddonView, exposeFullURL bool) redisAddonAPIView {
	out := redisAddonAPIView{projectAddonView: v}
	out.PolicyHint = redisMaxmemoryPolicyHint(v.MaxmemoryPolicy)
	release := v.K8sRelease
	if release == "" {
		release = redisAddonRelease(p.Slug, v.Environment)
	}
	out.GrafanaDashboardURL = grafanaRedisAddonDashboardURL(h.cfg.GrafanaURL, h.projectNamespace(p, v.Environment), release)
	if !v.HasConnection {
		return out
	}
	sec := h.getProjectAddonConnectionSecretName(ctx, p.ID, v.Engine, v.Environment)
	if sec != "" {
		out.ConnectionSecret = sec
	}
	vars, err := h.envVarsMap(ctx, p.ID, v.Environment)
	if err == nil {
		if u := strings.TrimSpace(vars["REDIS_URL"]); u != "" {
			out.ConnectionURLMasked = maskRedisURL(u)
			if exposeFullURL {
				out.ConnectionURL = u
			}
		}
	}
	return out
}

func (h *Handler) provisionRedisAddon(ctx context.Context, p projectRow, engine, env string, addon *projectAddonView) error {
	h.setProjectAddonStatus(ctx, p.ID, engine, env, "provisioning")
	runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	connSecret, err := h.applyRedisAddonObjects(runCtx, p, env, addon)
	if err != nil {
		h.setProjectAddonStatus(ctx, p.ID, engine, env, "failed")
		return err
	}
	h.setProjectAddonConnectionSecret(ctx, p.ID, engine, env, connSecret)
	h.setProjectAddonStatus(ctx, p.ID, engine, env, "running")
	addon.HasConnection = true
	addon.Status = "running"
	return nil
}

// ListProjectAddons GET /projects/{slug}/addons
func (h *Handler) ListProjectAddons(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	items, err := h.listProjectAddons(r.Context(), p.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i := range items {
		if items[i].Engine == "redis" {
			items[i] = h.reconcileRedisAddonStatus(r.Context(), p, &items[i])
		}
	}
	u, _ := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog":    addonCatalog,
		"items":      items,
		"can_manage": h.canManageProject(r.Context(), u, p.ID),
		"project":    map[string]string{"slug": p.Slug, "name": p.Name},
	})
}

// GetProjectAddon GET /projects/{slug}/addons/{engine}?environment=dev
func (h *Handler) GetProjectAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	engine := strings.TrimSpace(chi.URLParam(r, "engine"))
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
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
	catalogItem, catalogOK := addonByEngine(engine)
	if !catalogOK {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "addon không tồn tại"})
		return
	}

	item, err := h.getProjectAddon(r.Context(), p.ID, engine, env)
	u, _ := auth.UserFromContext(r.Context())
	canManage := h.canManageProject(r.Context(), u, p.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{
				"catalog":     catalogItem,
				"installed":   false,
				"can_manage":  canManage,
				"environment": env,
				"namespace":   h.projectNamespace(p, env),
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if engine == "redis" && canManage {
		needsProvision := !item.HasConnection && (item.Status == "pending" || item.Status == "provisioning" || item.Status == "failed")
		if needsProvision {
			_ = h.provisionRedisAddon(r.Context(), p, engine, env, item)
		}
	}

	addonPayload := any(item)
	if engine == "redis" {
		reconciled := h.reconcileRedisAddonStatus(r.Context(), p, item)
		addonPayload = h.enrichRedisAddonAPIView(r.Context(), p, reconciled, canManage, "")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"catalog":     catalogItem,
		"installed":   true,
		"addon":       addonPayload,
		"can_manage":  canManage,
		"environment": env,
		"namespace":   h.projectNamespace(p, env),
	})
}

func addonByEngine(engine string) (addonCatalogItem, bool) {
	for _, item := range addonCatalog {
		if item.Engine == engine {
			return item, true
		}
	}
	return addonCatalogItem{}, false
}

// CreateProjectAddon POST /projects/{slug}/addons/{engine}
func (h *Handler) CreateProjectAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	engine := strings.TrimSpace(chi.URLParam(r, "engine"))
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	if !validAddonEngine(engine) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "addon chưa hỗ trợ hoặc chưa bật"})
		return
	}

	var body struct {
		Environment      string `json:"environment"`
		MaxMemoryMB      int    `json:"max_memory_mb"`
		MaxClients       int    `json:"max_clients"`
		MaxmemoryPolicy  string `json:"maxmemory_policy"`
		DefaultKeyTTLSec int    `json:"default_key_ttl_sec"`
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
		writeAccessDenied(w)
		return
	}
	maxMem := body.MaxMemoryMB
	if maxMem < 64 || maxMem > 512 {
		maxMem = 128
	}
	maxClients := body.MaxClients
	if maxClients < 10 || maxClients > 1000 {
		maxClients = 100
	}
	policy := normalizeRedisMaxmemoryPolicy(body.MaxmemoryPolicy)
	defaultTTL := normalizeRedisDefaultKeyTTL(body.DefaultKeyTTLSec)
	if defaultTTL == 0 && body.DefaultKeyTTLSec == 0 {
		defaultTTL = 86400
	}

	ns := h.projectNamespace(p, env)
	if ns == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace chưa cấu hình"})
		return
	}

	release := p.Slug + "-" + engine + "-" + env
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO project_data_addons (project_id, engine, environment, status, k8s_release, max_memory_mb, max_clients, maxmemory_policy, default_key_ttl_sec)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7, $8)
		ON CONFLICT (project_id, engine, environment) DO UPDATE SET
			status = CASE WHEN project_data_addons.status IN ('stopped', 'failed') THEN 'pending' ELSE project_data_addons.status END,
			max_memory_mb = EXCLUDED.max_memory_mb,
			max_clients = EXCLUDED.max_clients,
			maxmemory_policy = EXCLUDED.maxmemory_policy,
			default_key_ttl_sec = EXCLUDED.default_key_ttl_sec,
			updated_at = now()`,
		p.ID, engine, env, release, maxMem, maxClients, policy, defaultTTL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	auditAction(r.Context(), h, r, "addon.create", slug, map[string]any{
		"engine": engine, "environment": env, "by": u.Email,
	})

	addon, err := h.getProjectAddon(r.Context(), p.ID, engine, env)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := h.provisionRedisAddon(r.Context(), p, engine, env, addon); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":       "Provision Redis thất bại: " + err.Error(),
			"status":      "failed",
			"engine":      engine,
			"environment": env,
			"k8s_release": release,
		})
		return
	}
	reconciled := h.reconcileRedisAddonStatus(r.Context(), p, addon)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "running",
		"message":     "Đã provision Redis, tạo REDIS_URL và sync vào app env",
		"engine":      engine,
		"environment": env,
		"k8s_release": release,
		"addon":       h.enrichRedisAddonAPIView(r.Context(), p, reconciled, true, ""),
	})
}

// ProvisionProjectAddon POST /projects/{slug}/addons/{engine}/provision
func (h *Handler) ProvisionProjectAddon(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	engine := strings.TrimSpace(chi.URLParam(r, "engine"))
	p, ok := h.requireProjectAccess(w, r, slug)
	if !ok {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if !h.canManageProject(r.Context(), u, p.ID) {
		writeAccessDenied(w)
		return
	}
	if engine != "redis" || !validAddonEngine(engine) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "re-provision chỉ hỗ trợ Redis"})
		return
	}

	var body struct {
		Environment      string `json:"environment"`
		MaxMemoryMB      int    `json:"max_memory_mb"`
		MaxClients       int    `json:"max_clients"`
		MaxmemoryPolicy  string `json:"maxmemory_policy"`
		DefaultKeyTTLSec int    `json:"default_key_ttl_sec"`
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
	if env == "prod" && !auth.CanWriteProd(u.Role) {
		writeAccessDenied(w)
		return
	}

	item, err := h.getProjectAddon(r.Context(), p.ID, engine, env)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "addon chưa được bật — dùng POST /addons/redis trước"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if body.MaxMemoryMB >= 64 && body.MaxMemoryMB <= 512 {
		item.MaxMemoryMB = body.MaxMemoryMB
		_, _ = h.db.Exec(r.Context(), `
			UPDATE project_data_addons SET max_memory_mb=$1, updated_at=now()
			WHERE project_id=$2 AND engine=$3 AND environment=$4`,
			item.MaxMemoryMB, p.ID, engine, env)
	}
	if body.MaxClients >= 10 && body.MaxClients <= 1000 {
		item.MaxClients = body.MaxClients
		_, _ = h.db.Exec(r.Context(), `
			UPDATE project_data_addons SET max_clients=$1, updated_at=now()
			WHERE project_id=$2 AND engine=$3 AND environment=$4`,
			item.MaxClients, p.ID, engine, env)
	}
	if strings.TrimSpace(body.MaxmemoryPolicy) != "" {
		item.MaxmemoryPolicy = normalizeRedisMaxmemoryPolicy(body.MaxmemoryPolicy)
		_, _ = h.db.Exec(r.Context(), `
			UPDATE project_data_addons SET maxmemory_policy=$1, updated_at=now()
			WHERE project_id=$2 AND engine=$3 AND environment=$4`,
			item.MaxmemoryPolicy, p.ID, engine, env)
	}
	if body.DefaultKeyTTLSec >= 0 && body.DefaultKeyTTLSec <= 2592000 {
		item.DefaultKeyTTLSec = normalizeRedisDefaultKeyTTL(body.DefaultKeyTTLSec)
		_, _ = h.db.Exec(r.Context(), `
			UPDATE project_data_addons SET default_key_ttl_sec=$1, updated_at=now()
			WHERE project_id=$2 AND engine=$3 AND environment=$4`,
			item.DefaultKeyTTLSec, p.ID, engine, env)
	}

	auditAction(r.Context(), h, r, "addon.provision", slug, map[string]any{
		"engine": engine, "environment": env, "by": u.Email,
	})

	if err := h.provisionRedisAddon(r.Context(), p, engine, env, item); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error":  "Re-provision Redis thất bại: " + err.Error(),
			"status": "failed",
		})
		return
	}
	reconciled := h.reconcileRedisAddonStatus(r.Context(), p, item)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "running",
		"message":     "Đã re-provision Redis — REDIS_URL mới đã sync vào app env",
		"environment": env,
		"addon":       h.enrichRedisAddonAPIView(r.Context(), p, reconciled, true, ""),
	})
}
