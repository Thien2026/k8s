package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func (c *Client) ListK8s(ctx context.Context, clusterOverride, key, namespace string, page, limit int) (ResourceList, error) {
	res, ok := K8sResourceByKey(key)
	if !ok {
		return ResourceList{}, fmt.Errorf("unknown resource: %s", key)
	}
	if !c.Enabled() {
		return ResourceList{}, fmt.Errorf("rancher not configured")
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}

	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return ResourceList{}, err
	}

	path := res.APIPath
	if key == "events" {
		path += "?limit=500"
	}
	if res.Namespaced && namespace != "" {
		path = namespacedAPIPath(path, namespace)
	}

	body, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s%s", clusterID, path))
	if err != nil {
		return ResourceList{}, err
	}

	all, total, err := parseK8sItems(body, K8sListItemKind(key))
	if err != nil {
		return ResourceList{}, err
	}
	if total == 0 {
		total = len(all)
	}

	start := (page - 1) * limit
	if start > len(all) {
		start = len(all)
	}
	end := start + limit
	if end > len(all) {
		end = len(all)
	}

	return ResourceList{
		ClusterID: clusterID,
		Resource:  key,
		Label:     res.Label,
		Total:     total,
		Page:      page,
		Limit:     limit,
		Items:     all[start:end],
	}, nil
}

// CountK8s — đếm resource trong namespace (limit=1 + remainingItemCount, không parse full list).
func (c *Client) CountK8s(ctx context.Context, clusterOverride, key, namespace string) (int, error) {
	res, ok := K8sResourceByKey(key)
	if !ok {
		return 0, fmt.Errorf("unknown resource: %s", key)
	}
	if !c.Enabled() {
		return 0, fmt.Errorf("rancher not configured")
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return 0, err
	}
	path := res.APIPath
	if res.Namespaced && namespace != "" {
		path = namespacedAPIPath(path, namespace)
	}
	if !strings.Contains(path, "?") {
		path += "?limit=1"
	} else {
		path += "&limit=1"
	}
	body, err := c.get(ctx, fmt.Sprintf("/k8s/clusters/%s%s", clusterID, path))
	if err != nil {
		return 0, err
	}
	var envelope struct {
		Metadata struct {
			RemainingItemCount *int64 `json:"remainingItemCount"`
		} `json:"metadata"`
		Items []json.RawMessage `json:"items"`
		Data  []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return 0, err
	}
	rawItems := envelope.Items
	if len(rawItems) == 0 {
		rawItems = envelope.Data
	}
	n := len(rawItems)
	if envelope.Metadata.RemainingItemCount != nil {
		n += int(*envelope.Metadata.RemainingItemCount)
	}
	return n, nil
}
