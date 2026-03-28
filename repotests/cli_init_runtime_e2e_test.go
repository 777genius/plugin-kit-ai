package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestPluginKitAIInitGoRuntimeLauncherFlow(t *testing.T) {
	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			root := RepoRoot(t)
			sdkDir := filepath.Join(root, "sdk", "plugin-kit-ai")
			pluginKitAIBin := buildPluginKitAI(t)

			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "go", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}
			if platform == "codex" {
				assertCodexConfig(t, plugRoot, "gpt-5.4-mini", "./bin/genplug")
			}

			replaceArg := "github.com/plugin-kit-ai/plugin-kit-ai/sdk=" + sdkDir
			modEdit := exec.Command("go", "mod", "edit", "-replace", replaceArg)
			modEdit.Dir = plugRoot
			modEdit.Env = append(os.Environ(), "GOWORK=off")
			if out, err := modEdit.CombinedOutput(); err != nil {
				t.Fatalf("go mod edit: %v\n%s", err, out)
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform)
			validate.Env = append(os.Environ(), "GOWORK=off")
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate: %v\n%s", err, out)
			}

			tidy := exec.Command("go", "mod", "tidy")
			tidy.Dir = plugRoot
			tidy.Env = append(os.Environ(), "GOWORK=off")
			if out, err := tidy.CombinedOutput(); err != nil {
				t.Fatalf("go mod tidy: %v\n%s", err, out)
			}

			binName := "genplug"
			if runtime.GOOS == "windows" {
				binName += ".exe"
			}
			build := exec.Command("go", "build", "-o", filepath.Join("bin", binName), "./cmd/genplug")
			build.Dir = plugRoot
			build.Env = append(os.Environ(), "GOWORK=off")
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("go build generated entrypoint: %v\n%s", err, out)
			}

			entry := filepath.Join(plugRoot, "bin", binName)
			switch platform {
			case "codex":
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("run go codex launcher: %v\n%s", err, out)
				}
				if strings.TrimSpace(string(out)) != "" {
					t.Fatalf("stdout = %q, want empty", string(out))
				}
			case "claude":
				assertClaudeStableSubsetEntry(t, entry)
			}
		})
	}
}

func TestPluginKitAIInitNodeRuntimeSupportsTypeScriptBuildThroughLauncher(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not in PATH")
	}
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not in PATH")
	}

	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "node", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}
			if platform == "codex" {
				assertCodexConfig(t, plugRoot, "gpt-5.4-mini", "./bin/genplug")
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform)
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate before TS conversion: %v\n%s", err, out)
			}

			writeRuntimeFile(t, plugRoot, filepath.Join("src", "main.ts"), tsPluginSource())
			writeRuntimeFile(t, plugRoot, "tsconfig.json", tsConfig())
			writeRuntimeFile(t, plugRoot, "package.json", tsPackageJSON())
			patchNodeLauncherForDist(t, plugRoot)

			npmInstall := exec.Command("npm", "install")
			npmInstall.Dir = plugRoot
			if out, err := npmInstall.CombinedOutput(); err != nil {
				t.Fatalf("npm install: %v\n%s", err, out)
			}

			npmBuild := exec.Command("npm", "run", "build")
			npmBuild.Dir = plugRoot
			if out, err := npmBuild.CombinedOutput(); err != nil {
				t.Fatalf("npm run build: %v\n%s", err, out)
			}

			entry := filepath.Join(plugRoot, "bin", "genplug")
			if runtime.GOOS == "windows" {
				entry += ".cmd"
			}
			switch platform {
			case "codex":
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("run TS-over-node codex launcher: %v\n%s", err, out)
				}
				if strings.TrimSpace(string(out)) != "" {
					t.Fatalf("stdout = %q, want empty", string(out))
				}
			case "claude":
				assertClaudeStableSubsetEntry(t, entry)
			}
		})
	}
}

