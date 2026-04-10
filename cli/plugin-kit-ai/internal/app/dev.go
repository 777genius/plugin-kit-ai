package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

type PluginDevOptions struct {
	Root      string
	Platform  string
	Event     string
	Fixture   string
	GoldenDir string
	All       bool
	Once      bool
	Interval  time.Duration
}

type PluginDevUpdate struct {
	Cycle  int
	Passed bool
	Lines  []string
}

type PluginDevSummary struct {
	Cycles     int
	LastPassed bool
}

func (svc PluginService) Dev(ctx context.Context, opts PluginDevOptions, emit func(PluginDevUpdate)) (PluginDevSummary, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = 750 * time.Millisecond
	}
	if emit == nil {
		emit = func(PluginDevUpdate) {}
	}

	selectedPlatform, err := resolveDevPlatform(root, opts.Platform)
	if err != nil {
		return PluginDevSummary{}, err
	}

	cycle := 0
	runCycle := func(trigger string, changed []string) (bool, error) {
		cycle++
		update := svc.runDevCycle(ctx, root, selectedPlatform, opts, cycle, trigger, changed)
		emit(update)
		return update.Passed, nil
	}

	lastPassed, err := runCycle("initial", nil)
	if err != nil {
		return PluginDevSummary{}, err
	}
	if opts.Once {
		return PluginDevSummary{Cycles: cycle, LastPassed: lastPassed}, nil
	}

	snapshot, err := takeDevSnapshot(root)
	if err != nil {
		return PluginDevSummary{}, err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return PluginDevSummary{Cycles: cycle, LastPassed: lastPassed}, nil
		case <-ticker.C:
			next, err := takeDevSnapshot(root)
			if err != nil {
				update := PluginDevUpdate{
					Cycle:  cycle + 1,
					Passed: false,
					Lines:  []string{fmt.Sprintf("Cycle %d [watch]: snapshot failed: %v", cycle+1, err)},
				}
				cycle++
				lastPassed = false
				emit(update)
				continue
			}
			changed := devSnapshotChanges(snapshot, next)
			if len(changed) == 0 {
				continue
			}
			lastPassed, _ = runCycle("watch", changed)
			snapshot, err = takeDevSnapshot(root)
			if err != nil {
				return PluginDevSummary{Cycles: cycle, LastPassed: lastPassed}, err
			}
		}
	}
}

func resolveDevPlatform(root, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if !isRuntimeTestPlatform(requested) {
			return "", runtimeDevUnsupportedPlatformError(nil, requested)
		}
		return strings.ToLower(requested), nil
	}
	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return "", err
	}
	enabledTargets := graph.Manifest.EnabledTargets()
	platform, err := resolveRuntimeTestPlatform(enabledTargets, "")
	if err != nil {
		return "", err
	}
	return platform, nil
}

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

	generated, err := svc.Generate(PluginGenerateOptions{Root: root, Target: platform})
	if err != nil {
		lines = append(lines, "Generate: "+err.Error())
		update.Lines = lines
		return update
	}
	lines = append(lines, fmt.Sprintf("Generate: wrote %d artifact(s)", len(generated)))

	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		lines = append(lines, "Runtime inspect: "+err.Error())
		update.Lines = lines
		return update
	}
	if buildLines, err := devAutoBuild(ctx, root, graph, project); err != nil {
		lines = append(lines, buildLines...)
		lines = append(lines, "Build: "+err.Error())
		update.Lines = lines
		return update
	} else if len(buildLines) > 0 {
		lines = append(lines, buildLines...)
	}

	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			lines = append(lines, "Validate: "+err.Error())
			update.Lines = lines
			return update
		}
	}
	if len(report.Failures) > 0 {
		lines = append(lines, fmt.Sprintf("Validate: %d failure(s)", len(report.Failures)))
		for _, failure := range report.Failures {
			lines = append(lines, "  - "+failure.Message)
		}
		update.Lines = lines
		return update
	}
	if len(report.Warnings) > 0 {
		lines = append(lines, fmt.Sprintf("Validate: %d warning(s) (strict)", len(report.Warnings)))
		for _, warning := range report.Warnings {
			lines = append(lines, "  - "+warning.Message)
		}
		update.Lines = lines
		return update
	}
	lines = append(lines, "Validate: ok")

	project, err = runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		lines = append(lines, "Runtime inspect: "+err.Error())
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

	supported := stableRuntimeSupport(platform)
	selected, err := selectRuntimeTestCases(supported, opts.Event, opts.All)
	if err != nil {
		lines = append(lines, err.Error())
		update.Lines = lines
		return update
	}
	if opts.All && strings.TrimSpace(opts.Fixture) != "" {
		lines = append(lines, "--fixture cannot be used with --all")
		update.Lines = lines
		return update
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
		lines = append(lines, formatRuntimeTestCaseLine(tc))
		if strings.TrimSpace(tc.Stdout) != "" {
			lines = append(lines, "  stdout: "+singleLinePreview(tc.Stdout))
		}
		if strings.TrimSpace(tc.Stderr) != "" {
			lines = append(lines, "  stderr: "+singleLinePreview(tc.Stderr))
		}
		if tc.Failure != "" {
			passed = false
			continue
		}
		if !tc.Passed {
			passed = false
		}
	}

	update.Passed = passed
	update.Lines = lines
	return update
}
