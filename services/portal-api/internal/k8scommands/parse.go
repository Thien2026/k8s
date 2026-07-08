package k8scommands

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Parsed — lệnh kubectl read-only đã parse.
type Parsed struct {
	Action     string
	Resource   string
	Name       string
	Namespace  string
	Container  string
	Tail       int
	InfraOnly  bool
	CommandKey string
}

var blockedSubstrings = []string{
	"apply", "delete", "create", "patch", "replace", "edit", "run ", "exec ",
	"scale", "rollout", "auth ", "config ", "port-forward", "cp ", "attach",
	";", "|", "&", "`", "$(", "..", "\n", "\r",
}

var blockedVerbs = []string{"top", "watch", "wait", "cordon", "uncordon", "drain"}

var allowedGetResources = map[string]string{
	"pods":         "pods",
	"pod":          "pods",
	"po":           "pods",
	"deployments":  "deployments",
	"deployment":   "deployments",
	"deploy":       "deployments",
	"statefulsets": "statefulsets",
	"statefulset":  "statefulsets",
	"sts":          "statefulsets",
	"daemonsets":   "daemonsets",
	"daemonset":    "daemonsets",
	"ds":           "daemonsets",
	"jobs":         "jobs",
	"job":          "jobs",
	"cronjobs":     "cronjobs",
	"cronjob":      "cronjobs",
	"cj":           "cronjobs",
	"svc":          "services",
	"service":      "services",
	"services":     "services",
	"ingress":      "ingresses",
	"ingresses":    "ingresses",
	"ing":          "ingresses",
	"configmaps":   "configmaps",
	"configmap":    "configmaps",
	"cm":           "configmaps",
	"secrets":      "secrets",
	"secret":       "secrets",
	"pvc":          "persistentvolumeclaims",
	"persistentvolumeclaims": "persistentvolumeclaims",
	"persistentvolumeclaim":  "persistentvolumeclaims",
	"hpa":          "horizontalpodautoscalers",
	"horizontalpodautoscalers": "horizontalpodautoscalers",
	"events":       "events",
	"event":        "events",
	"namespaces":   "namespaces",
	"namespace":    "namespaces",
	"ns":           "namespaces",
	"nodes":        "nodes",
	"node":         "nodes",
	"no":           "nodes",
	"pv":           "persistentvolumes",
	"persistentvolumes": "persistentvolumes",
	"persistentvolume":  "persistentvolumes",
	"storageclass": "storageclasses",
	"storageclasses": "storageclasses",
	"sc":           "storageclasses",
}

// ParseReadOnlyKubectl — chỉ get / describe / logs.
func ParseReadOnlyKubectl(raw string) (Parsed, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Parsed{}, fmt.Errorf("lệnh trống")
	}
	s = strings.TrimPrefix(s, "kubectl ")
	s = strings.TrimSpace(s)
	if s == "" {
		return Parsed{}, fmt.Errorf("thiếu lệnh sau kubectl")
	}
	lower := strings.ToLower(s)
	for _, bad := range blockedSubstrings {
		if strings.Contains(lower, bad) {
			return Parsed{}, fmt.Errorf("chỉ hỗ trợ lệnh đọc (get, describe, logs)")
		}
	}

	tokens := tokenize(s)
	if len(tokens) == 0 {
		return Parsed{}, fmt.Errorf("lệnh không hợp lệ")
	}

	verb := strings.ToLower(tokens[0])
	for _, bv := range blockedVerbs {
		if verb == bv {
			return Parsed{}, fmt.Errorf("lệnh %q — dùng copy mẫu hoặc SSH", verb)
		}
	}

	p := Parsed{Tail: 200}
	p.Namespace = flagValue(tokens, "-n", "--namespace")
	p.Container = flagValue(tokens, "-c", "--container")
	if tailStr := flagValue(tokens, "--tail"); tailStr != "" {
		if n, err := strconv.Atoi(tailStr); err == nil && n > 0 {
			if n > 2000 {
				n = 2000
			}
			p.Tail = n
		}
	}

	switch verb {
	case "get":
		return parseGet(tokens, p)
	case "describe", "desc":
		return parseDescribe(tokens, p)
	case "logs", "log":
		return parseLogs(tokens, p)
	default:
		return Parsed{}, fmt.Errorf("chỉ hỗ trợ get, describe, logs — không phải %q", verb)
	}
}

