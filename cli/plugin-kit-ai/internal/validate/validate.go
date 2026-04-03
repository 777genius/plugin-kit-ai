package validate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
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
	FailureUnsupportedTargetKind    FailureKind = "unsupported_target_kind"
)

type Failure struct {
	Kind    FailureKind
	Path    string
	Target  string
	Message string
}

type WarningKind string

const (
	WarningManifestUnknownField  WarningKind = "manifest_unknown_field"
	WarningGeminiDirNameMismatch WarningKind = "gemini_dir_name_mismatch"
	WarningGeminiMCPCommandStyle WarningKind = "gemini_mcp_command_style"
	WarningGeminiPolicyIgnored   WarningKind = "gemini_policy_ignored"
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
		return Report{}, &ReportError{Report: Report{
			Failures: []Failure{{
				Kind:    FailureManifestInvalid,
				Message: "unsupported project format: .plugin-kit-ai/project.toml is not supported; use plugin.yaml and targets/<platform>/...",
			}},
		}}
	}
	return Report{}, &ReportError{Report: Report{
		Failures: []Failure{{
			Kind:    FailureManifestMissing,
			Message: "required manifest missing: plugin.yaml",
		}},
	}}
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
		Checks:   []string{"plugin_manifest", "package_graph", "generated_artifacts", "runtime"},
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
	graph, _, err := pluginmanifest.Discover(root)
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Message: err.Error(),
		})
		return report, nil
	}
	for _, rel := range graph.SourceFiles {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureSourceFileMissing,
				Path:    rel,
				Message: "referenced source file missing: " + rel,
			})
		}
	}
	for _, targetName := range manifest.EnabledTargets() {
		if rule, ok := LookupRule(targetName); ok {
			for _, rel := range rule.ForbiddenFiles {
				if !fileExists(filepath.Join(root, rel)) {
					continue
				}
				report.Failures = append(report.Failures, Failure{
					Kind:    FailureForbiddenFilePresent,
					Path:    rel,
					Target:  targetName,
					Message: fmt.Sprintf("target %s forbids %s", targetName, rel),
				})
			}
		}
		entry, ok := targetcontracts.Lookup(targetName)
		if !ok {
			continue
		}
		tc := graph.Targets[targetName]
		supportedPortable := setOf(entry.PortableComponentKinds)
		if len(graph.Portable.Paths("skills")) > 0 && !supportedPortable["skills"] {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    "skills",
				Target:  targetName,
				Message: fmt.Sprintf("target %s does not support portable component kind skills", targetName),
			})
		}
		if graph.Portable.MCP != nil && !supportedPortable["mcp_servers"] {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    "mcp",
				Target:  targetName,
				Message: fmt.Sprintf("target %s does not support portable component kind mcp_servers", targetName),
			})
		}
		supportedNative := setOf(entry.TargetComponentKinds)
		for _, kind := range pluginmanifest.DiscoveredTargetKinds(tc) {
			if supportedNative[kind] {
				continue
			}
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    kind,
				Target:  targetName,
				Message: fmt.Sprintf("target %s does not support target-native component kind %s", targetName, kind),
			})
		}
		validateTargetExtraDocs(root, targetName, tc, &report)
		validateUnsupportedTargetSurfaces(root, targetName, &report)
		if adapter, ok := platformexec.Lookup(targetName); ok {
			diagnostics, err := adapter.Validate(root, graph, tc)
			if err != nil {
				report.Failures = append(report.Failures, Failure{
					Kind:    FailureManifestInvalid,
					Target:  targetName,
					Message: err.Error(),
				})
				continue
			}
			applyAdapterDiagnostics(&report, diagnostics)
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
	validatePluginRuntimeFiles(root, manifest, graph.Launcher, &report)
	return report, nil
}

func validateUnsupportedTargetSurfaces(root, target string, report *Report) {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return
	}
	for _, surface := range profile.SurfaceTiers {
		if surface.Tier != platformmeta.SurfaceTierUnsupported {
			continue
		}
		for _, path := range unsupportedSurfacePaths(root, target, surface.Kind, profile) {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    path,
				Target:  target,
				Message: fmt.Sprintf("target %s does not support authored surface %s", target, surface.Kind),
			})
		}
	}
}

