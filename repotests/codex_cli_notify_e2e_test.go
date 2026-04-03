package pluginkitairepo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestCodexProductionExampleNotifyUsesRealCLI(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	dir, binPath := newRenderedCodexRuntimeExampleWorkspace(t, pluginKitAIBin)
	markerFile := filepath.Join(t.TempDir(), "notify-marker.txt")
	notifyOverride := codexNotifyBinaryOverride(t, markerFile, binPath)

	logOutput, output := runCodexExecWithMarkerProbe(t, codexBin, dir, markerFile, "Reply with exactly OK.", "-c", notifyOverride, "-m", *codexModel)
	if strings.TrimSpace(output) != "OK" {
		t.Fatalf("codex exec last message = %q, want %q\n%s", strings.TrimSpace(output), "OK", logOutput)
	}
	if _, err := os.Stat(markerFile); err != nil {
		t.Fatalf("production example notify marker missing: %v\n%s", err, logOutput)
	}
}

func TestCodexCLINotifyUsesRenderedProjectConfig(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	hookBin := buildPluginKitAIE2E(t)
	trace := filepath.Join(t.TempDir(), "trace.ndjson")
	dir := newCodexRenderedNotifyWorkspace(t, pluginKitAIBin, hookBin, trace, *codexModel)

	logOutput, output, lines := runCodexExecWithProjectConfigProbe(t, codexBin, dir, trace, "Reply with exactly OK.")
	if strings.TrimSpace(output) != "OK" {
		t.Fatalf("codex exec last message = %q, want %q\n%s", strings.TrimSpace(output), "OK", logOutput)
	}
	if !strings.Contains(logOutput, "model: "+*codexModel) {
		t.Skipf("real codex exec did not honor project-local .codex/config.toml model %q in this build:\n%s", *codexModel, truncateRunes(logOutput, 4000))
	}
	rec, ok := traceFind(t, lines, "Notify")
	if !ok {
		t.Skipf("real codex exec did not invoke notify from project-local .codex/config.toml in this build:\n%s", truncateRunes(logOutput, 4000))
	}
	if strings.TrimSpace(rec.Outcome) != "continue" {
		t.Fatalf("notify outcome = %q; want continue", rec.Outcome)
	}
}

func TestCodexCLIMCPUsesRenderedProjectConfig(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	dir := newCodexRenderedMCPWorkspace(t, pluginKitAIBin)

	out := runCodexMCPGetProbe(t, codexBin, dir, "release-checks")
	if !strings.Contains(out, `"name":"release-checks"`) && !strings.Contains(out, `"name": "release-checks"`) {
		t.Skipf("real codex mcp get did not expose project-local .codex/config.toml MCP server in this build:\n%s", truncateRunes(out, 4000))
	}
	if !strings.Contains(out, `"/bin/echo"`) {
		t.Fatalf("codex mcp get output missing rendered command %q:\n%s", "/bin/echo", out)
	}
}

func TestCodexCLIMCPListUsesRenderedProjectConfig(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	dir := newCodexRenderedMCPWorkspace(t, pluginKitAIBin)

	out := runCodexMCPListWithProjectConfigProbe(t, codexBin, dir)
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Skipf("real codex mcp list did not return JSON for project-local .codex/config.toml in this build:\n%s", truncateRunes(out, 4000))
	}
	for _, entry := range entries {
		if strings.TrimSpace(fmt.Sprint(entry["name"])) != "release-checks" {
			continue
		}
		transport, ok := entry["transport"].(map[string]any)
		if !ok {
			t.Fatalf("codex mcp list release-checks entry missing transport:\n%s", out)
		}
		if strings.TrimSpace(fmt.Sprint(transport["command"])) != "/bin/echo" {
			t.Fatalf("codex mcp list release-checks command = %q want %q\n%s", transport["command"], "/bin/echo", out)
		}
		return
	}
	t.Skipf("real codex mcp list did not expose project-local .codex/config.toml MCP server in this build:\n%s", truncateRunes(out, 4000))
}

func TestCodexCLIMCPGetWithOverride(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	out := runCodexMCPGetWithArgs(t, codexBin, "release-checks",
		"-c", `mcp_servers.release-checks.command="/bin/echo"`,
		"-c", `mcp_servers.release-checks.args=["hello"]`,
	)
	if !strings.Contains(out, `"name":"release-checks"`) && !strings.Contains(out, `"name": "release-checks"`) {
		t.Fatalf("codex mcp get output missing server name:\n%s", out)
	}
	if !strings.Contains(out, `"/bin/echo"`) {
		t.Fatalf("codex mcp get output missing override command %q:\n%s", "/bin/echo", out)
	}
	if !strings.Contains(out, `"hello"`) {
		t.Fatalf("codex mcp get output missing override args:\n%s", out)
	}
}