func parseGet(tokens []string, p Parsed) (Parsed, error) {
	if len(tokens) < 2 {
		return Parsed{}, fmt.Errorf("get cần loại resource")
	}
	res := strings.ToLower(tokens[1])
	if res == "all" {
		return Parsed{}, fmt.Errorf("get all — chọn từng loại resource hoặc dùng mẫu copy")
	}
	key, ok := allowedGetResources[res]
	if !ok {
		return Parsed{}, fmt.Errorf("resource %q chưa hỗ trợ", res)
	}
	p.Action = "get"
	p.Resource = key
	p.CommandKey = listCommandKey(key)

	switch key {
	case "namespaces":
		p.CommandKey = "namespaces_list"
		return p, nil
	case "nodes":
		p.InfraOnly = true
		p.CommandKey = "nodes_list"
		return p, nil
	case "persistentvolumes":
		p.InfraOnly = true
		p.CommandKey = "pv_list"
		return p, nil
	case "storageclasses":
		p.InfraOnly = true
		p.CommandKey = "storageclasses_list"
		return p, nil
	case "events":
		if p.Namespace == "" {
			return Parsed{}, fmt.Errorf("get events cần -n <namespace>")
		}
		if strings.Contains(strings.ToLower(strings.Join(tokens, " ")), "type=warning") {
			p.CommandKey = "events_warnings"
		} else {
			p.CommandKey = "events_list"
		}
		return p, nil
	default:
		if p.Namespace == "" {
			return Parsed{}, fmt.Errorf("get %s cần -n <namespace>", res)
		}
		return p, nil
	}
}

func listCommandKey(key string) string {
	switch key {
	case "persistentvolumeclaims":
		return "pvc_list"
	case "horizontalpodautoscalers":
		return "hpa_list"
	case "persistentvolumes":
		return "pv_list"
	default:
		return key + "_list"
	}
}

func parseDescribe(tokens []string, p Parsed) (Parsed, error) {
	if len(tokens) < 2 {
		return Parsed{}, fmt.Errorf("describe cần loại resource và tên")
	}
	kind := strings.ToLower(tokens[1])
	name := ""
	if len(tokens) >= 3 && !strings.HasPrefix(tokens[2], "-") {
		name = tokens[2]
	}
	if name == "" {
		return Parsed{}, fmt.Errorf("describe cần tên resource")
	}
	p.Action = "describe"
	p.Name = name

	key, ok := allowedGetResources[kind]
	if !ok {
		return Parsed{}, fmt.Errorf("describe %q chưa hỗ trợ", kind)
	}
	p.Resource = key
	p.CommandKey = describeCommandKey(key)

	if key == "nodes" {
		p.InfraOnly = true
		return p, nil
	}
	if p.Namespace == "" {
		return Parsed{}, fmt.Errorf("describe cần -n <namespace>")
	}
	return p, nil
}

func describeCommandKey(key string) string {
	switch key {
	case "services":
		return "services_describe"
	case "ingresses":
		return "ingresses_describe"
	case "configmaps":
		return "configmaps_describe"
	case "secrets":
		return "secrets_describe"
	case "jobs":
		return "jobs_describe"
	case "statefulsets":
		return "statefulsets_describe"
	case "nodes":
		return "nodes_describe"
	default:
		if key == "pods" {
			return "pods_describe"
		}
		if key == "deployments" {
			return "deployments_describe"
		}
		return key + "_describe"
	}
}

func parseLogs(tokens []string, p Parsed) (Parsed, error) {
	if len(tokens) < 2 {
		return Parsed{}, fmt.Errorf("logs cần tên pod")
	}
	name := tokens[1]
	if strings.HasPrefix(name, "-") {
		return Parsed{}, fmt.Errorf("logs cần tên pod")
	}
	if strings.Contains(strings.ToLower(strings.Join(tokens, " ")), " -f") ||
		flagHas(tokens, "-f") {
		return Parsed{}, fmt.Errorf("logs -f chưa stream — dùng mẫu copy hoặc SSH")
	}
	if p.Namespace == "" {
		return Parsed{}, fmt.Errorf("logs cần -n <namespace>")
	}
	p.Action = "logs"
	p.Resource = "pods"
	p.Name = name
	if p.Container != "" {
		p.CommandKey = "pods_logs_container"
	} else {
		p.CommandKey = "pods_logs"
	}
	return p, nil
}

func flagHas(tokens []string, flag string) bool {
	for _, t := range tokens {
		if t == flag {
			return true
		}
	}
	return false
}

func flagValue(tokens []string, flags ...string) string {
	for i, t := range tokens {
		for _, f := range flags {
			if t == f && i+1 < len(tokens) {
				return tokens[i+1]
			}
			if strings.HasPrefix(t, f+"=") {
				return strings.TrimPrefix(t, f+"=")
			}
		}
	}
	return ""
}

var spaceSplit = regexp.MustCompile(`\s+`)

func tokenize(s string) []string {
	parts := spaceSplit.Split(strings.TrimSpace(s), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
