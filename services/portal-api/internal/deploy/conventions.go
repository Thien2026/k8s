package deploy

import (
	"fmt"
	"strings"

	"github.com/Thien2026/k8s/services/portal-api/internal/platformcontract"
)

const (
	ConventionAPIBasePath     = "/api"
	ConventionViteAPIBaseKey    = "VITE_API_BASE"
	ConventionNextAPIBaseKey    = "NEXT_PUBLIC_API_BASE"
	ConventionAPIRoutePrefixKey = "API_ROUTE_PREFIX"
	ConventionRedisURLKey       = "REDIS_URL"
	ConventionRedisTTLKey       = "REDIS_KEY_TTL_SECONDS"
	ConventionS3EndpointKey     = "S3_ENDPOINT"
	ConventionS3AccessKey       = "S3_ACCESS_KEY"
	ConventionS3SecretKey       = "S3_SECRET_KEY"
	ConventionS3BucketKey       = "S3_BUCKET"
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
			ConventionRedisURLKey: {
				Required:    false,
				Description: "Inject bởi Platform khi bật Redis addon — app đọc từ runtime env",
			},
			ConventionRedisTTLKey: {
				Required:    false,
				Description: "TTL mặc định (giây) gợi ý khi app SET key — inject bởi Platform",
			},
			ConventionS3EndpointKey: {
				Required:    false,
				Description: "Inject bởi Platform khi bật MinIO addon — endpoint S3 trong cluster",
			},
			ConventionS3AccessKey: {
				Required:    false,
				Description: "Inject bởi Platform khi bật MinIO addon",
			},
			ConventionS3SecretKey: {
				Required:    false,
				Description: "Inject bởi Platform khi bật MinIO addon",
			},
			ConventionS3BucketKey: {
				Required:    false,
				Description: "Bucket mặc định (app) — inject bởi Platform",
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

// ContainerProbePath — path HTTP probe trên container (Deployment readiness/liveness).
// Service api với ingress /api và route /health thực tế là /api/health trong app.
func ContainerProbePath(s ServiceDef) string {
	s = normalizeServiceDef(s)
	if !s.ExposeIngress || IsInternalIngressMarker(s.IngressPath) {
		return s.HealthPath
	}
	return PublicHealthPath(s.IngressPath, s.HealthPath)
}

// ServiceExtraEnv — biến runtime gợi ý theo stack/service (polyglot).
func ServiceExtraEnv(svc ServiceDef, port int) []map[string]any {
	svc = normalizeServiceDef(svc)
	var out []map[string]any
	lowName := strings.ToLower(svc.Name)
	lowCtx := strings.ToLower(svc.BuildContext)
	stack := NormalizeStack(svc.Stack)
	if stack == StackDotnet || strings.Contains(lowName, "dotnet") || strings.Contains(lowCtx, "dotnet") {
		out = append(out,
			map[string]any{"name": "DOTNET_USE_POLLING_FILE_WATCHER", "value": "true"},
			map[string]any{"name": "ASPNETCORE_URLS", "value": fmt.Sprintf("http://+:%d", port)},
		)
	}
	return out
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

// SmokeCheckPaths — đường dẫn HTTPS smoke theo layout (mọi service public trên Ingress).
func SmokeCheckPaths(layout string, services []ServiceDef) []string {
	if NormalizeLayout(layout) != LayoutMulti {
		return []string{"/health", "/"}
	}
	var paths []string
	seen := map[string]bool{}
	for _, s := range services {
		s = normalizeServiceDef(s)
		if !s.ExposeIngress || IsInternalIngressMarker(s.IngressPath) {
			continue
		}
		hp := PublicHealthPath(s.IngressPath, s.HealthPath)
		if hp == "" || seen[hp] {
			continue
		}
		paths = append(paths, hp)
		seen[hp] = true
	}
	if !seen["/"] {
		for _, s := range services {
			s = normalizeServiceDef(s)
			if s.ExposeIngress && !IsInternalIngressMarker(s.IngressPath) && s.IngressPath == "/" {
				paths = append(paths, "/")
				seen["/"] = true
				break
			}
		}
	}
	if len(paths) == 0 {
		paths = []string{"/"}
	}
	return paths
}
