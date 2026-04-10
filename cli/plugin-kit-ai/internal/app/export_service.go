package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

func (PluginService) Export(opts PluginExportOptions) (PluginExportResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	platform := strings.TrimSpace(opts.Platform)
	if platform == "" {
		return PluginExportResult{}, fmt.Errorf("export requires --platform")
	}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginExportResult{}, err
	}
	if _, err := graph.Manifest.SelectedTargets(platform); err != nil {
		return PluginExportResult{}, err
	}
	if platform != "codex-runtime" && platform != "claude" {
		return PluginExportResult{}, fmt.Errorf("export supports only launcher-based interpreted targets: codex-runtime or claude")
	}
	if graph.Launcher == nil {
		return PluginExportResult{}, fmt.Errorf("export requires launcher-based target %q with launcher.yaml", platform)
	}
	switch graph.Launcher.Runtime {
	case "python", "node", "shell":
	default:
		return PluginExportResult{}, fmt.Errorf("export supports only interpreted runtimes (python, node, shell); found %q", graph.Launcher.Runtime)
	}

	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			return PluginExportResult{}, err
		}
	}
	if failures := exportBlockingFailures(report.Failures); len(failures) > 0 {
		return PluginExportResult{}, fmt.Errorf("export requires validate --strict to pass for %s", platform)
	}
	if len(report.Warnings) > 0 {
		return PluginExportResult{}, fmt.Errorf("export requires validate --strict to pass for %s: %d warning(s) present", platform, len(report.Warnings))
	}

	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		return PluginExportResult{}, err
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return PluginExportResult{}, fmt.Errorf("export requires runtime readiness: %s", diagnosis.Reason)
	}

	generated, err := pluginmanifest.Generate(root, platform)
	if err != nil {
		return PluginExportResult{}, err
	}

	files, err := exportFileList(root, graph, project, generated.Artifacts)
	if err != nil {
		return PluginExportResult{}, err
	}
	outputPath := exportOutputPath(root, graph.Manifest.Name, platform, graph.Launcher.Runtime, opts.Output)
	if rel, ok := relWithinRoot(root, outputPath); ok {
		files = slices.DeleteFunc(files, func(path string) bool { return path == rel })
	}
	metadata := exportMetadata{
		PluginName:         graph.Manifest.Name,
		Platform:           platform,
		Runtime:            graph.Launcher.Runtime,
		Manager:            exportManager(project),
		BootstrapModel:     exportBootstrapModel(project),
		RuntimeRequirement: exportRuntimeRequirement(project.Runtime),
		RuntimeInstallHint: exportRuntimeInstallHint(project.Runtime),
		Next: []string{
			"plugin-kit-ai doctor .",
			"plugin-kit-ai bootstrap .",
			fmt.Sprintf("plugin-kit-ai validate . --platform %s --strict", platform),
		},
		BundleFormat: "tar.gz",
		GeneratedBy:  "plugin-kit-ai export",
	}
	if err := writeExportArchive(root, outputPath, files, metadata); err != nil {
		return PluginExportResult{}, err
	}

	lines := []string{
		project.ProjectLine(),
		"Exported bundle: " + outputPath,
		fmt.Sprintf("Included files: %d", len(files)+1),
	}
	if strings.TrimSpace(metadata.RuntimeRequirement) != "" {
		lines = append(lines, "Runtime requirement: "+metadata.RuntimeRequirement)
	}
	if strings.TrimSpace(metadata.RuntimeInstallHint) != "" {
		lines = append(lines, "Runtime install hint: "+metadata.RuntimeInstallHint)
	}
	lines = append(lines,
		"Next:",
		"  tar -xzf "+filepath.Base(outputPath),
		"  plugin-kit-ai doctor .",
		"  plugin-kit-ai bootstrap .",
		fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
	)
	return PluginExportResult{Lines: lines}, nil
}

func exportBlockingFailures(failures []validate.Failure) []validate.Failure {
	return slices.DeleteFunc(slices.Clone(failures), func(f validate.Failure) bool {
		return f.Kind == validate.FailureGeneratedContractInvalid
	})
}
