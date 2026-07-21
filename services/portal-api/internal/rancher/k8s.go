package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ResourceRow struct {
	Name                  string  `json:"name"`
	Namespace             string  `json:"namespace,omitempty"`
	Created               string  `json:"created,omitempty"`
	Status                string  `json:"status,omitempty"`
	Kind                  string  `json:"kind,omitempty"`
	Reason                string  `json:"reason,omitempty"`
	Message               string  `json:"message,omitempty"`
	Object                string  `json:"object,omitempty"`
	Restarts              int     `json:"restarts,omitempty"`
	Replicas              string  `json:"replicas,omitempty"`
	Scale                 string  `json:"scale,omitempty"`
	RestartPolicy         string  `json:"restart_policy,omitempty"`
	PodsMax               int     `json:"pods_max,omitempty"`
	PodsUsed              int     `json:"pods_used,omitempty"`
	CPUCores              float64 `json:"cpu_cores,omitempty"`
	MemGiB                float64 `json:"mem_gib,omitempty"`
	NodeIP                string  `json:"node_ip,omitempty"`
	PodIP                 string  `json:"pod_ip,omitempty"`
	HostIP                string  `json:"host_ip,omitempty"`
	Node                  string  `json:"node,omitempty"`
	Images                string  `json:"images,omitempty"`
	ServiceType           string  `json:"service_type,omitempty"`
	ClusterIP             string  `json:"cluster_ip,omitempty"`
	Ports                 string  `json:"ports,omitempty"`
	Host                  string  `json:"host,omitempty"`
	StorageClass          string  `json:"storage_class,omitempty"`
	Capacity              string  `json:"capacity,omitempty"`
	AccessModes           string  `json:"access_modes,omitempty"`
	Schedule              string  `json:"schedule,omitempty"`
	Suspend               string  `json:"suspend,omitempty"`
	Completions           string  `json:"completions,omitempty"`
	JobSucceeded          int     `json:"job_succeeded,omitempty"`
	JobFailed             int     `json:"job_failed,omitempty"`
	JobActive             int     `json:"job_active,omitempty"`
	Selector              string  `json:"selector,omitempty"`
	Project               string  `json:"project,omitempty"`
	Ready                 bool    `json:"ready,omitempty"`
	LastTerminationReason string  `json:"last_termination_reason,omitempty"`
	OwnerDeployment       string  `json:"owner_deployment,omitempty"`
}

type ResourceList struct {
	ClusterID string        `json:"cluster_id"`
	Resource  string        `json:"resource"`
	Label     string        `json:"label"`
	Total     int           `json:"total"`
	Page      int           `json:"page"`
	Limit     int           `json:"limit"`
	Items     []ResourceRow `json:"items"`
}

type clusterListV3 struct {
	Data []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		State    string `json:"state"`
		Provider string `json:"provider"`
		Driver   string `json:"driver"`
		Created  string `json:"created"`
		Version  *struct {
			GitVersion string `json:"gitVersion"`
		} `json:"version"`
		Nodes *struct {
			NodeCount int `json:"nodeCount"`
		} `json:"nodes"`
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Provider   string `json:"provider,omitempty"`
	Driver     string `json:"driver,omitempty"`
	K8sVersion string `json:"k8s_version,omitempty"`
	Nodes      int    `json:"nodes,omitempty"`
	Created    string `json:"created,omitempty"`
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

func namespacedAPIPath(basePath, namespace string) string {
	if strings.HasPrefix(basePath, "/apis/") {
		i := strings.LastIndex(basePath, "/")
		if i > 0 {
			return basePath[:i] + "/namespaces/" + namespace + basePath[i:]
		}
	}
	if strings.HasPrefix(basePath, "/api/v1/") {
		rest := strings.TrimPrefix(basePath, "/api/v1/")
		return "/api/v1/namespaces/" + namespace + "/" + rest
	}
	rest := strings.TrimPrefix(basePath, "/v1/")
	return "/v1/namespaces/" + namespace + "/" + rest
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
		row := ClusterRow{
			ID:       cl.ID,
			Name:     cl.Name,
			State:    cl.State,
			Provider: cl.Provider,
			Driver:   cl.Driver,
			Created:  cl.Created,
		}
		if cl.Version != nil {
			row.K8sVersion = cl.Version.GitVersion
		}
		if cl.Nodes != nil {
			row.Nodes = cl.Nodes.NodeCount
		}
		out = append(out, row)
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
