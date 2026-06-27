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
	RegistryProvider string
	Registry         registry.ProjectRegistry
	GitURL           string
	Branch           string
	BuildMode        string // dockerfile | buildpack
	DockerfilePath   string
	BuildContext     string
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
}

func (p Params) imageRef() string {
	tag := strings.TrimSpace(p.ImageTag)
	if tag == "" {
		tag = "latest"
	}
	prefix := strings.TrimSpace(p.Registry.ImagePrefix)
	if prefix == "" {
		prefix = "YOUR_REGISTRY/" + p.ProjectSlug
	}
	return prefix + "/app:" + tag
}

func (p Params) appName() string {
	return "app"
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
