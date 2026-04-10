package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
)

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
