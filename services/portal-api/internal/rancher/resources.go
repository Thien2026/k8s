package rancher

// K8sResource mô tả đường dẫn proxy Rancher → Kubernetes API.
type K8sResource struct {
	Key        string
	Label      string
	Section    string // platform | infra
	Group      string
	APIPath    string // sau /k8s/clusters/{id}
	Namespaced bool
}

var K8sResources = []K8sResource{
	{Key: "namespaces", Label: "Namespaces", Group: "Cluster", APIPath: "/api/v1/namespaces", Namespaced: false},
	{Key: "nodes", Label: "Nodes", Group: "Cluster", APIPath: "/api/v1/nodes", Namespaced: false},
	{Key: "events", Label: "Events", Group: "Cluster", APIPath: "/apis/events.k8s.io/v1/events", Namespaced: false},
	{Key: "deployments", Label: "Deployments", Group: "Workloads", APIPath: "/apis/apps/v1/deployments", Namespaced: true},
	{Key: "statefulsets", Label: "StatefulSets", Group: "Workloads", APIPath: "/apis/apps/v1/statefulsets", Namespaced: true},
	{Key: "daemonsets", Label: "DaemonSets", Group: "Workloads", APIPath: "/apis/apps/v1/daemonsets", Namespaced: true},
	{Key: "jobs", Label: "Jobs", Group: "Workloads", APIPath: "/apis/batch/v1/jobs", Namespaced: true},
	{Key: "cronjobs", Label: "CronJobs", Group: "Workloads", APIPath: "/apis/batch/v1/cronjobs", Namespaced: true},
	{Key: "pods", Label: "Pods", Group: "Workloads", APIPath: "/api/v1/pods", Namespaced: true},
	{Key: "services", Label: "Services", Group: "Networking", APIPath: "/api/v1/services", Namespaced: true},
	{Key: "ingresses", Label: "Ingresses", Group: "Networking", APIPath: "/apis/networking.k8s.io/v1/ingresses", Namespaced: true},
	{Key: "argocdapplications", Label: "ArgoCD Applications", Group: "GitOps", APIPath: "/apis/argoproj.io/v1alpha1/applications", Namespaced: true},
	{Key: "horizontalpodautoscalers", Label: "HorizontalPodAutoscalers", Group: "Networking", APIPath: "/apis/autoscaling/v2/horizontalpodautoscalers", Namespaced: true},
	{Key: "persistentvolumeclaims", Label: "PersistentVolumeClaims", Group: "Storage", APIPath: "/api/v1/persistentvolumeclaims", Namespaced: true},
	{Key: "persistentvolumes", Label: "PersistentVolumes", Group: "Storage", APIPath: "/api/v1/persistentvolumes", Namespaced: false},
	{Key: "storageclasses", Label: "StorageClasses", Group: "Storage", APIPath: "/apis/storage.k8s.io/v1/storageclasses", Namespaced: false},
	{Key: "configmaps", Label: "ConfigMaps", Group: "Config", APIPath: "/api/v1/configmaps", Namespaced: true},
	{Key: "secrets", Label: "Secrets", Group: "Config", APIPath: "/api/v1/secrets", Namespaced: true},
}

func K8sResourceByKey(key string) (K8sResource, bool) {
	for _, r := range K8sResources {
		if r.Key == key {
			return r, true
		}
	}
	return K8sResource{}, false
}

// K8sListItemKind — kind mặc định khi item list K8s không có field "kind" (Rancher proxy).
func K8sListItemKind(key string) string {
	switch key {
	case "deployments":
		return "Deployment"
	case "statefulsets":
		return "StatefulSet"
	case "daemonsets":
		return "DaemonSet"
	case "jobs":
		return "Job"
	case "cronjobs":
		return "CronJob"
	case "pods":
		return "Pod"
	case "services":
		return "Service"
	case "ingresses":
		return "Ingress"
	case "namespaces":
		return "Namespace"
	case "nodes":
		return "Node"
	case "events":
		return "Event"
	case "horizontalpodautoscalers":
		return "HorizontalPodAutoscaler"
	case "persistentvolumeclaims":
		return "PersistentVolumeClaim"
	case "persistentvolumes":
		return "PersistentVolume"
	case "storageclasses":
		return "StorageClass"
	case "configmaps":
		return "ConfigMap"
	case "secrets":
		return "Secret"
	default:
		return ""
	}
}
