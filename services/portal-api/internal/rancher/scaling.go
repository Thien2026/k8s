package rancher

import "context"

func (c *Client) buildScalingSummary(ctx context.Context) ScalingSummary {
	sum := ScalingSummary{}
	if hpa, err := c.ListK8s(ctx, "horizontalpodautoscalers", "", 1, 200); err == nil {
		sum.HPACount = hpa.Total
	}
	if pods, err := c.ListK8s(ctx, "pods", "", 1, 500); err == nil {
		for _, p := range pods.Items {
			if p.Restarts > 0 {
				sum.PodsWithRestart++
				sum.TotalRestarts += p.Restarts
			}
		}
	}
	return sum
}
