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
	writeCheckoutStep(&b, p)
	b.WriteString("\n")
	writePathsFilterStep(&b, svcs)

	if strings.TrimSpace(p.DeployHookURL) != "" {
		env := strings.TrimSpace(p.DeployEnvironment)
		if env == "" {
			env = "dev"
		}
		validateURL := strings.TrimRight(strings.TrimSpace(p.DeployHookURL), "/") + "/validate-config"
		b.WriteString("      - name: Kiểm tra cấu hình env (Platform)\n")
		b.WriteString("        run: |\n")
		b.WriteString("          CODE=$(curl -sS -o /tmp/platform-validate.json -w '%{http_code}' -X POST \"" + validateURL + "\" \\\n")
		b.WriteString("            -H \"Content-Type: application/json\" \\\n")
		b.WriteString("            -H \"X-Platform-Deploy-Token: ${{ secrets." + p.deployTokenSecret() + " }}\" \\\n")
		b.WriteString("            -d '{\"environment\":\"" + env + "\"}')\n")
		b.WriteString("          if [ \"$CODE\" != \"200\" ]; then\n")
		b.WriteString("            echo \"Platform validate-config HTTP $CODE:\"\n")
		b.WriteString("            cat /tmp/platform-validate.json\n")
		b.WriteString("            exit 22\n")
		b.WriteString("          fi\n\n")
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

	multiPathFilter := len(svcs) > 1
	if multiPathFilter {
		for _, svc := range svcs {
			writeRetagServiceStep(&b, svc, p.imageRefFor(svc))
		}
	}

	for _, svc := range svcs {
		sp := serviceParams(p, svc)
		image := p.imageRefFor(svc)
		stepName := svc.Name
		if svc.DisplayName != "" {
			stepName = svc.DisplayName
		}
		if multiPathFilter {
			b.WriteString("      - name: Build and push " + stepName + "\n")
			b.WriteString("        if: " + serviceBuildIf(svc.Name) + "\n")
		} else {
			b.WriteString("      - name: Build and push " + stepName + "\n")
		}
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
	if gitOpsWorkflowEnabled(p) {
		writeGitOpsSyncStep(&b, p, svcs)
		secrets = append(secrets,
			p.gitOpsTokenSecret()+" — PAT có quyền ghi repo GitOps",
		)
	}

	return Workflow{
		Filename:    filename,
		Content:     b.String(),
		SecretsHint: secrets,
	}
}

func gitOpsWorkflowEnabled(p Params) bool {
	return strings.TrimSpace(p.GitOpsRepoURL) != "" &&
		strings.TrimSpace(p.GitOpsRepoBranch) != "" &&
		strings.TrimSpace(p.GitOpsBasePath) != ""
}

func writeGitOpsSyncStep(b *strings.Builder, p Params, svcs []ServiceDef) {
	branch := strings.TrimSpace(p.GitOpsRepoBranch)
	if branch == "" {
		branch = "main"
	}
	env := strings.TrimSpace(p.DeployEnvironment)
	if env == "" {
		env = "dev"
	}
	overlayPath := strings.Trim(strings.TrimSpace(p.GitOpsBasePath), "/") + "/" + strings.TrimSpace(p.ProjectSlug) + "/overlays/" + env + "/kustomization.yaml"
	b.WriteString("      - name: Sync image tag to GitOps repo\n")
	b.WriteString("        env:\n")
	b.WriteString("          GITOPS_REPO_URL: " + p.GitOpsRepoURL + "\n")
	b.WriteString("          GITOPS_REPO_BRANCH: " + branch + "\n")
	b.WriteString("          GITOPS_FILE: " + overlayPath + "\n")
	b.WriteString("          GITOPS_TOKEN: ${{ secrets." + p.gitOpsTokenSecret() + " }}\n")
	b.WriteString("        run: |\n")
	b.WriteString("          set -euo pipefail\n")
	b.WriteString("          tmp_dir=\"$(mktemp -d)\"\n")
	b.WriteString("          git -c init.defaultBranch=main clone --depth 1 --branch \"$GITOPS_REPO_BRANCH\" \"https://x-access-token:${GITOPS_TOKEN}@${GITOPS_REPO_URL#https://}\" \"$tmp_dir/gitops\"\n")
	b.WriteString("          cd \"$tmp_dir/gitops\"\n")
	b.WriteString("          if [ ! -f \"$GITOPS_FILE\" ]; then\n")
	b.WriteString("            echo \"GitOps file not found: $GITOPS_FILE\" >&2\n")
	b.WriteString("            exit 2\n")
	b.WriteString("          fi\n")
	b.WriteString("          export GITOPS_FILE\n")
	b.WriteString("          export NEW_TAG=\"${{ github.sha }}\"\n")
	for _, svc := range svcs {
		b.WriteString("          export IMG_" + strings.ToUpper(svc.Name) + "=" + p.ImageRepositoryFor(svc) + "\n")
	}
	// Python trong heredoc phải thụt cùng cấp shell — nếu không YAML workflow invalid và GH Actions fail ngay.
	const py = "          "
	b.WriteString(py + "python3 - <<'PY'\n")
	b.WriteString(py + "from pathlib import Path\n")
	b.WriteString(py + "import os\n")
	b.WriteString(py + "f = Path(os.environ['GITOPS_FILE'])\n")
	b.WriteString(py + "lines = f.read_text().splitlines()\n")
	b.WriteString(py + "new_tag = os.environ['NEW_TAG']\n")
	b.WriteString(py + "targets = {\n")
	for _, svc := range svcs {
		b.WriteString(py + "    os.environ['IMG_" + strings.ToUpper(svc.Name) + "']: True,\n")
	}
	b.WriteString(py + "}\n")
	b.WriteString(py + "for i, line in enumerate(lines):\n")
	b.WriteString(py + "    s = line.strip()\n")
	b.WriteString(py + "    if not s.startswith('name:'):\n")
	b.WriteString(py + "        continue\n")
	b.WriteString(py + "    name = s.split(':', 1)[1].strip()\n")
	b.WriteString(py + "    if name not in targets:\n")
	b.WriteString(py + "        continue\n")
	b.WriteString(py + "    indent = line[:len(line)-len(line.lstrip())]\n")
	b.WriteString(py + "    j = i + 1\n")
	b.WriteString(py + "    replaced = False\n")
	b.WriteString(py + "    while j < len(lines):\n")
	b.WriteString(py + "        cur = lines[j].strip()\n")
	b.WriteString(py + "        if cur.startswith('- name:') or (cur.startswith('name:') and lines[j].startswith(indent)):\n")
	b.WriteString(py + "            break\n")
	b.WriteString(py + "        if cur.startswith('newTag:'):\n")
	b.WriteString(py + "            lines[j] = indent + '  newTag: ' + new_tag\n")
	b.WriteString(py + "            replaced = True\n")
	b.WriteString(py + "            break\n")
	b.WriteString(py + "        j += 1\n")
	b.WriteString(py + "    if not replaced:\n")
	b.WriteString(py + "        lines.insert(i + 1, indent + '  newTag: ' + new_tag)\n")
	b.WriteString(py + "f.write_text('\\n'.join(lines) + '\\n')\n")
	b.WriteString(py + "PY\n")
	b.WriteString("          git config user.name \"platform-bot\"\n")
	b.WriteString("          git config user.email \"platform-bot@users.noreply.github.com\"\n")
	b.WriteString("          git add \"$GITOPS_FILE\"\n")
	b.WriteString("          if git diff --cached --quiet; then\n")
	b.WriteString("            echo \"No GitOps change\"\n")
	b.WriteString("            exit 0\n")
	b.WriteString("          fi\n")
	b.WriteString("          git commit -m \"chore(gitops): " + p.ProjectSlug + " " + env + " -> ${GITHUB_SHA::12}\"\n")
	b.WriteString("          git push origin \"HEAD:$GITOPS_REPO_BRANCH\"\n\n")
}

func writeCheckoutStep(b *strings.Builder, p Params) {
	b.WriteString("      - uses: actions/checkout@v4\n")
	mode := NormalizeGitSubmodules(p.GitSubmodules)
	if mode == "" {
		return
	}
	b.WriteString("        with:\n")
	b.WriteString("          submodules: " + mode + "\n")
	b.WriteString("          token: ${{ secrets.GITHUB_TOKEN }}\n")
}

// NormalizeGitSubmodules chuẩn hóa giá trị lưu DB / contract cho checkout.
func NormalizeGitSubmodules(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "true", "yes", "1", "on":
		return "true"
	case "recursive", "recurse":
		return "recursive"
	default:
		return ""
	}
}

