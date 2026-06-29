package deploy

import (
	"context"
	"path"
	"strings"
)

type repoPathExists func(ctx context.Context, repoPath string) (bool, error)

// DetectStackFromContext suy stack từ file trong build_context (buildpack).
func DetectStackFromContext(ctx context.Context, exists repoPathExists, buildContext string) (string, error) {
	ctxPath := strings.TrimSpace(buildContext)
	if ctxPath == "" || ctxPath == "." {
		ctxPath = ""
	} else {
		ctxPath = strings.TrimSuffix(ctxPath, "/")
	}
	join := func(name string) string {
		if ctxPath == "" {
			return name
		}
		return path.Join(ctxPath, name)
	}
	checks := []struct {
		file  string
		stack string
	}{
		{join("requirements.txt"), StackPython},
		{join("pyproject.toml"), StackPython},
		{join("Pipfile"), StackPython},
		{join("package.json"), StackNode},
		{join("go.mod"), StackGo},
		{join("Gemfile"), StackRuby},
		{join("Program.cs"), StackDotnet},
		{join("app.csproj"), StackDotnet},
	}
	for _, c := range checks {
		ok, err := exists(ctx, c.file)
		if err != nil {
			return "", err
		}
		if ok {
			return c.stack, nil
		}
	}
	return "", nil
}

// DetectServiceBuild quét Dockerfile trong context service → mode + path; stack khi buildpack.
func DetectServiceBuild(ctx context.Context, exists repoPathExists, buildContext, dockerfilePath, stackHint string) (mode, detectedDockerfile, stack string, err error) {
	df := strings.TrimSpace(dockerfilePath)
	if df == "" {
		ctxPath := strings.TrimSpace(buildContext)
		if ctxPath == "" || ctxPath == "." {
			df = "Dockerfile"
		} else {
			df = strings.TrimSuffix(ctxPath, "/") + "/Dockerfile"
		}
	}
	mode, detectedDockerfile, err = DetectBuildMode(ctx, exists, df)
	if err != nil {
		return "", "", "", err
	}
	stack = NormalizeStack(stackHint)
	if mode == "buildpack" && stack == "" {
		stack, err = DetectStackFromContext(ctx, exists, buildContext)
		if err != nil {
			return mode, detectedDockerfile, "", err
		}
	}
	return mode, detectedDockerfile, stack, nil
}
