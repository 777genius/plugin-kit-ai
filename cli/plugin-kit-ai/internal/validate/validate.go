package validate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
)

type FailureKind string

const (
	FailureUnknownPlatform          FailureKind = "unknown_platform"
	FailureCannotInferPlatform      FailureKind = "cannot_infer_platform"
	FailureManifestMissing          FailureKind = "manifest_missing"
	FailureManifestInvalid          FailureKind = "manifest_invalid"
	FailureRequiredFileMissing      FailureKind = "required_file_missing"
	FailureForbiddenFilePresent     FailureKind = "forbidden_file_present"
	FailureBuildFailed              FailureKind = "build_failed"
	FailureRuntimeNotFound          FailureKind = "runtime_not_found"
	FailureEntrypointMismatch       FailureKind = "entrypoint_mismatch"
	FailureLauncherInvalid          FailureKind = "launcher_invalid"
	FailureRuntimeTargetMissing     FailureKind = "runtime_target_missing"
	FailureGeneratedContractInvalid FailureKind = "generated_contract_invalid"
	FailureSourceFileMissing        FailureKind = "source_file_missing"
)

type Failure struct {
	Kind    FailureKind
	Path    string
	Target  string
	Message string
}

type WarningKind string

const (
	WarningManifestUnknownField    WarningKind = "manifest_unknown_field"
	WarningManifestDeprecatedField WarningKind = "manifest_deprecated_field"
)

type Warning struct {
	Kind    WarningKind
	Path    string
	Message string
}

type Report struct {
	Platform string
	Checks   []string
	Warnings []Warning
	Failures []Failure
}

type ReportError struct {
	Report Report
}

func (e *ReportError) Error() string {
	if len(e.Report.Failures) == 0 {
		return "validation failed"
	}
	f := e.Report.Failures[0]
	switch f.Kind {
	case FailureRequiredFileMissing:
		return "required file missing: " + f.Path
	case FailureForbiddenFilePresent:
		return fmt.Sprintf("forbidden file present for platform %s: %s", e.Report.Platform, f.Path)
	case FailureBuildFailed:
		return fmt.Sprintf("go build %s: %s", f.Target, f.Message)
	case FailureManifestMissing, FailureManifestInvalid, FailureRuntimeNotFound, FailureEntrypointMismatch, FailureLauncherInvalid, FailureRuntimeTargetMissing:
		return f.Message
	default:
		return f.Message
	}
}

type Rule struct {
	Platform       string
	RequiredFiles  []string
	ForbiddenFiles []string
	BuildTargets   []string
}

func Run(root, platform string) error {
	report, err := Validate(root, platform)
	if err != nil {
		return err
	}
	if len(report.Failures) > 0 {
		return &ReportError{Report: report}
	}
	return nil
}

func Validate(root, platform string) (Report, error) {
	if fileExists(filepath.Join(root, pluginmanifest.FileName)) {
		return validatePluginProject(root, platform)
	}
	if fileExists(filepath.Join(root, ".plugin-kit-ai", "project.toml")) {
		return validateManifestProject(root, platform)
	}
	rule, err := resolveRule(root, platform)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Platform: rule.Platform,
		Checks:   []string{"required_files", "forbidden_files", "build_targets"},
	}
	for _, rel := range rule.RequiredFiles {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRequiredFileMissing,
				Path:    rel,
				Message: "required file missing: " + rel,
			})
		}
	}
	for _, rel := range rule.ForbiddenFiles {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err == nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureForbiddenFilePresent,
				Path:    rel,
				Message: fmt.Sprintf("forbidden file present for platform %s: %s", rule.Platform, rel),
			})
		}
	}
	if len(report.Failures) > 0 {
		return report, nil
	}
	for _, target := range rule.BuildTargets {
		cmd := exec.Command("go", "build", target)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureBuildFailed,
				Target:  target,
				Message: fmt.Sprintf("%v\n%s", err, out),
			})
		}
	}
	return report, nil
}

