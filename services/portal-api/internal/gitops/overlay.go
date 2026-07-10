package gitops

import (
	"fmt"
	"strings"
)

const pullSecretPatchFile = "pull-secret-patch.yaml"
const envFromPatchFile = "env-from-patch.yaml"

// RewriteOverlayImagesSection thay toàn bộ block images trong kustomization overlay.
func RewriteOverlayImagesSection(content string, repos []string, newTag string) (string, error) {
	newTag = strings.TrimSpace(newTag)
	if newTag == "" {
		return "", fmt.Errorf("image tag trống")
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)+len(repos)*2+2)
	inImages := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "images:" {
			inImages = true
			continue
		}
		if inImages {
			if trim == "" {
				continue
			}
			if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "name:") || strings.HasPrefix(trim, "newTag:") || strings.HasPrefix(trim, "newName:") {
				continue
			}
			inImages = false
		}
		if !inImages {
			out = append(out, line)
		}
	}
	out = append(out, "images:")
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo == "" {
			continue
		}
		if i := strings.Index(repo, ":"); i > 0 {
			repo = repo[:i]
		}
		out = append(out, "  - name: "+repo)
		out = append(out, "    newTag: "+newTag)
	}
	return strings.Join(out, "\n") + "\n", nil
}

// UpdateOverlayImageTags giữ tương thích — ưu tiên RewriteOverlayImagesSection.
func UpdateOverlayImageTags(content, newTag string, imageNames []string) (string, error) {
	return RewriteOverlayImagesSection(content, imageNames, newTag)
}

// PullSecretPatchFileName tên file patch imagePullSecrets trong overlay.
func PullSecretPatchFileName() string {
	return pullSecretPatchFile
}

// EnvFromPatchFileName tên file patch envFrom trong overlay.
func EnvFromPatchFileName() string {
	return envFromPatchFile
}

// PullSecretPatchYAML JSON6902 patch inject imagePullSecrets cho mọi Deployment.
func PullSecretPatchYAML(secretName string) string {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		return ""
	}
	return `- op: add
  path: /spec/template/spec/imagePullSecrets
  value:
    - name: ` + secretName + `
`
}

// OverlayDir thư mục overlay (không có kustomization.yaml).
func OverlayDir(basePath, slug, env string) string {
	return strings.TrimSuffix(OverlayPath(basePath, slug, env), "/kustomization.yaml")
}

// OverlayPatchPath đường dẫn file patch trong overlay.
func OverlayPatchPath(basePath, slug, env, patchFile string) string {
	return OverlayDir(basePath, slug, env) + "/" + strings.TrimPrefix(strings.TrimSpace(patchFile), "/")
}

// EnvFromPatchYAML JSON6902 patch inject envFrom app-env cho container đầu tiên mọi Deployment.
func EnvFromPatchYAML(secretName string) string {
	secretName = strings.TrimSpace(secretName)
	if secretName == "" {
		return ""
	}
	return `- op: add
  path: /spec/template/spec/containers/0/envFrom
  value:
    - secretRef:
        name: ` + secretName + `
`
}

func ensureOverlayPatchRef(content, patchFile string) (string, error) {
	patchFile = strings.TrimSpace(patchFile)
	if patchFile == "" || strings.Contains(content, patchFile) {
		return content, nil
	}
	item := []string{
		"  - path: " + patchFile,
		"    target:",
		"      kind: Deployment",
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	patchesStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "patches:" {
			patchesStart = i
			break
		}
	}
	if patchesStart < 0 {
		insertAt := len(lines)
		for i, line := range lines {
			if strings.TrimSpace(line) == "images:" {
				insertAt = i
				break
			}
		}
		block := append([]string{"patches:"}, item...)
		lines = append(lines[:insertAt], append(block, lines[insertAt:]...)...)
		return strings.Join(lines, "\n") + "\n", nil
	}
	insertAt := patchesStart + 1
	for i := patchesStart + 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			insertAt = i + 1
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
			insertAt = i
			break
		}
		insertAt = i + 1
	}
	lines = append(lines[:insertAt], append(item, lines[insertAt:]...)...)
	return ConsolidateOverlayKustomization(strings.Join(lines, "\n") + "\n"), nil
}

// ConsolidateOverlayKustomization gộp các block patches trùng và bỏ entry patch lỗi.
func ConsolidateOverlayKustomization(content string) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	var head, patchItems, tail []string
	inImages := false
	inPatches := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "images:" {
			inImages = true
			inPatches = false
			tail = append(tail, line)
			continue
		}
		if inImages {
			tail = append(tail, line)
			continue
		}
		if trim == "patches:" {
			inPatches = true
			continue
		}
		if inPatches {
			if trim == "" {
				continue
			}
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
				inPatches = false
				head = append(head, line)
				continue
			}
			patchItems = append(patchItems, line)
			continue
		}
		head = append(head, line)
	}
	if len(patchItems) == 0 {
		return strings.Join(append(head, tail...), "\n") + "\n"
	}
	seenPath := map[string]bool{}
	var cleaned []string
	for i := 0; i < len(patchItems); i++ {
		line := patchItems[i]
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- path:") {
			path := strings.TrimSpace(strings.TrimPrefix(trim, "- path:"))
			if seenPath[path] {
				continue
			}
			seenPath[path] = true
			cleaned = append(cleaned, line)
			if i+2 < len(patchItems) && strings.TrimSpace(patchItems[i+1]) == "target:" {
				cleaned = append(cleaned, patchItems[i+1], patchItems[i+2])
				i += 2
			}
			continue
		}
		if strings.HasPrefix(trim, "target:") || strings.HasPrefix(trim, "kind:") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	out := append([]string{}, head...)
	out = append(out, "patches:")
	out = append(out, cleaned...)
	out = append(out, tail...)
	return strings.Join(out, "\n") + "\n"
}

// EnsureOverlayPullSecret bổ sung patches pull-secret vào kustomization nếu chưa có.
func EnsureOverlayPullSecret(content, secretName string) (string, error) {
	if strings.TrimSpace(secretName) == "" {
		return content, nil
	}
	return ensureOverlayPatchRef(content, pullSecretPatchFile)
}

// EnsureOverlayEnvFrom bổ sung patches env-from vào kustomization nếu chưa có.
func EnsureOverlayEnvFrom(content, secretName string) (string, error) {
	if strings.TrimSpace(secretName) == "" {
		return content, nil
	}
	return ensureOverlayPatchRef(content, envFromPatchFile)
}
