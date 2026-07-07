package k8scommands

import "strings"

// Command — mục sổ lệnh (copy kubectl + chạy read-only qua API).
type Command struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Description  string   `json:"description,omitempty"`
	Category     string   `json:"category"`
	Scope        string   `json:"scope"` // platform | project | both
	InfraOnly    bool     `json:"infra_only,omitempty"`
	ReadOnly     bool     `json:"read_only"`
	Runnable     bool     `json:"runnable"`
	Kubectl      string   `json:"kubectl"`
	Placeholders []string `json:"placeholders,omitempty"`
}

var catalog = []Command{
	{
		ID: "pods_list", Label: "Liệt kê Pods", Category: "Pods", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl get pods -n {namespace}",
		Placeholders: []string{"namespace"},
	},
	{
		ID: "pods_describe", Label: "Chi tiết Pod", Category: "Pods", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl describe pod {pod} -n {namespace}",
		Placeholders: []string{"namespace", "pod"},
	},
	{
		ID: "pods_logs", Label: "Xem log Pod", Category: "Pods", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl logs {pod} -n {namespace} --tail={tail}",
		Placeholders: []string{"namespace", "pod", "tail"},
	},
	{
		ID: "deployments_list", Label: "Liệt kê Deployments", Category: "Deployments", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl get deployments -n {namespace}",
		Placeholders: []string{"namespace"},
	},
	{
		ID: "deployments_describe", Label: "Chi tiết Deployment", Category: "Deployments", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl describe deployment {deployment} -n {namespace}",
		Placeholders: []string{"namespace", "deployment"},
	},
	{
		ID: "services_list", Label: "Liệt kê Services", Category: "Services", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl get svc -n {namespace}",
		Placeholders: []string{"namespace"},
	},
	{
		ID: "ingresses_list", Label: "Liệt kê Ingress", Category: "Networking", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl get ingress -n {namespace}",
		Placeholders: []string{"namespace"},
	},
	{
		ID: "events_list", Label: "Events namespace", Category: "Debug", Scope: "both",
		ReadOnly: true, Runnable: true,
		Kubectl:      "kubectl get events -n {namespace} --sort-by=.lastTimestamp",
		Placeholders: []string{"namespace"},
	},
	{
		ID: "namespaces_list", Label: "Liệt kê Namespaces", Category: "Cluster", Scope: "platform",
		InfraOnly: true, ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get namespaces",
	},
	{
		ID: "nodes_list", Label: "Liệt kê Nodes", Category: "Cluster", Scope: "platform",
		InfraOnly: true, ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get nodes -o wide",
	},
}

func ByID(id string) (Command, bool) {
	for _, c := range catalog {
		if c.ID == id {
			return c, true
		}
	}
	return Command{}, false
}

// List — lọc theo scope (platform|project|all) và role.
func List(role, scope string, canInfra bool) []Command {
	out := make([]Command, 0, len(catalog))
	for _, c := range catalog {
		if c.InfraOnly && !canInfra {
			continue
		}
		if !scopeMatch(c.Scope, scope) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func scopeMatch(cmdScope, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" || filter == "all" {
		return true
	}
	if cmdScope == "both" {
		return filter == "platform" || filter == "project"
	}
	return cmdScope == filter
}