func validatePluginProject(root, platform string) (Report, error) {
	manifest, warnings, err := pluginmanifest.LoadWithWarnings(root)
	if err != nil {
		return Report{}, &ReportError{Report: Report{
			Failures: []Failure{{
				Kind:    FailureManifestInvalid,
				Message: err.Error(),
			}},
		}}
	}

	report := Report{
		Platform: strings.Join(manifest.EnabledTargets(), ","),
		Checks:   []string{"plugin_manifest", "source_files", "generated_artifacts", "runtime"},
	}
	if strings.TrimSpace(platform) != "" && !slices.Contains(manifest.EnabledTargets(), strings.TrimSpace(platform)) {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("plugin.yaml does not enable target %q", platform),
		})
	}
	for _, warning := range warnings {
		report.Warnings = append(report.Warnings, Warning{
			Kind:    mapManifestWarningKind(warning.Kind),
			Path:    warning.Path,
			Message: warning.Message,
		})
	}
	for _, rel := range manifest.ComponentPaths() {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureSourceFileMissing,
				Path:    rel,
				Message: "referenced source file missing: " + rel,
			})
		}
	}
	if drift, err := pluginmanifest.Drift(root, targetOrAll(platform)); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureGeneratedContractInvalid,
			Message: err.Error(),
		})
	} else {
		for _, rel := range drift {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureGeneratedContractInvalid,
				Path:    rel,
				Message: "generated artifact drift: " + rel,
			})
		}
	}
	validatePluginRuntimeFiles(root, manifest, &report)
	return report, nil
}

func mapManifestWarningKind(kind pluginmanifest.WarningKind) WarningKind {
	switch kind {
	case pluginmanifest.WarningDeprecatedField:
		return WarningManifestDeprecatedField
	default:
		return WarningManifestUnknownField
	}
}

func validateManifestProject(root, platform string) (Report, error) {
	manifest, err := loadManifest(root)
	if err != nil {
		if os.IsNotExist(err) {
			return Report{}, &ReportError{Report: Report{
				Failures: []Failure{{
					Kind:    FailureManifestMissing,
					Message: "required manifest missing: .plugin-kit-ai/project.toml",
				}},
			}}
		}
		return Report{}, &ReportError{Report: Report{
			Failures: []Failure{{
				Kind:    FailureManifestInvalid,
				Message: "invalid manifest: " + err.Error(),
			}},
		}}
	}

	report := Report{
		Platform: manifest.Platform,
		Checks:   []string{"manifest", "required_files", "entrypoint", "runtime"},
	}
	if strings.TrimSpace(platform) != "" && strings.TrimSpace(platform) != manifest.Platform {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("manifest platform %q does not match requested platform %q", manifest.Platform, platform),
		})
	}
	if manifest.SchemaVersion != 1 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("invalid manifest: unsupported schema_version %d", manifest.SchemaVersion),
		})
	}
	if manifest.Platform != "codex" && manifest.Platform != "claude" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("invalid manifest: unsupported platform %q", manifest.Platform),
		})
	}
	if manifest.Runtime != "go" && manifest.Runtime != "python" && manifest.Runtime != "node" && manifest.Runtime != "shell" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("invalid manifest: unsupported runtime %q", manifest.Runtime),
		})
	}
	if manifest.ExecutionMode != "direct" && manifest.ExecutionMode != "launcher" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("invalid manifest: unsupported execution_mode %q", manifest.ExecutionMode),
		})
	}
	if strings.TrimSpace(manifest.Entrypoint) == "" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: "invalid manifest: entrypoint required",
		})
	}

	for _, rel := range requiredPlatformFiles(manifest.Platform) {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRequiredFileMissing,
				Path:    rel,
				Message: "required file missing: " + rel,
			})
		}
	}
	for _, rel := range forbiddenPlatformFiles(manifest.Platform) {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err == nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureForbiddenFilePresent,
				Path:    rel,
				Message: fmt.Sprintf("forbidden file present for platform %s: %s", manifest.Platform, rel),
			})
		}
	}
	if err := validateEntrypointConfig(root, manifest); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureEntrypointMismatch,
			Message: err.Error(),
		})
	}
	validateRuntimeFiles(root, manifest, &report)
	return report, nil
}