func unsupportedSurfacePaths(root, target, kind string, profile platformmeta.PlatformProfile) []string {
	seen := map[string]struct{}{}
	for _, doc := range profile.NativeDocs {
		if doc.Kind != kind {
			continue
		}
		if fileExists(filepath.Join(root, doc.Path)) {
			seen[doc.Path] = struct{}{}
		}
	}
	dir := filepath.Join("targets", target, kind)
	if entries, err := os.ReadDir(filepath.Join(root, dir)); err == nil && len(entries) > 0 {
		seen[dir] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func applyAdapterDiagnostics(report *Report, diagnostics []platformexec.Diagnostic) {
	for _, diagnostic := range diagnostics {
		switch diagnostic.Severity {
		case platformexec.SeverityWarning:
			report.Warnings = append(report.Warnings, Warning{
				Kind:    mapAdapterWarningKind(diagnostic.Code),
				Path:    diagnostic.Path,
				Message: diagnostic.Message,
			})
		default:
			report.Failures = append(report.Failures, Failure{
				Kind:    mapAdapterFailureKind(diagnostic.Code),
				Path:    diagnostic.Path,
				Target:  diagnostic.Target,
				Message: diagnostic.Message,
			})
		}
	}
}

func mapAdapterFailureKind(code string) FailureKind {
	switch code {
	case platformexec.CodeGeneratedContractInvalid:
		return FailureGeneratedContractInvalid
	case platformexec.CodeEntrypointMismatch:
		return FailureEntrypointMismatch
	default:
		return FailureManifestInvalid
	}
}

func mapAdapterWarningKind(code string) WarningKind {
	switch code {
	case platformexec.CodeGeminiDirNameMismatch:
		return WarningGeminiDirNameMismatch
	case platformexec.CodeGeminiMCPCommandStyle:
		return WarningGeminiMCPCommandStyle
	case platformexec.CodeGeminiPolicyIgnored:
		return WarningGeminiPolicyIgnored
	default:
		return WarningManifestUnknownField
	}
}

func validateTargetExtraDocs(root, target string, tc pluginmanifest.TargetComponents, report *Report) {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return
	}
	for _, doc := range profile.NativeDocs {
		if doc.Role != platformmeta.NativeDocRoleExtra {
			continue
		}
		format := pluginmanifest.NativeDocFormatJSON
		if doc.Format == platformmeta.NativeDocTOML {
			format = pluginmanifest.NativeDocFormatTOML
		}
		label := target + " " + filepath.Base(doc.Path)
		validateTargetExtraDoc(root, target, tc.DocPath(doc.Kind), format, label, doc.ManagedKeys, report)
	}
}

func mapManifestWarningKind(kind pluginmanifest.WarningKind) WarningKind {
	switch kind {
	case pluginmanifest.WarningUnknownField:
		return WarningManifestUnknownField
	default:
		return WarningManifestUnknownField
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func validatePluginRuntimeFiles(root string, manifest pluginmanifest.Manifest, launcher *pluginmanifest.Launcher, report *Report) {
	requireLauncher := false
	for _, target := range manifest.EnabledTargets() {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		if profile.Launcher.Requirement == platformmeta.LauncherRequired {
			requireLauncher = true
			break
		}
	}
	if launcher == nil {
		if requireLauncher {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureLauncherInvalid,
				Path:    pluginmanifest.LauncherFileName,
				Message: "launcher invalid: missing " + pluginmanifest.LauncherFileName,
			})
		}
		return
	}
	switch launcher.Runtime {
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
		validatePluginLauncher(root, launcher, report)
		validateRuntimeFileExists(root, "src/main.py", report)
		if err := validatePythonRuntime(root, manifest.EnabledTargets(), launcher); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Message: err.Error(),
			})
		}
	case "node":
		validatePluginLauncher(root, launcher, report)
		validateRuntimeFileExists(root, "package.json", report)
		validateNodeRuntimeTarget(root, launcher.Entrypoint, report)
		if err := validateNodeRuntime(); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Message: err.Error(),
			})
		}
	case "shell":
		validatePluginLauncher(root, launcher, report)
		validateRuntimeTargetExecutable(root, "scripts/main.sh", report)
		if runtime.GOOS == "windows" {
			if _, err := exec.LookPath("bash"); err != nil {
				report.Failures = append(report.Failures, Failure{
					Kind:    FailureRuntimeNotFound,
					Message: "runtime not found: bash (shell runtime on Windows requires bash in PATH; install Git Bash or another bash-compatible shell)",
				})
			}
		}
	}
}