func TestCodexCLIMCPListWithOverride(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	out := runCodexMCPListWithArgs(t, codexBin,
		"-c", `mcp_servers.release-checks.command="/bin/echo"`,
		"-c", `mcp_servers.release-checks.args=["hello"]`,
	)
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("parse codex mcp list output: %v\n%s", err, out)
	}
	for _, entry := range entries {
		if strings.TrimSpace(fmt.Sprint(entry["name"])) != "release-checks" {
			continue
		}
		transport, ok := entry["transport"].(map[string]any)
		if !ok {
			t.Fatalf("codex mcp list release-checks entry missing transport:\n%s", out)
		}
		if strings.TrimSpace(fmt.Sprint(transport["command"])) != "/bin/echo" {
			t.Fatalf("codex mcp list release-checks command = %q want %q\n%s", transport["command"], "/bin/echo", out)
		}
		args, ok := transport["args"].([]any)
		if !ok || len(args) != 1 || strings.TrimSpace(fmt.Sprint(args[0])) != "hello" {
			t.Fatalf("codex mcp list release-checks args = %#v want [hello]\n%s", transport["args"], out)
		}
		return
	}
	t.Fatalf("codex mcp list output missing release-checks server:\n%s", out)
}

func TestCodexPackageMCPGetUsesRenderedSidecar(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	mcpBin := buildPortableMCPSmokeServer(t)
	workDir := newCodexPackageRenderedMCPWorkspace(t, pluginKitAIBin, mcpBin)
	mcpServer := readRenderedSharedMCPServer(t, workDir, "release-checks")
	configArgs := codexMCPConfigArgs("release-checks", mcpServer)

	out := runCodexMCPGetWithArgs(t, codexBin, "release-checks", configArgs...)
	if !strings.Contains(out, `"name":"release-checks"`) && !strings.Contains(out, `"name": "release-checks"`) {
		t.Fatalf("codex mcp get output missing rendered package MCP server name:\n%s", out)
	}
	wantCommand := filepath.ToSlash(mcpBin)
	if !strings.Contains(out, wantCommand) {
		t.Fatalf("codex mcp get output missing rendered package MCP command %q:\n%s", wantCommand, out)
	}
	if !strings.Contains(out, `"PLUGIN_KIT_AI_MCP_SMOKE_STATIC":"codex-package-live"`) &&
		!strings.Contains(out, `"PLUGIN_KIT_AI_MCP_SMOKE_STATIC": "codex-package-live"`) {
		t.Fatalf("codex mcp get output missing rendered package MCP env:\n%s", out)
	}
}

func TestCodexPackageMCPListUsesRenderedSidecar(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	mcpBin := buildPortableMCPSmokeServer(t)
	workDir := newCodexPackageRenderedMCPWorkspace(t, pluginKitAIBin, mcpBin)
	mcpServer := readRenderedSharedMCPServer(t, workDir, "release-checks")
	configArgs := codexMCPConfigArgs("release-checks", mcpServer)

	out := runCodexMCPListWithArgs(t, codexBin, configArgs...)
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("parse codex mcp list output: %v\n%s", err, out)
	}
	wantCommand := filepath.ToSlash(mcpBin)
	for _, entry := range entries {
		if strings.TrimSpace(fmt.Sprint(entry["name"])) != "release-checks" {
			continue
		}
		transport, ok := entry["transport"].(map[string]any)
		if !ok {
			t.Fatalf("codex mcp list release-checks entry missing transport:\n%s", out)
		}
		if strings.TrimSpace(fmt.Sprint(transport["type"])) != "stdio" {
			t.Fatalf("codex mcp list release-checks transport type = %q want %q\n%s", transport["type"], "stdio", out)
		}
		if strings.TrimSpace(fmt.Sprint(transport["command"])) != wantCommand {
			t.Fatalf("codex mcp list release-checks command = %q want %q\n%s", transport["command"], wantCommand, out)
		}
		env, ok := transport["env"].(map[string]any)
		if !ok {
			t.Fatalf("codex mcp list release-checks transport missing env:\n%s", out)
		}
		if strings.TrimSpace(fmt.Sprint(env["PLUGIN_KIT_AI_MCP_SMOKE_STATIC"])) != "codex-package-live" {
			t.Fatalf("codex mcp list release-checks env value = %q want %q\n%s", env["PLUGIN_KIT_AI_MCP_SMOKE_STATIC"], "codex-package-live", out)
		}
		return
	}
	t.Fatalf("codex mcp list output missing release-checks server:\n%s", out)
}

