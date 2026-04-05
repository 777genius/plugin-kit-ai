package runtimecheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func TestInspectPythonManagerDetection(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		manager PythonManager
		binary  string
	}{
		{
			name: "uv",
			files: map[string]string{
				"src/plugin.yaml":       minimalManifest("demo"),
				"src/launcher.yaml":     "runtime: python\nentrypoint: ./bin/demo\n",
				"uv.lock":           "version = 1\n",
				"bin/demo":          "#!/usr/bin/env bash\nexit 0\n",
				".venv/bin/python3": "ok",
			},
			manager: PythonManagerUV,
			binary:  "uv",
		},
		{
			name: "poetry",
			files: map[string]string{
				"src/plugin.yaml":       minimalManifest("demo"),
				"src/launcher.yaml":     "runtime: python\nentrypoint: ./bin/demo\n",
				"pyproject.toml":    "[tool.poetry]\nname='demo'\n",
				"bin/demo":          "#!/usr/bin/env bash\nexit 0\n",
				".venv/bin/python3": "ok",
			},
			manager: PythonManagerPoetry,
			binary:  "poetry",
		},
		{
			name: "pipenv",
			files: map[string]string{
				"src/plugin.yaml":       minimalManifest("demo"),
				"src/launcher.yaml":     "runtime: python\nentrypoint: ./bin/demo\n",
				"Pipfile.lock":      "{}\n",
				"bin/demo":          "#!/usr/bin/env bash\nexit 0\n",
				".venv/bin/python3": "ok",
			},
			manager: PythonManagerPipenv,
			binary:  "pipenv",
		},
		{
			name: "requirements",
			files: map[string]string{
				"src/plugin.yaml":      minimalManifest("demo"),
				"src/launcher.yaml":    "runtime: python\nentrypoint: ./bin/demo\n",
				"requirements.txt": "requests==2.32.0\n",
				"bin/demo":         "#!/usr/bin/env bash\nexit 0\n",
			},
			manager: PythonManagerRequirements,
			binary:  "python3",
		},
	}

	restoreLookPath := LookPath
	restoreRunCommand := RunCommand
	LookPath = func(name string) (string, error) { return name, nil }
	RunCommand = func(dir, name string, args ...string) (string, error) {
		base := filepath.Base(name)
		if len(args) == 1 && args[0] == "--version" {
			switch {
			case strings.Contains(filepath.ToSlash(name), ".venv/"), strings.Contains(filepath.ToSlash(name), ".venv\\"):
				return "Python 3.11.0", nil
			case strings.Contains(filepath.ToSlash(name), "external-env/"), strings.Contains(filepath.ToSlash(name), "external-env\\"):
				return "Python 3.11.0", nil
			case base == "python" || base == "python3" || base == "python.exe":
				return "Python 3.11.0", nil
			}
		}
		if base == "poetry" && len(args) == 3 && args[0] == "env" && args[1] == "info" && args[2] == "--path" {
			return filepath.Join(dir, "external-env"), nil
		}
		if base == "pipenv" && len(args) == 1 && args[0] == "--venv" {
			return filepath.Join(dir, "external-env"), nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() {
		LookPath = restoreLookPath
		RunCommand = restoreRunCommand
	})

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, body := range tc.files {
				writeRuntimeCheckFile(t, root, rel, body)
			}
			project, err := Inspect(Inputs{
				Root:    root,
				Targets: []string{"codex-runtime"},
				Launcher: &pluginmanifest.Launcher{
					Runtime:    "python",
					Entrypoint: "./bin/demo",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if project.Python.Manager != tc.manager {
				t.Fatalf("manager = %q want %q", project.Python.Manager, tc.manager)
			}
			if project.Python.ManagerBinary != tc.binary {
				t.Fatalf("binary = %q want %q", project.Python.ManagerBinary, tc.binary)
			}
		})
	}
}

