package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

type exportArchivePlan struct {
	outputPath string
	files      []string
	metadata   exportMetadata
}

func prepareExportArchivePlan(ctx exportServiceContext, output string) (exportArchivePlan, error) {
	generated, err := pluginmanifest.Generate(ctx.root, ctx.platform)
	if err != nil {
		return exportArchivePlan{}, err
	}

	files, err := exportFileList(ctx.root, ctx.graph, ctx.project, generated.Artifacts)
	if err != nil {
		return exportArchivePlan{}, err
	}
	outputPath := exportOutputPath(ctx.root, ctx.graph.Manifest.Name, ctx.platform, ctx.graph.Launcher.Runtime, output)
	if rel, ok := relWithinRoot(ctx.root, outputPath); ok {
		files = slices.DeleteFunc(files, func(path string) bool { return path == rel })
	}

	return exportArchivePlan{
		outputPath: outputPath,
		files:      files,
		metadata:   buildExportMetadata(ctx),
	}, nil
}

func buildExportMetadata(ctx exportServiceContext) exportMetadata {
	return exportMetadata{
		PluginName:         ctx.graph.Manifest.Name,
		Platform:           ctx.platform,
		Runtime:            ctx.graph.Launcher.Runtime,
		Manager:            exportManager(ctx.project),
		BootstrapModel:     exportBootstrapModel(ctx.project),
		RuntimeRequirement: exportRuntimeRequirement(ctx.project.Runtime),
		RuntimeInstallHint: exportRuntimeInstallHint(ctx.project.Runtime),
		Next: []string{
			"plugin-kit-ai doctor .",
			"plugin-kit-ai bootstrap .",
			fmt.Sprintf("plugin-kit-ai validate . --platform %s --strict", ctx.platform),
		},
		BundleFormat: "tar.gz",
		GeneratedBy:  "plugin-kit-ai export",
	}
}

func buildExportResultLines(ctx exportServiceContext, plan exportArchivePlan) []string {
	lines := []string{
		ctx.project.ProjectLine(),
		"Exported bundle: " + plan.outputPath,
		fmt.Sprintf("Included files: %d", len(plan.files)+1),
	}
	if strings.TrimSpace(plan.metadata.RuntimeRequirement) != "" {
		lines = append(lines, "Runtime requirement: "+plan.metadata.RuntimeRequirement)
	}
	if strings.TrimSpace(plan.metadata.RuntimeInstallHint) != "" {
		lines = append(lines, "Runtime install hint: "+plan.metadata.RuntimeInstallHint)
	}
	lines = append(lines,
		"Next:",
		"  tar -xzf "+filepath.Base(plan.outputPath),
		"  plugin-kit-ai doctor .",
		"  plugin-kit-ai bootstrap .",
		fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", ctx.platform),
	)
	return lines
}
