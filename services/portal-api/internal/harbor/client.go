package harbor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.baseURL != "" && c.password != ""
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func (c *Client) EnsureProject(ctx context.Context, name string) error {
	if !c.Enabled() {
		return fmt.Errorf("harbor chưa cấu hình")
	}
	q := url.Values{"project_name": {name}}
	checkURL := c.baseURL + "/api/v2.0/projects?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusOK && strings.Contains(string(body), `"name":"`+name+`"`) {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"project_name": name,
		"metadata":     map[string]string{"public": "false", "auto_scan": "true"},
	})
	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2.0/projects", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	postReq.SetBasicAuth(c.username, c.password)
	postReq.Header.Set("Content-Type", "application/json")
	postRes, err := c.http.Do(postReq)
	if err != nil {
		return err
	}
	defer postRes.Body.Close()
	postBody, _ := io.ReadAll(postRes.Body)
	if postRes.StatusCode >= 400 {
		return fmt.Errorf("harbor create project %d: %s", postRes.StatusCode, string(postBody))
	}
	return nil
}

type robotPermission struct {
	Kind      string       `json:"kind"`
	Namespace string       `json:"namespace"`
	Access    []robotAccess `json:"access"`
}

type robotAccess struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type robotCreateRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Duration    int               `json:"duration"`
	Level       string            `json:"level"`
	Disable     bool              `json:"disable"`
	Permissions []robotPermission `json:"permissions"`
}

type robotResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

type robotListResponse struct {
	Items []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
}

func parseRobotList(body []byte) []struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
} {
	var direct []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if json.Unmarshal(body, &direct) == nil && len(direct) > 0 {
		return direct
	}
	var wrapped robotListResponse
	if json.Unmarshal(body, &wrapped) == nil {
		return wrapped.Items
	}
	return nil
}

const ciRobotShortName = "platform-ci"

