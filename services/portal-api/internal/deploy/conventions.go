package deploy

import (
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
)

const (
	ConventionAPIBasePath     = "/api"
	ConventionViteAPIBaseKey    = "VITE_API_BASE"
	ConventionNextAPIBaseKey    = "NEXT_PUBLIC_API_BASE"
	ConventionAPIRoutePrefixKey = "API_ROUTE_PREFIX"
)

// DefaultMultiBuildContract — biến build gợi ý khi layout Backend + Frontend.
func DefaultMultiBuildContract() platformcontract.File {
	return platformcontract.File{
		Version: platformcontract.ContractVersion,
		Vars: map[string]platformcontract.VarSpec{
			ConventionViteAPIBaseKey: {
				Required:    true,
				Description: "Frontend gọi API — prod/dev deploy dùng /api (cùng domain)",
			},
			ConventionNextAPIBaseKey: {
				Required:    false,
				Description: "Next.js — cùng giá trị /api nếu dùng",
			},
		},
	}
}

// DefaultMultiRuntimeContract — biến runtime gợi ý cho service api.
func DefaultMultiRuntimeContract() platformcontract.File {
	return platformcontract.File{
		Version: platformcontract.ContractVersion,
		Vars: map[string]platformcontract.VarSpec{
			ConventionAPIRoutePrefixKey: {
				Required:    false,
				Description: "Prefix HTTP route backend — mặc định /api",
			},
		},
	}
}

// DefaultBuildEnvSeed — giá trị mặc định Console (dev build).
func DefaultBuildEnvSeed() map[string]string {
	return map[string]string{
		ConventionViteAPIBaseKey: ConventionAPIBasePath,
	}
}

// DefaultRuntimeEnvSeed — giá trị mặc định Console (dev + prod).
func DefaultRuntimeEnvSeed() map[string]string {
	return map[string]string{
		ConventionAPIRoutePrefixKey: ConventionAPIBasePath,
	}
}

// PublicHealthPath — URL công khai health check qua Ingress.
func PublicHealthPath(ingressPath, healthPath string) string {
	ip := strings.TrimSpace(ingressPath)
	if ip == "" {
		ip = "/"
	}
	ip = strings.TrimSuffix(ip, "/")
	if ip == "" {
		ip = "/"
	}
	hp := strings.TrimSpace(healthPath)
	if hp == "" {
		hp = "/health"
	}
	if !strings.HasPrefix(hp, "/") {
		hp = "/" + hp
	}
	if ip == "/" {
		return hp
	}
	return ip + hp
}

// SmokeCheckPaths — đường dẫn HTTPS smoke theo layout.
func SmokeCheckPaths(layout string, services []ServiceDef) []string {
	if NormalizeLayout(layout) != LayoutMulti {
		return []string{"/health", "/"}
	}
	paths := []string{"/"}
	seen := map[string]bool{"/": true}
	for _, s := range services {
		s = normalizeServiceDef(s)
		if s.Name != "api" && s.IngressPath != ConventionAPIBasePath {
			continue
		}
		hp := PublicHealthPath(s.IngressPath, s.HealthPath)
		if !seen[hp] {
			paths = append([]string{hp}, paths...)
			seen[hp] = true
		}
	}
	if !seen["/api/health"] && !seen[PublicHealthPath(ConventionAPIBasePath, "/health")] {
		fallback := PublicHealthPath(ConventionAPIBasePath, "/health")
		if !seen[fallback] {
			paths = append([]string{fallback}, paths...)
		}
	}
	return paths
}
