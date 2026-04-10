package runtimecheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

var LookPath = exec.LookPath
var RunCommand = defaultRunCommand

type DoctorStatus string

const (
	StatusReady          DoctorStatus = "ready"
	StatusNeedsBootstrap DoctorStatus = "needs_bootstrap"
	StatusNeedsBuild     DoctorStatus = "needs_build"
	StatusBlocked        DoctorStatus = "blocked"
)

type Inputs struct {
	Root     string
	Targets  []string
	Launcher *pluginmanifest.Launcher
}

type Project struct {
	Root               string
	Targets            []string
	Lane               string
	Runtime            string
	Entrypoint         string
	LauncherPath       string
	LauncherExists     bool
	LauncherExecutable bool
	Python             PythonShape
	Node               NodeShape
}

type PythonManager string

const (
	PythonManagerVenv         PythonManager = "venv"
	PythonManagerRequirements PythonManager = "requirements"
	PythonManagerUV           PythonManager = "uv"
	PythonManagerPoetry       PythonManager = "poetry"
	PythonManagerPipenv       PythonManager = "pipenv"
)

type PythonEnvSource string

const (
	PythonEnvSourceMissing      PythonEnvSource = "missing"
	PythonEnvSourceRepoLocal    PythonEnvSource = "repo-local .venv"
	PythonEnvSourceManagerOwned PythonEnvSource = "manager-owned env"
	PythonEnvSourceBroken       PythonEnvSource = "broken"
)

type NodeManager string

const (
	NodeManagerNPM  NodeManager = "npm"
	NodeManagerPNPM NodeManager = "pnpm"
	NodeManagerYarn NodeManager = "yarn"
	NodeManagerBun  NodeManager = "bun"
)

type PythonShape struct {
	Manager          PythonManager
	ManagerBinary    string
	ManifestPath     string
	HasVenv          bool
	VenvPath         string
	VenvRunnable     bool
	ProbedEnvPath    string
	ProbeAttempted   bool
	ProbeAvailable   bool
	ReadySource      PythonEnvSource
	ReadyInterpreter string
	VersionOutput    string
	BrokenReason     string
	ManagerAvailable bool
}

type NodeShape struct {
	Manager          NodeManager
	ManagerBinary    string
	PackageJSONPath  string
	PackageJSON      bool
	LauncherTarget   string
	RuntimeTarget    string
	RuntimeTargetOK  bool
	Installed        bool
	UsesBuiltOutput  bool
	IsTypeScript     bool
	TSConfigPath     string
	TSConfig         bool
	OutputDir        string
	BuildScript      string
	StructuralIssue  string
	ManagerAvailable bool
	PackageManager   string
}

type Diagnosis struct {
	Status DoctorStatus
	Reason string
	Next   []string
}

type packageJSON struct {
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

type tsConfig struct {
	Extends         string `json:"extends"`
	CompilerOptions struct {
		OutDir string `json:"outDir"`
	} `json:"compilerOptions"`
}

type pyProject struct {
	Tool map[string]map[string]any `toml:"tool"`
}

func Inspect(inputs Inputs) (Project, error) {
	root := strings.TrimSpace(inputs.Root)
	if root == "" {
		root = "."
	}
	project := Project{
		Root:    root,
		Targets: append([]string(nil), inputs.Targets...),
		Lane:    laneSummary(inputs.Targets),
	}
	if inputs.Launcher == nil {
		return project, nil
	}
	project.Runtime = strings.TrimSpace(inputs.Launcher.Runtime)
	project.Entrypoint = strings.TrimSpace(inputs.Launcher.Entrypoint)
	if project.Entrypoint != "" {
		project.LauncherPath = launcherPath(root, project.Entrypoint)
		if info, err := os.Stat(project.LauncherPath); err == nil {
			project.LauncherExists = true
			project.LauncherExecutable = runtime.GOOS == "windows" || info.Mode()&0o111 != 0
		}
	}
	switch project.Runtime {
	case "python":
		project.Python = inspectPython(root)
	case "node":
		project.Node = inspectNode(root, project.Entrypoint)
	}
	return project, nil
}

func Diagnose(project Project) Diagnosis {
	nextValidate := validateCommand(project.Targets)
	if project.Runtime == "" {
		return Diagnosis{
			Status: StatusReady,
			Reason: "no launcher-based runtime configured",
			Next:   []string{nextValidate},
		}
	}
	if !project.LauncherExists {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("launcher entrypoint %s is missing", strings.TrimSpace(project.Entrypoint)),
			Next:   []string{nextValidate},
		}
	}

	switch project.Runtime {
	case "go":
		return Diagnosis{
			Status: StatusReady,
			Reason: "Go runtime is configured",
			Next:   []string{nextValidate},
		}
	case "shell":
		if runtime.GOOS != "windows" && !project.LauncherExecutable {
			return Diagnosis{
				Status: StatusBlocked,
				Reason: fmt.Sprintf("launcher %s is not executable", filepath.ToSlash(project.LauncherPath)),
				Next:   []string{nextValidate},
			}
		}
		return Diagnosis{
			Status: StatusReady,
			Reason: "shell launcher is present",
			Next:   []string{nextValidate},
		}
	case "python":
		return diagnosePython(project, nextValidate)
	case "node":
		return diagnoseNode(project, nextValidate)
	default:
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("unsupported runtime %q", project.Runtime),
			Next:   []string{nextValidate},
		}
	}
}

func defaultRunCommand(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
