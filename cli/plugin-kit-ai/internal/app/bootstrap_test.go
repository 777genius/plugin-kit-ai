package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPluginServiceBootstrapPythonCreatesVenvAndInstallsRequirements(t *testing.T) {
	restoreBootstrapHelpers(t)
	dir := t.TempDir()
	writeBootstrapProjectFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["codex-runtime"]
`)
	writeBootstrapProjectFile(t, dir, "launcher.yaml", "runtime: python\nentrypoint: ./bin/demo\n")
	writeBootstrapProjectFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	writeBootstrapProjectFile(t, dir, "requirements.txt", "requests==2.32.0\n")

	var svc PluginService
	result, err := svc.Bootstrap(context.Background(), PluginBootstrapOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Lines) != 2 {
		t.Fatalf("lines = %v", result.Lines)
	}
	venvPython := filepath.Join(dir, ".venv", "bin", "python3")
	if runtime.GOOS == "windows" {
		venvPython = filepath.Join(dir, ".venv", "Scripts", "python.exe")
	}
	if _, err := os.Stat(venvPython); err != nil {
		t.Fatalf("expected virtualenv interpreter: %v", err)
	}
	marker := filepath.Join(dir, ".venv", "pip-installed.txt")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected pip install marker: %v", err)
	}
}

func TestPluginServiceBootstrapNodeTypeScriptRunsInstallAndBuild(t *testing.T) {
	restoreBootstrapHelpers(t)
	dir := t.TempDir()
	writeBootstrapProjectFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["codex-runtime"]
`)
	writeBootstrapProjectFile(t, dir, "launcher.yaml", "runtime: node\nentrypoint: ./bin/demo\n")
	writeBootstrapProjectFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	writeBootstrapProjectFile(t, dir, "tsconfig.json", "{}\n")
	writeBootstrapProjectFile(t, dir, "package.json", `{"scripts":{"build":"tsc -p tsconfig.json"}}`)
	writeBootstrapProjectFile(t, dir, filepath.Join("bin", "demo"), "#!/usr/bin/env bash\nexec node \"$ROOT/dist/main.js\" \"$@\"\n")
	mustChmodBootstrapExecutable(t, filepath.Join(dir, "bin", "demo"))

	var svc PluginService
	result, err := svc.Bootstrap(context.Background(), PluginBootstrapOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(result.Lines, "\n"); !strings.Contains(got, "Installed Node dependencies") || !strings.Contains(got, "Built TypeScript output") {
		t.Fatalf("lines = %v", result.Lines)
	}
	if _, err := os.Stat(filepath.Join(dir, "node_modules", ".installed")); err != nil {
		t.Fatalf("expected npm install marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "main.js")); err != nil {
		t.Fatalf("expected built output marker: %v", err)
	}
}

func TestPluginServiceBootstrapGoIsNoOp(t *testing.T) {
	restoreBootstrapHelpers(t)
	dir := t.TempDir()
	writeBootstrapProjectFile(t, dir, "plugin.yaml", `format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["codex-runtime"]
`)
	writeBootstrapProjectFile(t, dir, "launcher.yaml", "runtime: go\nentrypoint: ./bin/demo\n")
	writeBootstrapProjectFile(t, dir, filepath.Join("targets", "codex-runtime", "package.yaml"), "model_hint: gpt-5.4-mini\n")

	var svc PluginService
	result, err := svc.Bootstrap(context.Background(), PluginBootstrapOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(result.Lines, "\n"); !strings.Contains(got, "Bootstrap not required for Go projects") {
		t.Fatalf("lines = %v", result.Lines)
	}
}

func TestBootstrapHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_BOOTSTRAP_HELPER") != "1" {
		return
	}
	args := os.Args
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == -1 || sep+1 >= len(args) {
		os.Exit(2)
	}
	name := filepath.Base(args[sep+1])
	cmdArgs := args[sep+2:]
	switch name {
	case "python", "python3", "python.exe":
		runBootstrapPythonHelper(cmdArgs)
	case "npm":
		runBootstrapNPMHelper(cmdArgs)
	default:
		fmt.Fprintf(os.Stderr, "unexpected helper %s", name)
		os.Exit(2)
	}
}

func runBootstrapPythonHelper(args []string) {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(os.Stdout, "Python 3.11.0")
		return
	}
	if len(args) >= 3 && args[0] == "-m" && args[1] == "venv" {
		venvRoot := args[2]
		var interpreter string
		if runtime.GOOS == "windows" {
			interpreter = filepath.Join(venvRoot, "Scripts", "python.exe")
		} else {
			interpreter = filepath.Join(venvRoot, "bin", "python3")
		}
		if err := os.MkdirAll(filepath.Dir(interpreter), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(interpreter, []byte("bootstrap-helper"), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(args) >= 5 && args[0] == "-m" && args[1] == "pip" && args[2] == "install" && args[3] == "-r" {
		marker := filepath.Join(".venv", "pip-installed.txt")
		if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(marker, []byte(args[4]), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "unexpected python helper args: %v", args)
	os.Exit(2)
}

func runBootstrapNPMHelper(args []string) {
	if len(args) == 1 && args[0] == "install" {
		if err := os.MkdirAll(filepath.Join("node_modules"), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join("node_modules", ".installed"), []byte("ok"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(args) == 2 && args[0] == "run" && args[1] == "build" {
		if err := os.MkdirAll(filepath.Join("dist"), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(filepath.Join("dist", "main.js"), []byte("console.log('ok')\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "unexpected npm helper args: %v", args)
	os.Exit(2)
}

func restoreBootstrapHelpers(t *testing.T) {
	t.Helper()
	prevLookPath := bootstrapLookPath
	prevCommand := bootstrapCommandContext
	bootstrapLookPath = func(name string) (string, error) {
		switch name {
		case "python", "python3", "npm":
			return name, nil
		default:
			return "", exec.ErrNotFound
		}
	}
	bootstrapCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmdArgs := []string{"-test.run=TestBootstrapHelperProcess", "--", name}
		cmdArgs = append(cmdArgs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cmdArgs...)
		cmd.Env = append(os.Environ(), "GO_WANT_BOOTSTRAP_HELPER=1")
		return cmd
	}
	t.Cleanup(func() {
		bootstrapLookPath = prevLookPath
		bootstrapCommandContext = prevCommand
	})
}

func writeBootstrapProjectFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustChmodBootstrapExecutable(t *testing.T, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
