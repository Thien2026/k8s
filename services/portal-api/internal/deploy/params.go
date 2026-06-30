package deploy

import (
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

// Params đầu vào sinh CI workflow và manifest K8s cho một project.
type Params struct {
	ProjectSlug      string
	ProjectName      string
	Namespace        string
	Environment      string
	Layout           string // single | multi
	RegistryProvider string
	Registry         registry.ProjectRegistry
	GitURL           string
	Branch           string
	GitSubmodules    string // "" | true | recursive — actions/checkout submodules
	BuildMode        string // dockerfile | buildpack (single-service fallback)
	Stack            string // python | node | go | dotnet — buildpack hint
	DockerfilePath   string
	BuildContext     string
	Services         []ServiceDef
	ImageTag         string
	HarborHost       string
	ImagePullSecret  string
	AppEnvFromSecret string
	DeployHookURL       string
	DeployEnvironment   string
	DeployTokenSecret   string
	HarborUserSecret    string
	HarborPassSecret    string
	BuildArgs           []BuildArg
	ForceRolloutRestart bool // rollback / redeploy — ép pod mới dù spec image trùng
}

func (p Params) ImageRef() string {
	return p.imageRef()
}

func (p Params) imageRef() string {
	svcs := p.EffectiveServices()
	if len(svcs) == 1 {
		return p.imageRefFor(svcs[0])
	}
	refs := make([]string, 0, len(svcs))
	for _, s := range svcs {
		refs = append(refs, p.imageRefFor(s))
	}
	return strings.Join(refs, ", ")
}

func (p Params) appName() string {
	svcs := p.EffectiveServices()
	if len(svcs) == 0 {
		return "app"
	}
	return svcs[0].Name
}

func (p Params) deploymentName() string {
	return p.appName()
}

func (p Params) branch() string {
	b := strings.TrimSpace(p.Branch)
	if b == "" {
		return "main"
	}
	return b
}

// NormalizeBuildMode returns dockerfile or buildpack (stored after auto-detect).
func NormalizeBuildMode(mode string) string {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "buildpack" {
		return "buildpack"
	}
	return "dockerfile"
}

func (p Params) buildMode() string {
	return NormalizeBuildMode(p.BuildMode)
}

func (p Params) usesBuildpack() bool {
	return p.buildMode() == "buildpack"
}

func (p Params) dockerfile() string {
	df := strings.TrimSpace(p.DockerfilePath)
	if df == "" {
		return "Dockerfile"
	}
	return df
}

func (p Params) buildContext() string {
	ctx := strings.TrimSpace(p.BuildContext)
	if ctx == "" {
		return "."
	}
	return ctx
}

func (p Params) deployTokenSecret() string {
	if s := strings.TrimSpace(p.DeployTokenSecret); s != "" {
		return s
	}
	return DeployTokenSecretName(p.ProjectSlug)
}

func (p Params) harborUserSecret() string {
	if s := strings.TrimSpace(p.HarborUserSecret); s != "" {
		return s
	}
	return HarborUsernameSecretName(p.ProjectSlug)
}

func (p Params) harborPassSecret() string {
	if s := strings.TrimSpace(p.HarborPassSecret); s != "" {
		return s
	}
	return HarborPasswordSecretName(p.ProjectSlug)
}
