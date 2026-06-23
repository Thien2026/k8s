package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type healthCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (c *Client) fetchComponents(ctx context.Context, clusterID string) []Component {
	var out []Component

	body, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s/v1/componentstatuses", clusterID))
	if err == nil {
		var list struct {
			Data  []json.RawMessage `json:"data"`
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(body, &list) == nil {
			raw := list.Data
			if len(raw) == 0 {
				raw = list.Items
			}
			for _, r := range raw {
				var cs struct {
					ID         string            `json:"id"`
					Conditions []healthCondition `json:"conditions"`
					Metadata   struct {
						Name string `json:"name"`
					} `json:"metadata"`
				}
				if json.Unmarshal(r, &cs) != nil {
					continue
				}
				id := cs.ID
				if id == "" {
					id = cs.Metadata.Name
				}
				out = append(out, mapComponentStatus(id, cs.Conditions))
			}
		}
	}

	fleet := Component{Name: "Fleet", Status: "unknown", Message: "not checked"}
	if deps, err := c.ListK8s(ctx, "deployments", "cattle-fleet-system", 1, 50); err == nil {
		for _, d := range deps.Items {
			if strings.Contains(d.Name, "fleet-controller") {
				fleet.Status = "ok"
				fleet.Message = d.Status
				if d.Status != "" && !strings.Contains(strings.ToLower(d.Status), "ready") {
					fleet.Status = "degraded"
				}
				if fleet.Message == "" {
					fleet.Message = "running"
				}
				break
			}
		}
	}
	if fleet.Message == "not checked" {
		fleet.Status = "degraded"
		fleet.Message = "fleet-controller not found"
	}
	out = append(out, fleet)

	if len(out) == 0 {
		out = []Component{
			{Name: "Etcd", Status: "unknown", Message: "API unavailable"},
		}
	}
	return out
}

func mapComponentStatus(id string, conds []healthCondition) Component {
	name := componentDisplayName(id)
	comp := Component{Name: name, Status: "unknown", Message: ""}
	for _, cond := range conds {
		if cond.Type == "Healthy" {
			if strings.EqualFold(cond.Status, "True") {
				comp.Status = "ok"
			} else {
				comp.Status = "degraded"
			}
			comp.Message = cond.Message
			if comp.Message == "" {
				comp.Message = "healthy"
			}
			return comp
		}
	}
	comp.Message = "no health data"
	return comp
}

func componentDisplayName(id string) string {
	switch id {
	case "etcd-0":
		return "Etcd"
	case "controller-manager":
		return "Controller Manager"
	case "scheduler":
		return "Scheduler"
	default:
		return strings.ReplaceAll(id, "-", " ")
	}
}
