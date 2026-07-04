package platformcontract

import (
	"strings"

	"gopkg.in/yaml.v3"
)

// ServicesFileFromDefs — Console → services.yaml (multi thường: api + web + worker).
func ServicesFileFromDefs(layout string, services []ServiceSpec, submodules string) ServicesFile {
	f := ServicesFile{
		Version: ContractVersion,
		Layout:  normalizeServicesLayout(layout, len(services)),
	}
	submodules = strings.TrimSpace(submodules)
	if submodules != "" {
		f.Git = GitConfig{Submodules: submodules}
	}
	f.Services = services
	return f
}

// ServiceSpecsFromServiceDefs — rút gọn field ghi vào repo (path, ingress, expose).
func ServiceSpecsFromServiceDefs(services []ServiceSpec) []ServiceSpec {
	out := make([]ServiceSpec, 0, len(services))
	for _, s := range services {
		spec := normalizeServiceSpec(s)
		out = append(out, spec)
	}
	return out
}

// RenderServicesYAML — nội dung .platform/services.yaml.
func RenderServicesYAML(f ServicesFile) (string, error) {
	f.Version = ContractVersion
	if f.Layout == "" {
		f.Layout = LayoutSingle
	}
	raw, err := yaml.Marshal(f)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
