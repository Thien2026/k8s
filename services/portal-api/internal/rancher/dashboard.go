package rancher

import (
	"context"
	"encoding/json"
	"fmt"
)

type ClusterDashboard struct {
	ClusterID    string         `json:"cluster_id"`
	Name         string         `json:"name"`
	Provider     string         `json:"provider,omitempty"`
	K8sVersion   string         `json:"k8s_version,omitempty"`
	State        string         `json:"state"`
	Created      string         `json:"created,omitempty"`
	Counts       map[string]int `json:"counts"`
	Capacity     NodeCapacity   `json:"capacity"`
	Components   []Component    `json:"components"`
	RecentEvents []ResourceRow  `json:"recent_events"`
}

type Component struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type clusterDetailV3 struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Provider string `json:"provider"`
	Created  string `json:"created"`
	Version  *struct {
		GitVersion string `json:"gitVersion"`
	} `json:"version"`
}

func (c *Client) ClusterDashboard(ctx context.Context) (ClusterDashboard, error) {
	if !c.Enabled() {
		return ClusterDashboard{}, errNotConfigured()
	}
	clusterID, err := c.clusterID(ctx)
	if err != nil {
		return ClusterDashboard{}, err
	}

	dash := ClusterDashboard{
		ClusterID: clusterID,
		Counts:    map[string]int{},
		Components: []Component{
			{Name: "Etcd", Status: "ok"},
			{Name: "Scheduler", Status: "ok"},
			{Name: "Controller Manager", Status: "ok"},
			{Name: "Fleet", Status: "ok"},
		},
	}

	if body, err := c.get(ctx, "/v3/clusters/"+clusterID); err == nil {
		var cl clusterDetailV3
		if json.Unmarshal(body, &cl) == nil {
			dash.Name = cl.Name
			dash.State = cl.State
			dash.Provider = cl.Provider
			dash.Created = cl.Created
			if cl.Version != nil {
				dash.K8sVersion = cl.Version.GitVersion
			}
		}
	}
	if dash.Name == "" {
		dash.Name = clusterID
	}

	for _, key := range []string{"nodes", "deployments", "pods", "namespaces", "services", "ingresses"} {
		if list, err := c.ListK8s(ctx, key, "", 1, 1); err == nil {
			dash.Counts[key] = list.Total
		}
	}

	dash.Capacity = c.buildCapacity(ctx, clusterID)

	if events, err := c.ListK8s(ctx, "events", "", 1, 8); err == nil {
		dash.RecentEvents = events.Items
	}

	total := 0
	for _, v := range dash.Counts {
		total += v
	}
	dash.Counts["resources"] = total

	return dash, nil
}

func errNotConfigured() error {
	return fmt.Errorf("rancher not configured")
}
