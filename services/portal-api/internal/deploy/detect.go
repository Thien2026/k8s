package deploy

import (
	"context"
	"strings"
)

// DockerfileCandidates returns paths to probe (configured path first, then common defaults).
func DockerfileCandidates(configured string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	add(configured)
	add("Dockerfile")
	add("docker/Dockerfile")
	return out
}

// DetectBuildMode scans the repo for a Dockerfile. First hit → dockerfile, else buildpack.
func DetectBuildMode(ctx context.Context, exists func(ctx context.Context, path string) (bool, error), dockerfilePath string) (mode string, detectedPath string, err error) {
	for _, path := range DockerfileCandidates(dockerfilePath) {
		ok, err := exists(ctx, path)
		if err != nil {
			return "", "", err
		}
		if ok {
			return "dockerfile", path, nil
		}
	}
	return "buildpack", "", nil
}
