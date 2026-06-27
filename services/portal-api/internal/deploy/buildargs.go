package deploy

import "strings"

// BuildArg một biến truyền vào docker build-push-action (build-args).
type BuildArg struct {
	Key        string
	Value      string
	IsSecret   bool
	SecretName string
}

func yamlBuildArgValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "\"\""
	}
	if strings.ContainsAny(v, ":#{}[]&*!|>'\"%@`") {
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	}
	return v
}
