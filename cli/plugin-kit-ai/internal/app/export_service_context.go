package app

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

type exportServiceContext struct {
	root     string
	platform string
	graph    pluginmanifest.PackageGraph
	project  runtimecheck.Project
}

func loadExportServiceContext(opts PluginExportOptions) (exportServiceContext, error) {
	root, platform, err := resolveExportServiceInput(opts)
	if err != nil {
		return exportServiceContext{}, err
	}
	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return exportServiceContext{}, err
	}
	if err := validateExportServiceGraph(graph, platform); err != nil {
		return exportServiceContext{}, err
	}
	if err := validateExportServiceReadiness(root, platform); err != nil {
		return exportServiceContext{}, err
	}
	project, err := inspectReadyExportProject(root, platform, graph.Launcher)
	if err != nil {
		return exportServiceContext{}, err
	}
	return exportServiceContext{
		root:     root,
		platform: platform,
		graph:    graph,
		project:  project,
	}, nil
}

func resolveExportServiceInput(opts PluginExportOptions) (string, string, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	platform := strings.TrimSpace(opts.Platform)
	if platform == "" {
		return "", "", fmt.Errorf("export requires --platform")
	}
	return root, platform, nil
}

func validateExportServiceGraph(graph pluginmanifest.PackageGraph, platform string) error {
	if _, err := graph.Manifest.SelectedTargets(platform); err != nil {
		return err
	}
	if platform != "codex-runtime" && platform != "claude" {
		return fmt.Errorf("export supports only launcher-based interpreted targets: codex-runtime or claude")
	}
	if graph.Launcher == nil {
		return fmt.Errorf("export requires launcher-based target %q with launcher.yaml", platform)
	}
	switch graph.Launcher.Runtime {
	case "python", "node", "shell":
		return nil
	default:
		return fmt.Errorf("export supports only interpreted runtimes (python, node, shell); found %q", graph.Launcher.Runtime)
	}
}

func validateExportServiceReadiness(root, platform string) error {
	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			return err
		}
	}
	if failures := exportBlockingFailures(report.Failures); len(failures) > 0 {
		return fmt.Errorf("export requires validate --strict to pass for %s", platform)
	}
	if len(report.Warnings) > 0 {
		return fmt.Errorf("export requires validate --strict to pass for %s: %d warning(s) present", platform, len(report.Warnings))
	}
	return nil
}

func inspectReadyExportProject(root, platform string, launcher *pluginmanifest.Launcher) (runtimecheck.Project, error) {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: launcher,
	})
	if err != nil {
		return runtimecheck.Project{}, err
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return runtimecheck.Project{}, fmt.Errorf("export requires runtime readiness: %s", diagnosis.Reason)
	}
	return project, nil
}
