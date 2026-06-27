package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

type JoinInfo struct {
	ServerIP       string   `json:"server_ip"`
	ServerURL      string   `json:"server_url"`
	NodeCount      int      `json:"node_count"`
	RequiredPorts  []string `json:"required_ports"`
	JoinConfigured bool     `json:"join_configured"`
	GateRequired   bool     `json:"gate_required"`
}

type JoinScriptResponse struct {
	Script string `json:"script"`
	Note   string `json:"note"`
}

func BuildJoinInfo(cfg JoinConfig) JoinInfo {
	return JoinInfo{
		ServerIP:  cfg.ServerIP,
		ServerURL: cfg.ServerURL,
		RequiredPorts: []string{
			"9345/tcp (RKE2 supervisor)",
			"6443/tcp (Kubernetes API)",
			"10250/tcp (kubelet)",
			"8472/udp (Flannel VXLAN)",
		},
		JoinConfigured: cfg.ServerToken != "" && cfg.ServerURL != "",
		GateRequired:   cfg.GateSecret != "",
	}
}

type JoinConfig struct {
	ServerIP    string
	ServerURL   string
	ServerToken string
	GateSecret  string
}

func (c *JoinConfig) JoinScript(rke2Version string) (JoinScriptResponse, error) {
	if c.ServerToken == "" || c.ServerURL == "" {
		return JoinScriptResponse{}, fmt.Errorf("join chưa cấu hình trên server")
	}
	ver := rke2Version
	if ver != "" && !strings.HasPrefix(ver, "v") {
		ver = "v" + ver
	}
	installLine := "curl -sfL https://get.rke2.io | INSTALL_RKE2_TYPE=agent sh -"
	if ver != "" {
		installLine = fmt.Sprintf("curl -sfL https://get.rke2.io | INSTALL_RKE2_TYPE=agent INSTALL_RKE2_VERSION=%q sh -", ver)
	}

	script := fmt.Sprintf(`#!/bin/bash
# Chạy trên VPS worker mới (root). Firewall: mở outbound tới server.
set -euo pipefail

%s

mkdir -p /etc/rancher/rke2
cat >/etc/rancher/rke2/config.yaml <<'RKE2CFG'
server: %s
token: %s
RKE2CFG

systemctl enable rke2-agent
systemctl start rke2-agent
echo "Đang join cluster — kiểm tra: systemctl status rke2-agent"
`, installLine, c.ServerURL, c.ServerToken)

	return JoinScriptResponse{
		Script: script,
		Note:   "Token chỉ hiển thị một lần — không chia sẻ script công khai. Xóa script sau khi join xong.",
	}, nil
}

func (c *Client) JoinNodeCount(ctx context.Context, clusterID string) int {
	id := clusterID
	if id == "" {
		var err error
		id, err = c.clusterID(ctx)
		if err != nil {
			return 0
		}
	}
	body, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s/v1/nodes", id))
	if err != nil {
		return 0
	}
	_, total, err := parseK8sItems(body)
	if err != nil {
		return 0
	}
	return total
}

func resourceItemPath(res K8sResource, namespace, name string) string {
	path := res.APIPath
	if res.Namespaced {
		if namespace == "" {
			namespace = "default"
		}
		path = namespacedAPIPath(path, namespace)
	}
	return path + "/" + name
}

func (c *Client) GetK8sResource(ctx context.Context, clusterOverride, key, namespace, name string) (json.RawMessage, error) {
	res, ok := K8sResourceByKey(key)
	if !ok {
		return nil, fmt.Errorf("unknown resource: %s", key)
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/k8s/clusters/%s%s", clusterID, resourceItemPath(res, namespace, name))
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) GetK8sYAML(ctx context.Context, clusterOverride, key, namespace, name string) (string, error) {
	raw, err := c.GetK8sResource(ctx, clusterOverride, key, namespace, name)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *Client) GetPodLogs(ctx context.Context, clusterOverride, namespace, name, container string, tail int) (string, error) {
	if tail < 1 {
		tail = 500
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return "", err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/api/v1/namespaces/%s/pods/%s/log?tailLines=%d",
		clusterID, namespace, name, tail)
	if container != "" {
		path += "&container=" + container
	}
	body, err := c.getPlain(ctx, path)
	if err != nil {
		return "", err
	}
	// Rancher may return plain text
	if len(body) > 0 && body[0] != '{' {
		return string(body), nil
	}
	var wrapped struct {
		Data string `json:"data"`
	}
	if json.Unmarshal(body, &wrapped) == nil && wrapped.Data != "" {
		return wrapped.Data, nil
	}
	return string(body), nil
}

// ListNamespaceEvents — events trong namespace (core v1), có thể lọc theo pod.
func (c *Client) ListNamespaceEvents(ctx context.Context, clusterOverride, namespace, podName string, limit int) ([]ResourceRow, error) {
	if limit < 1 {
		limit = 30
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/api/v1/namespaces/%s/events?limit=%d",
		clusterID, namespace, limit)
	if podName != "" {
		path += "&fieldSelector=" + url.QueryEscape("involvedObject.name="+podName)
	}
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}
	rows, _, err := parseK8sItems(body)
	return rows, err
}

func (c *Client) DeleteK8sResource(ctx context.Context, clusterOverride, key, namespace, name string) error {
	res, ok := K8sResourceByKey(key)
	if !ok {
		return fmt.Errorf("unknown resource: %s", key)
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s%s", clusterID, resourceItemPath(res, namespace, name))
	_, _, err = c.do(ctx, "DELETE", path, nil, "")
	return err
}

func (c *Client) ScaleDeployment(ctx context.Context, clusterOverride, namespace, name string, replicas int) error {
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/apis/apps/v1/namespaces/%s/deployments/%s/scale",
		clusterID, namespace, name)
	payload := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, _, err = c.do(ctx, "PATCH", path, []byte(payload), "application/merge-patch+json")
	return err
}

func (c *Client) ListClustersWithDashboard(ctx context.Context) ([]ClusterRow, error) {
	return c.ListClusters(ctx)
}