func TestCodexPackageExecUsesRenderedSidecarMCP(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	mcpBin := buildPortableMCPSmokeServer(t)
	workDir := newCodexPackageRenderedMCPWorkspace(t, pluginKitAIBin, mcpBin)
	marker := filepath.Join(t.TempDir(), "codex-package-mcp-marker.json")

	runCodexExecWithPortableMCP(t, codexBin, workDir, marker, *codexModel, mcpBin)
	assertPortableMCPMarker(t, marker, "tools/call", "release_checks", "CODEX_PORTABLE_MCP_OK")
}

func TestCodexPackageProductionExampleMCPGetUsesRenderedSidecar(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	workDir := newRenderedCodexPackageExampleWorkspace(t, pluginKitAIBin)
	server := readRenderedSharedMCPServer(t, workDir, "docs")
	configArgs := codexMCPConfigArgsFromRenderedServer(t, "docs", server)

	out := runCodexMCPGetWithArgs(t, codexBin, "docs", configArgs...)
	if !strings.Contains(out, `"name":"docs"`) && !strings.Contains(out, `"name": "docs"`) {
		t.Fatalf("codex mcp get output missing docs server name:\n%s", out)
	}
	if !strings.Contains(out, `"type":"streamable_http"`) && !strings.Contains(out, `"type": "streamable_http"`) {
		t.Fatalf("codex mcp get output missing streamable_http transport:\n%s", out)
	}
	if !strings.Contains(out, `"url":"https://example.com/mcp"`) && !strings.Contains(out, `"url": "https://example.com/mcp"`) {
		t.Fatalf("codex mcp get output missing production example MCP URL:\n%s", out)
	}
}

func TestCodexPackageProductionExampleMCPListUsesRenderedSidecar(t *testing.T) {
	codexBin := codexBinaryOrSkip(t)
	pluginKitAIBin := buildPluginKitAI(t)
	workDir := newRenderedCodexPackageExampleWorkspace(t, pluginKitAIBin)
	server := readRenderedSharedMCPServer(t, workDir, "docs")
	configArgs := codexMCPConfigArgsFromRenderedServer(t, "docs", server)

	out := runCodexMCPListWithArgs(t, codexBin, configArgs...)
	var entries []map[string]any
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("parse codex mcp list output: %v\n%s", err, out)
	}
	for _, entry := range entries {
		if strings.TrimSpace(fmt.Sprint(entry["name"])) != "docs" {
			continue
		}
		transport, ok := entry["transport"].(map[string]any)
		if !ok {
			t.Fatalf("codex mcp list docs entry missing transport:\n%s", out)
		}
		if strings.TrimSpace(fmt.Sprint(transport["type"])) != "streamable_http" {
			t.Fatalf("codex mcp list docs transport type = %q want %q\n%s", transport["type"], "streamable_http", out)
		}
		if strings.TrimSpace(fmt.Sprint(transport["url"])) != "https://example.com/mcp" {
			t.Fatalf("codex mcp list docs transport url = %q want %q\n%s", transport["url"], "https://example.com/mcp", out)
		}
		return
	}
	t.Fatalf("codex mcp list output missing docs server:\n%s", out)
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

func codexNotifyBinaryOverride(t *testing.T, markerFile, hookBin string) string {
	t.Helper()
	absHook, err := filepath.Abs(hookBin)
	if err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(t.TempDir(), "codex-notify-binary-wrapper.sh")
	script := "#!/bin/sh\n" +
		"marker_file=\"$1\"\n" +
		"hook_bin=\"$2\"\n" +
		"shift 2\n" +
		"printf 'notify\\n' > \"$marker_file\"\n" +
		"exec \"$hook_bin\" \"$@\"\n"
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
		quoteTOMLString(markerFile), ",",
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

func newCodexRenderedNotifyWorkspace(t *testing.T, pluginKitAIBin, hookBin, traceFile, model string) string {
	t.Helper()
	root := RepoRoot(t)
	dir := t.TempDir()
	mustWriteRepoFile(t, dir, "README.md", "# codex rendered notify live smoke\n")
	mustWriteRepoFile(t, dir, "plugin.yaml", `format: "plugin-kit-ai/package"
name: "codex-rendered-live"
version: "0.1.0"
description: "codex rendered live smoke"
targets:
  - "codex-runtime"
`)
	mustWriteRepoFile(t, dir, "launcher.yaml", "runtime: shell\nentrypoint: ./bin/codex-rendered-live\n")
	mustWriteRepoFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: "+model+"\n")
	mustWriteRepoExecutable(t, dir, filepath.Join("scripts", "main.sh"), "#!/bin/sh\nexit 0\n")
	wrapper := "#!/bin/sh\n" +
		"PLUGIN_KIT_AI_E2E_TRACE=" + quoteShell(traceFile) + " exec " + quoteShell(hookBin) + " \"$@\"\n"
	mustWriteRepoExecutable(t, dir, filepath.Join("bin", "codex-rendered-live"), wrapper)

	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir))
	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", dir, "--platform", "codex-runtime", "--strict"))
	assertCodexConfig(t, dir, model, "./bin/codex-rendered-live")
	return dir
}

