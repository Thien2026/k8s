package registry

import "fmt"

type ghcrProvider struct {
	org string
}

func (g *ghcrProvider) name() string { return GHCR }

func (g *ghcrProvider) label() string { return "GitHub Container Registry" }

func (g *ghcrProvider) imagePrefix(slug string) string {
	org := g.org
	if org == "" {
		org = "YOUR_GITHUB_ORG"
	}
	return fmt.Sprintf("ghcr.io/%s/%s", org, slug)
}

func (g *ghcrProvider) projectRegistry(slug string, ready bool, hint string) ProjectRegistry {
	prefix := g.imagePrefix(slug)
	return ProjectRegistry{
		Provider:    GHCR,
		Label:       g.label(),
		ImagePrefix: prefix,
		PushExample: prefix + "/app:latest",
		LoginHint:   "echo $GITHUB_TOKEN | docker login ghcr.io -u GITHUB_USER --password-stdin",
		Ready:       ready,
		ReadyHint:   hint,
	}
}
