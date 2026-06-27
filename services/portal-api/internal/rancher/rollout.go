package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DeploymentRolloutStatus — nguồn sự thật rollout từ apps/v1 Deployment (qua Rancher proxy).
type DeploymentRolloutStatus struct {
	Name               string
	ReadyReplicas      int
	UpdatedReplicas    int
	AvailableReplicas  int
	Replicas           int
	Generation         int64
	ObservedGeneration int64
	Conditions         []DeploymentCondition
}

// DeploymentDetail — rollout + image từ spec (verify tag deploy).
type DeploymentDetail struct {
	DeploymentRolloutStatus
	ContainerImage   string
	PodImageTagLabel string
}

type DeploymentCondition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

func ParseDeploymentRollout(raw json.RawMessage) (DeploymentRolloutStatus, error) {
	var obj struct {
		Metadata struct {
			Name       string `json:"name"`
			Generation int64  `json:"generation"`
		} `json:"metadata"`
		Status struct {
			Replicas            int   `json:"replicas"`
			ReadyReplicas       int   `json:"readyReplicas"`
			UpdatedReplicas     int   `json:"updatedReplicas"`
			AvailableReplicas   int   `json:"availableReplicas"`
			ObservedGeneration  int64 `json:"observedGeneration"`
			Conditions          []DeploymentCondition `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return DeploymentRolloutStatus{}, err
	}
	return DeploymentRolloutStatus{
		Name:               obj.Metadata.Name,
		ReadyReplicas:      obj.Status.ReadyReplicas,
		UpdatedReplicas:    obj.Status.UpdatedReplicas,
		AvailableReplicas:  obj.Status.AvailableReplicas,
		Replicas:           obj.Status.Replicas,
		Generation:         obj.Metadata.Generation,
		ObservedGeneration: obj.Status.ObservedGeneration,
		Conditions:         obj.Status.Conditions,
	}, nil
}

func ParseDeploymentDetail(raw json.RawMessage) (DeploymentDetail, error) {
	var obj struct {
		Metadata struct {
			Name       string `json:"name"`
			Generation int64  `json:"generation"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Metadata struct {
					Labels map[string]string `json:"labels"`
				} `json:"metadata"`
				Spec struct {
					Containers []struct {
						Image string `json:"image"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
		Status struct {
			Replicas           int                   `json:"replicas"`
			ReadyReplicas      int                   `json:"readyReplicas"`
			UpdatedReplicas    int                   `json:"updatedReplicas"`
			AvailableReplicas  int                   `json:"availableReplicas"`
			ObservedGeneration int64                 `json:"observedGeneration"`
			Conditions         []DeploymentCondition `json:"conditions"`
		} `json:"status"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return DeploymentDetail{}, err
	}
	img := ""
	if len(obj.Spec.Template.Spec.Containers) > 0 {
		img = strings.TrimSpace(obj.Spec.Template.Spec.Containers[0].Image)
	}
	tagLabel := ""
	if obj.Spec.Template.Metadata.Labels != nil {
		tagLabel = strings.TrimSpace(obj.Spec.Template.Metadata.Labels["platform.7mlabs.com/image-tag"])
	}
	rollout := DeploymentRolloutStatus{
		Name:               obj.Metadata.Name,
		ReadyReplicas:      obj.Status.ReadyReplicas,
		UpdatedReplicas:    obj.Status.UpdatedReplicas,
		AvailableReplicas:  obj.Status.AvailableReplicas,
		Replicas:           obj.Status.Replicas,
		Generation:         obj.Metadata.Generation,
		ObservedGeneration: obj.Status.ObservedGeneration,
		Conditions:         obj.Status.Conditions,
	}
	return DeploymentDetail{
		DeploymentRolloutStatus: rollout,
		ContainerImage:            img,
		PodImageTagLabel:          tagLabel,
	}, nil
}

func (s DeploymentRolloutStatus) condition(typ string) (DeploymentCondition, bool) {
	for _, c := range s.Conditions {
		if c.Type == typ {
			return c, true
		}
	}
	return DeploymentCondition{}, false
}

func (s DeploymentRolloutStatus) IsReady() bool {
	if s.AvailableReplicas >= 1 && s.ReadyReplicas >= 1 {
		if c, ok := s.condition("Available"); ok {
			return strings.EqualFold(c.Status, "True")
		}
		return true
	}
	return false
}

func (s DeploymentRolloutStatus) FailureMessage() string {
	for _, c := range s.Conditions {
		if c.Type != "Progressing" || !strings.EqualFold(c.Status, "False") {
			continue
		}
		reason := strings.TrimSpace(c.Reason)
		switch reason {
		case "ProgressDeadlineExceeded", "InvalidConfig", "FailedCreate":
			msg := strings.TrimSpace(c.Message)
			if msg != "" {
				return c.Type + ": " + msg
			}
			if reason != "" {
				return c.Type + ": " + reason
			}
		}
	}
	// Available=False ("minimum availability") là trạng thái tạm khi rollout — không phải lỗi.
	return ""
}

func (s DeploymentRolloutStatus) IsRolloutInProgress() bool {
	if s.Generation > 0 && s.ObservedGeneration > 0 && s.ObservedGeneration < s.Generation {
		return true
	}
	want := maxInt(s.Replicas, 1)
	if s.UpdatedReplicas < want || s.ReadyReplicas < want || s.AvailableReplicas < want {
		if c, ok := s.condition("Progressing"); ok && strings.EqualFold(c.Status, "True") {
			return true
		}
	}
	return false
}

func (s DeploymentRolloutStatus) IsFailed() bool {
	return s.FailureMessage() != ""
}

func (s DeploymentRolloutStatus) Summary() string {
	parts := []string{
		fmt.Sprintf("%s: %d/%d ready", s.Name, s.ReadyReplicas, maxInt(s.Replicas, 1)),
	}
	if s.UpdatedReplicas > 0 {
		parts = append(parts, fmt.Sprintf("updated=%d", s.UpdatedReplicas))
	}
	if s.AvailableReplicas > 0 {
		parts = append(parts, fmt.Sprintf("available=%d", s.AvailableReplicas))
	}
	if s.Generation > 0 && s.ObservedGeneration > 0 && s.ObservedGeneration < s.Generation {
		parts = append(parts, "đang rollout")
	}
	if c, ok := s.condition("Progressing"); ok && c.Reason != "" {
		parts = append(parts, c.Reason)
	}
	return strings.Join(parts, " · ")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (c *Client) GetDeploymentRolloutStatus(ctx context.Context, clusterOverride, namespace, name string) (DeploymentRolloutStatus, error) {
	detail, err := c.GetDeploymentDetail(ctx, clusterOverride, namespace, name)
	if err != nil {
		return DeploymentRolloutStatus{}, err
	}
	return detail.DeploymentRolloutStatus, nil
}

func (c *Client) GetDeploymentDetail(ctx context.Context, clusterOverride, namespace, name string) (DeploymentDetail, error) {
	if !c.Enabled() {
		return DeploymentDetail{}, fmt.Errorf("rancher not configured")
	}
	raw, err := c.GetK8sResource(ctx, clusterOverride, "deployments", namespace, name)
	if err != nil {
		return DeploymentDetail{}, err
	}
	return ParseDeploymentDetail(raw)
}

// WaitDeploymentReady poll Deployment conditions qua Rancher — thay sleep loop thủ công.
func (c *Client) WaitDeploymentReady(ctx context.Context, clusterOverride, namespace, name string, timeout time.Duration) (DeploymentRolloutStatus, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	var last DeploymentRolloutStatus
	for {
		st, err := c.GetDeploymentRolloutStatus(ctx, clusterOverride, namespace, name)
		if err != nil {
			return last, err
		}
		last = st
		if st.IsFailed() {
			return st, fmt.Errorf("%s", st.FailureMessage())
		}
		if st.IsReady() {
			return st, nil
		}
		if time.Now().After(deadline) {
			return st, fmt.Errorf("timeout chờ deployment ready (%s)", st.Summary())
		}
		select {
		case <-ctx.Done():
			return st, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// ListPods list pod trong namespace với labelSelector (Rancher → K8s API).
func (c *Client) ListPods(ctx context.Context, clusterOverride, namespace, labelSelector string, limit int) ([]ResourceRow, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("rancher not configured")
	}
	if limit < 1 {
		limit = 50
	}
	clusterID, err := c.ClusterID(ctx, clusterOverride)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/k8s/clusters/%s/api/v1/namespaces/%s/pods?limit=%d", clusterID, namespace, limit)
	if strings.TrimSpace(labelSelector) != "" {
		path += "&labelSelector=" + url.QueryEscape(labelSelector)
	}
	body, status, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list pods: http %d", status)
	}
	rows, _, err := parseK8sItems(body)
	return rows, err
}