func newRenderedCodexRuntimeExampleWorkspace(t *testing.T, pluginKitAIBin string) (string, string) {
	t.Helper()
	root := RepoRoot(t)
	src := filepath.Join(root, "examples", "plugins", "codex-basic-prod")
	dir := filepath.Join(t.TempDir(), "codex-basic-prod")
	copyTree(t, src, dir)
	bootstrapGeneratedGoPlugin(t, dir)

	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", dir, "--platform", "codex-runtime", "--strict"))
	assertCodexConfig(t, dir, "gpt-5.4-mini", "./bin/codex-basic-prod")

	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binName := "codex-basic-prod"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(binDir, binName)
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/codex-basic-prod")
	buildCmd.Dir = dir
	buildCmd.Env = newGoModuleEnv(t)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build codex-basic-prod example: %v\n%s", err, out)
	}
	return dir, binPath
}

func newCodexRenderedMCPWorkspace(t *testing.T, pluginKitAIBin string) string {
	t.Helper()
	root := RepoRoot(t)
	dir := t.TempDir()
	mustWriteRepoFile(t, dir, "README.md", "# codex rendered mcp live smoke\n")
	mustWriteRepoFile(t, dir, "plugin.yaml", `format: "plugin-kit-ai/package"
name: "codex-rendered-mcp-live"
version: "0.1.0"
description: "codex rendered mcp live smoke"
targets:
  - "codex-runtime"
`)
	mustWriteRepoFile(t, dir, "launcher.yaml", "runtime: shell\nentrypoint: ./bin/codex-rendered-mcp-live\n")
	mustWriteRepoFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWriteRepoFile(t, dir, filepath.Join("targets", "codex-runtime", "config.extra.toml"), "[mcp_servers.release-checks]\ncommand = \"/bin/echo\"\nargs = [\"hello\"]\n")
	mustWriteRepoExecutable(t, dir, filepath.Join("scripts", "main.sh"), "#!/bin/sh\nexit 0\n")
	mustWriteRepoExecutable(t, dir, filepath.Join("bin", "codex-rendered-mcp-live"), "#!/bin/sh\nexit 0\n")

	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir))
	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", dir, "--platform", "codex-runtime", "--strict"))
	return dir
}

func newCodexPackageRenderedMCPWorkspace(t *testing.T, pluginKitAIBin, mcpBin string) string {
	t.Helper()
	root := RepoRoot(t)
	dir := t.TempDir()
	mustWriteRepoFile(t, dir, "README.md", "# codex package rendered mcp live smoke\n")
	mustWriteRepoFile(t, dir, "plugin.yaml", `format: "plugin-kit-ai/package"
name: "codex-package-rendered-mcp-live"
version: "0.1.0"
description: "codex package rendered mcp live smoke"
targets:
  - "codex-package"
`)
	mustWriteRepoFile(t, dir, filepath.Join("targets", "codex-package", "package.yaml"), "homepage: https://example.com/codex-package-rendered-mcp-live\n")
	mustWriteRepoFile(t, dir, filepath.Join("mcp", "servers.yaml"), fmt.Sprintf(`format: plugin-kit-ai/mcp
version: 1

servers:
  release-checks:
    description: Codex package live smoke server
    type: stdio
    stdio:
      command: %q
      env:
        PLUGIN_KIT_AI_MCP_SMOKE_STATIC: codex-package-live
    targets:
      - codex-package
`, filepath.ToSlash(mcpBin)))

	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir))
	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", dir, "--platform", "codex-package", "--strict"))
	return dir
}

