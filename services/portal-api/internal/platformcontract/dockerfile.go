package platformcontract

import (
	"regexp"
	"strings"
)

var dockerfileARG = regexp.MustCompile(`(?m)^\s*ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s*=.*)?\s*$`)

// ParseDockerfileARGs trích tên ARG từ Dockerfile (bỏ qua platform-injected).
func ParseDockerfileARGs(content string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range dockerfileARG.FindAllStringSubmatch(content, -1) {
		if len(m) < 2 {
			continue
		}
		key := strings.TrimSpace(m[1])
		if key == "" || PlatformBuildArgs[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}
