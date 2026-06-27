package deploy

import (
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

// Plan tổng hợp bước Git → image → deploy cho khách.
type Plan struct {
	Environment   string     `json:"environment"`
	Namespace     string     `json:"namespace"`
	Layout        string     `json:"layout"`
	Image         string     `json:"image"`
	Services      []string   `json:"services,omitempty"`
	Workflow      Workflow   `json:"workflow"`
	Manifest      Manifest   `json:"manifest"`
	Manifests     []Manifest `json:"manifests,omitempty"`
	Steps         []string   `json:"steps"`
	CanApply      bool       `json:"can_apply"`
	RancherReady  bool       `json:"rancher_ready"`
	RegistryReady bool       `json:"registry_ready"`
}

func BuildPlan(p Params, rancherReady, registryReady, canApply bool) (Plan, error) {
	manifests, err := K8sManifests(p)
	if err != nil {
		return Plan{}, err
	}
	manifest := Manifest{}
	if len(manifests) > 0 {
		manifest = manifests[0]
	}
	wf := GitHubWorkflow(p)
	svcs := p.EffectiveServices()
	serviceNames := make([]string, 0, len(svcs))
	for _, s := range svcs {
		serviceNames = append(serviceNames, s.Name)
	}
	imageDesc := p.imageRef()
	steps := []string{
		"Thêm file workflow vào repo Git (copy bên dưới vào " + wf.Filename + ").",
		"Cấu hình secrets GitHub Actions theo gợi ý.",
	}
	if p.IsMultiService() {
		steps = append(steps,
			"Push code lên branch "+p.branch()+" — CI build và push "+fmt.Sprintf("%d image", len(svcs))+" ("+strings.Join(serviceNames, ", ")+") cùng tag SHA.",
			"Deploy "+fmt.Sprintf("%d workload", len(svcs))+" vào namespace "+p.Namespace+" (Ingress: /api → api, / → web).",
		)
	} else {
		steps = append(steps,
			"Push code lên branch "+p.branch()+" — CI build và push image "+imageDesc+".",
			"Deploy workload vào namespace "+p.Namespace+" (nút Deploy trong Console hoặc kubectl apply manifest).",
		)
	}
	if p.RegistryProvider == registry.Harbor {
		steps = append(steps, "Platform tự tạo imagePullSecret Harbor trong namespace khi deploy.")
	} else {
		steps = append(steps, "Platform tự tạo imagePullSecret GHCR (platform-ghcr) trong namespace khi deploy.")
	}
	return Plan{
		Environment:   p.Environment,
		Namespace:     p.Namespace,
		Layout:        NormalizeLayout(p.Layout),
		Image:         imageDesc,
		Services:      serviceNames,
		Workflow:      wf,
		Manifest:      manifest,
		Manifests:     manifests,
		Steps:         steps,
		CanApply:      canApply,
		RancherReady:  rancherReady,
		RegistryReady: registryReady,
	}, nil
}
