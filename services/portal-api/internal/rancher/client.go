package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.baseURL != "" && c.token != ""
}

type ClusterSummary struct {
	Total     int `json:"total"`
	Ready     int `json:"ready"`
	NotReady  int `json:"not_ready"`
	Nodes     int `json:"nodes,omitempty"`
	Connected bool `json:"connected"`
}

type clusterList struct {
	Data []struct {
		State   string `json:"state"`
		Nodes   *struct{ NodeCount int `json:"nodeCount"` } `json:"nodes"`
		Driver  string `json:"driver"`
	} `json:"data"`
}

func (c *Client) ClusterSummary(ctx context.Context) (ClusterSummary, error) {
	if !c.Enabled() {
		return ClusterSummary{Connected: false}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v3/clusters", nil)
	if err != nil {
		return ClusterSummary{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return ClusterSummary{Connected: false}, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return ClusterSummary{Connected: false}, fmt.Errorf("rancher api %d: %s", res.StatusCode, string(body))
	}

	var list clusterList
	if err := json.Unmarshal(body, &list); err != nil {
		return ClusterSummary{}, err
	}

	sum := ClusterSummary{Total: len(list.Data), Connected: true}
	for _, cl := range list.Data {
		switch cl.State {
		case "active", "provisioned":
			sum.Ready++
		default:
			sum.NotReady++
		}
		if cl.Nodes != nil {
			sum.Nodes += cl.Nodes.NodeCount
		}
	}
	return sum, nil
}
