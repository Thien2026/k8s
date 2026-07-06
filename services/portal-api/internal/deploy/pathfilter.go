package deploy

import (
	"fmt"
	"strings"
)

// servicePathFilterGlob — pattern cho dorny/paths-filter theo build_context.
func servicePathFilterGlob(buildContext string) string {
	ctx := strings.TrimSpace(buildContext)
	ctx = strings.Trim(ctx, "/")
	if ctx == "" || ctx == "." {
		return "**"
	}
	return ctx + "/**"
}

func writePathsFilterStep(b *strings.Builder, svcs []ServiceDef) {
	if len(svcs) <= 1 {
		return
	}
	b.WriteString("      - uses: dorny/paths-filter@v3\n")
	b.WriteString("        id: changes\n")
	b.WriteString("        with:\n")
	b.WriteString("          filters: |\n")
	b.WriteString("            platform:\n")
	b.WriteString("              - '.platform/**'\n")
	b.WriteString("              - '.github/workflows/**'\n")
	for _, svc := range svcs {
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			continue
		}
		glob := servicePathFilterGlob(svc.BuildContext)
		b.WriteString("            " + name + ":\n")
		b.WriteString("              - '" + glob + "'\n")
	}
	b.WriteString("\n")
}

func serviceUnchangedIf(svcName string) string {
	return fmt.Sprintf(
		"steps.changes.outputs.%s != 'true' && steps.changes.outputs.platform != 'true' && github.event_name == 'push' && github.event.before != '' && github.event.before != '0000000000000000000000000000000000000000'",
		svcName,
	)
}

func serviceBuildIf(svcName string) string {
	return fmt.Sprintf(
		"steps.changes.outputs.%s == 'true' || steps.changes.outputs.platform == 'true' || github.event_name == 'workflow_dispatch' || steps.retag_%s.outcome == 'failure' || steps.retag_%s.outcome == 'skipped'",
		svcName, svcName, svcName,
	)
}

func writeRetagServiceStep(b *strings.Builder, svc ServiceDef, image string) {
	name := strings.TrimSpace(svc.Name)
	if name == "" {
		return
	}
	b.WriteString("      - name: Retag " + name + " (unchanged)\n")
	b.WriteString("        id: retag_" + name + "\n")
	b.WriteString("        if: " + serviceUnchangedIf(name) + "\n")
	b.WriteString("        continue-on-error: true\n")
	b.WriteString("        run: |\n")
	b.WriteString("          set -euo pipefail\n")
	b.WriteString("          BEFORE=\"${{ github.event.before }}\"\n")
	b.WriteString("          SHA=\"${{ github.sha }}\"\n")
	b.WriteString("          IMG=\"" + image + "\"\n")
	b.WriteString("          docker pull \"${IMG}:${BEFORE}\"\n")
	b.WriteString("          docker tag \"${IMG}:${BEFORE}\" \"${IMG}:${SHA}\"\n")
	b.WriteString("          docker push \"${IMG}:${SHA}\"\n\n")
}
