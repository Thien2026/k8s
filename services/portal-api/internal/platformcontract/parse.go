package platformcontract

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Parse đọc nội dung contract YAML.
func Parse(raw string) (File, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return File{}, fmt.Errorf("contract rỗng")
	}
	var f File
	if err := yaml.Unmarshal([]byte(raw), &f); err != nil {
		return File{}, fmt.Errorf("contract YAML không hợp lệ: %w", err)
	}
	if f.Version == 0 {
		f.Version = 1
	}
	if f.Version != ContractVersion {
		return File{}, fmt.Errorf("contract version %d không hỗ trợ (cần %d)", f.Version, ContractVersion)
	}
	if f.Vars == nil {
		f.Vars = map[string]VarSpec{}
	}
	normalized := map[string]VarSpec{}
	for k, v := range f.Vars {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		normalized[key] = v
	}
	f.Vars = normalized
	return f, nil
}
