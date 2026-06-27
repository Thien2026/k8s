package deploy

import (
	"regexp"
	"strings"
)

var secretSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// SecretSlugSuffix chuẩn hóa project slug → DEPLOYGHCR, RESEARCH_LABS (dùng trong tên GitHub secret).
func SecretSlugSuffix(projectSlug string) string {
	s := strings.ToLower(strings.TrimSpace(projectSlug))
	s = secretSlugRe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "PROJECT"
	}
	return strings.ToUpper(s)
}

func DeployTokenSecretName(projectSlug string) string {
	return "PLATFORM_DEPLOY_TOKEN_" + SecretSlugSuffix(projectSlug)
}

func HarborUsernameSecretName(projectSlug string) string {
	return "HARBOR_USERNAME_" + SecretSlugSuffix(projectSlug)
}

func HarborPasswordSecretName(projectSlug string) string {
	return "HARBOR_PASSWORD_" + SecretSlugSuffix(projectSlug)
}

var buildKeyRe = regexp.MustCompile(`[^A-Z0-9]+`)

// BuildArgSecretName tên GitHub secret cho build-arg nhạy cảm (theo project + key).
func BuildArgSecretName(projectSlug, key string) string {
	k := strings.ToUpper(strings.TrimSpace(key))
	k = buildKeyRe.ReplaceAllString(k, "_")
	k = strings.Trim(k, "_")
	if k == "" {
		k = "VAR"
	}
	return "BUILD_" + SecretSlugSuffix(projectSlug) + "_" + k
}
