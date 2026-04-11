package app

import (
	"fmt"
	"path/filepath"
	"strings"
)

func buildExportResultLines(ctx exportServiceContext, plan exportArchivePlan) []string {
	lines := []string{
		ctx.project.ProjectLine(),
		"Exported bundle: " + plan.outputPath,
		fmt.Sprintf("Included files: %d", len(plan.files)+1),
	}
	lines = appendExportRuntimeLines(lines, plan.metadata)
	lines = append(lines,
		"Next:",
		"  tar -xzf "+filepath.Base(plan.outputPath),
		"  plugin-kit-ai doctor .",
		"  plugin-kit-ai bootstrap .",
		fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", ctx.platform),
	)
	return lines
}

func appendExportRuntimeLines(lines []string, metadata exportMetadata) []string {
	if strings.TrimSpace(metadata.RuntimeRequirement) != "" {
		lines = append(lines, "Runtime requirement: "+metadata.RuntimeRequirement)
	}
	if strings.TrimSpace(metadata.RuntimeInstallHint) != "" {
		lines = append(lines, "Runtime install hint: "+metadata.RuntimeInstallHint)
	}
	return lines
}
