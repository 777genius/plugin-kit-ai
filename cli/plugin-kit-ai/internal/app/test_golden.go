package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
