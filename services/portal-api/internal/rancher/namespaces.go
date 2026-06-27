package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
