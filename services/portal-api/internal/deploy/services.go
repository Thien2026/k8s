package deploy

import "strings"

// IngressRoute — một path trên Ingress trỏ tới Service.
type IngressRoute struct {
	Path        string `json:"path"`
	PathType    string `json:"path_type"`
	ServiceName string `json:"service_name"`
	ServicePort int    `json:"service_port"`
}

// ServiceDef mô tả một workload (api, web, app…) trong project.
type ServiceDef struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name,omitempty"`
	BuildContext   string `json:"build_context"`
	BuildMode      string `json:"build_mode"`
	DockerfilePath string `json:"dockerfile_path"`
	ContainerPort  int    `json:"container_port"`
	HealthPath     string `json:"health_path"`
	IngressPath    string `json:"ingress_path"`
	SortOrder      int    `json:"sort_order"`
}

const (
	LayoutSingle = "single"
	LayoutMulti  = "multi"
)

// DefaultMultiServices — template monorepo backend/ + frontend/.
var DefaultMultiServices = []ServiceDef{
	{
		Name:           "api",
		DisplayName:    "API",
		BuildContext:   "backend",
		BuildMode:      "dockerfile",
		DockerfilePath: "backend/Dockerfile",
		ContainerPort:  8080,
		HealthPath:     "/health",
		IngressPath:    "/api",
		SortOrder:      0,
	},
	{
		Name:           "web",
		DisplayName:    "Web",
		BuildContext:   "frontend",
		BuildMode:      "dockerfile",
		DockerfilePath: "frontend/Dockerfile",
		ContainerPort:  8080,
		HealthPath:     "/",
		IngressPath:    "/",
		SortOrder:      1,
	},
}

func NormalizeLayout(layout string) string {
	if strings.EqualFold(strings.TrimSpace(layout), LayoutMulti) {
		return LayoutMulti
	}
	return LayoutSingle
}

func NormalizeServiceDef(s ServiceDef) ServiceDef {
	return normalizeServiceDef(s)
}

func (p Params) IsMultiService() bool {
	return NormalizeLayout(p.Layout) == LayoutMulti && len(p.Services) > 0
}

func (p Params) EffectiveServices() []ServiceDef {
	if p.IsMultiService() {
		out := make([]ServiceDef, 0, len(p.Services))
		for _, s := range p.Services {
			if strings.TrimSpace(s.Name) == "" {
				continue
			}
			out = append(out, normalizeServiceDef(s))
		}
		if len(out) > 0 {
			return out
		}
	}
	return []ServiceDef{p.defaultSingleService()}
}

func (p Params) defaultSingleService() ServiceDef {
	return normalizeServiceDef(ServiceDef{
		Name:           "app",
		DisplayName:    "App",
		BuildContext:   p.BuildContext,
		BuildMode:      p.BuildMode,
		DockerfilePath: p.DockerfilePath,
		ContainerPort:  8080,
		HealthPath:     "/health",
		IngressPath:    "/",
		SortOrder:      0,
	})
}

func normalizeServiceDef(s ServiceDef) ServiceDef {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		s.Name = "app"
	}
	if s.DisplayName == "" {
		s.DisplayName = strings.ToUpper(s.Name[:1]) + s.Name[1:]
	}
	if strings.TrimSpace(s.BuildContext) == "" {
		s.BuildContext = "."
	}
	s.BuildMode = NormalizeBuildMode(s.BuildMode)
	if strings.TrimSpace(s.DockerfilePath) == "" {
		s.DockerfilePath = "Dockerfile"
	}
	if s.ContainerPort <= 0 {
		s.ContainerPort = 8080
	}
	if strings.TrimSpace(s.HealthPath) == "" {
		s.HealthPath = "/health"
	}
	ip := strings.TrimSpace(s.IngressPath)
	if ip == "" {
		ip = "/"
	}
	if ip != "/" && !strings.HasPrefix(ip, "/") {
		ip = "/" + ip
	}
	s.IngressPath = ip
	return s
}

func (p Params) imageRefFor(svc ServiceDef) string {
	tag := strings.TrimSpace(p.ImageTag)
	if tag == "" {
		tag = "latest"
	}
	prefix := strings.TrimSpace(p.Registry.ImagePrefix)
	if prefix == "" {
		prefix = "YOUR_REGISTRY/" + p.ProjectSlug
	}
	return prefix + "/" + svc.Name + ":" + tag
}

// PrimaryService — workload dùng smoke test runtime (web nếu có, không thì service cuối).
func (p Params) PrimaryService() ServiceDef {
	for _, s := range p.EffectiveServices() {
		if s.Name == "web" {
			return s
		}
	}
	svcs := p.EffectiveServices()
	if len(svcs) == 0 {
		return p.defaultSingleService()
	}
	return svcs[len(svcs)-1]
}
