package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/auth"
	"github.com/Thien2026/k8s/services/portal-api/internal/k8scommands"
	"github.com/Thien2026/k8s/services/portal-api/internal/rancher"
)

func (h *Handler) ListK8sCommands(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = "all"
	}
	canInfra := auth.CanViewInfra(u.Role)
	list := k8scommands.List(u.Role, scope, canInfra)
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       list,
		"scope":       scope,
		"read_only":   true,
		"infra_scope": canInfra,
	})
}

type k8sRunRequest struct {
	CommandID  string `json:"command_id"`
	Kubectl    string `json:"kubectl"`
	Namespace  string `json:"namespace"`
	Pod        string `json:"pod"`
	Deployment string `json:"deployment"`
	Name       string `json:"name"`
	Container  string `json:"container"`
	Tail       int    `json:"tail"`
}

type k8sRunResult struct {
	CommandID string `json:"command_id"`
	Kind      string `json:"kind"` // table | logs | json | text
	Output    any    `json:"output"`
	Summary   string `json:"summary,omitempty"`
}

func (h *Handler) RunK8sCommand(w http.ResponseWriter, r *http.Request) {
	if !h.rancherRequired(w, r) {
		return
	}
	var body k8sRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload không hợp lệ"})
		return
	}

	kubectlRaw := strings.TrimSpace(body.Kubectl)
	cmdID := strings.TrimSpace(body.CommandID)

	var cmd k8scommands.Command
	var ok bool
	var parsed k8scommands.Parsed
	var parseErr error

	if kubectlRaw != "" {
		parsed, parseErr = k8scommands.ParseReadOnlyKubectl(kubectlRaw)
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": parseErr.Error()})
			return
		}
		cmd, ok = k8scommands.ByID(parsed.CommandKey)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lệnh chưa được triển khai trên server"})
			return
		}
		body = mergeRunBodyFromParsed(body, parsed)
		cmdID = cmd.ID
	} else {
		cmd, ok = k8scommands.ByID(cmdID)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lệnh không tồn tại"})
			return
		}
	}
	if !cmd.Runnable || !cmd.ReadOnly {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lệnh này chỉ hỗ trợ copy — chưa chạy qua API"})
		return
	}
	u, _ := auth.UserFromContext(r.Context())
	if cmd.InfraOnly && !auth.CanViewInfra(u.Role) {
		writeAccessDenied(w)
		return
	}
	if err := validateRunParams(cmd, body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Tail <= 0 {
		body.Tail = 200
	}
	clusterID := clusterQuery(r)
	ns := strings.TrimSpace(body.Namespace)
	if cmd.InfraOnly {
		if !auth.CanViewInfra(u.Role) {
			writeAccessDenied(w)
			return
		}
	} else if commandNeedsNamespace(cmd) {
		if ns == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "namespace bắt buộc"})
			return
		}
		if _, ok := h.guardK8sRead(w, r, resourceKeyForCommand(cmd.ID), ns); !ok {
			return
		}
	}

	result, err := h.executeK8sCommand(r, clusterID, cmd, body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func commandNeedsNamespace(cmd k8scommands.Command) bool {
	for _, ph := range cmd.Placeholders {
		if ph == "namespace" {
			return true
		}
	}
	return false
}

func mergeRunBodyFromParsed(body k8sRunRequest, p k8scommands.Parsed) k8sRunRequest {
	if p.Namespace != "" {
		body.Namespace = p.Namespace
	}
	if p.Name != "" {
		switch p.CommandKey {
		case "pods_logs", "pods_logs_container", "pods_describe":
			body.Pod = p.Name
		case "deployments_describe":
			body.Deployment = p.Name
		default:
			body.Name = p.Name
		}
	}
	if p.Container != "" {
		body.Container = p.Container
	}
	if p.Tail > 0 {
		body.Tail = p.Tail
	}
	return body
}

func resourceKeyForCommand(id string) string {
	if key, ok := k8scommands.ListKeyFromID(id); ok {
		if key == "events" {
			return "pods"
		}
		return key
	}
	if key, ok := k8scommands.DescribeKeyFromID(id); ok {
		return key
	}
	switch {
	case strings.HasPrefix(id, "pods_"):
		return "pods"
	case strings.HasPrefix(id, "deployments_"):
		return "deployments"
	default:
		return "pods"
	}
}

func validateRunParams(cmd k8scommands.Command, body k8sRunRequest) error {
	if body.Tail <= 0 {
		body.Tail = 200
	}
	need := map[string]string{
		"namespace":  strings.TrimSpace(body.Namespace),
		"pod":        strings.TrimSpace(body.Pod),
		"deployment": strings.TrimSpace(body.Deployment),
		"name":       strings.TrimSpace(body.Name),
		"container":  strings.TrimSpace(body.Container),
	}
	for _, ph := range cmd.Placeholders {
		if ph == "tail" {
			continue
		}
		if need[ph] == "" {
			return fmt.Errorf("thiếu {%s}", ph)
		}
	}
	return nil
}

func (h *Handler) executeK8sCommand(r *http.Request, clusterID string, cmd k8scommands.Command, body k8sRunRequest) (k8sRunResult, error) {
	ns := strings.TrimSpace(body.Namespace)
	tail := body.Tail
	if tail <= 0 {
		tail = 200
	}

	switch cmd.ID {
	case "pods_logs", "pods_logs_container":
		logs, err := h.rancher.GetPodLogs(r.Context(), clusterID, ns, strings.TrimSpace(body.Pod), strings.TrimSpace(body.Container), tail)
		if err != nil {
			return k8sRunResult{}, err
		}
		return k8sRunResult{
			CommandID: cmd.ID,
			Kind:      "logs",
			Output:    logs,
			Summary:   fmt.Sprintf("Log %s (tail %d)", body.Pod, tail),
		}, nil

	case "events_list", "events_warnings":
		events, err := h.rancher.ListNamespaceEvents(r.Context(), clusterID, ns, "", 80)
		if err != nil {
			return k8sRunResult{}, err
		}
		if cmd.ID == "events_warnings" {
			filtered := make([]rancher.ResourceRow, 0, len(events))
			for _, ev := range events {
				if strings.EqualFold(ev.Status, "Warning") {
					filtered = append(filtered, ev)
				}
			}
			events = filtered
		}
		return k8sRunResult{
			CommandID: cmd.ID,
			Kind:      "table",
			Output:    events,
			Summary:   fmt.Sprintf("%d events trong %s", len(events), ns),
		}, nil

	case "namespaces_list":
		u, _ := auth.UserFromContext(r.Context())
		scope, err := h.accessScope(r.Context(), u)
		if err != nil {
			return k8sRunResult{}, err
		}
		if scope.All {
			list, err := h.rancher.ListK8s(r.Context(), clusterID, "namespaces", "", 1, 200)
			if err != nil {
				return k8sRunResult{}, err
			}
			return k8sRunResult{CommandID: cmd.ID, Kind: "table", Output: list.Items, Summary: fmt.Sprintf("%d namespaces", len(list.Items))}, nil
		}
		rows := make([]rancher.ResourceRow, 0, len(scope.ReadNamespaces))
		for _, n := range scope.ReadNamespaces {
			rows = append(rows, rancher.ResourceRow{Name: n, Namespace: n, Status: "—"})
		}
		return k8sRunResult{CommandID: cmd.ID, Kind: "table", Output: rows, Summary: fmt.Sprintf("%d namespaces", len(rows))}, nil
	}

	if key, ok := k8scommands.ListKeyFromID(cmd.ID); ok {
		return h.runListCommand(r, clusterID, cmd.ID, key, ns)
	}

	if key, ok := k8scommands.DescribeKeyFromID(cmd.ID); ok {
		name := describeName(body)
		if name == "" {
			return k8sRunResult{}, fmt.Errorf("thiếu tên resource")
		}
		raw, err := h.rancher.GetK8sResource(r.Context(), clusterID, key, ns, name)
		if err != nil {
			return k8sRunResult{}, err
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return k8sRunResult{CommandID: cmd.ID, Kind: "json", Output: string(raw)}, nil
		}
		return k8sRunResult{
			CommandID: cmd.ID,
			Kind:      "json",
			Output:    obj,
			Summary:   fmt.Sprintf("%s/%s", key, name),
		}, nil
	}

	return k8sRunResult{}, fmt.Errorf("lệnh chưa được triển khai: %s", cmd.ID)
}

func describeName(body k8sRunRequest) string {
	if strings.TrimSpace(body.Pod) != "" {
		return strings.TrimSpace(body.Pod)
	}
	if strings.TrimSpace(body.Deployment) != "" {
		return strings.TrimSpace(body.Deployment)
	}
	return strings.TrimSpace(body.Name)
}

func (h *Handler) runListCommand(r *http.Request, clusterID, cmdID, key, ns string) (k8sRunResult, error) {
	list, err := h.rancher.ListK8s(r.Context(), clusterID, key, ns, 1, 100)
	if err != nil {
		return k8sRunResult{}, err
	}
	scope, _ := h.accessScopeFromRequest(r)
	if !scope.All {
		list = filterResourceList(scope, list)
	}
	return k8sRunResult{
		CommandID: cmdID,
		Kind:      "table",
		Output:    list.Items,
		Summary:   fmt.Sprintf("%d %s trong %s", len(list.Items), key, ns),
	}, nil
}