func resolveRule(root, platform string) (Rule, error) {
	if strings.TrimSpace(platform) != "" {
		rule, ok := LookupRule(platform)
		if !ok {
			return Rule{}, &ReportError{Report: Report{
				Failures: []Failure{{
					Kind:    FailureUnknownPlatform,
					Message: fmt.Sprintf("unknown platform %q", platform),
				}},
			}}
		}
		return rule, nil
	}
	if fileExists(filepath.Join(root, "AGENTS.md")) {
		rule, _ := LookupRule("codex")
		return rule, nil
	}
	if fileExists(filepath.Join(root, ".claude-plugin", "plugin.json")) {
		rule, _ := LookupRule("claude")
		return rule, nil
	}
	return Rule{}, &ReportError{Report: Report{
		Failures: []Failure{{
			Kind:    FailureCannotInferPlatform,
			Message: "could not infer platform",
		}},
	}}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func requiredPlatformFiles(platform string) []string {
	switch platform {
	case "codex":
		return []string{"README.md", "AGENTS.md", ".codex/config.toml", ".plugin-kit-ai/project.toml"}
	case "claude":
		return []string{"README.md", ".claude-plugin/plugin.json", "hooks/hooks.json", ".plugin-kit-ai/project.toml"}
	default:
		return nil
	}
}

func forbiddenPlatformFiles(platform string) []string {
	switch platform {
	case "codex":
		return []string{".claude-plugin/plugin.json", "hooks/hooks.json"}
	case "claude":
		return []string{"AGENTS.md", ".codex/config.toml"}
	default:
		return nil
	}
}

func validateEntrypointConfig(root string, manifest projectManifest) error {
	switch manifest.Platform {
	case "codex":
		body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
		if err != nil {
			return fmt.Errorf("entrypoint mismatch: %v", err)
		}
		want := fmt.Sprintf(`notify = ["%s", "notify"]`, manifest.Entrypoint)
		if !strings.Contains(string(body), want) {
			return fmt.Errorf("entrypoint mismatch: .codex/config.toml must contain %q", want)
		}
	case "claude":
		body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
		if err != nil {
			return fmt.Errorf("entrypoint mismatch: %v", err)
		}
		text := string(body)
		for _, hook := range []string{
			"SessionStart",
			"SessionEnd",
			"Notification",
			"PostToolUse",
			"PostToolUseFailure",
			"PermissionRequest",
			"SubagentStart",
			"SubagentStop",
			"PreCompact",
			"Setup",
			"Stop",
			"PreToolUse",
			"TeammateIdle",
			"TaskCompleted",
			"UserPromptSubmit",
			"ConfigChange",
			"WorktreeCreate",
			"WorktreeRemove",
		} {
			want := fmt.Sprintf(`"command": "%s %s"`, manifest.Entrypoint, hook)
			if !strings.Contains(text, want) {
				return fmt.Errorf("entrypoint mismatch: hooks/hooks.json must contain %q", want)
			}
		}
	}
	return nil
}

func validateRuntimeFiles(root string, manifest projectManifest, report *Report) {
	switch manifest.Runtime {
	case "go":
		validateRuntimeFileExists(root, "go.mod", report)
		if manifest.ExecutionMode != "direct" {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Message: fmt.Sprintf("invalid manifest: runtime %q requires execution_mode %q", manifest.Runtime, "direct"),
			})
			return
		}
		for _, target := range []string{"./..."} {
			cmd := exec.Command("go", "build", target)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "GOWORK=off")
			if out, err := cmd.CombinedOutput(); err != nil {
				report.Failures = append(report.Failures, Failure{
					Kind:    FailureBuildFailed,
					Target:  target,
					Message: fmt.Sprintf("%v\n%s", err, out),
				})
			}
		}
	case "python":
		validateLauncher(root, manifest, report)
		validateRuntimeFileExists(root, "src/main.py", report)
		if _, err := findPython(root); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Message: err.Error(),
			})
		}
	case "node":
		validateLauncher(root, manifest, report)
		validateRuntimeFileExists(root, "src/main.mjs", report)
		validateRuntimeFileExists(root, "package.json", report)
		if _, err := exec.LookPath("node"); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Message: "runtime not found: node",
			})
		}
	case "shell":
		validateLauncher(root, manifest, report)
		validateRuntimeTargetExecutable(root, "scripts/main.sh", report)
		if runtime.GOOS == "windows" {
			if _, err := exec.LookPath("bash"); err != nil {
				report.Failures = append(report.Failures, Failure{
					Kind:    FailureRuntimeNotFound,
					Message: "runtime not found: bash",
				})
			}
		}
	}
}