func serviceParams(p Params, svc ServiceDef) Params {
	sp := p
	sp.BuildMode = svc.BuildMode
	sp.Stack = svc.Stack
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

const buildpackBuilderImage = defaultBuildpackBuilder

func writeBuildpackBuildStep(b *strings.Builder, p Params, image string) {
	b.WriteString("        run: |\n")
	imageBase := image
	if i := strings.LastIndex(image, ":"); i > 0 {
		imageBase = image[:i]
	}
	ctx := p.buildContext()
	builder := BuildpackBuilderForStack(p.Stack)
	b.WriteString("          pack build \"" + image + "\" \\\n")
	b.WriteString("            --path \"" + ctx + "\" \\\n")
	b.WriteString("            --builder " + builder + " \\\n")
	b.WriteString("            --publish \\\n")
	b.WriteString("            --tag \"" + imageBase + ":${{ github.sha }}\" \\\n")
	b.WriteString("            --env \"GIT_SHA=${{ github.sha }}\" \\\n")
	b.WriteString("            --env \"GIT_REF=${{ github.ref_name }}\" \\\n")
	b.WriteString("            --env \"PORT=8080\"")
	for _, extra := range BuildpackExtraEnv(p.Stack) {
		b.WriteString(" \\\n            --env \"" + extra + "\"")
	}
	if NormalizeStack(p.Stack) == "" {
		b.WriteString(" \\\n            --env \"BP_NODE_VERSION=20\"")
	}
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
