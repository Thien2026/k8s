package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (c *Client) EnsureNamespace(ctx context.Context, clusterOverride, name string) error {
	if !c.Enabled() {
		return fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/v1/namespaces/%s", clusterID, name)
	_, status, err := c.do(ctx, http.MethodGet, path, nil, "application/json")
	if err == nil && status == http.StatusOK {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]string{"name": name},
	})
	_, status, err = c.do(ctx, http.MethodPost, fmt.Sprintf("/k8s/clusters/%s/v1/namespaces", clusterID), payload, "application/json")
	if err != nil && status != http.StatusConflict && status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}
	return nil
}

// PurgeNamespace gỡ workload phổ biến rồi xóa namespace (tránh sót khi Terminating kẹt).
func (c *Client) PurgeNamespace(ctx context.Context, clusterOverride, name string) error {
	if !c.Enabled() {
		return fmt.Errorf("rancher not configured")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	for _, kind := range []struct {
		apiPath  string
		resource string
	}{
		{"/apis/apps/v1/deployments", "deployments"},
		{"/api/v1/services", "services"},
		{"/apis/networking.k8s.io/v1/ingresses", "ingresses"},
	} {
		list, err := c.ListK8s(ctx, clusterOverride, kind.resource, name, 1, 200)
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			_ = c.DeleteNamespacedObject(ctx, clusterOverride, kind.apiPath, name, item.Name)
		}
	}
	return c.DeleteNamespace(ctx, clusterOverride, name)
}

// WaitNamespaceDeleted chờ namespace biến mất (sau DELETE async).
func (c *Client) WaitNamespaceDeleted(ctx context.Context, clusterOverride, name string, timeout time.Duration) error {
	if !c.Enabled() {
		return nil
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/v1/namespaces/%s", clusterID, name)
	deadline := time.Now().Add(timeout)
	for {
		_, status, _ := c.do(ctx, http.MethodGet, path, nil, "application/json")
		if status == http.StatusNotFound {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("namespace %s vẫn tồn tại sau %s — kiểm tra finalizer trên Rancher", name, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// DeleteNamespace xóa namespace trên cluster (xóa mọi workload bên trong).
func (c *Client) DeleteNamespace(ctx context.Context, clusterOverride, name string) error {
	if !c.Enabled() {
		return fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/v1/namespaces/%s", clusterID, name)
	_, status, err := c.do(ctx, http.MethodDelete, path, nil, "")
	if err != nil && status != http.StatusNotFound {
		return fmt.Errorf("delete namespace %s: %w", name, err)
	}
	return nil
}
