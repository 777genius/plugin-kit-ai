package pluginkitairepo_test

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Codex CLI --model for real hook e2e. Example:
//
//	PLUGIN_KIT_AI_RUN_CODEX_CLI=1 go test ./repotests -run TestCodexCLINotify -v -args -codex-model=gpt-5.4-mini
var codexModel = flag.String("codex-model", "gpt-5.4-mini", "codex exec --model for CLI e2e (notify smoke)")

func TestCodexCLINotify(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	hookBin := buildPluginKitAIE2E(t)
	trace := filepath.Join(t.TempDir(), "trace.ndjson")
	dir := t.TempDir()
	notifyOverride := codexNotifyOverride(t, trace, hookBin)

	runCodexExec(t, codexBin, dir, trace, *codexModel, "Reply with exactly OK.", "-c", notifyOverride)

	lines := waitForTraceLines(t, trace, 3*time.Second)
	rec, ok := traceFind(t, lines, "Notify")
	if !ok {
		t.Fatalf("expected Notify trace entry; got:\n%s", strings.Join(lines, "\n"))
	}
	if strings.TrimSpace(rec.Outcome) != "continue" {
		t.Fatalf("notify outcome = %q; want continue", rec.Outcome)
	}
	if strings.TrimSpace(rec.RawJSON) == "" {
		t.Fatalf("expected raw_json in trace entry; got %+v", rec)
	}
}

func codexBinaryOrSkip(t *testing.T) string {
	t.Helper()
	if strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_SKIP_CODEX_CLI")) == "1" {
		t.Skip("PLUGIN_KIT_AI_SKIP_CODEX_CLI=1")
	}
	if strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_RUN_CODEX_CLI")) != "1" {
		t.Skip("set PLUGIN_KIT_AI_RUN_CODEX_CLI=1 to run real Codex CLI e2e (see -args -codex-model)")
	}
	codexBin := strings.TrimSpace(os.Getenv("PLUGIN_KIT_AI_E2E_CODEX"))
	if codexBin == "" {
		var err error
		codexBin, err = exec.LookPath("codex")
		if err != nil {
			t.Skip("codex not in PATH; set PLUGIN_KIT_AI_E2E_CODEX or install Codex CLI")
		}
	}
	if out, err := exec.Command(codexBin, "login", "status").CombinedOutput(); err != nil {
		t.Skipf("codex login status failed (need login): %v\n%s", err, out)
	}
	return codexBin
}

func codexNotifyOverride(t *testing.T, traceFile, hookBin string) string {
	t.Helper()
	absHook, err := filepath.Abs(hookBin)
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(t.TempDir(), "codex-notify-wrapper.sh")
	script := "#!/bin/sh\n" +
		"trace_file=\"$1\"\n" +
		"hook_bin=\"$2\"\n" +
		"shift 2\n" +
		"PLUGIN_KIT_AI_E2E_TRACE=\"$trace_file\" exec \"$hook_bin\" \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	absWrapper, err := filepath.Abs(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	quoted := []string{
		"notify=[",
		quoteTOMLString(absWrapper), ",",
		quoteTOMLString(traceFile), ",",
		quoteTOMLString(absHook), ",",
		quoteTOMLString("notify"),
		"]",
	}
	return strings.Join(quoted, "")
}

func quoteTOMLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func runCodexExec(t *testing.T, codexBin, projectDir, traceFile, model, prompt string, extraArgs ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	outputFile := filepath.Join(t.TempDir(), "last-message.txt")
	logFile := filepath.Join(t.TempDir(), "codex.log")
	args := []string{
		"exec",
		"--skip-git-repo-check",
		"--ephemeral",
		"-C", projectDir,
		"-m", model,
		"--color", "never",
		"--output-last-message", outputFile,
	}
	args = append(args, extraArgs...)
	args = append(args, prompt)
	cmd := exec.CommandContext(ctx, codexBin, args...)
	cmd.Env = os.Environ()
	logfh, err := os.Create(logFile)
	if err != nil {
		t.Fatal(err)
	}
	defer logfh.Close()
	cmd.Stdout = logfh
	cmd.Stderr = logfh
	if err := cmd.Start(); err != nil {
		t.Fatalf("codex exec start: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	if err := waitForCodexInvariants(t, traceFile, outputFile, waitCh); err != nil {
		out := readLogFile(t, logFile)
		if codexRuntimeUnhealthy(out) {
			t.Skipf("codex runtime unhealthy in current environment:\n%s", truncateRunes(out, 4000))
		}
		t.Logf("codex output:\n%s", out)
		t.Fatal(err)
	}

	select {
	case err := <-waitCh:
		out := readLogFile(t, logFile)
		if err != nil {
			if codexRuntimeUnhealthy(out) {
				t.Skipf("codex runtime unhealthy in current environment:\n%s", truncateRunes(out, 4000))
			}
			t.Logf("codex output:\n%s", out)
			t.Fatalf("codex exec: %v", err)
		}
		t.Logf("codex output (truncated): %s", truncateRunes(out, 4000))
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
		out := readLogFile(t, logFile)
		t.Logf("codex output (truncated, process killed after invariants): %s", truncateRunes(out, 4000))
	}
}

func waitForCodexInvariants(t *testing.T, traceFile, outputFile string, waitCh <-chan error) error {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		if lines := readTraceLines(t, traceFile); len(lines) > 0 {
			if _, ok := traceFind(t, lines, "Notify"); ok {
				if b, err := os.ReadFile(outputFile); err == nil && strings.TrimSpace(string(b)) != "" {
					return nil
				}
			}
		}
		select {
		case err := <-waitCh:
			if err != nil {
				return fmt.Errorf("codex exec exited before invariants: %w", err)
			}
			if lines := readTraceLines(t, traceFile); len(lines) == 0 {
				return fmt.Errorf("codex exec exited without trace entry")
			}
			if b, err := os.ReadFile(outputFile); err != nil || strings.TrimSpace(string(b)) == "" {
				if err != nil {
					return fmt.Errorf("codex exec exited without last message file: %w", err)
				}
				return fmt.Errorf("codex exec exited with empty last message file")
			}
			return nil
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for codex notify invariants")
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, f)
	return buf.String()
}

func codexRuntimeUnhealthy(log string) bool {
	markers := []string{
		"Could not create otel exporter",
		"Attempted to create a NULL object.",
		"event loop thread panicked",
		"failed to refresh available models",
	}
	for _, marker := range markers {
		if strings.Contains(log, marker) {
			return true
		}
	}
	return false
}