func validatePluginRuntimeFiles(root string, manifest pluginmanifest.Manifest, report *Report) {
	switch manifest.Runtime {
	case "go":
		validateRuntimeFileExists(root, "go.mod", report)
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureBuildFailed,
				Target:  "./...",
				Message: fmt.Sprintf("%v\n%s", err, out),
			})
		}
	case "python":
		validatePluginLauncher(root, manifest, report)
		validateRuntimeFileExists(root, "src/main.py", report)
		if _, err := findPython(root); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Message: err.Error(),
			})
		}
	case "node":
		validatePluginLauncher(root, manifest, report)
		validateRuntimeFileExists(root, "src/main.mjs", report)
		validateRuntimeFileExists(root, "package.json", report)
		if _, err := exec.LookPath("node"); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Message: "runtime not found: node",
			})
		}
	case "shell":
		validatePluginLauncher(root, manifest, report)
		validateRuntimeTargetExecutable(root, "scripts/main.sh", report)
	}
}

func validatePluginLauncher(root string, manifest pluginmanifest.Manifest, report *Report) {
	info, err := statLauncher(root, manifest.Entrypoint)
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "launcher invalid: missing " + manifest.Entrypoint,
		})
		return
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "launcher invalid: not executable " + manifest.Entrypoint,
		})
	}
}

func targetOrAll(platform string) string {
	if strings.TrimSpace(platform) == "" {
		return "all"
	}
	return platform
}

func validateLauncher(root string, manifest projectManifest, report *Report) {
	if manifest.ExecutionMode != "launcher" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: fmt.Sprintf("invalid manifest: runtime %q requires execution_mode %q", manifest.Runtime, "launcher"),
		})
		return
	}
	info, err := statLauncher(root, manifest.Entrypoint)
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "launcher invalid: missing " + manifest.Entrypoint,
		})
		return
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "launcher invalid: not executable " + manifest.Entrypoint,
		})
	}
}

func statLauncher(root, entrypoint string) (os.FileInfo, error) {
	rel := strings.TrimPrefix(filepath.Clean(entrypoint), "./")
	candidates := []string{filepath.Join(root, rel)}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(root, rel+".cmd"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil {
			return info, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, os.ErrNotExist
}

func validateRuntimeFileExists(root, rel string, report *Report) {
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureRuntimeTargetMissing,
			Path:    rel,
			Message: "runtime target missing: " + rel,
		})
	}
}

func validateRuntimeTargetExecutable(root, rel string, report *Report) {
	info, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureRuntimeTargetMissing,
			Path:    rel,
			Message: "runtime target missing: " + rel,
		})
		return
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureRuntimeTargetMissing,
			Path:    rel,
			Message: "runtime target missing: " + rel + " is not executable",
		})
	}
}

func findPython(root string) (string, error) {
	candidates := pythonCandidates(root)
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	for _, name := range pythonPathNames() {
		path, err := exec.LookPath(name)
		if err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("runtime not found: %s", strings.Join(pythonPathNames(), " or "))
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
	return []string{"python3"}
}
