package config

import (
	"crypto/rand"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port                 string
	DatabaseURL          string
	CORSOrigin           string
	AllowedOrigins       []string
	RancherURL           string
	RancherToken         string
	JoinGateSecret       string
	RKE2ServerURL        string
	RKE2ServerToken      string
	RKE2ServerIP         string
	JWTSecret            string
	JWTAccessTTL         time.Duration
	JWTRefreshTTL        time.Duration
	CookieSecure         bool
	HarborURL            string
	HarborPassword       string
	HarborAdminUser      string
	RancherAdminPassword string
	GHCROrg              string
	GHCRPullUser         string
	GHCRPullToken        string
	PlatformDomain       string
	ApexDomain           string
	RedisZone            string
	MinioZone            string
	NodePublicIP         string
	GitHubClientID       string
	GitHubClientSecret   string
	GitHubRedirectURI    string
	PlatformPublicURL    string
	QuickLoginEnabled    bool
	QuickLoginEmail      string
	QuickLoginPassword   string
	PolicyUnlockPassphrase string
	ArgoCDURL            string
	ArgoCDNamespace      string
	GrafanaURL           string
	GrafanaAdminUser     string
	GrafanaAdminPassword string
	GitOpsRepoURL        string
	GitOpsRepoBranch     string
	GitOpsBasePath       string
	GitOpsPushToken      string
}

func readSecretFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func Load() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://platform:platform@localhost:5432/platform?sslmode=disable"
	}

	cors := os.Getenv("CORSOrigin")
	if cors == "" {
		cors = os.Getenv("CORS_ORIGIN")
	}
	if cors == "" {
		cors = "http://localhost:5173"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = readSecretFile("/etc/platform-auth/jwt_secret")
	}
	if len(jwtSecret) < 32 {
		// Dev local only — production bắt buộc set JWT_SECRET trong secret
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		jwtSecret = base64.RawURLEncoding.EncodeToString(b)
	}

	accessMin := envInt("JWT_ACCESS_MINUTES", 15)
	refreshDays := envInt("JWT_REFRESH_DAYS", 7)
	secure := os.Getenv("COOKIE_SECURE") != "false"
	argocdURL := strings.TrimSpace(os.Getenv("ARGOCD_URL"))
	if argocdURL == "" {
		if host := strings.TrimSpace(os.Getenv("ARGOCD_HOST")); host != "" {
			argocdURL = "https://" + host
		}
	}
	grafanaURL := strings.TrimSpace(os.Getenv("GRAFANA_URL"))
	if grafanaURL == "" {
		if host := strings.TrimSpace(os.Getenv("GRAFANA_HOST")); host != "" {
			grafanaURL = "https://" + host
		}
	}

	return Config{
		Port:                 port,
		DatabaseURL:          dbURL,
		CORSOrigin:           cors,
		AllowedOrigins:       []string{cors, strings.Replace(cors, "http://", "https://", 1)},
		RancherURL:           os.Getenv("RANCHER_URL"),
		RancherToken:         os.Getenv("RANCHER_TOKEN"),
		JoinGateSecret:       os.Getenv("JOIN_GATE_SECRET"),
		RKE2ServerURL:        firstNonEmpty(readSecretFile("/etc/rke2-join/server_url"), os.Getenv("RKE2_SERVER_URL")),
		RKE2ServerToken:      firstNonEmpty(readSecretFile("/etc/rke2-join/server_token"), os.Getenv("RKE2_SERVER_TOKEN")),
		RKE2ServerIP:         firstNonEmpty(readSecretFile("/etc/rke2-join/server_ip"), os.Getenv("RKE2_SERVER_IP")),
		JWTSecret:            jwtSecret,
		JWTAccessTTL:         time.Duration(accessMin) * time.Minute,
		JWTRefreshTTL:        time.Duration(refreshDays) * 24 * time.Hour,
		CookieSecure:         secure,
		HarborURL:            firstNonEmpty(os.Getenv("HARBOR_URL"), readSecretFile("/etc/harbor/url")),
		HarborPassword:       firstNonEmpty(os.Getenv("HARBOR_ADMIN_PASSWORD"), readSecretFile("/etc/harbor/admin_password")),
		HarborAdminUser:      firstNonEmpty(os.Getenv("HARBOR_ADMIN_USER"), "admin"),
		RancherAdminPassword: firstNonEmpty(os.Getenv("RANCHER_ADMIN_PASSWORD"), readSecretFile("/etc/rancher/admin_password")),
		GHCROrg:              os.Getenv("GHCR_ORG"),
		GHCRPullUser:         firstNonEmpty(os.Getenv("GHCR_PULL_USER"), os.Getenv("GHCR_ORG")),
		GHCRPullToken:        os.Getenv("GHCR_PULL_TOKEN"),
		PlatformDomain:       firstNonEmpty(os.Getenv("PLATFORM_DOMAIN"), os.Getenv("DOMAIN")),
		ApexDomain:           strings.TrimSpace(os.Getenv("DOMAIN")),
		RedisZone:            strings.TrimSpace(os.Getenv("REDIS_ZONE")),
		MinioZone:            strings.TrimSpace(os.Getenv("MINIO_ZONE")),
		NodePublicIP:         os.Getenv("NODE_PUBLIC_IP"),
		GitHubClientID:       os.Getenv("GITHUB_CLIENT_ID"),
		GitHubClientSecret:   os.Getenv("GITHUB_CLIENT_SECRET"),
		PlatformPublicURL:    firstNonEmpty(os.Getenv("PLATFORM_PUBLIC_URL"), os.Getenv("CORS_ORIGIN")),
		GitHubRedirectURI: firstNonEmpty(
			os.Getenv("GITHUB_REDIRECT_URI"),
			strings.TrimRight(firstNonEmpty(os.Getenv("PLATFORM_PUBLIC_URL"), os.Getenv("CORS_ORIGIN")), "/")+"/api/v1/github/oauth/callback",
		),
		QuickLoginEnabled:  os.Getenv("QUICK_LOGIN_ENABLED") == "true",
		QuickLoginEmail:    firstNonEmpty(os.Getenv("QUICK_LOGIN_EMAIL"), os.Getenv("PLATFORM_ADMIN_EMAIL")),
		QuickLoginPassword: firstNonEmpty(os.Getenv("QUICK_LOGIN_PASSWORD"), os.Getenv("PLATFORM_ADMIN_PASSWORD")),
		PolicyUnlockPassphrase: strings.TrimSpace(os.Getenv("POLICY_UNLOCK_PASSPHRASE")),
		ArgoCDURL:          argocdURL,
		ArgoCDNamespace:    firstNonEmpty(os.Getenv("ARGOCD_NAMESPACE"), "argocd"),
		GrafanaURL:           grafanaURL,
		GrafanaAdminUser:     firstNonEmpty(os.Getenv("GRAFANA_ADMIN_USER"), "admin"),
		GrafanaAdminPassword: firstNonEmpty(os.Getenv("GRAFANA_ADMIN_PASSWORD"), readSecretFile("/etc/grafana/admin_password")),
		GitOpsRepoURL:      firstNonEmpty(os.Getenv("GITOPS_REPO_URL"), os.Getenv("GITHUB_GITOPS_REPO")),
		GitOpsRepoBranch:   firstNonEmpty(os.Getenv("GITOPS_REPO_BRANCH"), "main"),
		GitOpsBasePath:     firstNonEmpty(os.Getenv("GITOPS_BASE_PATH"), "apps"),
		GitOpsPushToken:    os.Getenv("GITOPS_PUSH_TOKEN"),
	}
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
