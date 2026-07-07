package deploy

import "testing"

func TestContainerResources_None(t *testing.T) {
	if got := ContainerResources(ServiceDef{Name: "web", ResourcesMode: ResourcesNone}); got != nil {
		t.Fatalf("none mode want nil, got %v", got)
	}
}

func TestContainerResources_Custom(t *testing.T) {
	got := ContainerResources(ServiceDef{
		Name:          "web",
		ResourcesMode: ResourcesCustom,
		CPURequest:    "200m",
		MemoryRequest: "256Mi",
		CPULimit:      "1",
		MemoryLimit:   "1Gi",
	})
	req := got["requests"].(map[string]string)
	if req["cpu"] != "200m" || req["memory"] != "256Mi" {
		t.Fatalf("custom requests: %v", req)
	}
}

func TestContainerResources_PlatformWeb(t *testing.T) {
	got := ContainerResources(ServiceDef{Name: "web"})
	lim := got["limits"].(map[string]string)
	if lim["memory"] != "768Mi" {
		t.Fatalf("platform web memory limit want 768Mi, got %v", lim)
	}
}
