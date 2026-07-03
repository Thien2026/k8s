package platformcontract

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// GitConfig — checkout / monorepo (L4C submodules).
type GitConfig struct {
	Submodules string `yaml:"submodules"` // true | recursive
}

// ServicesFile — repo khai báo layout + fleet (L4A universal ingest).
type ServicesFile struct {
	Version    int           `yaml:"version"`
	Layout     string        `yaml:"layout"`
	Submodules string        `yaml:"submodules"` // shorthand top-level
	Git        GitConfig     `yaml:"git"`
	Services   []ServiceSpec `yaml:"services"`
}

// ServiceSpec một workload trong services.yaml.
type ServiceSpec struct {
	Name           string `yaml:"name"`
	DisplayName    string `yaml:"display_name"`
	Path           string `yaml:"path"`
	BuildContext   string `yaml:"build_context"`
	Dockerfile     string `yaml:"dockerfile"`
	DockerfilePath string `yaml:"dockerfile_path"`
	BuildMode      string `yaml:"build_mode"`
	Stack          string `yaml:"stack"`
	Port           int    `yaml:"port"`
	ContainerPort  int    `yaml:"container_port"`
	Health         string `yaml:"health"`
	HealthPath     string `yaml:"health_path"`
	Ingress        string `yaml:"ingress"`
	IngressPath    string `yaml:"ingress_path"`
	Expose         *bool  `yaml:"expose"`
	ExposeIngress  *bool  `yaml:"expose_ingress"`
}

const (
	LayoutSingle = "single"
	LayoutMulti  = "multi"
)

// ParseServices đọc .platform/services.yaml.
func ParseServices(raw string) (ServicesFile, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ServicesFile{}, fmt.Errorf("services contract rỗng")
	}
	var f ServicesFile
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return ServicesFile{}, fmt.Errorf("services YAML không hợp lệ: %w", err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Version != ContractVersion {
		return ServicesFile{}, fmt.Errorf("services version %d không hỗ trợ (cần %d)", f.Version, ContractVersion)
	}
	f.Layout = normalizeServicesLayout(f.Layout, len(f.Services))
	if f.Layout == LayoutMulti && len(f.Services) < 2 {
		return ServicesFile{}, fmt.Errorf("layout multi cần ít nhất 2 service trong services.yaml")
	}
	names := map[string]bool{}
	for i := range f.Services {
		spec := normalizeServiceSpec(f.Services[i])
		if spec.Name == "" {
			return ServicesFile{}, fmt.Errorf("service[%d]: thiếu name", i)
		}
		if names[spec.Name] {
			return ServicesFile{}, fmt.Errorf("service trùng tên: %s", spec.Name)
		}
		names[spec.Name] = true
		f.Services[i] = spec
	}
	if f.Layout == LayoutMulti {
		public := 0
		for _, s := range f.Services {
			if ServiceSpecExpose(s) {
				public++
			}
		}
		if public == 0 {
			return ServicesFile{}, fmt.Errorf("cần ít nhất 1 service public (expose: true hoặc có ingress)")
		}
	}
	return f, nil
}

func normalizeServicesLayout(layout string, serviceCount int) string {
	layout = strings.ToLower(strings.TrimSpace(layout))
	if layout == LayoutMulti || layout == LayoutSingle {
		return layout
	}
	if serviceCount >= 2 {
		return LayoutMulti
	}
	return LayoutSingle
}

func normalizeBuildModeContract(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "buildpack") {
		return "buildpack"
	}
	return "dockerfile"
}

func isInternalIngressContract(path string) bool {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "-", "internal", "none", "(internal)", "cluster", "private":
		return true
	default:
		return false
	}
}

func normalizeServiceSpec(s ServiceSpec) ServiceSpec {
	s.Name = strings.TrimSpace(s.Name)
	if s.DisplayName == "" && s.Name != "" {
		s.DisplayName = strings.ToUpper(s.Name[:1]) + s.Name[1:]
	}
	ctx := strings.TrimSpace(s.Path)
	if ctx == "" {
		ctx = strings.TrimSpace(s.BuildContext)
	}
	if ctx == "" {
		ctx = "."
	}
	s.Path = ctx
	s.BuildContext = ctx
	df := strings.TrimSpace(s.Dockerfile)
	if df == "" {
		df = strings.TrimSpace(s.DockerfilePath)
	}
	if df == "" {
		if ctx == "." {
			df = "Dockerfile"
		} else {
			df = strings.TrimSuffix(ctx, "/") + "/Dockerfile"
		}
	}
	s.Dockerfile = df
	s.DockerfilePath = df
	s.BuildMode = normalizeBuildModeContract(s.BuildMode)
	s.Stack = strings.ToLower(strings.TrimSpace(s.Stack))
	port := s.Port
	if port <= 0 {
		port = s.ContainerPort
	}
	if port <= 0 {
		port = 8080
	}
	s.Port = port
	s.ContainerPort = port
	health := strings.TrimSpace(s.Health)
	if health == "" {
		health = strings.TrimSpace(s.HealthPath)
	}
	if health == "" {
		ing := strings.TrimSpace(s.Ingress)
		if ing == "" {
			ing = strings.TrimSpace(s.IngressPath)
		}
		if ing == "/" {
			health = "/"
		} else {
			health = "/health"
		}
	}
	s.Health = health
	s.HealthPath = health
	ing := strings.TrimSpace(s.Ingress)
	if ing == "" {
		ing = strings.TrimSpace(s.IngressPath)
	}
	if !ServiceSpecExpose(s) {
		if ing == "" {
			ing = "-"
		}
	}
	if ing != "" && ServiceSpecExpose(s) && ing != "/" && !strings.HasPrefix(ing, "/") {
		ing = "/" + ing
	}
	s.Ingress = ing
	s.IngressPath = ing
	return s
}

// ServiceSpecExpose — public Ingress hay internal-only.
func ServiceSpecExpose(s ServiceSpec) bool {
	if s.ExposeIngress != nil {
		return *s.ExposeIngress
	}
	if s.Expose != nil {
		return *s.Expose
	}
	ing := strings.TrimSpace(s.Ingress)
	if ing == "" {
		ing = strings.TrimSpace(s.IngressPath)
	}
	if isInternalIngressContract(ing) {
		return false
	}
	if ing != "" {
		return true
	}
	return s.Name == "web" || s.Name == "api"
}

// ResolveSubmodulesMode trả về "" | "true" | "recursive" cho actions/checkout.
func ResolveSubmodulesMode(explicit ...string) string {
	for _, raw := range explicit {
		v := strings.ToLower(strings.TrimSpace(raw))
		switch v {
		case "", "false", "off", "none", "0", "no":
			continue
		case "true", "yes", "1", "on":
			return "true"
		case "recursive", "recurse":
			return "recursive"
		default:
			continue
		}
	}
	return ""
}

// ServicesDetectIssue — cảnh báo / lỗi khi đọc contract.
type ServicesDetectIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