func newRenderedCodexPackageExampleWorkspace(t *testing.T, pluginKitAIBin string) string {
	t.Helper()
	root := RepoRoot(t)
	src := filepath.Join(root, "examples", "plugins", "codex-package-prod")
	dir := filepath.Join(t.TempDir(), "codex-package-prod")
	copyTree(t, src, dir)
	runCmd(t, root, exec.Command(pluginKitAIBin, "render", dir, "--check"))
	runCmd(t, root, exec.Command(pluginKitAIBin, "validate", dir, "--platform", "codex-package", "--strict"))
	return dir
}

func quoteShell(s string) string {
	return "\"" + strings.ReplaceAll(s, `"`, `\"`) + "\""
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

func runCodexExecWithProjectConfigProbe(t *testing.T, codexBin, projectDir, traceFile, prompt string) (string, string, []string) {
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
		"--color", "never",
		"--output-last-message", outputFile,
		prompt,
	}
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
		return out, readOptionalTextFile(outputFile), waitForTraceLines(t, traceFile, 3*time.Second)
	case <-time.After(75 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
		out := readLogFile(t, logFile)
		if codexRuntimeUnhealthy(out) {
			t.Skipf("codex runtime unhealthy in current environment:\n%s", truncateRunes(out, 4000))
		}
		t.Fatalf("timed out waiting for codex exec using rendered project config:\n%s", truncateRunes(out, 4000))
		return "", "", nil
	}
}

func runCodexExecWithMarkerProbe(t *testing.T, codexBin, projectDir, markerFile, prompt string, extraArgs ...string) (string, string) {
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

	if err := waitForCodexMarkerInvariants(t, markerFile, outputFile, waitCh); err != nil {
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
		return out, readOptionalTextFile(outputFile)
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
		out := readLogFile(t, logFile)
		return out, readOptionalTextFile(outputFile)
	}
}

func readOptionalTextFile(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}

func runCodexMCPGetProbe(t *testing.T, codexBin, projectDir, serverName string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexBin, "-C", projectDir, "mcp", "get", serverName, "--json")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("codex mcp get %s timed out:\n%s", serverName, out)
	}
	if err != nil {
		return string(out)
	}
	return string(out)
}

func runCodexMCPListWithProjectConfigProbe(t *testing.T, codexBin, projectDir string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, codexBin, "-C", projectDir, "mcp", "list", "--json")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("codex mcp list timed out:\n%s", out)
	}
	if err != nil {
		return string(out)
	}
	return string(out)
}

func runCodexMCPGetWithArgs(t *testing.T, codexBin, serverName string, configArgs ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"mcp", "get", serverName, "--json"}
	args = append(args, configArgs...)
	cmd := exec.CommandContext(ctx, codexBin, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("codex mcp get %s timed out:\n%s", serverName, out)
	}
	if err != nil {
		t.Fatalf("codex mcp get %s: %v\n%s", serverName, err, out)
	}
	return string(out)
}

func runCodexMCPListWithArgs(t *testing.T, codexBin string, configArgs ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	args := []string{"mcp", "list", "--json"}
	args = append(args, configArgs...)
	cmd := exec.CommandContext(ctx, codexBin, args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("codex mcp list timed out:\n%s", out)
	}
	if err != nil {
		t.Fatalf("codex mcp list: %v\n%s", err, out)
	}
	return string(out)
}

func codexMCPConfigArgsFromRenderedServer(t *testing.T, name string, server map[string]any) []string {
	t.Helper()
	switch strings.TrimSpace(fmt.Sprint(server["type"])) {
	case "http":
		url := strings.TrimSpace(fmt.Sprint(server["url"]))
		if url == "" {
			t.Fatalf("rendered MCP server %q missing url: %#v", name, server)
		}
		return []string{"-c", fmt.Sprintf(`mcp_servers.%s.url=%q`, name, url)}
	case "stdio":
		return codexMCPConfigArgs(name, server)
	default:
		t.Fatalf("unsupported rendered MCP server type %q for %s: %#v", server["type"], name, server)
		return nil
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

func waitForCodexMarkerInvariants(t *testing.T, markerFile, outputFile string, waitCh <-chan error) error {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(markerFile); err == nil {
			if b, err := os.ReadFile(outputFile); err == nil && strings.TrimSpace(string(b)) != "" {
				return nil
			}
		}
		select {
		case err := <-waitCh:
			if err != nil {
				return fmt.Errorf("codex exec exited before production example invariants: %w", err)
			}
			if _, err := os.Stat(markerFile); err != nil {
				return fmt.Errorf("codex exec exited without notify marker: %w", err)
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
			return fmt.Errorf("timed out waiting for codex production example notify invariants")
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
