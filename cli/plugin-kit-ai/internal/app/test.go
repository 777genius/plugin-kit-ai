package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

var testCommandContext = exec.CommandContext

type PluginTestOptions struct {
	Root         string
	Platform     string
	Event        string
	Fixture      string
	GoldenDir    string
	UpdateGolden bool
	All          bool
}

type PluginTestCase struct {
	Platform     string               `json:"platform"`
	Event        string               `json:"event"`
	FixturePath  string               `json:"fixture_path"`
	Carrier      string               `json:"carrier"`
	Command      []string             `json:"command,omitempty"`
	Stdout       string               `json:"stdout"`
	Stderr       string               `json:"stderr"`
	ExitCode     int                  `json:"exit_code"`
	GoldenDir    string               `json:"golden_dir,omitempty"`
	GoldenStatus string               `json:"golden_status,omitempty"`
	GoldenFiles  []string             `json:"golden_files,omitempty"`
	Mismatches   []string             `json:"mismatches,omitempty"`
	MismatchInfo []PluginTestMismatch `json:"mismatch_info,omitempty"`
	Failure      string               `json:"failure,omitempty"`
	Passed       bool                 `json:"passed"`
}

type PluginTestMismatch struct {
	Field           string `json:"field"`
	GoldenFile      string `json:"golden_file,omitempty"`
	ExpectedPreview string `json:"expected_preview,omitempty"`
	ActualPreview   string `json:"actual_preview,omitempty"`
}

type PluginTestSummary struct {
	Total               int `json:"total"`
	Passed              int `json:"passed"`
	Failed              int `json:"failed"`
	GoldenMatched       int `json:"golden_matched"`
	GoldenUpdated       int `json:"golden_updated"`
	GoldenNotConfigured int `json:"golden_not_configured"`
	GoldenMismatch      int `json:"golden_mismatch"`
}

type PluginTestResult struct {
	Passed  bool              `json:"passed"`
	Summary PluginTestSummary `json:"summary"`
	Lines   []string          `json:"lines"`
	Cases   []PluginTestCase  `json:"cases"`
}

type runtimeTestSupport struct {
	Platform string
	Event    string
	Carrier  string
}

func (PluginService) Test(ctx context.Context, opts PluginTestOptions) (PluginTestResult, error) {
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

func resolveRuntimeTestPlatform(enabledTargets []string, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		if !isRuntimeTestPlatform(requested) {
			return "", fmt.Errorf("test supports only launcher-based runtime targets: claude or codex-runtime")
		}
		for _, target := range enabledTargets {
			if target == requested {
				return requested, nil
			}
		}
		return "", fmt.Errorf("plugin.yaml does not enable target %q", requested)
	}

	var candidates []string
	for _, target := range enabledTargets {
		if isRuntimeTestPlatform(target) {
			candidates = append(candidates, target)
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("test supports only launcher-based runtime targets: claude or codex-runtime")
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("test requires --platform when multiple launcher-based runtime targets are enabled (%s)", strings.Join(candidates, ", "))
	}
}

func isRuntimeTestPlatform(platform string) bool {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "claude", "codex-runtime":
		return true
	default:
		return false
	}
}

func stableRuntimeSupport(target string) []runtimeTestSupport {
	target = strings.ToLower(strings.TrimSpace(target))
	out := make([]runtimeTestSupport, 0, 4)
	for _, entry := range pluginkitai.Supported() {
		if mapSupportPlatformToTarget(string(entry.Platform)) != target {
			continue
		}
		if string(entry.Status) != "runtime_supported" || string(entry.Maturity) != "stable" {
			continue
		}
		out = append(out, runtimeTestSupport{
			Platform: target,
			Event:    string(entry.Event),
			Carrier:  string(entry.Carrier),
		})
	}
	return out
}

