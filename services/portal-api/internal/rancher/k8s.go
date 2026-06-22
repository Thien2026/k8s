package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ResourceRow struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Created   string `json:"created,omitempty"`
	Status    string `json:"status,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	Object    string `json:"object,omitempty"`
}

type ResourceList struct {
	ClusterID string        `json:"cluster_id"`
	Resource  string        `json:"resource"`
	Label     string        `json:"label"`
	Total     int           `json:"total"`
	Items     []ResourceRow `json:"items"`
}

type clusterListV3 struct {
	Data []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
	} `json:"data"`
}

type projectListV3 struct {
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		State       string `json:"state"`
		Description string `json:"description"`
		ClusterID   string `json:"clusterId"`
	} `json:"data"`
}

type ClusterRow struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

type ProjectRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	Description string `json:"description"`
	ClusterID   string `json:"cluster_id"`
}

func (c *Client) clusterID(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.cachedClusterID != "" {
		id := c.cachedClusterID
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	body, err := c.get(ctx, "/v3/clusters")
	if err != nil {
		return "", err
	}
	var list clusterListV3
	if err := json.Unmarshal(body, &list); err != nil {
		return "", err
	}
	for _, cl := range list.Data {
		if cl.State == "active" || cl.State == "provisioned" || cl.ID == "local" {
			c.mu.Lock()
			c.cachedClusterID = cl.ID
			c.mu.Unlock()
			return cl.ID, nil
		}
	}
	if len(list.Data) > 0 {
		c.mu.Lock()
		c.cachedClusterID = list.Data[0].ID
		c.mu.Unlock()
		return list.Data[0].ID, nil
	}
	return "", fmt.Errorf("no rancher clusters found")
}

func (c *Client) ListK8s(ctx context.Context, key, namespace string) (ResourceList, error) {
	res, ok := K8sResourceByKey(key)
	if !ok {
		return ResourceList{}, fmt.Errorf("unknown resource: %s", key)
	}
	if !c.Enabled() {
		return ResourceList{}, fmt.Errorf("rancher not configured")
	}

	clusterID, err := c.clusterID(ctx)
	if err != nil {
		return ResourceList{}, err
	}

	path := res.APIPath
	if key == "events" {
		path += "?limit=500"
	}
	if res.Namespaced && namespace != "" {
		// .../apis/apps/v1/namespaces/{ns}/deployments
		if strings.HasPrefix(path, "/apis/") {
			parts := strings.SplitN(path, "/", 4) // "", "apis", group/version, resource
			if len(parts) >= 4 {
				path = fmt.Sprintf("/apis/%s/namespaces/%s/%s", parts[2], namespace, parts[3])
			}
		} else {
			path = fmt.Sprintf("/v1/namespaces/%s/%s", namespace, strings.TrimPrefix(path, "/v1/"))
		}
	}

	body, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s%s", clusterID, path))
	if err != nil {
		return ResourceList{}, err
	}

	items, err := parseK8sItems(body)
	if err != nil {
		return ResourceList{}, err
	}

	return ResourceList{
		ClusterID: clusterID,
		Resource:  key,
		Label:     res.Label,
		Total:     len(items),
		Items:     items,
	}, nil
}

func (c *Client) ListClusters(ctx context.Context) ([]ClusterRow, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("rancher not configured")
	}
	body, err := c.get(ctx, "/v3/clusters")
	if err != nil {
		return nil, err
	}
	var list clusterListV3
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	out := make([]ClusterRow, 0, len(list.Data))
	for _, cl := range list.Data {
		out = append(out, ClusterRow{ID: cl.ID, Name: cl.Name, State: cl.State})
	}
	return out, nil
}

func (c *Client) ListProjects(ctx context.Context) ([]ProjectRow, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.clusterID(ctx)
	if err != nil {
		return nil, err
	}
	body, err := c.get(ctx, "/v3/projects?clusterId="+clusterID)
	if err != nil {
		return nil, err
	}
	var list projectListV3
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	out := make([]ProjectRow, 0, len(list.Data))
	for _, p := range list.Data {
		out = append(out, ProjectRow{
			ID:          p.ID,
			Name:        p.Name,
			State:       p.State,
			Description: p.Description,
			ClusterID:   p.ClusterID,
		})
	}
	return out, nil
}

func parseK8sItems(body []byte) ([]ResourceRow, error) {
	var list struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}

	rows := make([]ResourceRow, 0, len(list.Items))
	for _, raw := range list.Items {
		if row, ok := parseK8sItem(raw); ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func parseK8sItem(raw json.RawMessage) (ResourceRow, bool) {
	var obj struct {
		Kind     string `json:"kind"`
		Type     string `json:"type"`
		Reason   string `json:"reason"`
		Note     string `json:"note"`
		Message  string `json:"message"`
		EventTime string `json:"eventTime"`
		ReportingController string `json:"reportingController"`
		Metadata struct {
			Name              string `json:"name"`
			Namespace         string `json:"namespace"`
			CreationTimestamp string `json:"creationTimestamp"`
		} `json:"metadata"`
		Regarding struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"regarding"`
		InvolvedObject struct {
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"involvedObject"`
		LastTimestamp string `json:"lastTimestamp"`
		Status        json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ResourceRow{}, false
	}

	created := obj.EventTime
	if created == "" {
		created = obj.LastTimestamp
	}
	if created == "" {
		created = obj.Metadata.CreationTimestamp
	}

	row := ResourceRow{
		Name:      obj.Metadata.Name,
		Namespace: obj.Metadata.Namespace,
		Created:   created,
		Kind:      obj.Kind,
		Status:    extractStatus(obj.Kind, obj.Status),
	}

	// events.k8s.io/v1 (K8s 1.35+) hoặc core/v1 Event
	if obj.Type != "" || obj.Reason != "" || obj.Note != "" || obj.Regarding.Name != "" {
		row.Status = obj.Type
		row.Reason = obj.Reason
		row.Message = obj.Note
		if row.Message == "" {
			row.Message = obj.Message
		}
		ref := obj.Regarding
		if ref.Name == "" {
			ref = obj.InvolvedObject
		}
		if ref.Name != "" {
			row.Object = ref.Kind + "/" + ref.Name
			if row.Namespace == "" {
				row.Namespace = ref.Namespace
			}
		}
		if row.Status == "" && obj.ReportingController != "" {
			row.Status = "Normal"
		}
		return row, true
	}

	return row, true
}

func extractStatus(kind string, status json.RawMessage) string {
	if len(status) == 0 {
		return ""
	}
	var generic map[string]any
	if err := json.Unmarshal(status, &generic); err != nil {
		return ""
	}
	switch kind {
	case "Pod":
		if phase, ok := generic["phase"].(string); ok {
			return phase
		}
	case "Node":
		if conds, ok := generic["conditions"].([]any); ok {
			for _, c := range conds {
				m, _ := c.(map[string]any)
				if m["type"] == "Ready" {
					if s, ok := m["status"].(string); ok {
						return "Ready=" + s
					}
				}
			}
		}
	case "Deployment", "StatefulSet", "DaemonSet":
		if r, ok := generic["readyReplicas"].(float64); ok {
			d := generic["replicas"]
			return fmt.Sprintf("%v/%v ready", int(r), d)
		}
	case "Job":
		if s, ok := generic["succeeded"].(float64); ok {
			return fmt.Sprintf("succeeded=%d", int(s))
		}
	case "CronJob":
		if a, ok := generic["active"].([]any); ok {
			return fmt.Sprintf("active=%d", len(a))
		}
	case "PersistentVolume", "PersistentVolumeClaim":
		if phase, ok := generic["phase"].(string); ok {
			return phase
		}
	}
	if phase, ok := generic["phase"].(string); ok {
		return phase
	}
	if load, ok := generic["loadBalancer"].(map[string]any); ok && load != nil {
		return "LB"
	}
	return ""
}
