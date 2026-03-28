package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/runtimecheck"
)

var bootstrapCommandContext = exec.CommandContext

type PluginBootstrapOptions struct {
	Root string
}

type PluginBootstrapResult struct {
	Lines []string
}

func (PluginService) Bootstrap(ctx context.Context, opts PluginBootstrapOptions) (PluginBootstrapResult, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		return PluginBootstrapResult{}, err
	}
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  graph.Manifest.EnabledTargets(),
		Launcher: graph.Launcher,
	})
	if err != nil {
		return PluginBootstrapResult{}, err
	}

	lines := []string{project.ProjectLine()}
	nextValidate := "Next: " + runtimecheck.ValidateCommand(project.Targets)
	if project.Runtime == "" {
		lines = append(lines,
			fmt.Sprintf("Bootstrap not required for %s: no launcher-based runtime is configured.", project.Lane),
			nextValidate,
		)
		return PluginBootstrapResult{Lines: lines}, nil
	}

	switch project.Runtime {
	case "go":
		lines = append(lines,
			"Bootstrap not required for Go projects: run your normal Go build/test workflow.",
			nextValidate,
		)
		return PluginBootstrapResult{Lines: lines}, nil
	case "shell":
		lines = append(lines,
			"Bootstrap not required for shell runtime projects: ensure the shell target is executable on Unix.",
			nextValidate,
		)
		return PluginBootstrapResult{Lines: lines}, nil
	case "python":
		bootstrapLines, err := bootstrapPython(ctx, project)
		if err != nil {
			return PluginBootstrapResult{}, err
		}
		lines = append(lines, bootstrapLines...)
	case "node":
		bootstrapLines, err := bootstrapNode(ctx, project)
		if err != nil {
			return PluginBootstrapResult{}, err
		}
		lines = append(lines, bootstrapLines...)
	default:
		return PluginBootstrapResult{}, fmt.Errorf("unsupported bootstrap runtime %q", project.Runtime)
	}
	lines = append(lines, nextValidate)
	return PluginBootstrapResult{Lines: lines}, nil
}