func mapSupportPlatformToTarget(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "codex":
		return "codex-runtime"
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func selectRuntimeTestCases(supported []runtimeTestSupport, requestedEvent string, all bool) ([]runtimeTestSupport, error) {
	if all {
		if strings.TrimSpace(requestedEvent) != "" {
			return nil, fmt.Errorf("--event cannot be used with --all")
		}
		return append([]runtimeTestSupport(nil), supported...), nil
	}
	requestedEvent = strings.TrimSpace(requestedEvent)
	if requestedEvent == "" {
		if len(supported) == 1 {
			return []runtimeTestSupport{supported[0]}, nil
		}
		names := make([]string, 0, len(supported))
		for _, item := range supported {
			names = append(names, item.Event)
		}
		return nil, fmt.Errorf("test requires --event or --all; supported stable events: %s", strings.Join(names, ", "))
	}
	for _, item := range supported {
		if strings.EqualFold(item.Event, requestedEvent) {
			return []runtimeTestSupport{item}, nil
		}
	}
	names := make([]string, 0, len(supported))
	for _, item := range supported {
		names = append(names, item.Event)
	}
	return nil, fmt.Errorf("unsupported stable event %q for %s; supported: %s", requestedEvent, supported[0].Platform, strings.Join(names, ", "))
}

func runRuntimeTestCase(ctx context.Context, root string, project runtimecheck.Project, opts PluginTestOptions, support runtimeTestSupport) PluginTestCase {
	fixturePath := resolveFixturePath(root, opts.Fixture, support.Platform, support.Event)
	tc := PluginTestCase{
		Platform:    support.Platform,
		Event:       support.Event,
		FixturePath: fixturePath,
		Carrier:     support.Carrier,
		GoldenDir:   resolveGoldenDir(root, opts.GoldenDir, support.Platform),
	}

	payload, err := os.ReadFile(fixturePath)
	if err != nil {
		tc.Failure = fmt.Sprintf("fixture read failed: %v", err)
		return tc
	}

	args, stdin, err := runtimeTestInvocation(project.LauncherPath, support.Event, support.Carrier, payload, support.Platform)
	if err != nil {
		tc.Failure = err.Error()
		return tc
	}
	tc.Command = append([]string(nil), args...)

	stdout, stderr, exitCode, execErr := executeRuntimeTestCommand(ctx, root, args, stdin)
	if execErr != nil {
		tc.Failure = execErr.Error()
		return tc
	}
	tc.Stdout = stdout
	tc.Stderr = stderr
	tc.ExitCode = exitCode

	status, files, mismatches, mismatchInfo, failure := processGoldenAssertions(tc.GoldenDir, support.Event, stdout, stderr, exitCode, opts.UpdateGolden)
	tc.GoldenStatus = status
	tc.GoldenFiles = files
	tc.Mismatches = mismatches
	tc.MismatchInfo = mismatchInfo
	tc.Failure = failure
	switch status {
	case "updated":
		tc.Passed = failure == "" && len(mismatches) == 0
	case "not_configured":
		tc.Passed = failure == "" && exitCode == 0
	default:
		tc.Passed = failure == "" && len(mismatches) == 0
	}
	return tc
}

func resolveFixturePath(root, requested, platform, event string) string {
	if strings.TrimSpace(requested) == "" {
		return filepath.Join(root, "fixtures", platform, event+".json")
	}
	return resolvePath(root, requested)
}

func resolveGoldenDir(root, requested, platform string) string {
	if strings.TrimSpace(requested) == "" {
		return filepath.Join(root, "goldens", platform)
	}
	return resolvePath(root, requested)
}

func resolvePath(root, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func runtimeTestInvocation(entrypoint string, event, carrier string, payload []byte, platform string) ([]string, []byte, error) {
	invocation := runtimeTestInvocationName(platform, event)
	switch carrier {
	case "stdin_json":
		return []string{entrypoint, invocation}, append([]byte(nil), payload...), nil
	case "argv_json":
		return []string{entrypoint, invocation, string(payload)}, nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported carrier %q for %s/%s", carrier, platform, event)
	}
}

func runtimeTestInvocationName(platform, event string) string {
	if platform == "codex-runtime" {
		return strings.ToLower(strings.TrimSpace(event))
	}
	return strings.TrimSpace(event)
}

func executeRuntimeTestCommand(ctx context.Context, root string, args []string, stdin []byte) (string, string, int, error) {
	if len(args) == 0 {
		return "", "", 0, fmt.Errorf("missing command")
	}
	cmd := testCommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = root
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return "", "", 0, fmt.Errorf("execute %s: %w", strings.Join(args, " "), err)
		}
		exitCode = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), exitCode, nil
}

