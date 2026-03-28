package validate

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/platformmeta"
	"gopkg.in/yaml.v3"
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
		if len(graph.Portable.Paths("agents")) > 0 && !supportedPortable["agents"] {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    "agents",
				Target:  targetName,
				Message: fmt.Sprintf("target %s does not support portable component kind agents", targetName),
			})
		}
		if len(graph.Portable.Paths("contexts")) > 0 && !supportedPortable["contexts"] {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureUnsupportedTargetKind,
				Path:    "contexts",
				Target:  targetName,
				Message: fmt.Sprintf("target %s does not support portable component kind contexts", targetName),
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
		if targetName == "gemini" {
			validateGeminiTarget(root, manifest, graph, tc, &report)
		}
		if targetName == "claude" {
			validateClaudeTarget(root, graph.Launcher, tc, &report)
		}
		if targetName == "codex" {
			validateCodexTarget(root, graph.Launcher, tc, &report)
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
		if err := validatePythonRuntime(root); err != nil {
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

func validateGeminiTarget(root string, manifest pluginmanifest.Manifest, graph pluginmanifest.PackageGraph, tc pluginmanifest.TargetComponents, report *Report) {
	if err := pluginmanifest.ValidateGeminiExtensionName(manifest.Name); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    pluginmanifest.FileName,
			Target:  "gemini",
			Message: err.Error(),
		})
	}
	if base := filepath.Base(filepath.Clean(root)); base != manifest.Name {
		report.Warnings = append(report.Warnings, Warning{
			Kind:    WarningGeminiDirNameMismatch,
			Path:    root,
			Message: fmt.Sprintf("Gemini extension directory basename %q does not match extension name %q", base, manifest.Name),
		})
	}
	validateGeminiMCP(graph, report)
	validateGeminiContext(graph, tc, report)
	validateGeminiSettings(root, tc.ComponentPaths("settings"), report)
	validateGeminiThemes(root, tc.ComponentPaths("themes"), report)
	validateGeminiManifestExtra(root, tc, report)
	validateGeminiPolicies(root, tc.ComponentPaths("policies"), report)
	validateGeminiCommands(root, tc.ComponentPaths("commands"), report)
	validateGeminiJSONFileKinds(root, tc.ComponentPaths("hooks"), report)
}

func validateGeminiMCP(graph pluginmanifest.PackageGraph, report *Report) {
	if graph.Portable.MCP == nil {
		return
	}
	for serverName, raw := range graph.Portable.MCP.Servers {
		server, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, blocked := server["trust"]; blocked {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    graph.Portable.MCP.Path,
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini extension MCP server %q may not set trust", serverName),
			})
		}
		command, _ := server["command"].(string)
		_, hasArgs := server["args"]
		if strings.Contains(strings.TrimSpace(command), " ") && !hasArgs {
			report.Warnings = append(report.Warnings, Warning{
				Kind:    WarningGeminiMCPCommandStyle,
				Path:    graph.Portable.MCP.Path,
				Message: fmt.Sprintf("Gemini extension MCP server %q uses a space-delimited command string; prefer command plus args", serverName),
			})
		}
	}
}

func validateClaudeTarget(root string, launcher *pluginmanifest.Launcher, tc pluginmanifest.TargetComponents, report *Report) {
	if launcher == nil {
		return
	}
	for _, rel := range tc.ComponentPaths("hooks") {
		full := filepath.Join(root, rel)
		body, err := os.ReadFile(full)
		if err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    rel,
				Target:  "claude",
				Message: fmt.Sprintf("Claude hooks file %s is not readable: %v", rel, err),
			})
			continue
		}
		mismatches, err := pluginmanifest.ValidateClaudeHookEntrypoints(body, launcher.Entrypoint)
		if err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    rel,
				Target:  "claude",
				Message: fmt.Sprintf("Claude hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		for _, mismatch := range mismatches {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureEntrypointMismatch,
				Path:    rel,
				Target:  "claude",
				Message: mismatch,
			})
		}
	}
}

