package rancher

import "context"

func (c *Client) buildScalingSummary(ctx context.Context, clusterOverride string) ScalingSummary {
	sum := ScalingSummary{}
	if hpa, err := c.ListK8s(ctx, clusterOverride, "horizontalpodautoscalers", "", 1, 200); err == nil {
		sum.HPACount = hpa.Total
	}
	if pods, err := c.ListK8s(ctx, clusterOverride, "pods", "", 1, 500); err == nil {
		for _, p := range pods.Items {
			if p.Restarts > 0 {
				sum.PodsWithRestart++
				sum.TotalRestarts += p.Restarts
			}
		}
	}
	return sum
}
