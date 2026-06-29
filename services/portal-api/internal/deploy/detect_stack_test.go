package deploy

import (
	"context"
	"testing"
)

func TestDetectStackFromContext(t *testing.T) {
	files := map[string]bool{
		"worker/requirements.txt": true,
	}
	stack, err := DetectStackFromContext(context.Background(), func(_ context.Context, p string) (bool, error) {
		return files[p], nil
	}, "worker")
	if err != nil || stack != StackPython {
		t.Fatalf("want python, got %q err=%v", stack, err)
	}
}

func TestBuildpackBuilderForStack(t *testing.T) {
	if BuildpackBuilderForStack(StackPython) == defaultBuildpackBuilder {
		t.Fatal("python should use jammy-base")
	}
	if BuildpackBuilderForStack("") != defaultBuildpackBuilder {
		t.Fatal("empty stack should use default full builder")
	}
}

func TestDetectServiceBuild_Dockerfile(t *testing.T) {
	files := map[string]bool{"backend/Dockerfile": true}
	mode, df, stack, err := DetectServiceBuild(context.Background(), func(_ context.Context, p string) (bool, error) {
		return files[p], nil
	}, "backend", "backend/Dockerfile", "")
	if err != nil || mode != "dockerfile" || df != "backend/Dockerfile" || stack != "" {
		t.Fatalf("got mode=%s df=%s stack=%q err=%v", mode, df, stack, err)
	}
}

func TestDetectServiceBuild_PythonBuildpack(t *testing.T) {
	files := map[string]bool{"worker/requirements.txt": true}
	mode, _, stack, err := DetectServiceBuild(context.Background(), func(_ context.Context, p string) (bool, error) {
		return files[p], nil
	}, "worker", "worker/Dockerfile", "")
	if err != nil || mode != "buildpack" || stack != StackPython {
		t.Fatalf("got mode=%s stack=%q err=%v", mode, stack, err)
	}
}