func validateCodexTarget(root string, launcher *pluginmanifest.Launcher, tc pluginmanifest.TargetComponents, report *Report) {
	validateTargetExtraDoc(root, "codex", tc.DocPath("manifest_extra"), pluginmanifest.NativeDocFormatJSON, "codex manifest.extra.json", []string{
		"name",
		"version",
		"description",
		"skills",
		"mcpServers",
	}, report)
	validateTargetExtraDoc(root, "codex", tc.DocPath("config_extra"), pluginmanifest.NativeDocFormatTOML, "codex config.extra.toml", []string{
		"model",
		"notify",
	}, report)

	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureGeneratedContractInvalid,
			Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:  "codex",
			Message: fmt.Sprintf("Codex config file %s is not readable: %v", filepath.ToSlash(filepath.Join(".codex", "config.toml")), err),
		})
		return
	}
	var config struct {
		Model  string   `toml:"model"`
		Notify []string `toml:"notify"`
	}
	if err := toml.Unmarshal(body, &config); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:  "codex",
			Message: fmt.Sprintf("Codex config file %s is invalid TOML: %v", filepath.ToSlash(filepath.Join(".codex", "config.toml")), err),
		})
		return
	}
	if launcher == nil {
		return
	}
	expectedNotify := []string{launcher.Entrypoint, "notify"}
	if len(config.Notify) != len(expectedNotify) || len(config.Notify) == 0 || strings.TrimSpace(config.Notify[0]) != expectedNotify[0] || (len(config.Notify) > 1 && strings.TrimSpace(config.Notify[1]) != expectedNotify[1]) {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureEntrypointMismatch,
			Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:  "codex",
			Message: fmt.Sprintf("entrypoint mismatch: Codex notify argv uses %q; expected %q from plugin.yaml entrypoint", config.Notify, expectedNotify),
		})
	}
	if strings.TrimSpace(tc.Codex.ModelHint) != "" && strings.TrimSpace(config.Model) != strings.TrimSpace(tc.Codex.ModelHint) {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureGeneratedContractInvalid,
			Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Target:  "codex",
			Message: fmt.Sprintf("Codex config model %q does not match targets/codex/package.yaml model_hint %q", strings.TrimSpace(config.Model), strings.TrimSpace(tc.Codex.ModelHint)),
		})
	}
}

func validateGeminiContext(graph pluginmanifest.PackageGraph, tc pluginmanifest.TargetComponents, report *Report) {
	selected := strings.TrimSpace(tc.Gemini.ContextFileName)
	candidates := geminiContextMatches(graph, tc, "")
	if selected != "" {
		matches := geminiContextMatches(graph, tc, selected)
		switch len(matches) {
		case 0:
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    tc.DocPath("package_metadata"),
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini context_file_name %q does not resolve to a shared or Gemini-native context source", selected),
			})
		case 1:
			return
		default:
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    tc.DocPath("package_metadata"),
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini context_file_name %q is ambiguous across multiple context sources", selected),
			})
		}
		return
	}
	geminiMD := geminiContextMatches(graph, tc, "GEMINI.md")
	if len(geminiMD) > 1 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    "contexts",
			Target:  "gemini",
			Message: "Gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly",
		})
		return
	}
	if len(geminiMD) == 1 || len(candidates) <= 1 {
		return
	}
	report.Failures = append(report.Failures, Failure{
		Kind:    FailureManifestInvalid,
		Path:    "contexts",
		Target:  "gemini",
		Message: "Gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly",
	})
}

func geminiContextMatches(graph pluginmanifest.PackageGraph, tc pluginmanifest.TargetComponents, name string) []string {
	var matches []string
	seen := map[string]struct{}{}
	for _, rel := range append(append([]string{}, tc.ComponentPaths("contexts")...), graph.Portable.Paths("contexts")...) {
		rel = filepath.ToSlash(rel)
		if name == "" || filepath.Base(rel) == name {
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			matches = append(matches, rel)
		}
	}
	slices.Sort(matches)
	return matches
}

func validateGeminiSettings(root string, rels []string, report *Report) {
	for _, rel := range rels {
		body, raw, ok := readGeminiYAMLMap(root, rel, "setting", report)
		if !ok {
			continue
		}
		var setting pluginmanifest.GeminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			continue
		}
		_, hasSensitive := raw["sensitive"]
		if strings.TrimSpace(setting.Name) == "" || strings.TrimSpace(setting.Description) == "" || strings.TrimSpace(setting.EnvVar) == "" || !hasSensitive {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    rel,
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini setting file %s must define name, description, env_var, and sensitive", rel),
			})
		}
	}
}

func validateGeminiThemes(root string, rels []string, report *Report) {
	for _, rel := range rels {
		_, raw, ok := readGeminiYAMLMap(root, rel, "theme", report)
		if !ok {
			continue
		}
		name, _ := raw["name"].(string)
		if strings.TrimSpace(name) == "" {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    rel,
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini theme file %s must define name", rel),
			})
		}
	}
}

func validateGeminiManifestExtra(root string, tc pluginmanifest.TargetComponents, report *Report) {
	validateTargetExtraDoc(root, "gemini", tc.DocPath("manifest_extra"), pluginmanifest.NativeDocFormatJSON, "gemini manifest.extra.json", []string{
		"name",
		"version",
		"description",
		"mcpServers",
		"contextFileName",
		"excludeTools",
		"migratedTo",
		"settings",
		"themes",
		"plan.directory",
	}, report)
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

func validateGeminiPolicies(root string, rels []string, report *Report) {
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    rel,
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini policy file %s is not readable: %v", rel, err),
			})
			continue
		}
		text := string(body)
		for _, key := range []string{"allow", "yolo"} {
			if strings.Contains(text, key+" =") {
				report.Warnings = append(report.Warnings, Warning{
					Kind:    WarningGeminiPolicyIgnored,
					Path:    rel,
					Message: fmt.Sprintf("Gemini extension policies ignore %q at extension tier", key),
				})
			}
		}
	}
}