func TestPluginKitAIInitPythonRuntimeLauncherFlow(t *testing.T) {
	if !pythonRuntimeAvailable() {
		t.Skip("python runtime not available for launcher flow")
	}

	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "python", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}
			if platform == "codex" {
				assertCodexConfig(t, plugRoot, "gpt-5.4-mini", "./bin/genplug")
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform)
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate python runtime: %v\n%s", err, out)
			}

			entry := filepath.Join(plugRoot, "bin", "genplug")
			if runtime.GOOS == "windows" {
				entry += ".cmd"
			}
			switch platform {
			case "codex":
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("run python codex launcher: %v\n%s", err, out)
				}
				if strings.TrimSpace(string(out)) != "" {
					t.Fatalf("stdout = %q, want empty", string(out))
				}
			case "claude":
				assertClaudeStableSubsetEntry(t, entry)
			}
		})
	}
}

func TestPluginKitAIInitShellRuntimeLauncherFlow(t *testing.T) {
	if !shellRuntimeAvailable() {
		t.Skip("bash runtime not available for shell launcher flow")
	}

	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "shell", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}
			if platform == "codex" {
				assertCodexConfig(t, plugRoot, "gpt-5.4-mini", "./bin/genplug")
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform)
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate shell runtime: %v\n%s", err, out)
			}

			entry := filepath.Join(plugRoot, "bin", "genplug")
			if runtime.GOOS == "windows" {
				entry += ".cmd"
			}
			switch platform {
			case "codex":
				cmd := exec.Command(entry, "notify", `{"client":"codex-tui"}`)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("run shell codex launcher: %v\n%s", err, out)
				}
				if strings.TrimSpace(string(out)) != "" {
					t.Fatalf("stdout = %q, want empty", string(out))
				}
			case "claude":
				assertClaudeStableSubsetEntry(t, entry)
			}
		})
	}
}

func TestPluginKitAIInitPythonRuntimeBrokenVenvFailsValidate(t *testing.T) {
	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "python", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}

			if runtime.GOOS == "windows" {
				writeRuntimeFile(t, plugRoot, filepath.Join(".venv", "Scripts", "python.exe"), "not-a-real-exe")
			} else {
				writeRuntimeFile(t, plugRoot, filepath.Join(".venv", "bin", "python3"), "not-a-real-exe")
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform, "--strict")
			out, err := validate.CombinedOutput()
			if err == nil {
				t.Fatalf("expected validate failure for broken .venv:\n%s", out)
			}
			text := string(out)
			if !strings.Contains(text, "recreate .venv") {
				t.Fatalf("validate output missing broken .venv recovery guidance:\n%s", text)
			}
			if !strings.Contains(text, "Failure: runtime not found:") {
				t.Fatalf("validate output missing runtime failure line:\n%s", text)
			}
		})
	}
}

func TestPluginKitAIInitNodeRuntimeMissingBuiltOutputFailsValidate(t *testing.T) {
	if !nodeRuntimeAvailable() {
		t.Skip("node not in PATH")
	}

	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "node", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}

			patchNodeLauncherForDist(t, plugRoot)

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform, "--strict")
			out, err := validate.CombinedOutput()
			if err == nil {
				t.Fatalf("expected validate failure for missing built output:\n%s", out)
			}
			text := string(out)
			if !strings.Contains(text, "dist/main.js") {
				t.Fatalf("validate output missing built-output target path:\n%s", text)
			}
			if !strings.Contains(text, "npm install && npm run build") {
				t.Fatalf("validate output missing build recovery guidance:\n%s", text)
			}
		})
	}
}

func TestPluginKitAIInitShellRuntimeNonExecutableTargetFailsValidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only executable-bit check")
	}

	for _, platform := range []string{"claude", "codex"} {
		t.Run(platform, func(t *testing.T) {
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "shell", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
			}

			target := filepath.Join(plugRoot, "scripts", "main.sh")
			if err := os.Chmod(target, 0o644); err != nil {
				t.Fatalf("chmod shell target non-executable: %v", err)
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", platform, "--strict")
			out, err := validate.CombinedOutput()
			if err == nil {
				t.Fatalf("expected validate failure for non-executable shell target:\n%s", out)
			}
			text := string(out)
			if !strings.Contains(text, "scripts/main.sh is not executable") {
				t.Fatalf("validate output missing executable-bit guidance:\n%s", text)
			}
		})
	}
}

