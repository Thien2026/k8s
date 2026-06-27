package domains

import "strings"

// Platform cấu hình DNS / hostname tự động.
type Platform struct {
	Domain   string
	PublicIP string
}

func (p Platform) AutoHostname(slug, environment string) string {
	slug = strings.TrimSpace(slug)
	suffix := "dev"
	if environment == "prod" {
		suffix = "prod"
	}
	domain := strings.TrimSpace(p.Domain)
	if domain == "" {
		domain = "example.com"
	}
	return slug + "-" + suffix + "." + domain
}

func (p Platform) IngressCNAMETarget() string {
	domain := strings.TrimSpace(p.Domain)
	if domain != "" {
		return "ingress." + domain
	}
	return ""
}

func PublicURL(hostname string, tls bool) string {
	host := strings.TrimSpace(hostname)
	if host == "" {
		return ""
	}
	if tls {
		return "https://" + host
	}
	return "http://" + host
}
