package deploy

import (
	"sort"
	"strings"
)

// WorkflowProfileKey — layout + danh sách service (sorted) lúc sync workflow GitHub.
func WorkflowProfileKey(layout string, services []ServiceDef) (string, string) {
	layout = NormalizeLayout(layout)
	if layout != LayoutMulti {
		return LayoutSingle, "app"
	}
	names := make([]string, 0, len(services))
	for _, s := range services {
		name := strings.TrimSpace(s.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return LayoutMulti, ""
	}
	return LayoutMulti, strings.Join(names, ",")
}

// WorkflowProfileMatches — Console hiện tại khớp snapshot workflow đã sync.
func WorkflowProfileMatches(storedLayout, storedServices, currentLayout string, currentServices []ServiceDef) bool {
	wantLayout, wantServices := WorkflowProfileKey(currentLayout, currentServices)
	storedLayout = NormalizeLayout(storedLayout)
	if storedLayout == "" {
		return false
	}
	if storedLayout != wantLayout {
		return false
	}
	if wantLayout == LayoutSingle {
		return true
	}
	return strings.TrimSpace(storedServices) == wantServices
}
