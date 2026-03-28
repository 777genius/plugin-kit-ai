package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
)

var bootstrapLookPath = exec.LookPath
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
	if graph.Launcher == nil || strings.TrimSpace(graph.Launcher.Runtime) == "" {
		return PluginBootstrapResult{
			Lines: []string{
				fmt.Sprintf("Bootstrap not required for %s: no launcher-based runtime is configured.", laneSummary(graph.Manifest.EnabledTargets())),
			},
		}, nil
	}

	switch strings.TrimSpace(graph.Launcher.Runtime) {
	case "go":
		return PluginBootstrapResult{
			Lines: []string{
				"Bootstrap not required for Go projects: run `plugin-kit-ai validate --strict` and your normal Go build/test workflow.",
			},
		}, nil
	case "python":
		return bootstrapPython(ctx, root)
	case "node":
		return bootstrapNode(ctx, root, graph.Launcher.Entrypoint)
	case "shell":
		return PluginBootstrapResult{
			Lines: []string{
				"Bootstrap not required for shell runtime projects: ensure the shell target is executable on Unix, then run `plugin-kit-ai validate --strict`.",
			},
		}, nil
	default:
		return PluginBootstrapResult{}, fmt.Errorf("unsupported bootstrap runtime %q", graph.Launcher.Runtime)
	}
}

func laneSummary(targets []string) string {
	if len(targets) == 0 {
		return "this project"
	}
	return strings.Join(targets, ", ")
}

func bootstrapPython(ctx context.Context, root string) (PluginBootstrapResult, error) {
	lines := []string{}
	venvPython, err := bootstrapPythonInterpreter(root)
	if err != nil {
		if hasVenv(root) {
			return PluginBootstrapResult{}, err
		}
		systemPython, err := findSystemPython()
		if err != nil {
			return PluginBootstrapResult{}, err
		}
		if err := runBootstrapCommand(ctx, root, systemPython, "-m", "venv", ".venv"); err != nil {
			return PluginBootstrapResult{}, err
		}
		lines = append(lines, "Created project virtualenv in .venv")
		venvPython, err = bootstrapPythonInterpreter(root)
		if err != nil {
			return PluginBootstrapResult{}, err
		}
	}
	if fileExists(filepath.Join(root, "requirements.txt")) {
		if err := runBootstrapCommand(ctx, root, venvPython, "-m", "pip", "install", "-r", "requirements.txt"); err != nil {
			return PluginBootstrapResult{}, err
		}
		lines = append(lines, "Installed Python dependencies from requirements.txt")
	}
	if len(lines) == 0 {
		lines = append(lines, "Project virtualenv is already available; no dependency bootstrap was required")
	}
	return PluginBootstrapResult{Lines: lines}, nil
}

func bootstrapPythonInterpreter(root string) (string, error) {
	for _, candidate := range pythonCandidates(root) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			cmd := bootstrapCommandContext(context.Background(), candidate, "--version")
			if out, err := cmd.CombinedOutput(); err == nil {
				_ = out
				return candidate, nil
			}
		}
	}
	if hasVenv(root) {
		return "", fmt.Errorf("bootstrap failed: found .venv but no runnable interpreter; recreate .venv or repair the virtualenv")
	}
	return "", fmt.Errorf("bootstrap failed: project virtualenv not found")
}

func findSystemPython() (string, error) {
	for _, name := range pythonPathNames() {
		path, err := bootstrapLookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("bootstrap failed: python runtime required; install Python 3.10+ or provide python3/python in PATH")
}

func hasVenv(root string) bool {
	return fileExists(filepath.Join(root, ".venv")) || dirExists(filepath.Join(root, ".venv"))
}

func bootstrapNode(ctx context.Context, root, entrypoint string) (PluginBootstrapResult, error) {
	shape, err := detectNodeProjectShape(root, entrypoint)
	if err != nil {
		return PluginBootstrapResult{}, err
	}
	npmPath, err := bootstrapLookPath("npm")
	if err != nil {
		return PluginBootstrapResult{}, fmt.Errorf("bootstrap failed: npm not found in PATH; install Node.js 20+")
	}
	if err := runBootstrapCommand(ctx, root, npmPath, "install"); err != nil {
		return PluginBootstrapResult{}, err
	}
	lines := []string{"Installed Node dependencies with npm install"}
	if shape.IsTypeScript {
		if strings.TrimSpace(shape.BuildScript) == "" {
			return PluginBootstrapResult{}, fmt.Errorf("bootstrap failed: TypeScript lane detected but package.json is missing a build script")
		}
		if err := runBootstrapCommand(ctx, root, npmPath, "run", "build"); err != nil {
			return PluginBootstrapResult{}, err
		}
		lines = append(lines, "Built TypeScript output with npm run build")
	}
	return PluginBootstrapResult{Lines: lines}, nil
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

type nodeProjectShape struct {
	TargetRel    string
	BuiltOutput  bool
	IsTypeScript bool
	BuildScript  string
}

func detectNodeProjectShape(root, entrypoint string) (nodeProjectShape, error) {
	targetRel := detectNodeRuntimeTarget(root, entrypoint)
	builtOutput := strings.HasPrefix(targetRel, "dist/") || strings.HasPrefix(targetRel, "build/")
	buildScript := ""
	if body, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if err := json.Unmarshal(body, &pkg); err == nil && pkg.Scripts != nil {
			buildScript = strings.TrimSpace(pkg.Scripts["build"])
		}
	}
	isTypeScript := builtOutput && fileExists(filepath.Join(root, "tsconfig.json")) && buildScript != ""
	return nodeProjectShape{
		TargetRel:    targetRel,
		BuiltOutput:  builtOutput,
		IsTypeScript: isTypeScript,
		BuildScript:  buildScript,
	}, nil
}

func detectNodeRuntimeTarget(root, entrypoint string) string {
	body, err := os.ReadFile(bootstrapLauncherPath(root, entrypoint))
	if err != nil {
		return "src/main.mjs"
	}
	text := filepath.ToSlash(string(body))
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\$ROOT/([^"\s]+\.(?:mjs|js))`),
		regexp.MustCompile(`%ROOT%/([^"\r\n]+\.(?:mjs|js))`),
	}
	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) == 2 {
			return matches[1]
		}
	}
	return "src/main.mjs"
}

func bootstrapLauncherPath(root, entrypoint string) string {
	rel := strings.TrimPrefix(filepath.Clean(entrypoint), "./")
	full := filepath.Join(root, rel)
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(full + ".cmd"); err == nil {
			return full + ".cmd"
		}
	}
	return full
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
