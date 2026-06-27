package registry

import (
	"context"
	"fmt"

	"github.com/Thien2026/k8s/services/portal-api/internal/harbor"
	"github.com/Thien2026/k8s/services/portal-api/internal/plugins"
)

type Service struct {
	plugins *plugins.Store
	harbor  *harborProvider
	ghcr    *ghcrProvider
}

func NewService(pluginStore *plugins.Store, harborClient *harbor.Client, ghcrOrg string) *Service {
	return &Service{
		plugins: pluginStore,
		harbor:  &harborProvider{client: harborClient},
		ghcr:    &ghcrProvider{org: ghcrOrg},
	}
}

func (s *Service) ListProviders(ctx context.Context) ([]ProviderInfo, error) {
	ghcrOn, _ := s.plugins.Enabled(ctx, plugins.GHCR)
	harborOn, _ := s.plugins.Enabled(ctx, plugins.Harbor)
	harborReady := s.harbor.client != nil && s.harbor.client.Enabled()

	list := []ProviderInfo{
		{
			Name:        GHCR,
			Label:       s.ghcr.label(),
			Description: "Push image qua GitHub Actions — không cài thêm trên VPS",
			Available:   ghcrOn,
			Default:     true,
			Ready:       ghcrOn,
			ReadyHint:   "Cấu hình GHCR_ORG + GHCR_PULL_TOKEN (PAT read:packages) trên VPS",
		},
	}
	if harborOn {
		hint := "Bật addon Harbor trong Console"
		if harborReady {
			hint = "Harbor đã kết nối"
		} else if harborOn {
			hint = "Harbor addon bật nhưng chưa cài — Addons → cài Harbor"
		}
		list = append(list, ProviderInfo{
			Name:        Harbor,
			Label:       s.harbor.label(),
			Description: "Registry on-prem — scan image, robot CI",
			Available:   harborOn,
			Default:     false,
			Ready:       harborReady,
			ReadyHint:   hint,
		})
	}
	return list, nil
}

func (s *Service) ValidateProvider(ctx context.Context, provider string) error {
	switch provider {
	case GHCR:
		ok, _ := s.plugins.Enabled(ctx, plugins.GHCR)
		if !ok {
			return fmt.Errorf("GHCR chưa bật")
		}
		return nil
	case Harbor:
		ok, _ := s.plugins.Enabled(ctx, plugins.Harbor)
		if !ok {
			return fmt.Errorf("Harbor addon chưa bật — vào Addons để kích hoạt")
		}
		if s.harbor.client == nil || !s.harbor.client.Enabled() {
			return fmt.Errorf("Harbor chưa sẵn sàng — chạy bootstrap/addons/run.sh harbor")
		}
		return nil
	default:
		return fmt.Errorf("registry không hợp lệ: %s (ghcr | harbor)", provider)
	}
}

func (s *Service) ProjectRegistry(ctx context.Context, provider, slug, harborProject string) ProjectRegistry {
	switch provider {
	case Harbor:
		ready := s.harbor.client != nil && s.harbor.client.Enabled()
		hint := "Harbor chưa cấu hình"
		if ready {
			hint = "Sẵn sàng push vào project " + harborProject
		}
		return s.harbor.projectRegistry(slug, ready, hint)
	default:
		ghcrOn, _ := s.plugins.Enabled(ctx, plugins.GHCR)
		hint := "Mặc định — push qua GitHub Actions"
		if !ghcrOn {
			hint = "GHCR plugin tắt"
		}
		return s.ghcr.projectRegistry(slug, ghcrOn, hint)
	}
}

func (s *Service) Provision(ctx context.Context, provider, slug string) (ProvisionResult, error) {
	if err := s.ValidateProvider(ctx, provider); err != nil && provider == Harbor {
		return ProvisionResult{}, err
	}
	res := ProvisionResult{Provider: provider, Warnings: []string{}}
	switch provider {
	case Harbor:
		if err := s.harbor.ensureProject(ctx, slug); err != nil {
			return ProvisionResult{}, err
		}
		res.HarborProject = slug
		pr := s.harbor.projectRegistry(slug, true, "")
		res.ImagePrefix = pr.ImagePrefix
	case GHCR, "":
		res.Provider = GHCR
		pr := s.ghcr.projectRegistry(slug, true, "")
		res.ImagePrefix = pr.ImagePrefix
	default:
		return ProvisionResult{}, fmt.Errorf("registry không hỗ trợ: %s", provider)
	}
	return res, nil
}

func (s *Service) DefaultProvider(ctx context.Context) string {
	ok, _ := s.plugins.Enabled(ctx, plugins.GHCR)
	if ok {
		return GHCR
	}
	return GHCR
}
