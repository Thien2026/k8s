package deploy

import "strings"

const defaultBuildpackBuilder = "paketobuildpacks/builder-jammy-full"

// Stack constants — map tới buildpack builder/env.
const (
	StackPython = "python"
	StackNode   = "node"
	StackGo     = "go"
	StackDotnet = "dotnet"
	StackRuby   = "ruby"
)

// NormalizeStack chuẩn hóa stack hint từ services.yaml / Console.
func NormalizeStack(stack string) string {
	switch strings.ToLower(strings.TrimSpace(stack)) {
	case "python", "py":
		return StackPython
	case "node", "nodejs", "javascript", "js":
		return StackNode
	case "go", "golang":
		return StackGo
	case "dotnet", "csharp", "cs", ".net":
		return StackDotnet
	case "ruby", "rb":
		return StackRuby
	default:
		return ""
	}
}

// BuildpackBuilderForStack chọn Paketo builder theo stack.
func BuildpackBuilderForStack(stack string) string {
	switch NormalizeStack(stack) {
	case StackPython, StackNode, StackGo, StackDotnet, StackRuby:
		return "paketobuildpacks/builder-jammy-base"
	default:
		return defaultBuildpackBuilder
	}
}

// BuildpackExtraEnv — biến môi trường buildpack theo stack (append sau PORT).
func BuildpackExtraEnv(stack string) []string {
	switch NormalizeStack(stack) {
	case StackPython:
		return []string{`BP_CPYTHON_VERSION=3.11`}
	case StackNode:
		return []string{`BP_NODE_VERSION=20`}
	case StackGo:
		return []string{`BP_GO_TARGET=./...`}
	case StackDotnet:
		return []string{`BP_DOTNET_VERSION=8.*`}
	default:
		return nil
	}
}
