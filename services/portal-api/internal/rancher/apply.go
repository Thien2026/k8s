package rancher

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ApplyNamespacedObject tạo hoặc cập nhật resource namespaced.
// POST để tạo mới; nếu đã tồn tại (409) thì GET resource để lấy resourceVersion,
// merge vào payload rồi PUT — đảm bảo update đúng và giữ nguyên annotations ngoài.
func (c *Client) ApplyNamespacedObject(ctx context.Context, clusterOverride, apiPath, namespace string, payload []byte) error {
	if !c.Enabled() {
		return fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return fmt.Errorf("invalid manifest json: %w", err)
	}
	meta, _ := obj["metadata"].(map[string]any)
	name, _ := meta["name"].(string)
	if name == "" {
		return fmt.Errorf("manifest thiếu metadata.name")
	}
	collection := fmt.Sprintf("/k8s/clusters/%s%s", clusterID, namespacedAPIPath(apiPath, namespace))
	item := collection + "/" + name

	_, status, err := c.do(ctx, http.MethodPost, collection, payload, "application/json")
	if err == nil && (status == http.StatusCreated || status == http.StatusOK) {
		return nil
	}
	if status == http.StatusConflict || status == http.StatusUnprocessableEntity || status == http.StatusBadRequest {
		// GET resource hiện tại để lấy resourceVersion (bắt buộc cho PUT).
		existing, _, getErr := c.do(ctx, http.MethodGet, item, nil, "application/json")
		if getErr == nil && len(existing) > 0 {
			var cur map[string]any
			if json.Unmarshal(existing, &cur) == nil {
				curMeta, _ := cur["metadata"].(map[string]any)
				if rv, _ := curMeta["resourceVersion"].(string); rv != "" {
					meta["resourceVersion"] = rv
					// Giữ lại annotations hiện tại (Rancher, cert-manager...) rồi merge vào.
					if curAnn, ok := curMeta["annotations"].(map[string]any); ok {
						merged := map[string]any{}
						for k, v := range curAnn {
							merged[k] = v
						}
						if newAnn, ok := meta["annotations"].(map[string]any); ok {
							for k, v := range newAnn {
								merged[k] = v
							}
						}
						meta["annotations"] = merged
					}
					obj["metadata"] = meta
					payload, _ = json.Marshal(obj)
				}
			}
		}
		_, _, putErr := c.do(ctx, http.MethodPut, item, payload, "application/json")
		return putErr
	}
	return fmt.Errorf("apply %s/%s: %w", namespace, name, err)
}

// PatchNamespacedObject áp dụng JSON patch lên resource namespaced.
func (c *Client) PatchNamespacedObject(ctx context.Context, clusterOverride, apiPath, namespace, name string, patch []byte) error {
	if !c.Enabled() {
		return fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	item := fmt.Sprintf("/k8s/clusters/%s%s/%s", clusterID, namespacedAPIPath(apiPath, namespace), name)
	_, _, err = c.do(ctx, http.MethodPatch, item, patch, "application/json-patch+json")
	return err
}

// ApplyDeploymentAndService đảm bảo namespace rồi apply Deployment + Service.
func (c *Client) ApplyDeploymentAndService(ctx context.Context, clusterOverride, namespace string, deployment, service []byte) error {
	if err := c.EnsureNamespace(ctx, clusterOverride, namespace); err != nil {
		return err
	}
	if err := c.ApplyNamespacedObject(ctx, clusterOverride, "/apis/apps/v1/deployments", namespace, deployment); err != nil {
		return fmt.Errorf("deployment: %w", err)
	}
	if err := c.ApplyNamespacedObject(ctx, clusterOverride, "/api/v1/services", namespace, service); err != nil {
		return fmt.Errorf("service: %w", err)
	}
	return nil
}

// DeleteNamespacedObject xóa resource namespaced (bỏ qua nếu không tồn tại).
func (c *Client) DeleteNamespacedObject(ctx context.Context, clusterOverride, apiPath, namespace, name string) error {
	if !c.Enabled() {
		return nil
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s%s", clusterID, namespacedAPIPath(apiPath, namespace)) + "/" + name
	_, status, err := c.do(ctx, http.MethodDelete, path, nil, "")
	if err != nil && status != http.StatusNotFound {
		return err
	}
	return nil
}

// RolloutRestartDeployment khởi động lại pod bằng annotation restartedAt.
func (c *Client) RolloutRestartDeployment(ctx context.Context, clusterOverride, namespace, name string) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 150 * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}
		lastErr = c.rolloutRestartDeploymentOnce(ctx, clusterOverride, namespace, name)
		if lastErr == nil {
			return nil
		}
		if !isK8sConflict(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

func isK8sConflict(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "rancher api 409") || strings.Contains(s, `"reason":"Conflict"`)
}

func (c *Client) rolloutRestartDeploymentOnce(ctx context.Context, clusterOverride, namespace, name string) error {
	if !c.Enabled() {
		return fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/apis/apps/v1/namespaces/%s/deployments/%s", clusterID, namespace, name)
	body, status, err := c.do(ctx, http.MethodGet, path, nil, "application/json")
	if err != nil {
		if status == http.StatusNotFound {
			return nil
		}
		return err
	}
	var dep map[string]any
	if err := json.Unmarshal(body, &dep); err != nil {
		return err
	}
	spec, _ := dep["spec"].(map[string]any)
	if spec == nil {
		return fmt.Errorf("deployment %s/%s thiếu spec", namespace, name)
	}
	tpl, _ := spec["template"].(map[string]any)
	if tpl == nil {
		return fmt.Errorf("deployment %s/%s thiếu template", namespace, name)
	}
	meta, _ := tpl["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		tpl["metadata"] = meta
	}
	ann, _ := meta["annotations"].(map[string]any)
	if ann == nil {
		ann = map[string]any{}
		meta["annotations"] = ann
	}
	ann["kubectl.kubernetes.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339)
	payload, err := json.Marshal(dep)
	if err != nil {
		return err
	}
	_, _, err = c.do(ctx, http.MethodPut, path, payload, "application/json")
	return err
}

// GetOpaqueSecretData đọc Secret Opaque trên cluster (data base64-decoded).
func (c *Client) GetOpaqueSecretData(ctx context.Context, clusterOverride, namespace, name string) (map[string]string, bool, error) {
	if !c.Enabled() {
		return nil, false, fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return nil, false, err
	}
	path := fmt.Sprintf("/k8s/clusters/%s%s", clusterID, namespacedAPIPath("/api/v1/secrets", namespace)) + "/" + name
	body, status, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, false, err
	}
	if status == http.StatusNotFound {
		return nil, false, nil
	}
	if status != http.StatusOK {
		return nil, false, fmt.Errorf("get secret %s/%s: http %d", namespace, name, status)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, false, err
	}
	out := map[string]string{}
	if raw, ok := obj["data"].(map[string]any); ok {
		for k, v := range raw {
			s, _ := v.(string)
			if s == "" {
				continue
			}
			dec, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				continue
			}
			out[k] = string(dec)
		}
	}
	return out, true, nil
}
