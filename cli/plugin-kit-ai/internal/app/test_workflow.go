package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

func runPluginTests(ctx context.Context, opts PluginTestOptions) (PluginTestResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginTestResult{}, err
	}

	platform, err := resolveRuntimeTestPlatform(graph.Manifest.EnabledTargets(), opts.Platform)
	if err != nil {
		return PluginTestResult{}, err
	}
	if graph.Launcher == nil {
		return PluginTestResult{}, fmt.Errorf("test requires launcher-based target %q with launcher.yaml", platform)
	}

	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			return PluginTestResult{}, err
		}
	}
	if len(report.Failures) > 0 {
		return PluginTestResult{}, fmt.Errorf("test requires validate --strict to pass for %s", platform)
	}
	if len(report.Warnings) > 0 {
		return PluginTestResult{}, fmt.Errorf("test requires validate --strict to pass for %s: %d warning(s) present", platform, len(report.Warnings))
	}

	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: graph.Launcher,
	})
	if err != nil {
		return PluginTestResult{}, err
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return PluginTestResult{}, fmt.Errorf("test requires runtime readiness: %s", diagnosis.Reason)
	}

	supported := stableRuntimeSupport(platform)
	if len(supported) == 0 {
		return PluginTestResult{}, fmt.Errorf("test supports only stable runtime targets with built-in event metadata: claude or codex-runtime")
	}
	selected, err := selectRuntimeTestCases(supported, opts.Event, opts.All)
	if err != nil {
		return PluginTestResult{}, err
	}
	if opts.All && strings.TrimSpace(opts.Fixture) != "" {
		return PluginTestResult{}, fmt.Errorf("--fixture cannot be used with --all")
	}

	lines := []string{
		project.ProjectLine(),
		"Validate: ok",
	}
	cases := make([]PluginTestCase, 0, len(selected))
	anyNotConfigured := false
	passed := true
	summary := PluginTestSummary{Total: len(selected)}
	for _, item := range selected {
		tc := runRuntimeTestCase(ctx, root, project, opts, item)
		if tc.GoldenStatus == "not_configured" {
			anyNotConfigured = true
		}
		if !tc.Passed {
			passed = false
			summary.Failed++
		} else {
			summary.Passed++
		}
		switch tc.GoldenStatus {
		case "matched":
			summary.GoldenMatched++
		case "updated":
			summary.GoldenUpdated++
		case "not_configured":
			summary.GoldenNotConfigured++
		case "mismatch":
			summary.GoldenMismatch++
		}
		cases = append(cases, tc)
		lines = append(lines, formatRuntimeTestCaseLine(tc))
		lines = append(lines, formatRuntimeTestCaseDetails(tc)...)
	}
	lines = append(lines, formatRuntimeTestSummary(summary))
	if anyNotConfigured {
		lines = append(lines, "Tip: rerun with --update-golden to capture the current stdout/stderr/exit contract.")
		lines = append(lines, "CI hint: once goldens are committed, `plugin-kit-ai test --format json` provides machine-readable case and summary output.")
	}
	return PluginTestResult{
		Passed:  passed,
		Summary: summary,
		Lines:   lines,
		Cases:   cases,
	}, nil
}