func TestInspectPythonManagerOwnedEnvDetection(t *testing.T) {
	restoreLookPath := LookPath
	restoreRunCommand := RunCommand
	LookPath = func(name string) (string, error) { return name, nil }
	RunCommand = func(dir, name string, args ...string) (string, error) {
		base := filepath.Base(name)
		if len(args) == 1 && args[0] == "--version" && strings.Contains(filepath.ToSlash(name), "external-env") {
			return "Python 3.11.0", nil
		}
		if base == "poetry" && len(args) == 3 {
			return filepath.Join(dir, "external-env"), nil
		}
		if base == "pipenv" && len(args) == 1 {
			return filepath.Join(dir, "external-env"), nil
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() {
		LookPath = restoreLookPath
		RunCommand = restoreRunCommand
	})

	cases := []struct {
		name    string
		files   map[string]string
		manager PythonManager
	}{
		{
			name: "poetry external env",
			files: map[string]string{
				"src/plugin.yaml":              minimalManifest("demo"),
				"src/launcher.yaml":            "runtime: python\nentrypoint: ./bin/demo\n",
				"pyproject.toml":           "[tool.poetry]\nname='demo'\n",
				"bin/demo":                 "#!/usr/bin/env bash\nexit 0\n",
				"external-env/bin/python3": "ok",
			},
			manager: PythonManagerPoetry,
		},
		{
			name: "pipenv external env",
			files: map[string]string{
				"src/plugin.yaml":              minimalManifest("demo"),
				"src/launcher.yaml":            "runtime: python\nentrypoint: ./bin/demo\n",
				"Pipfile.lock":             "{}\n",
				"bin/demo":                 "#!/usr/bin/env bash\nexit 0\n",
				"external-env/bin/python3": "ok",
			},
			manager: PythonManagerPipenv,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for rel, body := range tc.files {
				writeRuntimeCheckFile(t, root, rel, body)
			}
			project, err := Inspect(Inputs{
				Root:    root,
				Targets: []string{"codex-runtime"},
				Launcher: &pluginmanifest.Launcher{
					Runtime:    "python",
					Entrypoint: "./bin/demo",
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if project.Python.Manager != tc.manager {
				t.Fatalf("manager = %q want %q", project.Python.Manager, tc.manager)
			}
			if project.Python.ReadySource != PythonEnvSourceManagerOwned {
				t.Fatalf("ready source = %q", project.Python.ReadySource)
			}
			if project.Python.ReadyInterpreter == "" {
				t.Fatal("expected manager-owned interpreter")
			}
		})
	}
}

func TestInspectPythonBrokenVenvBlocksManagerProbe(t *testing.T) {
	restoreLookPath := LookPath
	restoreRunCommand := RunCommand
	LookPath = func(name string) (string, error) { return name, nil }
	RunCommand = func(dir, name string, args ...string) (string, error) {
		base := filepath.Base(name)
		if base == "poetry" || base == "pipenv" {
			t.Fatalf("manager probe should not run when .venv is broken")
		}
		if len(args) == 1 && args[0] == "--version" {
			return "", exec.ErrNotFound
		}
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() {
		LookPath = restoreLookPath
		RunCommand = restoreRunCommand
	})

	root := t.TempDir()
	writeRuntimeCheckFile(t, root, filepath.Join("src", "plugin.yaml"), minimalManifest("demo"))
	writeRuntimeCheckFile(t, root, filepath.Join("src", "launcher.yaml"), "runtime: python\nentrypoint: ./bin/demo\n")
	writeRuntimeCheckFile(t, root, "pyproject.toml", "[tool.poetry]\nname='demo'\n")
	writeRuntimeCheckFile(t, root, filepath.Join("bin", "demo"), "#!/usr/bin/env bash\nexit 0\n")
	writeRuntimeCheckFile(t, root, filepath.Join(".venv", "bin", "python3"), "broken")

	project, err := Inspect(Inputs{
		Root:    root,
		Targets: []string{"codex-runtime"},
		Launcher: &pluginmanifest.Launcher{
			Runtime:    "python",
			Entrypoint: "./bin/demo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Python.ReadySource != PythonEnvSourceBroken {
		t.Fatalf("ready source = %q", project.Python.ReadySource)
	}
	if !strings.Contains(project.Python.BrokenReason, "found .venv") {
		t.Fatalf("broken reason = %q", project.Python.BrokenReason)
	}
}

func TestInspectNodeTypeScriptOutDirDetection(t *testing.T) {
	root := t.TempDir()
	writeRuntimeCheckFile(t, root, filepath.Join("src", "plugin.yaml"), minimalManifest("demo"))
	writeRuntimeCheckFile(t, root, filepath.Join("src", "launcher.yaml"), "runtime: node\nentrypoint: ./bin/demo\n")
	writeRuntimeCheckFile(t, root, "package.json", `{"packageManager":"yarn@4.1.0","scripts":{"build":"tsc -p tsconfig.json"}}`)
	writeRuntimeCheckFile(t, root, "yarn.lock", "# yarn lockfile\n")
	writeRuntimeCheckFile(t, root, "tsconfig.json", `{"compilerOptions":{"outDir":"build"}}`)
	writeRuntimeCheckFile(t, root, filepath.Join("bin", "demo"), "#!/usr/bin/env bash\nexec node \"$ROOT/build/main.js\" \"$@\"\n")
	writeRuntimeCheckFile(t, root, filepath.Join("build", "main.js"), "console.log('ok')\n")
	writeRuntimeCheckFile(t, root, ".yarnrc.yml", "nodeLinker: node-modules\n")
	writeRuntimeCheckFile(t, root, filepath.Join("node_modules", ".installed"), "ok")

	restoreLookPath := LookPath
	LookPath = func(name string) (string, error) { return name, nil }
	t.Cleanup(func() { LookPath = restoreLookPath })

	project, err := Inspect(Inputs{
		Root:    root,
		Targets: []string{"codex-runtime"},
		Launcher: &pluginmanifest.Launcher{
			Runtime:    "node",
			Entrypoint: "./bin/demo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if project.Node.Manager != NodeManagerYarn {
		t.Fatalf("manager = %q", project.Node.Manager)
	}
	if project.Node.OutputDir != "build" {
		t.Fatalf("outDir = %q", project.Node.OutputDir)
	}
	if !project.Node.IsTypeScript {
		t.Fatal("expected TypeScript lane")
	}
	if project.Node.StructuralIssue != "" {
		t.Fatalf("unexpected structural issue: %s", project.Node.StructuralIssue)
	}
}

func TestInspectNodeOutDirMismatch(t *testing.T) {
	root := t.TempDir()
	writeRuntimeCheckFile(t, root, filepath.Join("src", "plugin.yaml"), minimalManifest("demo"))
	writeRuntimeCheckFile(t, root, filepath.Join("src", "launcher.yaml"), "runtime: node\nentrypoint: ./bin/demo\n")
	writeRuntimeCheckFile(t, root, "package.json", `{"scripts":{"build":"tsc -p tsconfig.json"}}`)
	writeRuntimeCheckFile(t, root, "tsconfig.json", `{"compilerOptions":{"outDir":"dist"}}`)
	writeRuntimeCheckFile(t, root, filepath.Join("bin", "demo"), "#!/usr/bin/env bash\nexec node \"$ROOT/build/main.js\" \"$@\"\n")

	restoreLookPath := LookPath
	LookPath = func(name string) (string, error) { return name, nil }
	t.Cleanup(func() { LookPath = restoreLookPath })

	project, err := Inspect(Inputs{
		Root:    root,
		Targets: []string{"codex-runtime"},
		Launcher: &pluginmanifest.Launcher{
			Runtime:    "node",
			Entrypoint: "./bin/demo",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(project.Node.StructuralIssue, "outside tsconfig outDir dist") {
		t.Fatalf("structural issue = %q", project.Node.StructuralIssue)
	}
}

func minimalManifest(name string) string {
	return "api_version: v1\nname: \"" + name + "\"\nversion: \"0.1.0\"\ndescription: \"demo\"\ntargets: [\"codex-runtime\"]\n"
}

func writeRuntimeCheckFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o644)
	if strings.HasPrefix(rel, "bin/") || strings.Contains(rel, "/bin/") {
		mode = 0o755
	}
	if err := os.WriteFile(full, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

var _ = exec.ErrNotFound
