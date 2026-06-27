package deploy

import (
	"context"
	"testing"
)

func TestDockerfileCandidates(t *testing.T) {
	got := DockerfileCandidates("")
	if len(got) < 2 || got[0] != "Dockerfile" {
		t.Fatalf("default candidates: %v", got)
	}
	custom := DockerfileCandidates("ci/Dockerfile")
	if custom[0] != "ci/Dockerfile" {
		t.Fatalf("configured first: %v", custom)
	}
}

func TestDetectBuildMode_Dockerfile(t *testing.T) {
	files := map[string]bool{"Dockerfile": true}
	mode, path, err := DetectBuildMode(context.Background(), func(_ context.Context, p string) (bool, error) {
		return files[p], nil
	}, "")
	if err != nil || mode != "dockerfile" || path != "Dockerfile" {
		t.Fatalf("got %s %s %v", mode, path, err)
	}
}

func TestDetectBuildMode_Buildpack(t *testing.T) {
	mode, path, err := DetectBuildMode(context.Background(), func(_ context.Context, _ string) (bool, error) {
		return false, nil
	}, "")
	if err != nil || mode != "buildpack" || path != "" {
		t.Fatalf("got %s %s %v", mode, path, err)
	}
}

func TestDetectBuildMode_CustomPath(t *testing.T) {
	files := map[string]bool{"docker/Dockerfile": true}
	mode, path, err := DetectBuildMode(context.Background(), func(_ context.Context, p string) (bool, error) {
		return files[p], nil
	}, "docker/Dockerfile")
	if err != nil || mode != "dockerfile" || path != "docker/Dockerfile" {
		t.Fatalf("got %s %s %v", mode, path, err)
	}
}
