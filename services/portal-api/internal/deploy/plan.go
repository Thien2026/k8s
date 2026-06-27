package deploy

import (
	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

// Plan tổng hợp bước Git → image → deploy cho khách.
type Plan struct {
	Environment   string   `json:"environment"`
	Namespace     string   `json:"namespace"`
	Image         string   `json:"image"`
	Workflow      Workflow `json:"workflow"`
	Manifest      Manifest `json:"manifest"`
	Steps         []string `json:"steps"`
	CanApply      bool     `json:"can_apply"`
	RancherReady  bool     `json:"rancher_ready"`
	RegistryReady bool     `json:"registry_ready"`
}

func BuildPlan(p Params, rancherReady, registryReady, canApply bool) (Plan, error) {
	manifest, err := K8sManifest(p)
	if err != nil {
		return Plan{}, err
	}
	wf := GitHubWorkflow(p)
	steps := []string{
		"Thêm file workflow vào repo Git (copy bên dưới vào " + wf.Filename + ").",
		"Cấu hình secrets GitHub Actions theo gợi ý.",
		"Push code lên branch " + p.branch() + " — CI build và push image " + p.imageRef() + ".",
		"Deploy workload vào namespace " + p.Namespace + " (nút Deploy trong Console hoặc kubectl apply manifest).",
	}
	if p.RegistryProvider == registry.Harbor {
		steps = append(steps, "Platform tự tạo imagePullSecret Harbor trong namespace khi deploy.")
	} else {
		steps = append(steps, "Platform tự tạo imagePullSecret GHCR (platform-ghcr) trong namespace khi deploy.")
	}
	return Plan{
		Environment:   p.Environment,
		Namespace:     p.Namespace,
		Image:         p.imageRef(),
		Workflow:      wf,
		Manifest:      manifest,
		Steps:         steps,
		CanApply:      canApply,
		RancherReady:  rancherReady,
		RegistryReady: registryReady,
	}, nil
}
