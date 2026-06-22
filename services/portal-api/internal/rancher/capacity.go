package rancher

import (
	"context"
	"encoding/json"
	"fmt"
)

type CapacityMetric struct {
	Used        float64 `json:"used"`
	Total       float64 `json:"total"`
	UsedPct     float64 `json:"used_pct"`
	Reserved    float64 `json:"reserved,omitempty"`
	ReservedPct float64 `json:"reserved_pct,omitempty"`
	Unit        string  `json:"unit,omitempty"`
}

type NodeCapacity struct {
	Pods   CapacityMetric `json:"pods"`
	CPU    CapacityMetric `json:"cpu"`
	Memory CapacityMetric `json:"memory"`
}

func (c *Client) buildCapacity(ctx context.Context, clusterID string) NodeCapacity {
	cap := NodeCapacity{
		Pods:   CapacityMetric{Unit: "pods"},
		CPU:    CapacityMetric{Unit: "cores"},
		Memory: CapacityMetric{Unit: "GiB"},
	}

	nodesBody, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s/v1/nodes", clusterID))
	if err == nil {
		var nodes struct {
			Items []json.RawMessage `json:"items"`
			Data  []json.RawMessage `json:"data"`
		}
		if json.Unmarshal(nodesBody, &nodes) == nil {
			raw := nodes.Items
			if len(raw) == 0 {
				raw = nodes.Data
			}
			for _, n := range raw {
				var node struct {
					Status struct {
						Allocatable map[string]string `json:"allocatable"`
						Capacity    map[string]string `json:"capacity"`
					} `json:"status"`
				}
				if json.Unmarshal(n, &node) != nil {
					continue
				}
				src := node.Status.Allocatable
				if len(src) == 0 {
					src = node.Status.Capacity
				}
				cap.Pods.Total += float64(parseQuantityInt(src["pods"]))
				cap.CPU.Total += parseCPU(src["cpu"])
				cap.Memory.Total += parseMemGiB(src["memory"])
			}
		}
	}

	podsBody, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s/v1/pods", clusterID))
	if err == nil {
		var pods struct {
			Items []json.RawMessage `json:"items"`
			Data  []json.RawMessage `json:"data"`
		}
		if json.Unmarshal(podsBody, &pods) == nil {
			raw := pods.Items
			if len(raw) == 0 {
				raw = pods.Data
			}
			for _, p := range raw {
				var pod struct {
					Status struct {
						Phase string `json:"phase"`
					} `json:"status"`
					Spec struct {
						Containers []struct {
							Resources struct {
								Requests map[string]string `json:"requests"`
							} `json:"resources"`
						} `json:"containers"`
					} `json:"spec"`
				}
				if json.Unmarshal(p, &pod) != nil {
					continue
				}
				if pod.Status.Phase == "Succeeded" || pod.Status.Phase == "Failed" {
					continue
				}
				cap.Pods.Used++
				for _, ctr := range pod.Spec.Containers {
					req := ctr.Resources.Requests
					cap.CPU.Reserved += parseCPU(req["cpu"])
					cap.Memory.Reserved += parseMemGiB(req["memory"])
				}
			}
		}
	}

	metricsBody, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s/apis/metrics.k8s.io/v1beta1/nodes", clusterID))
	if err == nil {
		var metrics struct {
			Items []struct {
				Usage struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"usage"`
			} `json:"items"`
		}
		if json.Unmarshal(metricsBody, &metrics) == nil {
			for _, m := range metrics.Items {
				cap.CPU.Used += parseCPU(m.Usage.CPU)
				cap.Memory.Used += parseMemGiB(m.Usage.Memory)
			}
		}
	}

	cap.Pods.UsedPct = pct(cap.Pods.Used, cap.Pods.Total)
	cap.CPU.UsedPct = pct(cap.CPU.Used, cap.CPU.Total)
	cap.CPU.ReservedPct = pct(cap.CPU.Reserved, cap.CPU.Total)
	cap.Memory.UsedPct = pct(cap.Memory.Used, cap.Memory.Total)
	cap.Memory.ReservedPct = pct(cap.Memory.Reserved, cap.Memory.Total)

	cap.Pods.Total = float64(int(cap.Pods.Total))
	cap.Pods.Used = float64(int(cap.Pods.Used))
	cap.CPU.Total = round1(cap.CPU.Total)
	cap.CPU.Used = round2(cap.CPU.Used)
	cap.CPU.Reserved = round2(cap.CPU.Reserved)
	cap.Memory.Total = round1(cap.Memory.Total)
	cap.Memory.Used = round2(cap.Memory.Used)
	cap.Memory.Reserved = round2(cap.Memory.Reserved)

	return cap
}

func pct(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return round1(used / total * 100)
}

func parseQuantityInt(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
