package registry

const (
	GHCR   = "ghcr"
	Harbor = "harbor"
)

// ProviderInfo mô tả registry khả dụng trên platform.
type ProviderInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
	Default     bool   `json:"default"`
	Ready       bool   `json:"ready"`
	ReadyHint   string `json:"ready_hint,omitempty"`
}

// ProjectRegistry cấu hình registry của một project.
type ProjectRegistry struct {
	Provider    string `json:"provider"`
	Label       string `json:"label"`
	ImagePrefix string `json:"image_prefix"`
	PushExample string `json:"push_example,omitempty"`
	LoginHint   string `json:"login_hint,omitempty"`
	Ready       bool   `json:"ready"`
	ReadyHint   string `json:"ready_hint,omitempty"`
}

// ProvisionResult sau khi provision registry cho project mới/đổi provider.
type ProvisionResult struct {
	Provider    string
	ImagePrefix string
	HarborProject string
	Warnings    []string
}