func processGoldenAssertions(goldenDir, event, stdout, stderr string, exitCode int, update bool) (string, []string, []string, []PluginTestMismatch, string) {
	stdoutPath, stderrPath, exitCodePath := runtimeTestGoldenPaths(goldenDir, event)
	files := []string{stdoutPath, stderrPath, exitCodePath}
	if update {
		if err := os.MkdirAll(goldenDir, 0o755); err != nil {
			return "mismatch", files, nil, nil, fmt.Sprintf("golden write failed: %v", err)
		}
		if err := os.WriteFile(stdoutPath, []byte(stdout), 0o644); err != nil {
			return "mismatch", files, nil, nil, fmt.Sprintf("golden write failed: %v", err)
		}
		if err := os.WriteFile(stderrPath, []byte(stderr), 0o644); err != nil {
			return "mismatch", files, nil, nil, fmt.Sprintf("golden write failed: %v", err)
		}
		if err := os.WriteFile(exitCodePath, []byte(strconv.Itoa(exitCode)+"\n"), 0o644); err != nil {
			return "mismatch", files, nil, nil, fmt.Sprintf("golden write failed: %v", err)
		}
		return "updated", files, nil, nil, ""
	}

	existing := 0
	for _, path := range files {
		if _, err := os.Stat(path); err == nil {
			existing++
		}
	}
	if existing == 0 {
		return "not_configured", files, nil, nil, ""
	}
	if existing != len(files) {
		return "mismatch", files, []string{"golden_files"}, []PluginTestMismatch{{
			Field:           "golden_files",
			ExpectedPreview: "stdout/stderr/exitcode goldens must all exist",
			ActualPreview:   fmt.Sprintf("%d of %d files present", existing, len(files)),
		}}, "golden files are partially configured"
	}

	var mismatches []string
	var mismatchInfo []PluginTestMismatch
	wantStdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		return "mismatch", files, nil, nil, fmt.Sprintf("golden read failed: %v", err)
	}
	if string(wantStdout) != stdout {
		mismatches = append(mismatches, "stdout")
		mismatchInfo = append(mismatchInfo, PluginTestMismatch{
			Field:           "stdout",
			GoldenFile:      stdoutPath,
			ExpectedPreview: runtimeTestPreview(string(wantStdout)),
			ActualPreview:   runtimeTestPreview(stdout),
		})
	}
	wantStderr, err := os.ReadFile(stderrPath)
	if err != nil {
		return "mismatch", files, nil, nil, fmt.Sprintf("golden read failed: %v", err)
	}
	if string(wantStderr) != stderr {
		mismatches = append(mismatches, "stderr")
		mismatchInfo = append(mismatchInfo, PluginTestMismatch{
			Field:           "stderr",
			GoldenFile:      stderrPath,
			ExpectedPreview: runtimeTestPreview(string(wantStderr)),
			ActualPreview:   runtimeTestPreview(stderr),
		})
	}
	wantExitCodeRaw, err := os.ReadFile(exitCodePath)
	if err != nil {
		return "mismatch", files, nil, nil, fmt.Sprintf("golden read failed: %v", err)
	}
	wantExitCode, err := strconv.Atoi(strings.TrimSpace(string(wantExitCodeRaw)))
	if err != nil {
		return "mismatch", files, nil, nil, fmt.Sprintf("golden read failed: invalid exit code in %s", exitCodePath)
	}
	if wantExitCode != exitCode {
		mismatches = append(mismatches, "exit_code")
		mismatchInfo = append(mismatchInfo, PluginTestMismatch{
			Field:           "exit_code",
			GoldenFile:      exitCodePath,
			ExpectedPreview: strconv.Itoa(wantExitCode),
			ActualPreview:   strconv.Itoa(exitCode),
		})
	}
	if len(mismatches) > 0 {
		return "mismatch", files, mismatches, mismatchInfo, ""
	}
	return "matched", files, nil, nil, ""
}

func runtimeTestGoldenPaths(goldenDir, event string) (string, string, string) {
	base := filepath.Join(goldenDir, event)
	return base + ".stdout", base + ".stderr", base + ".exitcode"
}

func formatRuntimeTestCaseLine(tc PluginTestCase) string {
	status := "PASS"
	if !tc.Passed {
		status = "FAIL"
	}
	line := fmt.Sprintf("%s %s/%s", status, tc.Platform, tc.Event)
	if tc.FixturePath != "" {
		line += " fixture=" + tc.FixturePath
	}
	if tc.Failure != "" {
		line += " reason=" + tc.Failure
		return line
	}
	line += fmt.Sprintf(" exit=%d", tc.ExitCode)
	if tc.GoldenStatus != "" {
		line += " golden=" + tc.GoldenStatus
	}
	if len(tc.Mismatches) > 0 {
		line += " mismatches=" + strings.Join(tc.Mismatches, ",")
	}
	return line
}

func formatRuntimeTestCaseDetails(tc PluginTestCase) []string {
	var lines []string
	if tc.GoldenStatus == "not_configured" {
		lines = append(lines, "  goldens: not configured")
	}
	for _, mismatch := range tc.MismatchInfo {
		label := mismatch.Field
		if mismatch.GoldenFile != "" {
			label += " (" + mismatch.GoldenFile + ")"
		}
		lines = append(lines, fmt.Sprintf("  %s expected=%s actual=%s", label, mismatch.ExpectedPreview, mismatch.ActualPreview))
	}
	return lines
}

func formatRuntimeTestSummary(summary PluginTestSummary) string {
	return fmt.Sprintf(
		"Summary: total=%d passed=%d failed=%d golden_matched=%d golden_updated=%d golden_not_configured=%d golden_mismatch=%d",
		summary.Total,
		summary.Passed,
		summary.Failed,
		summary.GoldenMatched,
		summary.GoldenUpdated,
		summary.GoldenNotConfigured,
		summary.GoldenMismatch,
	)
}

func runtimeTestPreview(text string) string {
	switch {
	case text == "":
		return `"<empty>"`
	default:
		text = strings.ReplaceAll(text, "\n", `\n`)
		if len(text) > 120 {
			text = text[:120] + "..."
		}
		return strconv.Quote(text)
	}
}
