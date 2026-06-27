package rancher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client

	mu              sync.Mutex
	cachedClusterID string
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	body, _, err := c.do(ctx, http.MethodGet, path, nil, "application/json")
	return body, err
}

func (c *Client) getPlain(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "text/plain, */*")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return body, fmt.Errorf("rancher api %d: %s", res.StatusCode, string(body))
	}
	return body, nil
}

func (c *Client) do(ctx context.Context, method, path string, payload []byte, contentType string) ([]byte, int, error) {
	var bodyReader io.Reader
	if len(payload) > 0 {
		bodyReader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return body, res.StatusCode, fmt.Errorf("rancher api %d: %s", res.StatusCode, string(body))
	}
	return body, res.StatusCode, nil
}

func (c *Client) SetClusterID(id string) {
	c.mu.Lock()
	c.cachedClusterID = id
	c.mu.Unlock()
}

func (c *Client) ClusterID(ctx context.Context, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	return c.clusterID(ctx)
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
