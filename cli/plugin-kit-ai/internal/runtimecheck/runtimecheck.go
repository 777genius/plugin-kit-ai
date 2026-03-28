package runtimecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
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

var launcherTargetPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\$ROOT/([^"\s]+\.(?:mjs|js|cjs))`),
	regexp.MustCompile(`%ROOT%/([^"\r\n]+\.(?:mjs|js|cjs))`),
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

func diagnoseNode(project Project, nextValidate string) Diagnosis {
	shape := project.Node
	if shape.StructuralIssue != "" {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: shape.StructuralIssue,
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if !shape.ManagerAvailable {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("%s not found in PATH", shape.ManagerBinary),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if !shape.Installed {
		return Diagnosis{
			Status: StatusNeedsBootstrap,
			Reason: fmt.Sprintf("%s install state is missing", shape.ManagerDisplay()),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if shape.IsTypeScript && !shape.RuntimeTargetOK {
		return Diagnosis{
			Status: StatusNeedsBuild,
			Reason: fmt.Sprintf("built output %s is missing", shape.RuntimeTarget),
			Next:   []string{"plugin-kit-ai bootstrap .", nextValidate},
		}
	}
	if !shape.RuntimeTargetOK {
		return Diagnosis{
			Status: StatusBlocked,
			Reason: fmt.Sprintf("runtime target %s is missing", shape.RuntimeTarget),
			Next:   []string{nextValidate},
		}
	}
	return Diagnosis{
		Status: StatusReady,
		Reason: fmt.Sprintf("Node runtime is ready via %s", shape.ManagerDisplay()),
		Next:   []string{nextValidate},
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

func inspectNode(root, entrypoint string) NodeShape {
	shape := NodeShape{
		Manager:         detectNodeManager(root),
		LauncherTarget:  detectNodeRuntimeTarget(root, entrypoint),
		OutputDir:       "dist",
		PackageJSONPath: "package.json",
	}
	if body, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		shape.PackageJSON = true
		var pkg packageJSON
		if json.Unmarshal(body, &pkg) == nil {
			shape.BuildScript = strings.TrimSpace(pkg.Scripts["build"])
			shape.PackageManager = strings.TrimSpace(pkg.PackageManager)
		}
	}
	if fileExists(filepath.Join(root, "tsconfig.json")) {
		shape.TSConfig = true
		shape.TSConfigPath = "tsconfig.json"
		if outDir := parseTSOutDir(root); outDir != "" {
			shape.OutputDir = outDir
		}
	}
	shape.ManagerBinary = string(shape.Manager)
	shape.ManagerAvailable = lookupBinary(shape.ManagerBinary)
	shape.UsesBuiltOutput = isBuiltOutputTarget(shape.LauncherTarget, shape.OutputDir)
	shape.RuntimeTarget = shape.LauncherTarget
	shape.RuntimeTargetOK = fileExists(filepath.Join(root, filepath.FromSlash(shape.RuntimeTarget)))
	shape.Installed = nodeInstallStatePresent(root)

	if !shape.PackageJSON {
		shape.StructuralIssue = "package.json is missing for node runtime"
		return shape
	}
	if shape.UsesBuiltOutput {
		if !shape.TSConfig {
			shape.StructuralIssue = fmt.Sprintf("launcher target %s points to built output but tsconfig.json is missing", shape.LauncherTarget)
			return shape
		}
		if shape.BuildScript == "" {
			shape.StructuralIssue = "TypeScript lane is missing package.json scripts.build"
			return shape
		}
		if !strings.HasPrefix(shape.LauncherTarget, shape.OutputDir+"/") {
			shape.StructuralIssue = fmt.Sprintf("launcher target %s is outside tsconfig outDir %s", shape.LauncherTarget, shape.OutputDir)
			return shape
		}
		shape.IsTypeScript = true
		return shape
	}
	if shape.TSConfig && shape.BuildScript != "" && strings.HasSuffix(shape.LauncherTarget, ".js") && !strings.HasPrefix(shape.LauncherTarget, shape.OutputDir+"/") {
		shape.StructuralIssue = fmt.Sprintf("launcher target %s is outside tsconfig outDir %s", shape.LauncherTarget, shape.OutputDir)
	}
	return shape
}

func (p Project) ProjectLine() string {
	manager := "none"
	switch p.Runtime {
	case "python":
		manager = p.Python.ManagerDisplay()
	case "node":
		manager = p.Node.ManagerDisplay()
	case "":
		manager = "n/a"
	}
	return fmt.Sprintf("Project: lane=%s runtime=%s manager=%s", p.Lane, valueOrDefault(p.Runtime, "none"), manager)
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

func (n NodeShape) ManagerDisplay() string {
	if n.Manager == "" {
		return "npm"
	}
	return string(n.Manager)
}

func (n NodeShape) BuildCommandString() string {
	switch n.Manager {
	case NodeManagerPNPM:
		return "pnpm run build"
	case NodeManagerYarn:
		return "yarn build"
	case NodeManagerBun:
		return "bun run build"
	default:
		return "npm run build"
	}
}

func ValidateCommand(targets []string) string {
	return validateCommand(targets)
}

func laneSummary(targets []string) string {
	if len(targets) == 0 {
		return "none"
	}
	return strings.Join(targets, ",")
}

func validateCommand(targets []string) string {
	if len(targets) == 1 {
		return fmt.Sprintf("plugin-kit-ai validate . --platform %s --strict", targets[0])
	}
	return "plugin-kit-ai validate . --strict"
}

func firstExisting(root string, names ...string) string {
	for _, name := range names {
		if fileExists(filepath.Join(root, name)) {
			return name
		}
	}
	return ""
}

func firstAvailableBinary(names []string) string {
	for _, name := range names {
		if lookupBinary(name) {
			return name
		}
	}
	return ""
}

func lookupBinary(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	_, err := LookPath(name)
	return err == nil
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

func detectNodeManager(root string) NodeManager {
	switch {
	case fileExists(filepath.Join(root, "bun.lock")) || fileExists(filepath.Join(root, "bun.lockb")):
		return NodeManagerBun
	case fileExists(filepath.Join(root, "pnpm-lock.yaml")):
		return NodeManagerPNPM
	case fileExists(filepath.Join(root, "yarn.lock")):
		return NodeManagerYarn
	default:
		return NodeManagerNPM
	}
}

func parseTSOutDir(root string) string {
	body, err := os.ReadFile(filepath.Join(root, "tsconfig.json"))
	if err != nil {
		return ""
	}
	var cfg tsConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return ""
	}
	outDir := strings.TrimSpace(filepath.ToSlash(cfg.CompilerOptions.OutDir))
	if outDir == "" {
		return ""
	}
	return strings.TrimPrefix(outDir, "./")
}

func isBuiltOutputTarget(target, outDir string) bool {
	target = filepath.ToSlash(strings.TrimSpace(target))
	if outDir != "" && strings.HasPrefix(target, strings.Trim(strings.TrimSpace(outDir), "/")+"/") {
		return true
	}
	return strings.HasPrefix(target, "dist/") || strings.HasPrefix(target, "build/")
}

func nodeInstallStatePresent(root string) bool {
	if dirExists(filepath.Join(root, "node_modules")) {
		return true
	}
	return fileExists(filepath.Join(root, ".pnp.cjs")) || fileExists(filepath.Join(root, ".pnp.loader.mjs"))
}

func detectNodeRuntimeTarget(root, entrypoint string) string {
	body, err := os.ReadFile(launcherPath(root, entrypoint))
	if err != nil {
		return "src/main.mjs"
	}
	text := filepath.ToSlash(string(body))
	for _, pattern := range launcherTargetPatterns {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) == 2 {
			return matches[1]
		}
	}
	return "src/main.mjs"
}

func launcherPath(root, entrypoint string) string {
	rel := strings.TrimPrefix(filepath.Clean(entrypoint), "./")
	full := filepath.Join(root, rel)
	if runtime.GOOS == "windows" {
		if _, err := os.Stat(full + ".cmd"); err == nil {
			return full + ".cmd"
		}
	}
	return full
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

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func YarnBerry(root string, packageManager string) bool {
	if fileExists(filepath.Join(root, ".yarnrc.yml")) {
		return true
	}
	if !strings.HasPrefix(packageManager, "yarn@") {
		return false
	}
	version := strings.TrimPrefix(packageManager, "yarn@")
	majorText := version
	if idx := strings.Index(majorText, "."); idx >= 0 {
		majorText = majorText[:idx]
	}
	major, err := strconv.Atoi(majorText)
	return err == nil && major >= 2
}

func pythonVersion(root, path string) (string, error) {
	return RunCommand(root, path, "--version")
}

func defaultRunCommand(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
