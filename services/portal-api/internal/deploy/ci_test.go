package deploy

import (
	"strings"
	"testing"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

func TestGitHubWorkflowBuildpack(t *testing.T) {
	wf := GitHubWorkflow(Params{
		ProjectSlug:      "demo",
		Branch:           "main",
		BuildMode:        "buildpack",
		BuildContext:     ".",
		RegistryProvider: registry.Harbor,
		Registry: registry.ProjectRegistry{
			ImagePrefix: "harbor.example.com/demo",
		},
		HarborHost: "harbor.example.com",
		BuildArgs: []BuildArg{
			{Key: "BUILD_LABEL", Value: "hello", IsSecret: false},
		},
	})
	if !strings.Contains(wf.Content, "buildpacks/github-actions/setup-pack") {
		t.Fatal("expected setup-pack in workflow")
	}
	if !strings.Contains(wf.Content, "pack build") {
		t.Fatal("expected pack build in workflow")
	}
	if strings.Contains(wf.Content, "docker/build-push-action") {
		t.Fatal("docker build-push should not appear in buildpack mode")
	}
	if !strings.Contains(wf.Content, `BUILD_LABEL=hello`) {
		t.Fatal("expected BUILD_LABEL in buildpack env")
	}
	if !strings.Contains(wf.Content, "PORT=8080") {
		t.Fatal("expected PORT=8080 for K8s manifest")
	}
	if !strings.Contains(wf.Content, buildpackBuilderImage) {
		t.Fatal("expected jammy-full builder for buildpack")
	}
	if !strings.Contains(wf.Content, "BP_NODE_VERSION=20") {
		t.Fatal("expected pinned Node LTS for buildpack")
	}
}

func TestGitHubWorkflowSubmodules(t *testing.T) {
	wf := GitHubWorkflow(Params{
		ProjectSlug:      "demo",
		Branch:           "main",
		GitSubmodules:    "recursive",
		BuildMode:        "dockerfile",
		RegistryProvider: registry.GHCR,
		Registry: registry.ProjectRegistry{
			ImagePrefix: "ghcr.io/org/demo",
		},
	})
	if !strings.Contains(wf.Content, "submodules: recursive") {
		t.Fatal("expected submodules checkout")
	}
	if !strings.Contains(wf.Content, "token: ${{ secrets.GITHUB_TOKEN }}") {
		t.Fatal("expected GITHUB_TOKEN for submodule checkout")
	}
}

func TestNormalizeGitSubmodules(t *testing.T) {
	if NormalizeGitSubmodules("recursive") != "recursive" {
		t.Fatal("recursive")
	}
	if NormalizeGitSubmodules("") != "" {
		t.Fatal("empty")
	}
}

func TestGitHubWorkflowDockerfile(t *testing.T) {
	wf := GitHubWorkflow(Params{
		ProjectSlug:      "demo",
		Branch:           "main",
		BuildMode:        "dockerfile",
		DockerfilePath:   "Dockerfile",
		RegistryProvider: registry.GHCR,
		Registry: registry.ProjectRegistry{
			ImagePrefix: "ghcr.io/org/demo",
		},
	})
	if !strings.Contains(wf.Content, "docker/build-push-action") {
		t.Fatal("expected docker build-push in dockerfile mode")
	}
	if strings.Contains(wf.Content, "pack build") {
		t.Fatal("buildpack should not appear in dockerfile mode")
	}
}

func TestGitHubWorkflowPathFilter(t *testing.T) {
	wf := GitHubWorkflow(Params{
		ProjectSlug:      "shop",
		Branch:           "main",
		Layout:           LayoutMulti,
		Services:         DefaultMultiServices,
		RegistryProvider: registry.Harbor,
		Registry: registry.ProjectRegistry{
			ImagePrefix: "harbor.example.com/shop",
		},
		HarborHost: "harbor.example.com",
	})
	if !strings.Contains(wf.Content, "dorny/paths-filter@v3") {
		t.Fatal("expected paths-filter for multi-service")
	}
	if !strings.Contains(wf.Content, "Retag api (unchanged)") {
		t.Fatal("expected retag step for api")
	}
	if !strings.Contains(wf.Content, "if: steps.changes.outputs.api == 'true'") {
		t.Fatal("expected conditional build for api")
	}
}

func TestNormalizeBuildMode(t *testing.T) {
	if NormalizeBuildMode("buildpack") != "buildpack" {
		t.Fatal("buildpack")
	}
	if NormalizeBuildMode("") != "dockerfile" {
		t.Fatal("default dockerfile")
	}
}

func TestGitHubWorkflowGitOpsSyncStep(t *testing.T) {
	wf := GitHubWorkflow(Params{
		ProjectSlug:      "shop",
		Branch:           "main",
		Layout:           LayoutMulti,
		Services:         DefaultMultiServices,
		RegistryProvider: registry.GHCR,
		Registry: registry.ProjectRegistry{
			ImagePrefix: "ghcr.io/org/shop",
		},
		DeployEnvironment: "dev",
		GitOpsRepoURL:     "https://github.com/org/company-gitops",
		GitOpsRepoBranch:  "main",
		GitOpsBasePath:    "apps",
	})
	if !strings.Contains(wf.Content, "Sync image tag to GitOps repo") {
		t.Fatal("expected gitops sync step in workflow")
	}
	if !strings.Contains(wf.Content, "PLATFORM_GITOPS_TOKEN") {
		t.Fatal("expected gitops token secret hint in workflow")
	}
	if !strings.Contains(wf.Content, "apps/shop/overlays/dev/kustomization.yaml") {
		t.Fatal("expected gitops overlay path")
	}
}
