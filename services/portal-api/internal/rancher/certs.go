package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CertificateReady trả về ready | pending | failed | unknown từ cert-manager Certificate.
func (c *Client) CertificateReady(ctx context.Context, clusterOverride, namespace, certName string) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/apis/cert-manager.io/v1/namespaces/%s/certificates/%s",
		clusterID, namespace, certName)
	body, status, err := c.do(ctx, http.MethodGet, path, nil, "application/json")
	if status == 404 || err != nil {
		return "pending", nil
	}
	var cert struct {
		Status struct {
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
				Reason string `json:"reason"`
			} `json:"conditions"`
		} `json:"status"`
	}
	if json.Unmarshal(body, &cert) != nil {
		return "unknown", nil
	}
	for _, cond := range cert.Status.Conditions {
		if cond.Type == "Ready" {
			if strings.EqualFold(cond.Status, "True") {
				return "ready", nil
			}
			if strings.EqualFold(cond.Reason, "Failed") || strings.Contains(strings.ToLower(cond.Reason), "fail") {
				return "failed", nil
			}
			return "pending", nil
		}
	}
	return "pending", nil
}
