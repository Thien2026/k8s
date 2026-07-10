package gitops

import (
	"strings"
	"testing"
)

func TestUpdateOverlayImageTags(t *testing.T) {
	in := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: demo-prod
resources:
  - ../../base
images:
  - name: harbor.example/demo/api
    newTag: oldsha111111
  - name: harbor.example/demo/web
    newTag: oldsha111111
`
	out, err := RewriteOverlayImagesSection(in, []string{
		"harbor.example/demo/api",
		"harbor.example/demo/web",
	}, "newsha222222")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "newTag: newsha222222") {
		t.Fatalf("expected updated tags, got:\n%s", out)
	}
	if !strings.Contains(out, "harbor.example/demo/api") {
		t.Fatalf("expected repo without tag in name, got:\n%s", out)
	}
}

func TestEnvFromPatchYAML(t *testing.T) {
	out := EnvFromPatchYAML("app-env")
	if !strings.Contains(out, "op: add") || !strings.Contains(out, "envFrom") || !strings.Contains(out, "app-env") {
		t.Fatalf("expected JSON6902 envFrom patch, got:\n%s", out)
	}
}

func TestOverlayPatchPath(t *testing.T) {
	got := OverlayPatchPath("apps", "shop", "prod", pullSecretPatchFile)
	want := "apps/shop/overlays/prod/pull-secret-patch.yaml"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPullSecretPatchYAML(t *testing.T) {
	out := PullSecretPatchYAML("platform-harbor")
	if !strings.Contains(out, "op: add") || !strings.Contains(out, "imagePullSecrets") {
		t.Fatalf("expected JSON6902 pull secret patch, got:\n%s", out)
	}
}

func TestConsolidateOverlayKustomization(t *testing.T) {
	in := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: demo-prod
resources:
  - ../../base
patches:
  - path: env-from-patch.yaml
    target:
      kind: Deployment
patches:
  - path: pull-secret-patch.yaml
    target:
      kind: Deployment
    target:
      kind: Deployment
images:
  - name: harbor.example/demo/api
    newTag: abc123
`
	out := ConsolidateOverlayKustomization(in)
	if strings.Count(out, "patches:") != 1 {
		t.Fatalf("expected single patches block, got:\n%s", out)
	}
	if !strings.Contains(out, "pull-secret-patch.yaml") || !strings.Contains(out, "env-from-patch.yaml") {
		t.Fatalf("expected both patch files, got:\n%s", out)
	}
}

func TestEnsureOverlayPullSecret(t *testing.T) {
	in := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: demo-prod
resources:
  - ../../base
images:
  - name: harbor.example/demo/api
    newTag: latest
`
	out, err := EnsureOverlayPullSecret(in, "platform-harbor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, pullSecretPatchFile) || !strings.Contains(out, "kind: Deployment") {
		t.Fatalf("expected pull secret patch refs, got:\n%s", out)
	}
}
