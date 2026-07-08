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
	// ── Pods ──
	{ID: "pods_list", Label: "Liệt kê Pods", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get pods -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "pods_list_wide", Label: "Pods (wide)", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get pods -n {namespace} -o wide", Placeholders: []string{"namespace"}},
	{ID: "pods_list_running", Label: "Pods đang Running", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get pods -n {namespace} --field-selector=status.phase=Running", Placeholders: []string{"namespace"}},
	{ID: "pods_list_label", Label: "Pods theo label", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl get pods -n {namespace} -l {label}", Placeholders: []string{"namespace", "label"},
		Description: "Chỉ copy — dùng khi biết label selector"},
	{ID: "pods_describe", Label: "Chi tiết Pod", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe pod {pod} -n {namespace}", Placeholders: []string{"namespace", "pod"}},
	{ID: "pods_logs", Label: "Log Pod (tail)", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl logs {pod} -n {namespace} --tail={tail}", Placeholders: []string{"namespace", "pod", "tail"}},
	{ID: "pods_logs_container", Label: "Log container cụ thể", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl logs {pod} -n {namespace} -c {container} --tail={tail}", Placeholders: []string{"namespace", "pod", "container", "tail"}},
	{ID: "pods_logs_follow", Label: "Log theo dõi (-f)", Category: "Pods", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl logs -f {pod} -n {namespace} --tail={tail}", Placeholders: []string{"namespace", "pod", "tail"},
		Description: "Chỉ copy — Console chưa stream; dùng SSH/kubectl local"},

	// ── Deployments ──
	{ID: "deployments_list", Label: "Liệt kê Deployments", Category: "Deployments", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get deployments -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "deployments_list_wide", Label: "Deployments (wide)", Category: "Deployments", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get deployments -n {namespace} -o wide", Placeholders: []string{"namespace"}},
	{ID: "deployments_describe", Label: "Chi tiết Deployment", Category: "Deployments", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe deployment {deployment} -n {namespace}", Placeholders: []string{"namespace", "deployment"}},
	{ID: "replicasets_list", Label: "ReplicaSets", Category: "Deployments", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl get rs -n {namespace}", Placeholders: []string{"namespace"},
		Description: "Chỉ copy — chạy qua SSH"},

	// ── Workloads khác ──
	{ID: "statefulsets_list", Label: "StatefulSets", Category: "Workloads", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get statefulsets -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "statefulsets_describe", Label: "Chi tiết StatefulSet", Category: "Workloads", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe statefulset {name} -n {namespace}", Placeholders: []string{"namespace", "name"}},
	{ID: "daemonsets_list", Label: "DaemonSets", Category: "Workloads", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get daemonsets -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "jobs_list", Label: "Jobs", Category: "Workloads", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get jobs -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "jobs_describe", Label: "Chi tiết Job", Category: "Workloads", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe job {name} -n {namespace}", Placeholders: []string{"namespace", "name"}},
	{ID: "cronjobs_list", Label: "CronJobs", Category: "Workloads", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get cronjobs -n {namespace}", Placeholders: []string{"namespace"}},

	// ── Networking ──
	{ID: "services_list", Label: "Services", Category: "Networking", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get svc -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "services_describe", Label: "Chi tiết Service", Category: "Networking", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe svc {name} -n {namespace}", Placeholders: []string{"namespace", "name"}},
	{ID: "ingresses_list", Label: "Ingress", Category: "Networking", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get ingress -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "ingresses_describe", Label: "Chi tiết Ingress", Category: "Networking", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe ingress {name} -n {namespace}", Placeholders: []string{"namespace", "name"}},
	{ID: "endpoints_list", Label: "Endpoints", Category: "Networking", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl get endpoints -n {namespace}", Placeholders: []string{"namespace"},
		Description: "Chỉ copy"},

	// ── Config & Storage ──
	{ID: "configmaps_list", Label: "ConfigMaps", Category: "Config & Storage", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get configmaps -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "configmaps_describe", Label: "Chi tiết ConfigMap", Category: "Config & Storage", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe configmap {name} -n {namespace}", Placeholders: []string{"namespace", "name"}},
	{ID: "secrets_list", Label: "Secrets (tên)", Category: "Config & Storage", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get secrets -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "secrets_describe", Label: "Chi tiết Secret", Category: "Config & Storage", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe secret {name} -n {namespace}", Placeholders: []string{"namespace", "name"}},
	{ID: "pvc_list", Label: "PVC", Category: "Config & Storage", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get pvc -n {namespace}", Placeholders: []string{"namespace"}},
	{ID: "hpa_list", Label: "HPA (autoscaler)", Category: "Config & Storage", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get hpa -n {namespace}", Placeholders: []string{"namespace"}},

	// ── Debug ──
	{ID: "events_list", Label: "Events (mới nhất)", Category: "Debug", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get events -n {namespace} --sort-by=.lastTimestamp", Placeholders: []string{"namespace"}},
	{ID: "events_warnings", Label: "Events Warning", Category: "Debug", Scope: "both", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get events -n {namespace} --field-selector type=Warning --sort-by=.lastTimestamp", Placeholders: []string{"namespace"}},
	{ID: "events_pod", Label: "Events của Pod", Category: "Debug", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl get events -n {namespace} --field-selector involvedObject.name={pod}", Placeholders: []string{"namespace", "pod"},
		Description: "Chỉ copy"},
	{ID: "get_all", Label: "Tất cả resource (all)", Category: "Debug", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl get all -n {namespace}", Placeholders: []string{"namespace"},
		Description: "Chỉ copy — gộp nhiều loại resource"},

	// ── Cluster (infra) ──
	{ID: "namespaces_list", Label: "Namespaces", Category: "Cluster", Scope: "platform", ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get namespaces"},
	{ID: "nodes_list", Label: "Nodes (wide)", Category: "Cluster", Scope: "platform", InfraOnly: true, ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get nodes -o wide"},
	{ID: "nodes_describe", Label: "Chi tiết Node", Category: "Cluster", Scope: "platform", InfraOnly: true, ReadOnly: true, Runnable: true,
		Kubectl: "kubectl describe node {name}", Placeholders: []string{"name"}},
	{ID: "pv_list", Label: "PersistentVolumes", Category: "Cluster", Scope: "platform", InfraOnly: true, ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get pv"},
	{ID: "storageclasses_list", Label: "StorageClasses", Category: "Cluster", Scope: "platform", InfraOnly: true, ReadOnly: true, Runnable: true,
		Kubectl: "kubectl get storageclass"},
	{ID: "top_pods", Label: "Top Pods (CPU/RAM)", Category: "Cluster", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl top pods -n {namespace}", Placeholders: []string{"namespace"},
		Description: "Chỉ copy — cần metrics-server + kubectl local"},
	{ID: "top_nodes", Label: "Top Nodes", Category: "Cluster", Scope: "platform", InfraOnly: true, ReadOnly: true, Runnable: false,
		Kubectl: "kubectl top nodes", Description: "Chỉ copy"},
	{ID: "auth_can_i", Label: "Kiểm tra quyền (can-i)", Category: "Cluster", Scope: "both", ReadOnly: true, Runnable: false,
		Kubectl: "kubectl auth can-i get pods -n {namespace}", Placeholders: []string{"namespace"},
		Description: "Chỉ copy"},
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

// ListKeyFromID — deployments_list → deployments.
func ListKeyFromID(id string) (string, bool) {
	if !strings.HasSuffix(id, "_list") && !strings.HasSuffix(id, "_list_wide") && !strings.HasSuffix(id, "_list_running") {
		if strings.HasSuffix(id, "_warnings") {
			return "events", true
		}
	}
	switch id {
	case "pods_list", "pods_list_wide", "pods_list_running":
		return "pods", true
	case "deployments_list", "deployments_list_wide":
		return "deployments", true
	case "pvc_list":
		return "persistentvolumeclaims", true
	case "hpa_list":
		return "horizontalpodautoscalers", true
	case "pv_list":
		return "persistentvolumes", true
	case "storageclasses_list":
		return "storageclasses", true
	case "events_list", "events_warnings":
		return "events", true
	}
	if strings.HasSuffix(id, "_list") {
		base := strings.TrimSuffix(id, "_list")
		return base, true
	}
	return "", false
}

// DescribeKeyFromID — pods_describe → pods.
func DescribeKeyFromID(id string) (string, bool) {
	if !strings.HasSuffix(id, "_describe") {
		return "", false
	}
	base := strings.TrimSuffix(id, "_describe")
	switch base {
	case "pods", "pod":
		return "pods", true
	case "deployments", "deployment":
		return "deployments", true
	case "services", "service", "svc":
		return "services", true
	case "ingresses", "ingress":
		return "ingresses", true
	case "statefulsets", "statefulset":
		return "statefulsets", true
	case "configmaps", "configmap":
		return "configmaps", true
	case "secrets", "secret":
		return "secrets", true
	case "jobs", "job":
		return "jobs", true
	case "cronjobs", "cronjob":
		return "cronjobs", true
	case "nodes", "node":
		return "nodes", true
	default:
		return base, true
	}
}
