package deploy

import "strings"

const (
	ResourcesPlatform = "platform"
	ResourcesNone     = "none"
	ResourcesCustom   = "custom"
)

// NormalizeResourcesMode — platform | none | custom.
func NormalizeResourcesMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ResourcesNone:
		return ResourcesNone
	case ResourcesCustom:
		return ResourcesCustom
	default:
		return ResourcesPlatform
	}
}

// ContainerResources — requests/limits cho manifest; nil = không inject (mode none).
func ContainerResources(svc ServiceDef) map[string]any {
	svc = normalizeServiceDef(svc)
	switch NormalizeResourcesMode(svc.ResourcesMode) {
	case ResourcesNone:
		return nil
	case ResourcesCustom:
		if res := customContainerResources(svc); res != nil {
			return res
		}
	}
	return platformDefaultResources(svc)
}

func customContainerResources(svc ServiceDef) map[string]any {
	cpuReq := strings.TrimSpace(svc.CPURequest)
	memReq := strings.TrimSpace(svc.MemoryRequest)
	cpuLim := strings.TrimSpace(svc.CPULimit)
	memLim := strings.TrimSpace(svc.MemoryLimit)
	if cpuReq == "" && memReq == "" && cpuLim == "" && memLim == "" {
		return nil
	}
	req := map[string]string{}
	lim := map[string]string{}
	if cpuReq != "" {
		req["cpu"] = cpuReq
	}
	if memReq != "" {
		req["memory"] = memReq
	}
	if cpuLim != "" {
		lim["cpu"] = cpuLim
	}
	if memLim != "" {
		lim["memory"] = memLim
	}
	out := map[string]any{}
	if len(req) > 0 {
		out["requests"] = req
	}
	if len(lim) > 0 {
		out["limits"] = lim
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func platformDefaultResources(svc ServiceDef) map[string]any {
	svc = normalizeServiceDef(svc)
	name := strings.ToLower(strings.TrimSpace(svc.Name))

	cpuReq, memReq := "100m", "128Mi"
	cpuLim, memLim := "500m", "512Mi"

	switch name {
	case "web":
		cpuReq, memReq = "50m", "128Mi"
		cpuLim, memLim = "500m", "768Mi"
	case "worker":
		cpuReq, memReq = "100m", "256Mi"
		cpuLim, memLim = "1000m", "1Gi"
	case "dotnet":
		cpuReq, memReq = "100m", "256Mi"
		cpuLim, memLim = "1000m", "1Gi"
	case "node":
		cpuReq, memReq = "100m", "192Mi"
		cpuLim, memLim = "750m", "768Mi"
	}

	return map[string]any{
		"requests": map[string]string{
			"cpu":    cpuReq,
			"memory": memReq,
		},
		"limits": map[string]string{
			"cpu":    cpuLim,
			"memory": memLim,
		},
	}
}

// ServiceResourcesFromDef copy resource fields onto ServiceDef.
func ServiceResourcesFromDef(dst *ServiceDef, mode, cpuReq, memReq, cpuLim, memLim string) {
	if dst == nil {
		return
	}
	dst.ResourcesMode = NormalizeResourcesMode(mode)
	dst.CPURequest = strings.TrimSpace(cpuReq)
	dst.MemoryRequest = strings.TrimSpace(memReq)
	dst.CPULimit = strings.TrimSpace(cpuLim)
	dst.MemoryLimit = strings.TrimSpace(memLim)
}
