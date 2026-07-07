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
	Namespace  string `json:"namespace"`
	Pod        string `json:"pod"`
	Deployment string `json:"deployment"`
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
	cmd, ok := k8scommands.ByID(strings.TrimSpace(body.CommandID))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "lệnh không tồn tại"})
		return
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
	} else {
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

func resourceKeyForCommand(id string) string {
	switch {
	case strings.HasPrefix(id, "pods_"):
		return "pods"
	case strings.HasPrefix(id, "deployments_"):
		return "deployments"
	case strings.HasPrefix(id, "services_"):
		return "services"
	case strings.HasPrefix(id, "ingresses_"):
		return "ingresses"
	case id == "events_list":
		return "pods"
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
	case "pods_list":
		return h.runListCommand(r, clusterID, cmd.ID, "pods", ns)
	case "deployments_list":
		return h.runListCommand(r, clusterID, cmd.ID, "deployments", ns)
	case "services_list":
		return h.runListCommand(r, clusterID, cmd.ID, "services", ns)
	case "ingresses_list":
		return h.runListCommand(r, clusterID, cmd.ID, "ingresses", ns)
	case "pods_logs":
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

	case "pods_describe", "deployments_describe":
		key := "pods"
		name := strings.TrimSpace(body.Pod)
		if cmd.ID == "deployments_describe" {
			key = "deployments"
			name = strings.TrimSpace(body.Deployment)
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

	case "events_list":
		events, err := h.rancher.ListNamespaceEvents(r.Context(), clusterID, ns, "", 80)
		if err != nil {
			return k8sRunResult{}, err
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

	case "nodes_list":
		list, err := h.rancher.ListK8s(r.Context(), clusterID, "nodes", "", 1, 50)
		if err != nil {
			return k8sRunResult{}, err
		}
		return k8sRunResult{CommandID: cmd.ID, Kind: "table", Output: list.Items, Summary: fmt.Sprintf("%d nodes", len(list.Items))}, nil

	default:
		return k8sRunResult{}, fmt.Errorf("lệnh chưa được triển khai: %s", cmd.ID)
	}
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