func validateGeminiCommands(root string, rels []string, report *Report) {
	for _, rel := range rels {
		if filepath.Ext(rel) != ".toml" {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureManifestInvalid,
				Path:    rel,
				Target:  "gemini",
				Message: fmt.Sprintf("Gemini command file %s must use the .toml extension", rel),
			})
			continue
		}
		validateGeminiEncodedFile(root, rel, "command file", "TOML", func(body []byte) error {
			var discard map[string]any
			return toml.Unmarshal(body, &discard)
		}, report)
	}
}

func validateGeminiJSONFileKinds(root string, rels []string, report *Report) {
	for _, rel := range rels {
		validateGeminiEncodedFile(root, rel, "JSON asset", "JSON", func(body []byte) error {
			var discard any
			return json.Unmarshal(body, &discard)
		}, report)
	}
}

func readGeminiYAMLMap(root, rel, kind string, report *Report) ([]byte, map[string]any, bool) {
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  "gemini",
			Message: fmt.Sprintf("Gemini %s file %s is not readable: %v", kind, rel, err),
		})
		return nil, nil, false
	}
	var raw map[string]any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  "gemini",
			Message: fmt.Sprintf("Gemini %s file %s is invalid YAML: %v", kind, rel, err),
		})
		return nil, nil, false
	}
	return body, raw, true
}

func validateGeminiEncodedFile(root, rel, kind, format string, parse func([]byte) error, report *Report) {
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  "gemini",
			Message: fmt.Sprintf("Gemini %s %s is not readable: %v", kind, rel, err),
		})
		return
	}
	if err := parse(body); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureManifestInvalid,
			Path:    rel,
			Target:  "gemini",
			Message: fmt.Sprintf("Gemini %s %s is invalid %s: %v", kind, rel, format, err),
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

type pythonRuntimeResolution struct {
	Path   string
	Source string
}

func validatePythonRuntime(root string) error {
	resolution, err := findPython(root)
	if err != nil {
		return err
	}
	out, err := exec.Command(resolution.Path, "--version").CombinedOutput()
	if err != nil {
		switch resolution.Source {
		case "project-venv":
			return fmt.Errorf("runtime not found: found project virtualenv interpreter at %s but it is not runnable (%v); recreate .venv or install Python 3.10+", resolution.Path, err)
		default:
			return fmt.Errorf("runtime not found: found %s at %s but it is not runnable (%v); install Python 3.10+ or repair your PATH", resolution.Source, resolution.Path, err)
		}
	}
	if err := requireMinVersion("python", string(out), 3, 10); err != nil {
		switch resolution.Source {
		case "project-venv":
			return fmt.Errorf("runtime not found: found project virtualenv interpreter at %s but %v; recreate .venv with Python 3.10+ or repair the virtualenv", resolution.Path, err)
		default:
			return fmt.Errorf("runtime not found: found %s at %s but %v; install Python 3.10+ or repair your PATH", resolution.Source, resolution.Path, err)
		}
	}
	return nil
}

func findPython(root string) (pythonRuntimeResolution, error) {
	candidates := pythonCandidates(root)
	venvExists := fileExists(filepath.Join(root, ".venv")) || dirExists(filepath.Join(root, ".venv"))
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return pythonRuntimeResolution{Path: candidate, Source: "project-venv"}, nil
		}
	}
	checkedVenv := strings.Join(candidates, ", ")
	checkedPath := strings.Join(pythonPathNames(), ", ")
	if venvExists {
		return pythonRuntimeResolution{}, fmt.Errorf("runtime not found: python runtime required; checked project virtualenv (%s); found .venv but no runnable interpreter. Recreate .venv or install Python 3.10+", checkedVenv)
	}
	for _, name := range pythonPathNames() {
		path, err := exec.LookPath(name)
		if err == nil {
			return pythonRuntimeResolution{Path: path, Source: "system-path"}, nil
		}
	}
	return pythonRuntimeResolution{}, fmt.Errorf("runtime not found: python runtime required; checked PATH runtimes (%s). Install Python 3.10+ or create .venv with python3 -m venv .venv", checkedPath)
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
	rel := detectNodeRuntimeTarget(root, entrypoint)
	full := filepath.Join(root, filepath.FromSlash(rel))
	if _, err := os.Stat(full); err == nil {
		return
	}
	message := "runtime target missing: " + rel
	if strings.HasPrefix(rel, "dist/") || strings.HasPrefix(rel, "build/") {
		message += " (launcher points to built output; run npm install && npm run build, or restore the launcher target)"
	} else {
		message += " (restore the generated scaffold target or update the launcher)"
	}
	report.Failures = append(report.Failures, Failure{
		Kind:    FailureRuntimeTargetMissing,
		Path:    rel,
		Message: message,
	})
}

func detectNodeRuntimeTarget(root, entrypoint string) string {
	body, err := os.ReadFile(launcherPath(root, entrypoint))
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