func validatePluginLauncher(root string, launcher *pluginmanifest.Launcher, report *Report) {
	if launcher == nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    pluginmanifest.LauncherFileName,
			Message: "launcher invalid: missing " + pluginmanifest.LauncherFileName,
		})
		return
	}
	info, err := statLauncher(root, launcher.Entrypoint)
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "launcher invalid: missing " + launcher.Entrypoint,
		})
		return
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "launcher invalid: not executable " + launcher.Entrypoint,
		})
	}
}

func targetOrAll(platform string) string {
	if strings.TrimSpace(platform) == "" {
		return "all"
	}
	return platform
}

func validateTargetExtraDoc(root, target, rel string, format pluginmanifest.NativeDocFormat, label string, managedPaths []string, report *Report) {
	if strings.TrimSpace(rel) == "" {
		return
	}
	doc, err := pluginmanifest.LoadNativeExtraDoc(root, rel, format)
	if err != nil {
		formatName := strings.ToUpper(string(format))
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  target,
			Message: fmt.Sprintf("%s %s is invalid %s: %v", label, rel, formatName, err),
		})
		return
	}
	if err := pluginmanifest.ValidateNativeExtraDocConflicts(doc, label, managedPaths); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  target,
			Message: err.Error(),
		})
	}
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
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

func validatePythonRuntime(root string, targets []string, launcher *pluginmanifest.Launcher) error {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  targets,
		Launcher: launcher,
	})
	if err != nil {
		return fmt.Errorf("runtime not found: python runtime inspection failed: %v", err)
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return fmt.Errorf("runtime not found: %s. %s", diagnosis.Reason, pythonRecoveryMessage(project.Python))
	}
	if err := requireMinVersion("python", project.Python.VersionOutput, 3, 10); err != nil {
		return fmt.Errorf("runtime not found: found %s interpreter at %s but %v. %s",
			project.Python.ReadySourceDisplay(),
			filepath.ToSlash(project.Python.ReadyInterpreter),
			err,
			pythonRecoveryMessage(project.Python),
		)
	}
	return nil
}

func pythonRecoveryMessage(shape runtimecheck.PythonShape) string {
	message := "Run plugin-kit-ai doctor ., then plugin-kit-ai bootstrap ."
	if fallback := shape.BootstrapFallbackCommand(); strings.TrimSpace(fallback) != "" {
		message += " If needed, fall back to " + fallback + "."
	}
	return message
}

func validateNodeRuntime() error {
	path, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("runtime not found: node runtime required; checked PATH for node. Install Node.js 20+")
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("runtime not found: found node at %s but it is not runnable (%v); install or repair Node.js 20+", path, err)
	}
	if err := requireMinVersion("node", string(out), 20, 0); err != nil {
		return fmt.Errorf("runtime not found: found node at %s but %v; install or repair Node.js 20+", path, err)
	}
	return nil
}

func validateNodeRuntimeTarget(root, entrypoint string, report *Report) {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root: root,
		Launcher: &pluginmanifest.Launcher{
			Runtime:    "node",
			Entrypoint: entrypoint,
		},
	})
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Message: "node runtime inspection failed: " + err.Error(),
		})
		return
	}
	shape := project.Node
	if shape.StructuralIssue != "" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    shape.LauncherTarget,
			Message: "node runtime configuration invalid: " + shape.StructuralIssue,
		})
		return
	}
	if shape.RuntimeTargetOK {
		return
	}
	message := "runtime target missing: " + shape.RuntimeTarget
	if shape.UsesBuiltOutput {
		if shape.IsTypeScript {
			message += " (TypeScript scaffold expects built output; run plugin-kit-ai bootstrap . or " + shape.BuildCommandString() + ")"
		} else {
			message += " (launcher points to built output; run plugin-kit-ai bootstrap . or restore the launcher target)"
		}
	} else {
		message += " (restore the generated scaffold target or update the launcher)"
	}
	report.Failures = append(report.Failures, Failure{
		Kind:    FailureRuntimeTargetMissing,
		Path:    shape.RuntimeTarget,
		Message: message,
	})
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

func requireMinVersion(runtimeName, output string, wantMajor, wantMinor int) error {
	major, minor, err := parseMajorMinor(output)
	if err != nil {
		return fmt.Errorf("reported unsupported version output %q", strings.TrimSpace(output))
	}
	if major > wantMajor || (major == wantMajor && minor >= wantMinor) {
		return nil
	}
	return fmt.Errorf("reported version %d.%d is below the supported minimum %d.%d", major, minor, wantMajor, wantMinor)
}

func parseMajorMinor(output string) (int, int, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("no major.minor version found")
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, err
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, err
	}
	return major, minor, nil
}
