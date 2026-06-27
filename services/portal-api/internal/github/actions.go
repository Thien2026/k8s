package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type WorkflowRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	HeadSHA    string `json:"head_sha"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type WorkflowJob struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	Status     string           `json:"status"`
	Conclusion string           `json:"conclusion"`
	HTMLURL    string           `json:"html_url"`
	Steps      []WorkflowJobStep `json:"steps"`
}

type WorkflowJobStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Number     int    `json:"number"`
	StartedAt  string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

func (c *Client) ListRepoWorkflowRuns(ctx context.Context, token, owner, repo string, limit int) ([]WorkflowRun, error) {
	if limit < 1 {
		limit = 10
	}
	path := fmt.Sprintf("/repos/%s/%s/actions/runs?per_page=%d", owner, repo, limit)
	raw, code, err := c.api(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("workflow runs %d: %s", code, string(raw))
	}
	var out struct {
		WorkflowRuns []WorkflowRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.WorkflowRuns, nil
}

func (c *Client) ListWorkflowRuns(ctx context.Context, token, owner, repo, workflowFile string, limit int) ([]WorkflowRun, error) {
	if limit < 1 {
		limit = 10
	}
	q := url.Values{}
	q.Set("per_page", fmt.Sprintf("%d", limit))
	path := fmt.Sprintf("/repos/%s/%s/actions/workflows/%s/runs?%s",
		owner, repo, url.PathEscape(workflowFile), q.Encode())
	raw, code, err := c.api(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("workflow runs %d: %s", code, string(raw))
	}
	var out struct {
		WorkflowRuns []WorkflowRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.WorkflowRuns, nil
}

func (c *Client) GetWorkflowRun(ctx context.Context, token, owner, repo string, runID int64) (WorkflowRun, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)
	raw, code, err := c.api(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return WorkflowRun{}, err
	}
	if code >= 400 {
		return WorkflowRun{}, fmt.Errorf("workflow run %d: %s", code, string(raw))
	}
	var run WorkflowRun
	if err := json.Unmarshal(raw, &run); err != nil {
		return WorkflowRun{}, err
	}
	return run, nil
}

func (c *Client) GetWorkflowRunJobs(ctx context.Context, token, owner, repo string, runID int64) ([]WorkflowJob, error) {
	path := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=20", owner, repo, runID)
	raw, code, err := c.api(ctx, token, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code >= 400 {
		return nil, fmt.Errorf("workflow jobs %d: %s", code, string(raw))
	}
	var out struct {
		Jobs []WorkflowJob `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Jobs, nil
}