func assertClaudeStableSubsetEntry(t *testing.T, entry string) {
	t.Helper()
	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "Stop",
			payload: `{"session_id":"s","transcript_path":"t","cwd":".","permission_mode":"default","hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"ok"}`,
		},
		{
			name:    "PreToolUse",
			payload: `{"session_id":"e2e-session","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"toolu_e2e","tool_input":{"command":"echo ok"}}`,
		},
		{
			name:    "UserPromptSubmit",
			payload: `{"session_id":"e2e-session","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"hello e2e"}`,
		},
	}
	for _, tc := range cases {
		cmd := exec.Command(entry, tc.name)
		cmd.Stdin = strings.NewReader(tc.payload)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run claude launcher %s: %v\n%s", tc.name, err, out)
		}
		if strings.TrimSpace(string(out)) != "{}" {
			t.Fatalf("claude %s stdout = %q, want {}", tc.name, string(out))
		}
	}
}

func TestPluginKitAIInitClaudeExtendedHooksRuntimeFlow(t *testing.T) {
	cases := []struct {
		runtime string
		skip    func() bool
		prepare func(t *testing.T, root string)
		entry   func(root string) string
	}{
		{
			runtime: "go",
			prepare: func(t *testing.T, root string) {
				bootstrapGeneratedGoPlugin(t, root)
				binName := "genplug"
				if runtime.GOOS == "windows" {
					binName += ".exe"
				}
				build := exec.Command("go", "build", "-o", filepath.Join("bin", binName), "./cmd/genplug")
				build.Dir = root
				build.Env = append(os.Environ(), "GOWORK=off")
				if out, err := build.CombinedOutput(); err != nil {
					t.Fatalf("go build generated extended Claude entrypoint: %v\n%s", err, out)
				}
			},
			entry: func(root string) string {
				binName := "genplug"
				if runtime.GOOS == "windows" {
					binName += ".exe"
				}
				return filepath.Join(root, "bin", binName)
			},
		},
		{
			runtime: "node",
			skip: func() bool {
				_, err := exec.LookPath("node")
				return err != nil
			},
			entry: func(root string) string {
				path := filepath.Join(root, "bin", "genplug")
				if runtime.GOOS == "windows" {
					path += ".cmd"
				}
				return path
			},
		},
		{
			runtime: "python",
			skip: func() bool {
				return !pythonRuntimeAvailable()
			},
			entry: func(root string) string {
				path := filepath.Join(root, "bin", "genplug")
				if runtime.GOOS == "windows" {
					path += ".cmd"
				}
				return path
			},
		},
		{
			runtime: "shell",
			skip: func() bool {
				return !shellRuntimeAvailable()
			},
			entry: func(root string) string {
				path := filepath.Join(root, "bin", "genplug")
				if runtime.GOOS == "windows" {
					path += ".cmd"
				}
				return path
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.runtime, func(t *testing.T) {
			if tc.skip != nil && tc.skip() {
				t.Skipf("%s runtime not available", tc.runtime)
			}
			pluginKitAIBin := buildPluginKitAI(t)
			plugRoot := runtimeProjectRoot(t)
			run := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "claude", "--runtime", tc.runtime, "--claude-extended-hooks", "-o", plugRoot, "--extras")
			if out, err := run.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai init extended Claude %s: %v\n%s", tc.runtime, err, out)
			}

			hooksBody, err := os.ReadFile(filepath.Join(plugRoot, "targets", "claude", "hooks", "hooks.json"))
			if err != nil {
				t.Fatal(err)
			}
			hooks := string(hooksBody)
			for _, want := range []string{`"SessionStart"`, `"WorktreeRemove"`, `"Stop"`} {
				if !strings.Contains(hooks, want) {
					t.Fatalf("extended Claude hooks missing %s:\n%s", want, hooks)
				}
			}

			if tc.prepare != nil {
				tc.prepare(t, plugRoot)
			}

			validate := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "claude")
			validate.Env = append(os.Environ(), "GOWORK=off")
			if out, err := validate.CombinedOutput(); err != nil {
				t.Fatalf("plugin-kit-ai validate extended Claude %s: %v\n%s", tc.runtime, err, out)
			}

			entry := tc.entry(plugRoot)
			assertClaudeExtendedHookEntry(t, entry, "SessionStart", `{"session_id":"s","cwd":"/tmp","hook_event_name":"SessionStart"}`)
			assertClaudeExtendedHookEntry(t, entry, "WorktreeRemove", `{"session_id":"s","cwd":"/tmp","hook_event_name":"WorktreeRemove","worktree_path":"/tmp/demo"}`)
		})
	}
}

func writeRuntimeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if strings.HasSuffix(rel, ".sh") {
		mode = 0o755
	}
	if err := os.WriteFile(full, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func assertClaudeExtendedHookEntry(t *testing.T, entry, hookName, payload string) {
	t.Helper()
	cmd := exec.Command(entry, hookName)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run extended claude launcher %s: %v\n%s", hookName, err, out)
	}
	if strings.TrimSpace(string(out)) != "{}" {
		t.Fatalf("extended claude %s stdout = %q, want {}", hookName, string(out))
	}
}

func pythonRuntimeAvailable() bool {
	bin := ""
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("python"); err == nil {
			bin = "python"
		} else if _, err := exec.LookPath("python3"); err == nil {
			bin = "python3"
		}
	} else if _, err := exec.LookPath("python3"); err == nil {
		bin = "python3"
	}
	if bin == "" {
		return false
	}
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return false
	}
	major, minor, ok := parsePythonVersion(string(out))
	return ok && (major > 3 || (major == 3 && minor >= 10))
}

func parsePythonVersion(version string) (major, minor int, ok bool) {
	fields := strings.Fields(strings.TrimSpace(version))
	if len(fields) < 2 {
		return 0, 0, false
	}
	parts := strings.Split(fields[1], ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func patchNodeLauncherForDist(t *testing.T, root string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		writeRuntimeFile(t, root, filepath.Join("bin", "genplug.cmd"), "@echo off\r\nsetlocal\r\nset \"ROOT=%~dp0..\"\r\nnode \"%ROOT%\\dist\\main.js\" %*\r\n")
		return
	}
	writeRuntimeFile(t, root, filepath.Join("bin", "genplug"), "#!/usr/bin/env bash\nset -euo pipefail\nROOT=\"$(CDPATH= cd -- \"$(dirname -- \"$0\")/..\" && pwd)\"\nif ! command -v node >/dev/null 2>&1; then\n  echo \"plugin-kit-ai launcher: node not found\" >&2\n  exit 1\nfi\nNODE=\"$(command -v node)\"\nexec \"$NODE\" \"$ROOT/dist/main.js\" \"$@\"\n")
}

func tsPluginSource() string {
	return `import fs from "node:fs";

function readStdin(): string {
  return fs.readFileSync(0, "utf8");
}

function handleClaude(): number {
  const event = JSON.parse(readStdin());
  void event;
  process.stdout.write("{}");
  return 0;
}

function handleCodex(): number {
  const payload = process.argv[3];
  if (!payload) {
    process.stderr.write("missing notify payload\n");
    return 1;
  }
  const event = JSON.parse(payload);
  void event;
  return 0;
}

function main(): number {
  const hookName = process.argv[2];
  if (!hookName) {
    process.stderr.write("usage: main.ts <hook-name>\n");
    return 1;
  }
  if (hookName === "notify") {
    return handleCodex();
  }
  return handleClaude();
}

process.exit(main());
`
}

func tsConfig() string {
	return `{
  "compilerOptions": {
    "target": "ES2022",
    "module": "NodeNext",
    "moduleResolution": "NodeNext",
    "types": ["node"],
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "skipLibCheck": true
  },
  "include": ["src/**/*.ts"]
}
`
}

func tsPackageJSON() string {
	return `{
  "name": "genplug",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "build": "tsc -p tsconfig.json"
  },
  "devDependencies": {
    "@types/node": "^24.0.0",
    "typescript": "^5.9.0"
  }
}
`
}