func bootstrapPython(ctx context.Context, project runtimecheck.Project) ([]string, error) {
	root := project.Root
	shape := project.Python
	lines := []string{fmt.Sprintf("Detected Python manager: %s", shape.ManagerDisplay())}
	switch shape.Manager {
	case runtimecheck.PythonManagerUV:
		if !shape.ManagerAvailable {
			return nil, fmt.Errorf("bootstrap failed: uv not found in PATH")
		}
		if err := runBootstrapCommand(ctx, root, "uv", "sync"); err != nil {
			return nil, err
		}
		lines = append(lines, "Ran: uv sync")
	case runtimecheck.PythonManagerPoetry:
		if !shape.ManagerAvailable {
			return nil, fmt.Errorf("bootstrap failed: poetry not found in PATH")
		}
		if err := runBootstrapCommand(ctx, root, "poetry", "install", "--no-root"); err != nil {
			return nil, err
		}
		lines = append(lines, "Ran: poetry install --no-root")
	case runtimecheck.PythonManagerPipenv:
		if !shape.ManagerAvailable {
			return nil, fmt.Errorf("bootstrap failed: pipenv not found in PATH")
		}
		if fileExists(filepath.Join(root, "Pipfile.lock")) {
			if err := runBootstrapCommand(ctx, root, "pipenv", "sync"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: pipenv sync")
		} else {
			if err := runBootstrapCommand(ctx, root, "pipenv", "install"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: pipenv install")
		}
	default:
		venvPython, created, err := ensureProjectVenv(ctx, root)
		if err != nil {
			return nil, err
		}
		if created {
			lines = append(lines, "Ran: python -m venv .venv")
		}
		switch shape.Manager {
		case runtimecheck.PythonManagerRequirements:
			if err := runBootstrapCommand(ctx, root, venvPython, "-m", "pip", "install", "-r", "requirements.txt"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: python -m pip install -r requirements.txt")
		default:
			if !created {
				lines = append(lines, "Project virtualenv is already available; no dependency bootstrap was required")
			}
		}
	}
	lines = append(lines, "Canonical Python environment source: "+shape.CanonicalEnvSourceDisplay())
	return lines, nil
}

func ensureProjectVenv(ctx context.Context, root string) (string, bool, error) {
	if venvPython := runnableVenvPython(root); venvPython != "" {
		return venvPython, false, nil
	}
	if hasVenv(root) {
		return "", false, fmt.Errorf("bootstrap failed: found .venv but no runnable interpreter; recreate .venv or repair the virtualenv")
	}
	systemPython, err := findSystemPython()
	if err != nil {
		return "", false, err
	}
	if err := runBootstrapCommand(ctx, root, systemPython, "-m", "venv", ".venv"); err != nil {
		return "", false, err
	}
	venvPython := runnableVenvPython(root)
	if venvPython == "" {
		return "", false, fmt.Errorf("bootstrap failed: created .venv but no runnable interpreter was found")
	}
	return venvPython, true, nil
}

func bootstrapNode(ctx context.Context, project runtimecheck.Project) ([]string, error) {
	root := project.Root
	shape := project.Node
	lines := []string{fmt.Sprintf("Detected Node manager: %s", shape.ManagerDisplay())}
	if !shape.ManagerAvailable {
		return nil, fmt.Errorf("bootstrap failed: %s not found in PATH", shape.ManagerBinary)
	}
	switch shape.Manager {
	case runtimecheck.NodeManagerPNPM:
		if err := runBootstrapCommand(ctx, root, "pnpm", "install", "--frozen-lockfile"); err != nil {
			return nil, err
		}
		lines = append(lines, "Ran: pnpm install --frozen-lockfile")
	case runtimecheck.NodeManagerYarn:
		if runtimecheck.YarnBerry(root, shape.PackageManager) {
			if err := runBootstrapCommand(ctx, root, "yarn", "install", "--immutable"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: yarn install --immutable")
		} else {
			if err := runBootstrapCommand(ctx, root, "yarn", "install", "--frozen-lockfile"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: yarn install --frozen-lockfile")
		}
	case runtimecheck.NodeManagerBun:
		if err := runBootstrapCommand(ctx, root, "bun", "install"); err != nil {
			return nil, err
		}
		lines = append(lines, "Ran: bun install")
	default:
		if fileExists(filepath.Join(root, "package-lock.json")) || fileExists(filepath.Join(root, "npm-shrinkwrap.json")) {
			if err := runBootstrapCommand(ctx, root, "npm", "ci"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: npm ci")
		} else {
			if err := runBootstrapCommand(ctx, root, "npm", "install"); err != nil {
				return nil, err
			}
			lines = append(lines, "Ran: npm install")
		}
	}
	if shape.IsTypeScript {
		if strings.TrimSpace(shape.BuildScript) == "" {
			return nil, fmt.Errorf("bootstrap failed: TypeScript lane detected but package.json is missing a build script")
		}
		if err := runBootstrapCommand(ctx, root, shape.ManagerBinary, buildCommandArgs(shape.Manager)...); err != nil {
			return nil, err
		}
		lines = append(lines, "Ran: "+shape.BuildCommandString())
	}
	return lines, nil
}

func buildCommandArgs(manager runtimecheck.NodeManager) []string {
	switch manager {
	case runtimecheck.NodeManagerYarn:
		return []string{"build"}
	case runtimecheck.NodeManagerBun:
		return []string{"run", "build"}
	case runtimecheck.NodeManagerPNPM:
		return []string{"run", "build"}
	default:
		return []string{"run", "build"}
	}
}

func runBootstrapCommand(ctx context.Context, root, bin string, args ...string) error {
	cmd := bootstrapCommandContext(ctx, bin, args...)
	cmd.Dir = root
	if len(cmd.Env) == 0 {
		cmd.Env = os.Environ()
	} else {
		cmd.Env = append(os.Environ(), cmd.Env...)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bootstrap failed: %s %s: %v\n%s", filepath.Base(bin), strings.Join(args, " "), err, out)
	}
	return nil
}

func findSystemPython() (string, error) {
	for _, name := range pythonPathNames() {
		if _, err := runtimecheck.LookPath(name); err == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("bootstrap failed: python runtime required; install Python 3.10+ or provide python3/python in PATH")
}

func runnableVenvPython(root string) string {
	for _, candidate := range pythonCandidates(root) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			cmd := bootstrapCommandContext(context.Background(), candidate, "--version")
			if _, err := cmd.CombinedOutput(); err == nil {
				return candidate
			}
		}
	}
	return ""
}

func hasVenv(root string) bool {
	return fileExists(filepath.Join(root, ".venv")) || dirExists(filepath.Join(root, ".venv"))
}

func pythonCandidates(root string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(root, ".venv", "Scripts", "python.exe"),
			filepath.Join(root, ".venv", "bin", "python3"),
		}
	}
	return []string{
		filepath.Join(root, ".venv", "bin", "python3"),
		filepath.Join(root, ".venv", "Scripts", "python.exe"),
	}
}

func pythonPathNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
