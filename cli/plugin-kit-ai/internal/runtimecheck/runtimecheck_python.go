package runtimecheck

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func diagnosePython(project Project, nextValidate string) Diagnosis {
	shape := project.Python
	if shape.BrokenReason != "" {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: shape.BrokenReason,
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if shape.ReadySource != PythonEnvSourceMissing {
		return Diagnosis{
			Status: StatusReady,
			Reason: fmt.Sprintf("Python runtime is ready via %s using %s", shape.ManagerDisplay(), shape.ReadySourceDisplay()),
			Next:   []string{nextValidate},
		}
	}
	if !shape.ManagerAvailable {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("%s not found in PATH", shape.ManagerBinary),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	return Diagnosis{
		Status: StatusNeedsBootstrap,
		Reason: fmt.Sprintf("%s environment is not ready", shape.CanonicalSourceDisplay()),
		Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
	}
}

func inspectPython(root string) PythonShape {
	hasUV, hasPoetry := parsePyProjectTools(root)
	shape := PythonShape{}
	switch {
	case fileExists(filepath.Join(root, "uv.lock")) || hasUV:
		shape = PythonShape{
			Manager:          PythonManagerUV,
			ManagerBinary:    "uv",
			ManifestPath:     firstExisting(root, "uv.lock", "pyproject.toml"),
			ManagerAvailable: lookupBinary("uv"),
		}
	case fileExists(filepath.Join(root, "poetry.lock")) || hasPoetry:
		shape = PythonShape{
			Manager:          PythonManagerPoetry,
			ManagerBinary:    "poetry",
			ManifestPath:     firstExisting(root, "poetry.lock", "pyproject.toml"),
			ManagerAvailable: lookupBinary("poetry"),
		}
	case fileExists(filepath.Join(root, "Pipfile.lock")) || fileExists(filepath.Join(root, "Pipfile")):
		shape = PythonShape{
			Manager:          PythonManagerPipenv,
			ManagerBinary:    "pipenv",
			ManifestPath:     firstExisting(root, "Pipfile.lock", "Pipfile"),
			ManagerAvailable: lookupBinary("pipenv"),
		}
	case fileExists(filepath.Join(root, "requirements.txt")):
		shape = PythonShape{
			Manager:          PythonManagerRequirements,
			ManagerBinary:    firstAvailableBinary(pythonPathNames()),
			ManifestPath:     "requirements.txt",
			ManagerAvailable: firstAvailableBinary(pythonPathNames()) != "",
		}
	default:
		shape = PythonShape{
			Manager:          PythonManagerVenv,
			ManagerBinary:    firstAvailableBinary(pythonPathNames()),
			ManagerAvailable: firstAvailableBinary(pythonPathNames()) != "",
		}
	}
	shape.HasVenv = hasVenv(root)
	shape.VenvPath = pythonInterpreter(root)
	if shape.VenvPath != "" {
		if version, err := pythonVersion(root, shape.VenvPath); err == nil {
			shape.VenvRunnable = true
			shape.ReadySource = PythonEnvSourceRepoLocal
			shape.ReadyInterpreter = shape.VenvPath
			shape.VersionOutput = version
			return shape
		}
		shape.BrokenReason = "found .venv but no runnable interpreter; recreate .venv or repair the virtualenv"
		shape.ReadySource = PythonEnvSourceBroken
		return shape
	}
	if shape.HasVenv {
		shape.BrokenReason = "found .venv but no runnable interpreter; recreate .venv or repair the virtualenv"
		shape.ReadySource = PythonEnvSourceBroken
		return shape
	}
	switch shape.Manager {
	case PythonManagerPoetry, PythonManagerPipenv:
		shape.ProbeAttempted = shape.ManagerAvailable
		if shape.ManagerAvailable {
			envRoot, ok := probeManagedPythonEnv(root, shape.Manager)
			shape.ProbeAvailable = ok
			if ok {
				shape.ProbedEnvPath = envRoot
				interpreter := pythonInterpreterInEnv(envRoot)
				if interpreter == "" {
					shape.BrokenReason = fmt.Sprintf("%s reported env %s but no runnable interpreter was found", shape.ManagerDisplay(), filepath.ToSlash(envRoot))
					shape.ReadySource = PythonEnvSourceBroken
					return shape
				}
				version, err := pythonVersion(root, interpreter)
				if err != nil {
					shape.BrokenReason = fmt.Sprintf("%s reported env %s but its interpreter is not runnable", shape.ManagerDisplay(), filepath.ToSlash(envRoot))
					shape.ReadySource = PythonEnvSourceBroken
					return shape
				}
				shape.ReadySource = PythonEnvSourceManagerOwned
				shape.ReadyInterpreter = interpreter
				shape.VersionOutput = version
				return shape
			}
		}
	}
	if shape.ReadySource == "" {
		shape.ReadySource = PythonEnvSourceMissing
	}
	return shape
}

func (p PythonShape) ManagerDisplay() string {
	switch p.Manager {
	case PythonManagerRequirements:
		return "requirements.txt (pip)"
	case PythonManagerVenv:
		return "venv"
	default:
		return string(p.Manager)
	}
}

func (p PythonShape) ReadySourceDisplay() string {
	switch p.ReadySource {
	case PythonEnvSourceRepoLocal:
		return "repo-local .venv"
	case PythonEnvSourceManagerOwned:
		return "manager-owned env"
	default:
		return "missing env"
	}
}

func (p PythonShape) CanonicalSourceDisplay() string {
	switch p.Manager {
	case PythonManagerPoetry, PythonManagerPipenv:
		return "manager-owned Python"
	default:
		return "project-local Python"
	}
}

func (p PythonShape) CanonicalEnvSourceDisplay() string {
	switch p.Manager {
	case PythonManagerPoetry, PythonManagerPipenv:
		return "manager-owned env"
	default:
		return "repo-local .venv"
	}
}

func (p PythonShape) BootstrapFallbackCommand() string {
	switch p.Manager {
	case PythonManagerUV:
		return "uv sync"
	case PythonManagerPoetry:
		return "poetry install --no-root"
	case PythonManagerPipenv:
		if p.ManifestPath == "Pipfile.lock" {
			return "pipenv sync"
		}
		return "pipenv install"
	case PythonManagerRequirements, PythonManagerVenv:
		if strings.TrimSpace(p.ManagerBinary) != "" {
			return p.ManagerBinary + " -m venv .venv"
		}
	}
	return ""
}

func parsePyProjectTools(root string) (bool, bool) {
	body, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return false, false
	}
	var project pyProject
	if err := toml.Unmarshal(body, &project); err != nil {
		return false, false
	}
	_, hasUV := project.Tool["uv"]
	_, hasPoetry := project.Tool["poetry"]
	return hasUV, hasPoetry
}

func probeManagedPythonEnv(root string, manager PythonManager) (string, bool) {
	switch manager {
	case PythonManagerPoetry:
		out, err := RunCommand(root, "poetry", "env", "info", "--path")
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(out), strings.TrimSpace(out) != ""
	case PythonManagerPipenv:
		out, err := RunCommand(root, "pipenv", "--venv")
		if err != nil {
			return "", false
		}
		return strings.TrimSpace(out), strings.TrimSpace(out) != ""
	default:
		return "", false
	}
}

func hasVenv(root string) bool {
	return fileExists(filepath.Join(root, ".venv")) || dirExists(filepath.Join(root, ".venv"))
}

func pythonInterpreter(root string) string {
	for _, candidate := range pythonCandidates(root) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func pythonInterpreterInEnv(envRoot string) string {
	for _, candidate := range pythonEnvCandidates(envRoot) {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func pythonCandidates(root string) []string {
	return pythonEnvCandidates(filepath.Join(root, ".venv"))
}

func pythonEnvCandidates(envRoot string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			filepath.Join(envRoot, "Scripts", "python.exe"),
			filepath.Join(envRoot, "bin", "python3"),
		}
	}
	return []string{
		filepath.Join(envRoot, "bin", "python3"),
		filepath.Join(envRoot, "Scripts", "python.exe"),
	}
}

func pythonPathNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"python", "python3"}
	}
	return []string{"python3", "python"}
}

func pythonVersion(root, path string) (string, error) {
	return RunCommand(root, path, "--version")
}
