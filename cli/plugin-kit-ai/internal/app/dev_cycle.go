package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

func (svc PluginService) runDevCycle(ctx context.Context, root, platform string, opts PluginDevOptions, cycle int, trigger string, changed []string) PluginDevUpdate {
	lines := []string{devCycleHeader(cycle, trigger, changed)}
	update := PluginDevUpdate{Cycle: cycle, Passed: false}

	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		lines = append(lines, "Discover: "+err.Error())
		update.Lines = lines
		return update
	}
	if _, err := graph.Manifest.SelectedTargets(platform); err != nil {
		lines = append(lines, err.Error())
		update.Lines = lines
		return update
	}
	if graph.Launcher == nil {
		lines = append(lines, "launcher invalid: missing launcher.yaml")
		update.Lines = lines
		return update
	}

	if !svc.runDevGenerate(root, platform, &lines) {
		update.Lines = lines
		return update
	}

	project, ok := inspectRuntimeForDev(root, platform, graph, &lines)
	if !ok {
		update.Lines = lines
		return update
	}
	if ok := svc.runDevAutoBuild(ctx, root, graph, project, &lines); !ok {
		update.Lines = lines
		return update
	}
	if ok := runDevValidate(root, platform, &lines); !ok {
		update.Lines = lines
		return update
	}

	project, ok = inspectRuntimeForDev(root, platform, graph, &lines)
	if !ok {
		update.Lines = lines
		return update
	}
	diagnosis := runtimecheck.Diagnose(project)
	lines = append(lines, project.ProjectLine())
	lines = append(lines, fmt.Sprintf("Runtime: %s (%s)", diagnosis.Status, diagnosis.Reason))
	if diagnosis.Status != runtimecheck.StatusReady {
		update.Lines = lines
		return update
	}

	update.Passed = runDevTests(ctx, root, platform, project, opts, &lines)
	update.Lines = lines
	return update
}

func (svc PluginService) runDevGenerate(root, platform string, lines *[]string) bool {
	generated, err := svc.Generate(PluginGenerateOptions{Root: root, Target: platform})
	if err != nil {
		*lines = append(*lines, "Generate: "+err.Error())
		return false
	}
	*lines = append(*lines, fmt.Sprintf("Generate: wrote %d artifact(s)", len(generated)))
	return true
}

func inspectRuntimeForDev(root, platform string, graph pluginmanifest.PackageGraph, lines *[]string) (runtimecheck.Project, bool) {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		*lines = append(*lines, "Runtime inspect: "+err.Error())
		return runtimecheck.Project{}, false
	}
	return project, true
}

func (svc PluginService) runDevAutoBuild(ctx context.Context, root string, graph pluginmanifest.PackageGraph, project runtimecheck.Project, lines *[]string) bool {
	buildLines, err := devAutoBuild(ctx, root, graph, project)
	*lines = append(*lines, buildLines...)
	if err != nil {
		*lines = append(*lines, "Build: "+err.Error())
		return false
	}
	return true
}

func runDevValidate(root, platform string, lines *[]string) bool {
	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			*lines = append(*lines, "Validate: "+err.Error())
			return false
		}
	}
	if len(report.Failures) > 0 {
		*lines = append(*lines, fmt.Sprintf("Validate: %d failure(s)", len(report.Failures)))
		for _, failure := range report.Failures {
			*lines = append(*lines, "  - "+failure.Message)
		}
		return false
	}
	if len(report.Warnings) > 0 {
		*lines = append(*lines, fmt.Sprintf("Validate: %d warning(s) (strict)", len(report.Warnings)))
		for _, warning := range report.Warnings {
			*lines = append(*lines, "  - "+warning.Message)
		}
		return false
	}
	*lines = append(*lines, "Validate: ok")
	return true
}

func runDevTests(ctx context.Context, root, platform string, project runtimecheck.Project, opts PluginDevOptions, lines *[]string) bool {
	supported := stableRuntimeSupport(platform)
	selected, err := selectRuntimeTestCases(supported, opts.Event, opts.All)
	if err != nil {
		*lines = append(*lines, err.Error())
		return false
	}
	if opts.All && strings.TrimSpace(opts.Fixture) != "" {
		*lines = append(*lines, "--fixture cannot be used with --all")
		return false
	}

	passed := true
	for _, item := range selected {
		tc := runRuntimeTestCase(ctx, root, project, PluginTestOptions{
			Root:      root,
			Platform:  platform,
			Event:     opts.Event,
			Fixture:   opts.Fixture,
			GoldenDir: opts.GoldenDir,
			All:       opts.All,
		}, item)
		*lines = append(*lines, formatRuntimeTestCaseLine(tc))
		if strings.TrimSpace(tc.Stdout) != "" {
			*lines = append(*lines, "  stdout: "+singleLinePreview(tc.Stdout))
		}
		if strings.TrimSpace(tc.Stderr) != "" {
			*lines = append(*lines, "  stderr: "+singleLinePreview(tc.Stderr))
		}
		if tc.Failure != "" || !tc.Passed {
			passed = false
		}
	}
	return passed
}
