package deploy

import (
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/registry"
)

// Workflow gợi ý GitHub Actions cho project.
type Workflow struct {
	Filename    string   `json:"filename"`
	Content     string   `json:"content"`
	SecretsHint []string `json:"secrets_hint,omitempty"`
}

func GitHubWorkflow(p Params) Workflow {
	filename := ".github/workflows/platform-deploy-" + p.ProjectSlug + ".yml"
	var b strings.Builder
	svcs := p.EffectiveServices()

	b.WriteString("name: Build and push (" + p.ProjectSlug + ")\n\n")
	b.WriteString("on:\n")
	b.WriteString("  push:\n")
	b.WriteString("    branches: [" + quoteYAML(p.branch()) + "]\n")
	b.WriteString("  workflow_dispatch:\n\n")
	b.WriteString("env:\n")
	if len(svcs) == 1 {
		b.WriteString("  IMAGE: " + p.imageRefFor(svcs[0]) + "\n\n")
	} else {
		for _, svc := range svcs {
			key := strings.ToUpper(svc.Name) + "_IMAGE"
			b.WriteString("  " + key + ": " + p.imageRefFor(svc) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("jobs:\n")
	b.WriteString("  build:\n")
	b.WriteString("    runs-on: ubuntu-latest\n")
	b.WriteString("    permissions:\n")
	b.WriteString("      contents: read\n")
	if p.RegistryProvider == registry.GHCR {
		b.WriteString("      packages: write\n")
	}
	b.WriteString("    steps:\n")
	b.WriteString("      - uses: actions/checkout@v4\n\n")

	if strings.TrimSpace(p.DeployHookURL) != "" {
		env := strings.TrimSpace(p.DeployEnvironment)
		if env == "" {
			env = "dev"
		}
		validateURL := strings.TrimRight(strings.TrimSpace(p.DeployHookURL), "/") + "/validate-config"
		b.WriteString("      - name: Kiểm tra cấu hình env (Platform)\n")
		b.WriteString("        run: |\n")
		b.WriteString("          curl -fsS -X POST \"" + validateURL + "\" \\\n")
		b.WriteString("            -H \"Content-Type: application/json\" \\\n")
		b.WriteString("            -H \"X-Platform-Deploy-Token: ${{ secrets." + p.deployTokenSecret() + " }}\" \\\n")
		b.WriteString("            -d '{\"environment\":\"" + env + "\"}'\n\n")
	}

	if strings.TrimSpace(p.DeployHookURL) != "" {
		env := strings.TrimSpace(p.DeployEnvironment)
		if env == "" {
			env = "dev"
		}
		eventURL := strings.TrimRight(strings.TrimSpace(p.DeployHookURL), "/") + "/event"
		b.WriteString("      - name: Notify platform (build started)\n")
		b.WriteString("        continue-on-error: true\n")
		b.WriteString("        run: |\n")
		b.WriteString("          curl -fsS -X POST \"" + eventURL + "\" \\\n")
		b.WriteString("            -H \"Content-Type: application/json\" \\\n")
		b.WriteString("            -H \"X-Platform-Deploy-Token: ${{ secrets." + p.deployTokenSecret() + " }}\" \\\n")
		b.WriteString("            -d '{\"event\":\"build_started\",\"image_tag\":\"'\"${{ github.sha }}\"'\",\"environment\":\"" + env + "\"}'\n\n")
	}

	if p.RegistryProvider == registry.Harbor {
		host := strings.TrimSpace(p.HarborHost)
		if host == "" {
			host = "harbor.example.com"
		}
		b.WriteString("      - name: Login Harbor\n")
		b.WriteString("        uses: docker/login-action@v3\n")
		b.WriteString("        with:\n")
		b.WriteString("          registry: " + host + "\n")
		b.WriteString("          username: ${{ secrets." + p.harborUserSecret() + " }}\n")
		b.WriteString("          password: ${{ secrets." + p.harborPassSecret() + " }}\n\n")
	} else {
		b.WriteString("      - name: Login GHCR\n")
		b.WriteString("        uses: docker/login-action@v3\n")
		b.WriteString("        with:\n")
		b.WriteString("          registry: ghcr.io\n")
		b.WriteString("          username: ${{ github.repository_owner }}\n")
		b.WriteString("          password: ${{ secrets.GITHUB_TOKEN }}\n\n")
	}

	needsPackSetup := false
	for _, svc := range svcs {
		sp := serviceParams(p, svc)
		if sp.usesBuildpack() {
			needsPackSetup = true
			break
		}
	}
	if needsPackSetup {
		b.WriteString("      - name: Setup pack (Buildpack)\n")
		b.WriteString("        uses: buildpacks/github-actions/setup-pack@v5.12.5\n\n")
	}

	for _, svc := range svcs {
		sp := serviceParams(p, svc)
		image := p.imageRefFor(svc)
		stepName := svc.Name
		if svc.DisplayName != "" {
			stepName = svc.DisplayName
		}
		b.WriteString("      - name: Build and push " + stepName + "\n")
		if sp.usesBuildpack() {
			writeBuildpackBuildStep(&b, sp, image)
		} else {
			b.WriteString("        uses: docker/build-push-action@v6\n")
			b.WriteString("        with:\n")
			writeDockerfileBuildWith(&b, sp, image)
		}
		b.WriteString("\n")
	}

	secrets := []string{}
	if p.RegistryProvider == registry.Harbor {
		secrets = []string{
			p.harborUserSecret() + " — platform tự inject khi kết nối GitHub",
			p.harborPassSecret() + " — platform tự inject khi kết nối GitHub",
		}
	} else {
		secrets = []string{
			"GITHUB_TOKEN — mặc định có sẵn, dùng push ghcr.io",
		}
	}

	if strings.TrimSpace(p.DeployHookURL) != "" {
		env := strings.TrimSpace(p.DeployEnvironment)
		if env == "" {
			env = "dev"
		}
		b.WriteString("      - name: Deploy to Platform\n")
		b.WriteString("        run: |\n")
		b.WriteString("          curl -fsS -X POST \"" + p.DeployHookURL + "\" \\\n")
		b.WriteString("            -H \"Content-Type: application/json\" \\\n")
		b.WriteString("            -H \"X-Platform-Deploy-Token: ${{ secrets." + p.deployTokenSecret() + " }}\" \\\n")
		b.WriteString("            -d '{\"image_tag\":\"'\"${{ github.sha }}\"'\",\"environment\":\"" + env + "\"}'\n")
		secrets = append(secrets, p.deployTokenSecret()+" — riêng từng project (monorepo an toàn)")
	}

	return Workflow{
		Filename:    filename,
		Content:     b.String(),
		SecretsHint: secrets,
	}
}

func serviceParams(p Params, svc ServiceDef) Params {
	sp := p
	sp.BuildMode = svc.BuildMode
	sp.BuildContext = svc.BuildContext
	sp.DockerfilePath = svc.DockerfilePath
	return sp
}

func quoteYAML(s string) string {
	if strings.ContainsAny(s, " \t#:[]{},") {
		return fmt.Sprintf("%q", s)
	}
	return s
}

func writeDockerfileBuildWith(b *strings.Builder, p Params, image string) {
	b.WriteString("          context: " + p.buildContext() + "\n")
	b.WriteString("          file: " + p.dockerfile() + "\n")
	b.WriteString("          push: true\n")
	if p.RegistryProvider == registry.GHCR {
		b.WriteString("          provenance: false\n")
		b.WriteString("          sbom: false\n")
	}
	b.WriteString("          tags: |\n")
	b.WriteString("            " + image + "\n")
	imageBase := image
	if i := strings.LastIndex(image, ":"); i > 0 {
		imageBase = image[:i]
	}
	b.WriteString("            " + imageBase + ":${{ github.sha }}\n")
	b.WriteString("          build-args: |\n")
	writeBuildInjectLines(b, p, "            ", true)
}

const buildpackBuilderImage = "paketobuildpacks/builder-jammy-full"

func writeBuildpackBuildStep(b *strings.Builder, p Params, image string) {
	b.WriteString("        run: |\n")
	imageBase := image
	if i := strings.LastIndex(image, ":"); i > 0 {
		imageBase = image[:i]
	}
	ctx := p.buildContext()
	b.WriteString("          pack build \"" + image + "\" \\\n")
	b.WriteString("            --path \"" + ctx + "\" \\\n")
	b.WriteString("            --builder " + buildpackBuilderImage + " \\\n")
	b.WriteString("            --publish \\\n")
	b.WriteString("            --tag \"" + imageBase + ":${{ github.sha }}\" \\\n")
	b.WriteString("            --env \"GIT_SHA=${{ github.sha }}\" \\\n")
	b.WriteString("            --env \"GIT_REF=${{ github.ref_name }}\" \\\n")
	b.WriteString("            --env \"PORT=8080\" \\\n")
	b.WriteString("            --env \"BP_NODE_VERSION=20\"")
	for _, arg := range p.BuildArgs {
		key := strings.TrimSpace(arg.Key)
		if key == "" || key == "GIT_SHA" || key == "GIT_REF" || key == "PORT" {
			continue
		}
		b.WriteString(" \\\n")
		if arg.IsSecret {
			secret := strings.TrimSpace(arg.SecretName)
			if secret == "" {
				secret = BuildArgSecretName(p.ProjectSlug, key)
			}
			b.WriteString("            --env \"" + key + "=${{ secrets." + secret + " }}\"")
		} else {
			b.WriteString("            --env \"" + key + "=" + yamlBuildArgValue(arg.Value) + "\"")
		}
	}
	b.WriteString("\n")
}

// writeBuildInjectLines emits GIT_SHA, GIT_REF and contract build vars (docker build-args or buildpack env).
func writeBuildInjectLines(b *strings.Builder, p Params, indent string, dockerArgs bool) {
	b.WriteString(indent + "GIT_SHA=${{ github.sha }}\n")
	b.WriteString(indent + "GIT_REF=${{ github.ref_name }}\n")
	for _, arg := range p.BuildArgs {
		key := strings.TrimSpace(arg.Key)
		if key == "" || key == "GIT_SHA" || key == "GIT_REF" || key == "PORT" {
			continue
		}
		if arg.IsSecret {
			secret := strings.TrimSpace(arg.SecretName)
			if secret == "" {
				secret = BuildArgSecretName(p.ProjectSlug, key)
			}
			b.WriteString(indent + key + "=${{ secrets." + secret + " }}\n")
		} else if dockerArgs {
			b.WriteString(indent + key + "=" + yamlBuildArgValue(arg.Value) + "\n")
		} else {
			b.WriteString(indent + key + "=" + yamlBuildArgValue(arg.Value) + "\n")
		}
	}
}
