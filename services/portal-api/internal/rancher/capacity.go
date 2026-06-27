package rancher

import (
	"context"
	"encoding/json"
	"fmt"
)

type CapacityMetric struct {
	Used          float64 `json:"used"`
	Total         float64 `json:"total"`
	UsedPct       float64 `json:"used_pct"`
	Reserved      float64 `json:"reserved"`
	ReservedTotal float64 `json:"reserved_total"`
	ReservedPct   float64 `json:"reserved_pct"`
	Unit          string  `json:"unit,omitempty"`
}

type NodeCapacity struct {
	Pods   CapacityMetric `json:"pods"`
	CPU    CapacityMetric `json:"cpu"`
	Memory CapacityMetric `json:"memory"`
	Disk   CapacityMetric `json:"disk"`
}

func (c *Client) buildCapacity(ctx context.Context, clusterID string) NodeCapacity {
	cap := NodeCapacity{
		Pods:   CapacityMetric{Unit: "pods"},
		CPU:    CapacityMetric{Unit: "cores"},
		Memory: CapacityMetric{Unit: "GiB"},
		Disk:   CapacityMetric{Unit: "GiB"},
	}

	var nodeNames []string

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
					Metadata struct {
						Name string `json:"name"`
					} `json:"metadata"`
					Status struct {
						Allocatable map[string]string `json:"allocatable"`
						Capacity    map[string]string `json:"capacity"`
					} `json:"status"`
				}
				if json.Unmarshal(n, &node) != nil {
					continue
				}
				if node.Metadata.Name != "" {
					nodeNames = append(nodeNames, node.Metadata.Name)
				}
				src := node.Status.Allocatable
				if len(src) == 0 {
					src = node.Status.Capacity
				}
				cap.Pods.Total += float64(parseQuantityInt(src["pods"]))
				cap.CPU.Total += parseCPU(src["cpu"])
				cap.Memory.Total += parseMemGiB(src["memory"])
				cap.Disk.ReservedTotal += parseStorageGiB(src["ephemeral-storage"])
			}
		}
	}

	for _, nodeName := range nodeNames {
		summaryBody, err := c.get(ctx, fmt.Sprintf(
			"/k8s/clusters/%s/api/v1/nodes/%s/proxy/stats/summary",
			clusterID, nodeName,
		))
		if err != nil {
			continue
		}
		var summary struct {
			Node struct {
				Fs struct {
					UsedBytes     uint64 `json:"usedBytes"`
					CapacityBytes uint64 `json:"capacityBytes"`
				} `json:"fs"`
			} `json:"node"`
		}
		if json.Unmarshal(summaryBody, &summary) != nil {
			continue
		}
		if summary.Node.Fs.CapacityBytes > 0 {
			cap.Disk.Used += bytesToGiB(float64(summary.Node.Fs.UsedBytes))
			// Ưu tiên số thật từ kubelet hơn ephemeral-storage allocatable
			cap.Disk.Total += bytesToGiB(float64(summary.Node.Fs.CapacityBytes))
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
					cap.Disk.Reserved += parseStorageGiB(req["ephemeral-storage"])
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
	cap.Disk.UsedPct = pct(cap.Disk.Used, cap.Disk.Total)
	cap.Disk.ReservedPct = pct(cap.Disk.Reserved, cap.Disk.ReservedTotal)

	cap.Pods.Total = float64(int(cap.Pods.Total))
	cap.Pods.Used = float64(int(cap.Pods.Used))
	cap.CPU.Total = round1(cap.CPU.Total)
	cap.CPU.Used = round2(cap.CPU.Used)
	cap.CPU.Reserved = round2(cap.CPU.Reserved)
	cap.Memory.Total = round1(cap.Memory.Total)
	cap.Memory.Used = round2(cap.Memory.Used)
	cap.Memory.Reserved = round2(cap.Memory.Reserved)
	cap.Disk.Total = round1(cap.Disk.Total)
	cap.Disk.Used = round1(cap.Disk.Used)
	cap.Disk.Reserved = round1(cap.Disk.Reserved)
	cap.Disk.ReservedTotal = round1(cap.Disk.ReservedTotal)

	return cap
}

func bytesToGiB(b float64) float64 {
	return b / (1024 * 1024 * 1024)
}

func parseStorageGiB(s string) float64 {
	if s == "" {
		return 0
	}
	var v float64
	if len(s) > 2 && s[len(s)-2:] == "Ki" {
		fmt.Sscanf(s, "%fKi", &v)
		return v / (1024 * 1024)
	}
	if len(s) > 2 && s[len(s)-2:] == "Mi" {
		fmt.Sscanf(s, "%fMi", &v)
		return v / 1024
	}
	if len(s) > 2 && s[len(s)-2:] == "Gi" {
		fmt.Sscanf(s, "%fGi", &v)
		return v
	}
	if len(s) > 1 && s[len(s)-1:] == "G" {
		fmt.Sscanf(s, "%fG", &v)
		return v * 1000 / 1024
	}
	// Số không suffix = bytes (Kubernetes resource.Quantity)
	fmt.Sscanf(s, "%f", &v)
	return v / (1024 * 1024 * 1024)
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