// EnsureCIRobot tạo hoặc làm mới robot Harbor cho CI push image vào project.
// Trả về username đầy đủ (robot$project+platform-ci) và secret dùng docker login.
func (c *Client) EnsureCIRobot(ctx context.Context, projectName string) (username, secret string, err error) {
	if !c.Enabled() {
		return "", "", fmt.Errorf("harbor chưa cấu hình")
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return "", "", fmt.Errorf("thiếu tên harbor project")
	}

	if id, fullName, ok := c.findCIRobot(ctx, projectName); ok {
		secret, err = c.refreshRobotSecret(ctx, id)
		if err != nil {
			return "", "", err
		}
		return fullName, secret, nil
	}

	payload, _ := json.Marshal(robotCreateRequest{
		Name:        ciRobotShortName,
		Description: "Platform managed CI (auto-provisioned)",
		Duration:    -1,
		Level:       "project",
		Disable:     false,
		Permissions: []robotPermission{{
			Kind:      "project",
			Namespace: projectName,
			Access: []robotAccess{
				{Resource: "repository", Action: "pull"},
				{Resource: "repository", Action: "push"},
			},
		}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v2.0/robots", strings.NewReader(string(payload)))
	if err != nil {
		return "", "", err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", "", fmt.Errorf("harbor create robot %d: %s", res.StatusCode, string(body))
	}
	var created robotResponse
	if err := json.Unmarshal(body, &created); err != nil {
		return "", "", err
	}
	if strings.TrimSpace(created.Name) == "" || strings.TrimSpace(created.Secret) == "" {
		return "", "", fmt.Errorf("harbor robot response thiếu name/secret")
	}
	return created.Name, created.Secret, nil
}

func (c *Client) findCIRobot(ctx context.Context, projectName string) (id int64, fullName string, ok bool) {
	q := url.Values{
		"page":      {"1"},
		"page_size": {"50"},
		"q":         {"name," + ciRobotShortName},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2.0/robots?"+q.Encode(), nil)
	if err != nil {
		return 0, "", false
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil || res.StatusCode != http.StatusOK {
		if res != nil {
			res.Body.Close()
		}
		return 0, "", false
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	items := parseRobotList(body)
	suffix := "+" + ciRobotShortName
	prefix := "robot$" + projectName + "+"
	for _, item := range items {
		if strings.HasPrefix(item.Name, prefix) && strings.HasSuffix(item.Name, suffix) {
			return item.ID, item.Name, true
		}
	}
	return 0, "", false
}

func (c *Client) refreshRobotSecret(ctx context.Context, robotID int64) (string, error) {
	payload, _ := json.Marshal(map[string]string{"secret": ""})
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		fmt.Sprintf("%s/api/v2.0/robots/%d", c.baseURL, robotID), strings.NewReader(string(payload)))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("harbor refresh robot %d: %s", res.StatusCode, string(body))
	}
	var updated robotResponse
	if err := json.Unmarshal(body, &updated); err != nil {
		return "", err
	}
	if strings.TrimSpace(updated.Secret) == "" {
		return "", fmt.Errorf("harbor refresh robot không trả secret")
	}
	return updated.Secret, nil
}

// ScanOverview kết quả quét CVE Trivy (Harbor).
type ScanOverview struct {
	Status   string
	Total    int
	Fixable  int
	Severity map[string]int
	Detail   string
}

// EnableAutoScan bật quét tự động khi push image (Harbor 2.x: PUT project metadata).
func (c *Client) EnableAutoScan(ctx context.Context, projectName string) error {
	if !c.Enabled() {
		return fmt.Errorf("harbor chưa cấu hình")
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return fmt.Errorf("thiếu tên harbor project")
	}
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v2.0/projects/"+url.PathEscape(projectName), nil)
	if err != nil {
		return err
	}
	getReq.SetBasicAuth(c.username, c.password)
	getRes, err := c.http.Do(getReq)
	if err != nil {
		return err
	}
	getBody, _ := io.ReadAll(getRes.Body)
	getRes.Body.Close()
	if getRes.StatusCode >= 400 {
		return fmt.Errorf("harbor get project %d: %s", getRes.StatusCode, string(getBody))
	}
	var proj struct {
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(getBody, &proj); err != nil {
		return err
	}
	if proj.Metadata != nil && strings.EqualFold(proj.Metadata["auto_scan"], "true") {
		return nil
	}
	meta := map[string]string{"public": "false", "auto_scan": "true"}
	if proj.Metadata != nil {
		for k, v := range proj.Metadata {
			if k != "auto_scan" {
				meta[k] = v
			}
		}
	}
	payload, _ := json.Marshal(map[string]any{"metadata": meta})
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/api/v2.0/projects/"+url.PathEscape(projectName),
		strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	putReq.SetBasicAuth(c.username, c.password)
	putReq.Header.Set("Content-Type", "application/json")
	putRes, err := c.http.Do(putReq)
	if err != nil {
		return err
	}
	defer putRes.Body.Close()
	if putRes.StatusCode >= 400 {
		body, _ := io.ReadAll(putRes.Body)
		return fmt.Errorf("harbor auto_scan %d: %s", putRes.StatusCode, string(body))
	}
	return nil
}

// ArtifactExists kiểm tra artifact tag/sha có tồn tại trong Harbor.
func (c *Client) ArtifactExists(ctx context.Context, projectName, repoName, reference string) (bool, error) {
	if !c.Enabled() {
		return false, fmt.Errorf("harbor chưa cấu hình")
	}
	projectName = strings.TrimSpace(projectName)
	repoName = strings.Trim(strings.TrimSpace(repoName), "/")
	reference = strings.TrimSpace(reference)
	if projectName == "" || repoName == "" || reference == "" {
		return false, nil
	}
	apiURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s",
		c.baseURL, url.PathEscape(projectName), url.PathEscape(repoName), url.PathEscape(reference))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode >= 400 {
		body, _ := io.ReadAll(res.Body)
		return false, fmt.Errorf("harbor artifact %d: %s", res.StatusCode, string(body))
	}
	return true, nil
}

// ArtifactScanOverview lấy tổng quan CVE từ Harbor Trivy cho image tag/sha.
func (c *Client) ArtifactScanOverview(ctx context.Context, projectName, repoName, reference string) (*ScanOverview, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("harbor chưa cấu hình")
	}
	projectName = strings.TrimSpace(projectName)
	repoName = strings.Trim(strings.TrimSpace(repoName), "/")
	if repoName == "" {
		repoName = "app"
	}
	reference = strings.TrimSpace(reference)
	if projectName == "" || reference == "" {
		return nil, fmt.Errorf("thiếu project hoặc image tag")
	}
	apiURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s?with_scan_overview=true",
		c.baseURL, url.PathEscape(projectName), url.PathEscape(repoName), url.PathEscape(reference))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return &ScanOverview{Status: "pending", Detail: "Chưa có artifact hoặc đang chờ quét"}, nil
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("harbor scan %d: %s", res.StatusCode, string(body))
	}
	var raw struct {
		ScanOverview map[string]json.RawMessage `json:"scan_overview"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := &ScanOverview{Status: "pending", Severity: map[string]int{}}
	for _, ovRaw := range raw.ScanOverview {
		var ov struct {
			ScanStatus      string `json:"scan_status"`
			CompletePercent int    `json:"complete_percent"`
			Summary         *struct {
				Total   int            `json:"total"`
				Fixable int            `json:"fixable"`
				Summary map[string]int `json:"summary"`
			} `json:"summary"`
		}
		if err := json.Unmarshal(ovRaw, &ov); err != nil {
			continue
		}
		st := strings.ToLower(strings.TrimSpace(ov.ScanStatus))
		switch st {
		case "success", "completed":
			out.Status = "success"
		case "running", "pending", "queued":
			out.Status = "running"
		case "error", "failed":
			out.Status = "failed"
			out.Detail = "Quét CVE lỗi — xem Harbor UI"
		default:
			if st != "" {
				out.Status = st
			}
		}
		if ov.Summary != nil {
			out.Total = ov.Summary.Total
			out.Fixable = ov.Summary.Fixable
			for sev, n := range ov.Summary.Summary {
				out.Severity[sev] += n
			}
		}
		break
	}
	if out.Status == "success" && out.Total == 0 {
		out.Detail = "Không phát hiện CVE"
	} else if out.Status == "success" {
		out.Detail = fmt.Sprintf("%d CVE (%d Critical/High)", out.Total, out.Severity["Critical"]+out.Severity["High"])
	} else if out.Status == "running" {
		out.Detail = "Trivy đang quét image…"
	} else if out.Status == "pending" && out.Detail == "" {
		out.Detail = "Chờ Harbor Trivy quét sau push"
	}
	return out, nil
}

// Vulnerability một CVE từ báo cáo Trivy (Harbor).
type Vulnerability struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Package    string `json:"package"`
	Version    string `json:"version"`
	FixVersion string `json:"fix_version,omitempty"`
	Status     string `json:"status,omitempty"`
}

const acceptVulnerabilities = "application/vnd.security.vulnerability.report; version=1.1, application/vnd.scanner.adapter.vuln.report.harbor+json; version=1.0"

func severityRank(sev string) int {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	default:
		return 4
	}
}

// ArtifactVulnerabilities lấy danh sách CVE (Trivy) — dùng hiển thị inline trên Console.
func (c *Client) ArtifactVulnerabilities(ctx context.Context, projectName, repoName, reference string, limit int) ([]Vulnerability, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("harbor chưa cấu hình")
	}
	projectName = strings.TrimSpace(projectName)
	repoName = strings.Trim(strings.TrimSpace(repoName), "/")
	if repoName == "" {
		repoName = "app"
	}
	reference = strings.TrimSpace(reference)
	if projectName == "" || reference == "" {
		return nil, fmt.Errorf("thiếu project hoặc image tag")
	}
	apiURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s/artifacts/%s/additions/vulnerabilities",
		c.baseURL, url.PathEscape(projectName), url.PathEscape(repoName), url.PathEscape(reference))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Accept-Vulnerabilities", acceptVulnerabilities)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("harbor vulnerabilities %d: %s", res.StatusCode, string(body))
	}
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return nil, err
	}
	var vulns []Vulnerability
	for _, raw := range wrapped {
		var report struct {
			Vulnerabilities []Vulnerability `json:"vulnerabilities"`
		}
		if err := json.Unmarshal(raw, &report); err != nil {
			continue
		}
		vulns = append(vulns, report.Vulnerabilities...)
	}
	if len(vulns) == 0 {
		return nil, nil
	}
	sort.SliceStable(vulns, func(i, j int) bool {
		rankI, rankJ := severityRank(vulns[i].Severity), severityRank(vulns[j].Severity)
		if rankI != rankJ {
			return rankI < rankJ
		}
		return vulns[i].ID < vulns[j].ID
	})
	if limit > 0 && len(vulns) > limit {
		vulns = vulns[:limit]
	}
	return vulns, nil
}

func (c *Client) ArtifactUIURL(projectName, repoName, reference string) string {
	if !c.Enabled() {
		return ""
	}
	repoName = strings.Trim(strings.TrimSpace(repoName), "/")
	if repoName == "" {
		repoName = "app"
	}
	return fmt.Sprintf("%s/harbor/projects/%s/repositories/%s/artifacts-tab/artifacts/%s",
		c.baseURL, url.PathEscape(projectName), url.PathEscape(repoName), url.PathEscape(reference))
}

type harborRepository struct {
	Name string `json:"name"`
}

// PurgeProject xóa toàn bộ repository rồi xóa Harbor project (VPS registry).
func (c *Client) PurgeProject(ctx context.Context, projectName string) error {
	if !c.Enabled() {
		return fmt.Errorf("harbor chưa cấu hình")
	}
	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return fmt.Errorf("thiếu tên harbor project")
	}
	for page := 1; page <= 50; page++ {
		repos, err := c.listRepositories(ctx, projectName, page)
		if err != nil {
			return err
		}
		if len(repos) == 0 {
			break
		}
		for _, repo := range repos {
			if err := c.deleteRepository(ctx, projectName, repo.Name); err != nil {
				return err
			}
		}
		if len(repos) < 100 {
			break
		}
	}
	return c.deleteProject(ctx, projectName)
}

func (c *Client) listRepositories(ctx context.Context, projectName string, page int) ([]harborRepository, error) {
	q := url.Values{
		"page":      {fmt.Sprintf("%d", page)},
		"page_size": {"100"},
	}
	apiURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories?%s",
		c.baseURL, url.PathEscape(projectName), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("harbor list repositories %d: %s", res.StatusCode, string(body))
	}
	var repos []harborRepository
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func (c *Client) deleteRepository(ctx context.Context, projectName, repoName string) error {
	repoName = strings.TrimSpace(repoName)
	if repoName == "" {
		return nil
	}
	apiURL := fmt.Sprintf("%s/api/v2.0/projects/%s/repositories/%s",
		c.baseURL, url.PathEscape(projectName), encodeHarborRepositoryName(repoName))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("harbor delete repository %s %d: %s", repoName, res.StatusCode, string(body))
	}
	return nil
}

func (c *Client) deleteProject(ctx context.Context, projectName string) error {
	apiURL := fmt.Sprintf("%s/api/v2.0/projects/%s", c.baseURL, url.PathEscape(projectName))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, apiURL, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("X-Is-Resource-Name", "true")
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.StatusCode >= 400 {
		return fmt.Errorf("harbor delete project %d: %s", res.StatusCode, string(body))
	}
	return nil
}

func encodeHarborRepositoryName(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if i := strings.Index(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return url.PathEscape(name)
}
