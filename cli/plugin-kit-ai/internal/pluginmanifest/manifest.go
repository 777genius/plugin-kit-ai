package pluginmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
	"github.com/777genius/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const (
	FileName         = pluginmodel.FileName
	LauncherFileName = pluginmodel.LauncherFileName
	FormatMarker     = pluginmodel.FormatMarker
)

var geminiExtensionNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type WarningKind = pluginmodel.WarningKind

const (
	WarningUnknownField  = pluginmodel.WarningUnknownField
	WarningIgnoredImport = pluginmodel.WarningIgnoredImport
	WarningFidelity      = pluginmodel.WarningFidelity
)

type Warning = pluginmodel.Warning
type Manifest = pluginmodel.Manifest
type Launcher = pluginmodel.Launcher
type PortableMCP = pluginmodel.PortableMCP
type PortableComponents = pluginmodel.PortableComponents
type NativeDocFormat = pluginmodel.NativeDocFormat

const (
	NativeDocFormatJSON = pluginmodel.NativeDocFormatJSON
	NativeDocFormatTOML = pluginmodel.NativeDocFormatTOML
)

type NativeExtraDoc = pluginmodel.NativeExtraDoc
type TargetState = pluginmodel.TargetState
type TargetComponents = pluginmodel.TargetState

type GeminiSetting struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	EnvVar      string `yaml:"env_var" json:"envVar"`
	Sensitive   bool   `yaml:"sensitive" json:"sensitive"`
}

type PackageGraph = pluginmodel.PackageGraph
type Artifact = pluginmodel.Artifact

func newPortableComponents() PortableComponents {
	return pluginmodel.NewPortableComponents()
}

func newTargetState(target string) TargetState {
	return pluginmodel.NewTargetState(target)
}

func newTargetComponents(target string) TargetComponents {
	return pluginmodel.NewTargetState(target)
}

type InspectTarget struct {
	Target              string                    `json:"target"`
	PlatformFamily      string                    `json:"platform_family"`
	TargetClass         string                    `json:"target_class"`
	LauncherRequirement string                    `json:"launcher_requirement"`
	TargetNoun          string                    `json:"target_noun,omitempty"`
	ProductionClass     string                    `json:"production_class"`
	RuntimeContract     string                    `json:"runtime_contract"`
	InstallModel        string                    `json:"install_model,omitempty"`
	DevModel            string                    `json:"dev_model,omitempty"`
	ActivationModel     string                    `json:"activation_model,omitempty"`
	NativeRoot          string                    `json:"native_root,omitempty"`
	PortableKinds       []string                  `json:"portable_kinds"`
	TargetNativeKinds   []string                  `json:"target_native_kinds"`
	NativeSurfaces      []targetcontracts.Surface `json:"native_surfaces,omitempty"`
	ManagedArtifacts    []string                  `json:"managed_artifacts"`
	UnsupportedKinds    []string                  `json:"unsupported_kinds,omitempty"`
}

type Inspection struct {
	Manifest    Manifest           `json:"manifest"`
	Portable    PortableComponents `json:"portable"`
	Targets     []InspectTarget    `json:"targets"`
	SourceFiles []string           `json:"source_files"`
}

type RenderResult struct {
	Artifacts  []Artifact
	StalePaths []string
}

type importedClaudeHooksFile struct {
	Hooks map[string][]importedClaudeHookEntry `json:"hooks"`
}

type importedClaudeHookEntry struct {
	Hooks []importedClaudeHookCommand `json:"hooks"`
}

type importedClaudeHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type importedCodexConfig struct {
	Model  string   `toml:"model"`
	Notify []string `toml:"notify"`
}

type importedCodexPluginManifest = codexmanifest.ImportedPluginManifest

type importedCodexNativeConfig struct {
	Model  string
	Notify []string
	Extra  map[string]any
}

type importedCodexTargetMeta struct {
	ModelHint string `yaml:"model_hint,omitempty"`
}

type importedGeminiTargetMeta struct {
	ContextFileName string   `yaml:"context_file_name,omitempty"`
	ExcludeTools    []string `yaml:"exclude_tools,omitempty"`
	MigratedTo      string   `yaml:"migrated_to,omitempty"`
	PlanDirectory   string   `yaml:"plan_directory,omitempty"`
}

type importedGeminiExtension struct {
	Name        string
	Version     string
	Description string
	Meta        importedGeminiTargetMeta
	MCPServers  map[string]any
	Settings    []any
	Themes      []any
	Extra       map[string]any
}

func Load(root string) (Manifest, error) {
	manifest, _, err := LoadWithWarnings(root)
	return manifest, err
}

func LoadLauncher(root string) (Launcher, error) {
	launcher, _, err := LoadLauncherWithWarnings(root)
	return launcher, err
}

func LoadWithWarnings(root string) (Manifest, []Warning, error) {
	body, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		return Manifest{}, nil, err
	}
	return Analyze(body)
}

func LoadLauncherWithWarnings(root string) (Launcher, []Warning, error) {
	body, err := os.ReadFile(filepath.Join(root, LauncherFileName))
	if err != nil {
		return Launcher{}, nil, err
	}
	return AnalyzeLauncher(body)
}

func Parse(body []byte) (Manifest, error) {
	manifest, _, err := Analyze(body)
	return manifest, err
}

func ParseLauncher(body []byte) (Launcher, error) {
	launcher, _, err := AnalyzeLauncher(body)
	return launcher, err
}

func Analyze(body []byte) (Manifest, []Warning, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse plugin.yaml: %w", err)
	}
	if _, ok := raw["schema_version"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: schema_version-based manifests are not supported; use package-standard plugin.yaml with targets")
	}
	if _, ok := raw["components"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: flat components inventory is not supported; use package-standard plugin.yaml plus conventions")
	}
	if rawTargets, ok := raw["targets"]; ok {
		if _, legacy := rawTargets.(map[string]any); legacy {
			return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: legacy targets object is not supported; use targets as a YAML sequence")
		}
	}
	if _, ok := raw["runtime"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: runtime moved to %s", LauncherFileName)
	}
	if _, ok := raw["entrypoint"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: entrypoint moved to %s", LauncherFileName)
	}
	warnings, err := collectWarnings(body)
	if err != nil {
		return Manifest{}, nil, err
	}
	var out Manifest
	if err := yaml.Unmarshal(body, &out); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse plugin.yaml: %w", err)
	}
	normalizeManifest(&out)
	if err := out.Validate(); err != nil {
		return Manifest{}, warnings, err
	}
	return out, warnings, nil
}

func AnalyzeLauncher(body []byte) (Launcher, []Warning, error) {
	var out Launcher
	if err := yaml.Unmarshal(body, &out); err != nil {
		return Launcher{}, nil, fmt.Errorf("parse %s: %w", LauncherFileName, err)
	}
	normalizeLauncher(&out)
	if err := out.Validate(); err != nil {
		return Launcher{}, nil, err
	}
	return out, nil, nil
}

func ValidateGeminiExtensionName(name string) error {
	name = strings.TrimSpace(name)
	if !geminiExtensionNameRe.MatchString(name) {
		return fmt.Errorf("invalid Gemini extension name %q: use lowercase letters, digits, and hyphens only", name)
	}
	return nil
}

func Default(projectName, platform, runtime, description string, _ bool) Manifest {
	platform = normalizeTarget(platform)
	if strings.TrimSpace(description) == "" {
		description = "plugin-kit-ai plugin"
	}
	return Manifest{
		Format:      FormatMarker,
		Name:        projectName,
		Version:     "0.1.0",
		Description: description,
		Targets:     []string{platform},
	}
}

func DefaultLauncher(projectName, runtime string) Launcher {
	runtime = normalizeRuntime(runtime)
	if runtime == "" {
		runtime = "go"
	}
	return Launcher{
		Runtime:    runtime,
		Entrypoint: "./bin/" + projectName,
	}
}

func Save(root string, manifest Manifest, force bool) error {
	normalizeManifest(&manifest)
	if err := manifest.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, FileName)
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", FileName)
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal plugin.yaml: %w", err)
	}
	return os.WriteFile(full, body, 0o644)
}

func SaveLauncher(root string, launcher Launcher, force bool) error {
	normalizeLauncher(&launcher)
	if err := launcher.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, LauncherFileName)
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", LauncherFileName)
	}
	body, err := yaml.Marshal(launcher)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", LauncherFileName, err)
	}
	return os.WriteFile(full, body, 0o644)
}

func Normalize(root string, force bool) ([]Warning, error) {
	manifest, warnings, err := LoadWithWarnings(root)
	if err != nil {
		return nil, err
	}
	if err := Save(root, manifest, force); err != nil {
		return warnings, err
	}
	if launcher, err := LoadLauncher(root); err == nil {
		if err := SaveLauncher(root, launcher, force); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

func Discover(root string) (PackageGraph, []Warning, error) {
	manifest, warnings, err := LoadWithWarnings(root)
	if err != nil {
		return PackageGraph{}, nil, err
	}
	launcher, err := loadLauncherForTargets(root, manifest.EnabledTargets())
	if err != nil {
		return PackageGraph{}, nil, err
	}
	graph := PackageGraph{
		Manifest: manifest,
		Launcher: launcher,
		Portable: newPortableComponents(),
		Targets:  make(map[string]TargetState, len(manifest.Targets)),
	}
	if err := validateRemovedPortableInputs(root, manifest.EnabledTargets()); err != nil {
		return PackageGraph{}, warnings, err
	}
	sourceSet := map[string]struct{}{FileName: {}}
	if launcher != nil {
		sourceSet[LauncherFileName] = struct{}{}
	}

	skillPaths := discoverFiles(root, filepath.Join("skills"), func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	graph.Portable.Add("skills", skillPaths...)
	addSourceFiles(sourceSet, skillPaths)

	if mcpDoc, ok, err := discoverMCP(root); err != nil {
		return PackageGraph{}, warnings, err
	} else if ok {
		graph.Portable.MCP = mcpDoc
		sourceSet[mcpDoc.Path] = struct{}{}
	}

	for _, target := range manifest.EnabledTargets() {
		state, err := discoverTarget(root, target)
		if err != nil {
			return PackageGraph{}, warnings, err
		}
		graph.Targets[target] = state
		addSourceFiles(sourceSet, targetFiles(state))
	}

	graph.SourceFiles = sortedKeys(sourceSet)
	return graph, warnings, nil
}

func validateRemovedPortableInputs(root string, targets []string) error {
	if fileExists(filepath.Join(root, "agents")) && !looksLikeManagedAgentsOutput(root, targets) {
		return fmt.Errorf(rootAgentsMigrationMessage(targets))
	}
	if fileExists(filepath.Join(root, "contexts")) && !looksLikeManagedContextsOutput(root, targets) {
		return fmt.Errorf(rootContextsMigrationMessage(targets))
	}
	return nil
}

func looksLikeManagedAgentsOutput(root string, targets []string) bool {
	targetSet := setOf(targets)
	if !targetSet["claude"] {
		return false
	}
	return len(discoverFiles(root, filepath.Join("targets", "claude", "agents"), nil)) > 0
}

func looksLikeManagedContextsOutput(root string, targets []string) bool {
	targetSet := setOf(targets)
	if targetSet["gemini"] && len(discoverFiles(root, filepath.Join("targets", "gemini", "contexts"), nil)) > 0 {
		return true
	}
	if targetSet["codex-runtime"] && len(discoverFiles(root, filepath.Join("targets", "codex-runtime", "contexts"), nil)) > 0 {
		return true
	}
	return false
}

func rootAgentsMigrationMessage(targets []string) string {
	targetSet := setOf(targets)
	switch {
	case targetSet["claude"]:
		return "portable agents were removed: move repo-root agents/ into targets/claude/agents/; Gemini agents remain preview-only and Codex lanes do not support agents"
	case targetSet["gemini"]:
		return "portable agents were removed: repo-root agents/ is no longer supported; Gemini agents remain preview-only in this wave"
	default:
		return "portable agents were removed: repo-root agents/ is no longer a canonical authored input"
	}
}

func rootContextsMigrationMessage(targets []string) string {
	targetSet := setOf(targets)
	var destinations []string
	if targetSet["gemini"] {
		destinations = append(destinations, "targets/gemini/contexts/")
	}
	if targetSet["codex-runtime"] {
		destinations = append(destinations, "targets/codex-runtime/contexts/")
	}
	if len(destinations) > 0 {
		return fmt.Sprintf("portable contexts were removed: move repo-root contexts/ into %s", strings.Join(destinations, " and/or "))
	}
	return "portable contexts were removed: repo-root contexts/ is no longer supported for these targets"
}

func loadLauncherForTargets(root string, targets []string) (*Launcher, error) {
	requires := false
	for _, target := range targets {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		if profile.Launcher.Requirement == platformmeta.LauncherRequired {
			requires = true
			break
		}
	}
	launcher, err := LoadLauncher(root)
	if err == nil {
		return &launcher, nil
	}
	if os.IsNotExist(err) && !requires {
		return nil, nil
	}
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("required launcher missing: %s", LauncherFileName)
	}
	return nil, err
}

func Inspect(root string, target string) (Inspection, []Warning, error) {
	graph, warnings, err := Discover(root)
	if err != nil {
		return Inspection{}, nil, err
	}
	selected, err := graph.Manifest.SelectedTargets(target)
	if err != nil {
		return Inspection{}, warnings, err
	}
	inspection := Inspection{
		Manifest:    graph.Manifest,
		Portable:    graph.Portable,
		SourceFiles: append([]string(nil), graph.SourceFiles...),
	}
	for _, name := range selected {
		entry, ok := targetcontracts.Lookup(name)
		if !ok {
			continue
		}
		tc := graph.Targets[name]
		inspection.Targets = append(inspection.Targets, InspectTarget{
			Target:              name,
			PlatformFamily:      entry.PlatformFamily,
			TargetClass:         entry.TargetClass,
			LauncherRequirement: entry.LauncherRequirement,
			TargetNoun:          entry.TargetNoun,
			ProductionClass:     entry.ProductionClass,
			RuntimeContract:     entry.RuntimeContract,
			InstallModel:        entry.InstallModel,
			DevModel:            entry.DevModel,
			ActivationModel:     entry.ActivationModel,
			NativeRoot:          entry.NativeRoot,
			PortableKinds:       entry.PortableComponentKinds,
			TargetNativeKinds:   DiscoveredTargetKinds(tc),
			NativeSurfaces:      append([]targetcontracts.Surface(nil), entry.NativeSurfaces...),
			ManagedArtifacts:    expectedManagedPaths(root, graph, []string{name}),
			UnsupportedKinds:    unsupportedKinds(entry, graph, tc),
		})
	}
	return inspection, warnings, nil
}

func Render(root string, target string) (RenderResult, error) {
	graph, _, err := Discover(root)
	if err != nil {
		return RenderResult{}, err
	}
	selected, err := graph.Manifest.SelectedTargets(target)
	if err != nil {
		return RenderResult{}, err
	}
	artifactMap := map[string][]byte{}
	for _, name := range selected {
		rendered, err := renderTargetArtifacts(root, graph, name)
		if err != nil {
			return RenderResult{}, err
		}
		for _, artifact := range rendered {
			relPath := filepath.ToSlash(filepath.Clean(artifact.RelPath))
			if existing, ok := artifactMap[relPath]; ok {
				if !bytes.Equal(existing, artifact.Content) {
					return RenderResult{}, fmt.Errorf("conflicting generated artifact %s across targets", relPath)
				}
				continue
			}
			artifactMap[relPath] = artifact.Content
		}
	}
	artifacts := make([]Artifact, 0, len(artifactMap))
	for path, content := range artifactMap {
		artifacts = append(artifacts, Artifact{RelPath: path, Content: content})
	}
	slices.SortFunc(artifacts, func(a, b Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })

	expected := map[string]struct{}{}
	for _, artifact := range artifacts {
		expected[artifact.RelPath] = struct{}{}
	}
	var stale []string
	for _, path := range expectedManagedPaths(root, graph, selected) {
		if _, ok := expected[path]; ok {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, path)); err == nil {
			stale = append(stale, path)
		}
	}
	slices.Sort(stale)
	return RenderResult{Artifacts: artifacts, StalePaths: stale}, nil
}

func WriteArtifacts(root string, artifacts []Artifact) error {
	for _, artifact := range artifacts {
		full := filepath.Join(root, filepath.FromSlash(artifact.RelPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, artifact.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func RemoveArtifacts(root string, relPaths []string) error {
	for _, relPath := range relPaths {
		full := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func Drift(root string, target string) ([]string, error) {
	result, err := Render(root, target)
	if err != nil {
		return nil, err
	}
	var drift []string
	for _, artifact := range result.Artifacts {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.RelPath)))
		if err != nil {
			drift = append(drift, artifact.RelPath)
			continue
		}
		if !artifactContentEqual(body, artifact.Content) {
			drift = append(drift, artifact.RelPath)
		}
	}
	drift = append(drift, result.StalePaths...)
	slices.Sort(drift)
	return slices.Compact(drift), nil
}

func artifactContentEqual(actual, expected []byte) bool {
	if bytes.Equal(actual, expected) {
		return true
	}
	if !looksLikeText(actual) || !looksLikeText(expected) {
		return false
	}
	return bytes.Equal(normalizeTextNewlines(actual), normalizeTextNewlines(expected))
}

func looksLikeText(body []byte) bool {
	return utf8.Valid(body) && !bytes.Contains(body, []byte{0})
}

func normalizeTextNewlines(body []byte) []byte {
	body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
	body = bytes.ReplaceAll(body, []byte("\r"), []byte("\n"))
	return body
}

func Import(root string, from string, force bool, includeUserScope bool) (Manifest, []Warning, error) {
	if fileExists(filepath.Join(root, ".plugin-kit-ai", "project.toml")) {
		return Manifest{}, nil, fmt.Errorf("unsupported project format for import: .plugin-kit-ai/project.toml is not supported; rewrite the project into the package standard layout")
	}
	explicitFrom := strings.TrimSpace(from) != ""
	from = normalizeTarget(from)
	if from == "" {
		matches := platformexec.DetectImport(root)
		switch {
		case len(matches) == 2 && detectCombinedCodexImport(matches):
			return importCombinedCodex(root, force)
		case len(matches) == 0:
			from = ""
		case len(matches) == 1:
			from = matches[0].ID()
		default:
			var ids []string
			for _, match := range matches {
				ids = append(ids, match.ID())
			}
			return Manifest{}, nil, fmt.Errorf("ambiguous import source: detected multiple native layouts (%s); pass --from explicitly", strings.Join(ids, ", "))
		}
	}
	if explicitFrom && from == "codex" {
		return Manifest{}, nil, fmt.Errorf("unsupported import source %q", from)
	}
	if !isSupportedImportSource(from) {
		return Manifest{}, nil, fmt.Errorf("unsupported import source %q", from)
	}
	if from == "codex-native" {
		return importCombinedCodex(root, force)
	}
	adapter, ok := platformexec.Lookup(from)
	if !ok {
		return Manifest{}, nil, fmt.Errorf("unsupported import source %q", from)
	}
	seed := platformexec.ImportSeed{
		Manifest:         Default(defaultName(root), from, inferRuntime(root), "plugin-kit-ai plugin", false),
		Explicit:         explicitFrom,
		IncludeUserScope: includeUserScope,
	}
	if requiresLauncherForTarget(from) {
		launcher := DefaultLauncher(defaultName(root), inferRuntime(root))
		seed.Launcher = &launcher
	}
	imported, err := adapter.Import(root, seed)
	if err != nil {
		return Manifest{}, nil, err
	}
	artifacts := append([]Artifact{}, imported.Artifacts...)
	if mcpArtifacts, err := importedPortableMCPArtifacts(root); err != nil {
		return Manifest{}, imported.Warnings, err
	} else {
		artifacts = append(artifacts, mcpArtifacts...)
	}
	if fileExists(filepath.Join(root, ".mcp.json")) {
		imported.Warnings = append(imported.Warnings, Warning{
			Kind:    WarningFidelity,
			Path:    ".mcp.json",
			Message: "portable MCP will be preserved under mcp/servers.yaml",
		})
	}
	if err := Save(root, imported.Manifest, force); err != nil {
		return imported.Manifest, imported.Warnings, err
	}
	if imported.Launcher != nil {
		if err := SaveLauncher(root, *imported.Launcher, force); err != nil {
			return imported.Manifest, imported.Warnings, err
		}
	}
	if err := WriteArtifacts(root, artifacts); err != nil {
		return Manifest{}, imported.Warnings, err
	}
	return imported.Manifest, imported.Warnings, nil
}

func detectCombinedCodexImport(matches []platformexec.Adapter) bool {
	seen := map[string]bool{}
	for _, match := range matches {
		seen[match.ID()] = true
	}
	return len(matches) == 2 && seen["codex-package"] && seen["codex-runtime"]
}

func isSupportedImportSource(from string) bool {
	if from == "codex-native" {
		return true
	}
	return slices.Contains(platformmeta.IDs(), from)
}

func requiresLauncherForTarget(target string) bool {
	profile, ok := platformmeta.Lookup(target)
	return ok && profile.Launcher.Requirement == platformmeta.LauncherRequired
}

func importCombinedCodex(root string, force bool) (Manifest, []Warning, error) {
	name := defaultName(root)
	runtime := inferRuntime(root)
	manifest := Default(name, "codex-package", runtime, "plugin-kit-ai plugin", false)
	manifest.Targets = []string{"codex-package", "codex-runtime"}
	launcher := DefaultLauncher(name, runtime)

	packageAdapter, _ := platformexec.Lookup("codex-package")
	packageImported, err := packageAdapter.Import(root, platformexec.ImportSeed{Manifest: manifest})
	if err != nil {
		return Manifest{}, nil, err
	}
	runtimeAdapter, _ := platformexec.Lookup("codex-runtime")
	runtimeImported, err := runtimeAdapter.Import(root, platformexec.ImportSeed{
		Manifest: manifest,
		Launcher: &launcher,
	})
	if err != nil {
		return Manifest{}, nil, err
	}

	importedManifest := packageImported.Manifest
	importedManifest.Targets = []string{"codex-package", "codex-runtime"}
	var importedLauncher *Launcher
	if runtimeImported.Launcher != nil {
		copied := *runtimeImported.Launcher
		importedLauncher = &copied
	}
	warnings := append([]Warning{}, packageImported.Warnings...)
	warnings = append(warnings, runtimeImported.Warnings...)
	artifacts := append([]Artifact{}, packageImported.Artifacts...)
	artifacts = append(artifacts, runtimeImported.Artifacts...)
	if mcpArtifacts, err := importedPortableMCPArtifacts(root); err != nil {
		return Manifest{}, warnings, err
	} else {
		artifacts = append(artifacts, mcpArtifacts...)
	}
	if fileExists(filepath.Join(root, ".mcp.json")) {
		warnings = append(warnings, Warning{
			Kind:    WarningFidelity,
			Path:    ".mcp.json",
			Message: "portable MCP will be preserved under mcp/servers.yaml",
		})
	}
	artifacts = compactArtifacts(artifacts)
	if err := Save(root, importedManifest, force); err != nil {
		return importedManifest, warnings, err
	}
	if importedLauncher != nil {
		if err := SaveLauncher(root, *importedLauncher, force); err != nil {
			return importedManifest, warnings, err
		}
	}
	if err := WriteArtifacts(root, artifacts); err != nil {
		return Manifest{}, warnings, err
	}
	return importedManifest, warnings, nil
}

func renderTargetArtifacts(root string, graph PackageGraph, target string) ([]Artifact, error) {
	tc := graph.Targets[target]
	adapter, ok := platformexec.Lookup(target)
	if !ok {
		return nil, fmt.Errorf("unsupported target %q", target)
	}
	return adapter.Render(root, graph, tc)
}

func renderClaude(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	adapter, _ := platformexec.Lookup("claude")
	return adapter.Render(root, graph, tc)
}

func renderCodex(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	adapter, _ := platformexec.Lookup("codex-runtime")
	return adapter.Render(root, graph, tc)
}

func renderGemini(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	adapter, _ := platformexec.Lookup("gemini")
	return adapter.Render(root, graph, tc)
}

func loadGeminiSettings(root string, rels []string) ([]map[string]any, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	settings := make([]map[string]any, 0, len(rels))
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		var setting GeminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		_, hasSensitive := raw["sensitive"]
		if strings.TrimSpace(setting.Name) == "" || strings.TrimSpace(setting.Description) == "" || strings.TrimSpace(setting.EnvVar) == "" || !hasSensitive {
			return nil, fmt.Errorf("invalid %s: Gemini settings require name, description, env_var, and sensitive", rel)
		}
		settings = append(settings, map[string]any{
			"name":        setting.Name,
			"description": setting.Description,
			"envVar":      setting.EnvVar,
			"sensitive":   setting.Sensitive,
		})
	}
	return settings, nil
}

func loadGeminiThemes(root string, rels []string) ([]map[string]any, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	themes := make([]map[string]any, 0, len(rels))
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if raw == nil {
			raw = map[string]any{}
		}
		name, _ := raw["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid %s: Gemini themes require name", rel)
		}
		theme := map[string]any{}
		for key, value := range raw {
			switch strings.TrimSpace(key) {
			case "":
				continue
			case "name":
				theme["name"] = value
			default:
				theme[key] = value
			}
		}
		themes = append(themes, theme)
	}
	return themes, nil
}

func discoverTarget(root string, target string) (TargetComponents, error) {
	profile, ok := platformmeta.Lookup(target)
	if !ok {
		return TargetComponents{}, fmt.Errorf("unsupported target %q", target)
	}
	state := newTargetComponents(target)
	docKinds := map[string]struct{}{}
	mirrorKinds := map[string]struct{}{}
	for _, spec := range profile.ManagedArtifacts {
		if spec.Kind == platformmeta.ManagedArtifactMirror {
			mirrorKinds[spec.ComponentKind] = struct{}{}
		}
	}
	for _, spec := range profile.NativeDocs {
		docKinds[spec.Kind] = struct{}{}
		path := filepath.ToSlash(spec.Path)
		if fileExists(filepath.Join(root, path)) {
			state.SetDoc(spec.Kind, path)
			if _, ok := mirrorKinds[spec.Kind]; ok {
				state.AddComponent(spec.Kind, path)
			}
		}
	}
	for _, kind := range profile.Contract.TargetComponentKinds {
		if _, isDoc := docKinds[kind]; isDoc {
			continue
		}
		dir := filepath.Join("targets", target, kind)
		state.AddComponent(kind, discoverFiles(root, dir, nil)...)
	}
	adapter, ok := platformexec.Lookup(target)
	if !ok {
		return TargetComponents{}, fmt.Errorf("unsupported target %q", target)
	}
	if err := adapter.RefineDiscovery(root, &state); err != nil {
		return TargetComponents{}, err
	}
	return state, nil
}

func optionalTargetDocPath(root, target, name string) string {
	path := filepath.Join("targets", target, name)
	if fileExists(filepath.Join(root, path)) {
		return filepath.ToSlash(path)
	}
	return ""
}

func discoverGeminiYAMLFiles(root, dir string, kind string) ([]string, error) {
	files := discoverFiles(root, dir, nil)
	for _, rel := range files {
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".yaml", ".yml":
			continue
		default:
			return nil, fmt.Errorf("unsupported Gemini %s file %s: use .yaml or .yml", kind, rel)
		}
	}
	return files, nil
}

func discoverFiles(root, dir string, keep func(rel string) bool) []string {
	full := filepath.Join(root, dir)
	var out []string
	_ = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if keep != nil && !keep(rel) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	slices.Sort(out)
	return out
}

func discoverMCP(root string) (*PortableMCP, bool, error) {
	for _, legacyRel := range []string{"mcp/servers.json", "mcp/servers.yml"} {
		if fileExists(filepath.Join(root, legacyRel)) {
			return nil, false, fmt.Errorf("unsupported portable MCP authored path %s: use mcp/servers.yaml", legacyRel)
		}
	}
	for _, rel := range []string{"mcp/servers.yaml"} {
		full := filepath.Join(root, rel)
		body, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		parsed, err := pluginmodel.ParsePortableMCP(rel, body)
		if err != nil {
			return nil, false, err
		}
		return &PortableMCP{Path: rel, Servers: parsed.Servers, File: parsed.File}, true, nil
	}
	return nil, false, nil
}

func collectWarnings(body []byte) ([]Warning, error) {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse plugin.yaml: %w", err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	var warnings []Warning
	seen := map[string]struct{}{}
	walkNode(doc.Content[0], "", manifestSchema(), seen, &warnings)
	return warnings, nil
}

type schemaSpec struct {
	Fields map[string]schemaSpec
	Seq    *schemaSpec
}

func manifestSchema() schemaSpec {
	return schemaSpec{Fields: map[string]schemaSpec{
		"format":      {},
		"name":        {},
		"version":     {},
		"description": {},
		"targets":     {Seq: &schemaSpec{}},
	}}
}

func walkNode(node *yaml.Node, path string, spec schemaSpec, seen map[string]struct{}, warnings *[]Warning) {
	if node == nil {
		return
	}
	if len(spec.Fields) > 0 && node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			key := strings.TrimSpace(keyNode.Value)
			keyPath := joinPath(path, key)
			child, ok := spec.Fields[key]
			if !ok {
				appendWarning(seen, warnings, Warning{
					Kind:    WarningUnknownField,
					Path:    keyPath,
					Message: "unknown plugin.yaml field: " + keyPath,
				})
				continue
			}
			walkNode(valNode, keyPath, child, seen, warnings)
		}
		return
	}
	if spec.Seq != nil && node.Kind == yaml.SequenceNode {
		for idx, item := range node.Content {
			walkNode(item, fmt.Sprintf("%s[%d]", path, idx), *spec.Seq, seen, warnings)
		}
	}
}

func importManifest(root, from string) (Manifest, Launcher, []Warning, error) {
	warnings := []Warning{}
	manifest := Default(defaultName(root), from, inferRuntime(root), "plugin-kit-ai plugin", false)
	launcher := DefaultLauncher(defaultName(root), inferRuntime(root))
	enrichFromNative(root, &manifest, &launcher, from, &warnings)
	return manifest, launcher, warnings, nil
}

func enrichFromNative(root string, manifest *Manifest, launcher *Launcher, from string, warnings *[]Warning) {
	switch from {
	case "claude":
		loadClaudeMetadata(root, manifest, launcher)
	case "codex":
		loadCodexMetadata(root, manifest, launcher)
	case "gemini":
		loadGeminiMetadata(root, manifest)
	}
	if fileExists(filepath.Join(root, ".mcp.json")) {
		*warnings = append(*warnings, Warning{
			Kind:    WarningFidelity,
			Path:    ".mcp.json",
			Message: "portable MCP will be preserved under mcp/servers.yaml",
		})
	}
	if from == "codex" && fileExists(filepath.Join(root, "agents")) {
		*warnings = append(*warnings, Warning{
			Kind:    WarningIgnoredImport,
			Path:    "agents",
			Message: "ignored unsupported import asset: agents",
		})
	}
}

func loadClaudeMetadata(root string, manifest *Manifest, launcher *Launcher) {
	type meta struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if body, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json")); err == nil {
		var m meta
		if json.Unmarshal(body, &m) == nil {
			if strings.TrimSpace(m.Name) != "" {
				manifest.Name = m.Name
			}
			if strings.TrimSpace(m.Version) != "" {
				manifest.Version = m.Version
			}
			if strings.TrimSpace(m.Description) != "" {
				manifest.Description = m.Description
			}
		}
	}
	if body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json")); err == nil {
		if entrypoint, ok := inferClaudeEntrypoint(body); ok {
			launcher.Entrypoint = entrypoint
		}
	}
}

func loadCodexMetadata(root string, manifest *Manifest, launcher *Launcher) {
	if data, _, err := readImportedCodexPluginManifest(root); err == nil {
		if strings.TrimSpace(data.Name) != "" {
			manifest.Name = data.Name
		}
		if strings.TrimSpace(data.Version) != "" {
			manifest.Version = data.Version
		}
		if strings.TrimSpace(data.Description) != "" {
			manifest.Description = data.Description
		}
	}
	if config, _, err := readImportedCodexConfig(root); err == nil {
		if len(config.Notify) > 0 && strings.TrimSpace(config.Notify[0]) != "" {
			launcher.Entrypoint = strings.TrimSpace(config.Notify[0])
		}
	}
}

func loadGeminiMetadata(root string, manifest *Manifest) {
	if data, ok, err := readImportedGeminiExtension(root); err == nil && ok {
		if strings.TrimSpace(data.Name) != "" {
			manifest.Name = data.Name
		}
		if strings.TrimSpace(data.Version) != "" {
			manifest.Version = data.Version
		}
		if strings.TrimSpace(data.Description) != "" {
			manifest.Description = data.Description
		}
	}
}

func decodeImportedGeminiExtension(body []byte) (importedGeminiExtension, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return importedGeminiExtension{}, err
	}
	out := importedGeminiExtension{}
	if value, ok := raw["name"].(string); ok && strings.TrimSpace(value) != "" {
		out.Name = value
	}
	if value, ok := raw["version"].(string); ok && strings.TrimSpace(value) != "" {
		out.Version = value
	}
	if value, ok := raw["description"].(string); ok && strings.TrimSpace(value) != "" {
		out.Description = value
	}
	if servers, ok := raw["mcpServers"].(map[string]any); ok && len(servers) > 0 {
		out.MCPServers = servers
	}
	if value, ok := raw["contextFileName"].(string); ok && strings.TrimSpace(value) != "" {
		out.Meta.ContextFileName = value
	}
	if values, ok := raw["excludeTools"].([]any); ok {
		out.Meta.ExcludeTools = jsonStringArray(values)
	}
	if value, ok := raw["migratedTo"].(string); ok && strings.TrimSpace(value) != "" {
		out.Meta.MigratedTo = value
	}
	if plan, ok := raw["plan"].(map[string]any); ok {
		if directory, ok := plan["directory"].(string); ok && strings.TrimSpace(directory) != "" {
			out.Meta.PlanDirectory = directory
			delete(plan, "directory")
			if len(plan) == 0 {
				delete(raw, "plan")
			} else {
				raw["plan"] = plan
			}
		}
	}
	if values, ok := raw["settings"].([]any); ok {
		out.Settings = values
	}
	if values, ok := raw["themes"].([]any); ok {
		out.Themes = values
	}
	delete(raw, "name")
	delete(raw, "version")
	delete(raw, "description")
	delete(raw, "mcpServers")
	delete(raw, "contextFileName")
	delete(raw, "excludeTools")
	delete(raw, "migratedTo")
	delete(raw, "settings")
	delete(raw, "themes")
	if plan, ok := raw["plan"].(map[string]any); ok && len(plan) == 0 {
		delete(raw, "plan")
	}
	if len(raw) > 0 {
		out.Extra = raw
	}
	return out, nil
}

func readImportedGeminiExtension(root string) (importedGeminiExtension, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, "gemini-extension.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return importedGeminiExtension{}, false, nil
		}
		return importedGeminiExtension{}, false, err
	}
	data, err := decodeImportedGeminiExtension(body)
	if err != nil {
		return importedGeminiExtension{}, false, err
	}
	return data, true, nil
}

func importedGeminiPrimaryContextName(root string, meta importedGeminiTargetMeta) string {
	if strings.TrimSpace(meta.ContextFileName) != "" {
		return filepath.Base(strings.TrimSpace(meta.ContextFileName))
	}
	if fileExists(filepath.Join(root, "GEMINI.md")) {
		return "GEMINI.md"
	}
	return ""
}

func jsonStringArray(values []any) []string {
	var out []string
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func importCodexModel(root string) string {
	config, _, err := readImportedCodexConfig(root)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(config.Model)
}

func loadImportedCodexConfig(root string) (importedCodexConfig, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		return importedCodexConfig{}, err
	}
	var config importedCodexConfig
	if err := toml.Unmarshal(body, &config); err != nil {
		return importedCodexConfig{}, err
	}
	return config, nil
}

func readImportedCodexConfig(root string) (importedCodexNativeConfig, []byte, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		return importedCodexNativeConfig{}, nil, err
	}
	var raw map[string]any
	if err := toml.Unmarshal(body, &raw); err != nil {
		return importedCodexNativeConfig{}, nil, err
	}
	config := importedCodexNativeConfig{}
	if value, ok := raw["model"].(string); ok {
		config.Model = strings.TrimSpace(value)
	}
	if values, ok := raw["notify"].([]any); ok {
		config.Notify = jsonStringArray(values)
	}
	delete(raw, "model")
	delete(raw, "notify")
	if len(raw) > 0 {
		config.Extra = raw
	}
	return config, body, nil
}

func readImportedCodexPluginManifest(root string) (importedCodexPluginManifest, []byte, error) {
	body, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		return importedCodexPluginManifest{}, nil, err
	}
	out, err := codexmanifest.DecodeImportedPluginManifest(body)
	if err != nil {
		return importedCodexPluginManifest{}, nil, err
	}
	return out, body, nil
}

func inferClaudeEntrypoint(body []byte) (string, bool) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return "", false
	}
	for _, hookName := range claudeHookNames() {
		for _, entry := range hooks.Hooks[hookName] {
			for _, command := range entry.Hooks {
				if command.Type != "command" {
					continue
				}
				entrypoint, ok := trimClaudeHookCommand(command.Command, hookName)
				if ok {
					return entrypoint, true
				}
			}
		}
	}
	return "", false
}

func ValidateClaudeHookEntrypoints(body []byte, entrypoint string) ([]string, error) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for hookName, entries := range hooks.Hooks {
		expected := entrypoint + " " + hookName
		foundCommand := false
		for _, entry := range entries {
			for _, command := range entry.Hooks {
				foundCommand = true
				if command.Type != "command" {
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q uses type %q; expected command %q", hookName, command.Type, expected))
					continue
				}
				if command.Command != expected {
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q uses %q; expected %q from launcher.yaml entrypoint", hookName, command.Command, expected))
				}
			}
		}
		if !foundCommand {
			mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q declares no command hooks; expected %q", hookName, expected))
		}
	}
	return mismatches, nil
}

func parseClaudeHooks(body []byte) (importedClaudeHooksFile, error) {
	var hooks importedClaudeHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		return importedClaudeHooksFile{}, err
	}
	return hooks, nil
}

func trimClaudeHookCommand(command, hookName string) (string, bool) {
	command = strings.TrimSpace(command)
	suffix := " " + strings.TrimSpace(hookName)
	if !strings.HasSuffix(command, suffix) {
		return "", false
	}
	entrypoint := strings.TrimSpace(strings.TrimSuffix(command, suffix))
	if entrypoint == "" {
		return "", false
	}
	return entrypoint, true
}

func inferNativePlatform(root string) string {
	switch {
	case fileExists(filepath.Join(root, ".claude-plugin", "plugin.json")) || fileExists(filepath.Join(root, "hooks", "hooks.json")):
		return "claude"
	case fileExists(filepath.Join(root, ".codex", "config.toml")) || fileExists(filepath.Join(root, ".codex-plugin", "plugin.json")):
		return "codex"
	case fileExists(filepath.Join(root, "gemini-extension.json")):
		return "gemini"
	default:
		return ""
	}
}

func inferRuntime(root string) string {
	switch {
	case fileExists(filepath.Join(root, "go.mod")):
		return "go"
	case fileExists(filepath.Join(root, "src", "main.py")):
		return "python"
	case fileExists(filepath.Join(root, "src", "main.mjs")):
		return "node"
	case fileExists(filepath.Join(root, "scripts", "main.sh")):
		return "shell"
	default:
		return "go"
	}
}

type artifactDir struct {
	src string
	dst string
}

type managedPluginArtifactOptions struct {
	Name          string
	Manifest      Manifest
	Portable      PortableComponents
	IncludeAgents bool
	RelPath       string
	Extra         NativeExtraDoc
	Label         string
	ManagedPaths  []string
}

func renderManagedPluginArtifacts(opts managedPluginArtifactOptions) ([]Artifact, error) {
	doc := map[string]any{
		"name":        opts.Name,
		"version":     opts.Manifest.Version,
		"description": opts.Manifest.Description,
	}
	if len(opts.Portable.Paths("skills")) > 0 {
		doc["skills"] = "./skills/"
	}
	if opts.IncludeAgents && len(opts.Portable.Paths("agents")) > 0 {
		doc["agents"] = "./agents/"
	}
	if opts.Portable.MCP != nil {
		doc["mcpServers"] = "./.mcp.json"
	}
	if err := mergeNativeExtraObject(doc, opts.Extra, opts.Label, opts.ManagedPaths); err != nil {
		return nil, err
	}
	pluginJSON, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts := []Artifact{{RelPath: opts.RelPath, Content: pluginJSON}}
	if opts.Portable.MCP != nil {
		projected, err := opts.Portable.MCP.RenderForTarget("")
		if err != nil {
			return nil, err
		}
		mcpJSON, err := marshalJSON(projected)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, Artifact{RelPath: ".mcp.json", Content: mcpJSON})
	}
	return artifacts, nil
}

func LoadNativeExtraDoc(root, rel string, format NativeDocFormat) (NativeExtraDoc, error) {
	return loadNativeExtraDoc(root, rel, format)
}

func loadNativeExtraDoc(root, rel string, format NativeDocFormat) (NativeExtraDoc, error) {
	if strings.TrimSpace(rel) == "" {
		return NativeExtraDoc{}, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return NativeExtraDoc{}, err
	}
	fields, err := parseNativeExtraDocFields(body, format)
	if err != nil {
		return NativeExtraDoc{}, fmt.Errorf("parse %s: %w", rel, err)
	}
	return NativeExtraDoc{
		Path:   rel,
		Format: format,
		Raw:    body,
		Fields: fields,
	}, nil
}

func parseNativeExtraDocFields(body []byte, format NativeDocFormat) (map[string]any, error) {
	fields := map[string]any{}
	switch format {
	case NativeDocFormatJSON:
		if err := json.Unmarshal(body, &fields); err != nil {
			return nil, err
		}
	case NativeDocFormatTOML:
		if err := toml.Unmarshal(body, &fields); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported native doc format %q", format)
	}
	if fields == nil {
		fields = map[string]any{}
	}
	return fields, nil
}

func ValidateNativeExtraDocConflicts(doc NativeExtraDoc, label string, managedPaths []string) error {
	if len(doc.Fields) == 0 {
		return nil
	}
	if conflict, ok := findManagedPathConflict(doc.Fields, "", setOf(managedPaths)); ok {
		return fmt.Errorf("%s may not override canonical field %q", label, conflict)
	}
	return nil
}

func mergeNativeExtraObject(base map[string]any, doc NativeExtraDoc, label string, managedPaths []string) error {
	if len(doc.Fields) == 0 {
		return nil
	}
	if err := ValidateNativeExtraDocConflicts(doc, label, managedPaths); err != nil {
		return err
	}
	mergeExtraObject(base, doc.Fields)
	return nil
}

func mergeExtraObject(base, extra map[string]any) {
	for key, value := range extra {
		existing, hasExisting := asStringMap(base[key])
		incoming, incomingIsMap := asStringMap(value)
		if hasExisting && incomingIsMap {
			mergeExtraObject(existing, incoming)
			base[key] = existing
			continue
		}
		base[key] = value
	}
}

func findManagedPathConflict(values map[string]any, prefix string, managed map[string]bool) (string, bool) {
	for key, value := range values {
		path := joinPath(prefix, key)
		if _, blocked := managed[path]; blocked {
			return path, true
		}
		if nested, ok := asStringMap(value); ok {
			if conflict, found := findManagedPathConflict(nested, path, managed); found {
				return conflict, true
			}
			continue
		}
		for managedPath := range managed {
			if strings.HasPrefix(managedPath, path+".") {
				return managedPath, true
			}
		}
	}
	return "", false
}

func asStringMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return typed, true
}

func trimmedExtraBody(doc NativeExtraDoc) []byte {
	return bytes.TrimSpace(doc.Raw)
}

func isCanonicalCodexNotify(notify []string) bool {
	return len(notify) == 2 && strings.TrimSpace(notify[0]) != "" && strings.TrimSpace(notify[1]) == "notify"
}

func copyArtifactDirs(root string, dirs ...artifactDir) ([]Artifact, error) {
	var artifacts []Artifact
	for _, dir := range dirs {
		copied, err := copyArtifacts(root, dir.src, dir.dst)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	}
	return artifacts, nil
}

func copyArtifacts(root, srcDir, dstRoot string) ([]Artifact, error) {
	full := filepath.Join(root, srcDir)
	var artifacts []Artifact
	if _, err := os.Stat(full); err != nil {
		return nil, nil
	}
	err := filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, Artifact{
			RelPath: filepath.ToSlash(filepath.Join(dstRoot, rel)),
			Content: body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(artifacts, func(a, b Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return artifacts, nil
}

func copySingleArtifactIfExists(root, srcRel, dstRel string) ([]Artifact, error) {
	body, err := os.ReadFile(filepath.Join(root, srcRel))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return []Artifact{{RelPath: filepath.ToSlash(dstRel), Content: body}}, nil
}

func importedPortableMCPArtifacts(root string) ([]Artifact, error) {
	body, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse portable MCP .mcp.json: %w", err)
	}
	normalized, err := pluginmodel.ImportedPortableMCPYAML("", doc)
	if err != nil {
		return nil, err
	}
	return []Artifact{{RelPath: filepath.Join("mcp", "servers.yaml"), Content: normalized}}, nil
}

func compactArtifacts(artifacts []Artifact) []Artifact {
	slices.SortFunc(artifacts, func(a, b Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		n := len(out)
		if n > 0 && out[n-1].RelPath == artifact.RelPath {
			out[n-1] = artifact
			continue
		}
		out = append(out, artifact)
	}
	return out
}

func expectedManagedPaths(root string, graph PackageGraph, selected []string) []string {
	seen := map[string]struct{}{}
	for _, target := range selected {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		tc := graph.Targets[target]
		for _, spec := range profile.ManagedArtifacts {
			switch spec.Kind {
			case platformmeta.ManagedArtifactStatic:
				seen[spec.Path] = struct{}{}
			case platformmeta.ManagedArtifactPortableMCP:
				if graph.Portable.MCP != nil {
					seen[spec.Path] = struct{}{}
				}
			case platformmeta.ManagedArtifactPortableSkills:
				addManagedCopies(seen, graph.Portable.Paths("skills"), "skills", spec.OutputRoot)
			case platformmeta.ManagedArtifactMirror:
				if spec.OutputRoot == "" {
					rel := filepath.ToSlash(strings.TrimSpace(tc.DocPath(spec.ComponentKind)))
					if rel == "" {
						continue
					}
					relPath, err := filepath.Rel(filepath.ToSlash(spec.SourceRoot), rel)
					if err != nil {
						continue
					}
					seen[filepath.ToSlash(filepath.Join(spec.OutputRoot, relPath))] = struct{}{}
					continue
				}
				addManagedCopies(seen, tc.ComponentPaths(spec.ComponentKind), spec.SourceRoot, spec.OutputRoot)
			case platformmeta.ManagedArtifactSelectedContext:
				continue
			}
		}
		if adapter, ok := platformexec.Lookup(target); ok {
			extraPaths, err := adapter.ManagedPaths(root, graph, tc)
			if err == nil {
				for _, path := range extraPaths {
					seen[path] = struct{}{}
				}
			}
		}
	}
	return sortedKeys(seen)
}

func addManagedCopies(set map[string]struct{}, files []string, srcDir, dstRoot string) {
	for _, rel := range files {
		relPath, err := filepath.Rel(filepath.ToSlash(srcDir), rel)
		if err != nil {
			continue
		}
		set[filepath.ToSlash(filepath.Join(dstRoot, relPath))] = struct{}{}
	}
}

func DiscoveredTargetKinds(tc TargetComponents) []string {
	var kinds []string
	for kind, path := range tc.Docs {
		if strings.TrimSpace(path) != "" {
			kinds = append(kinds, kind)
		}
	}
	for kind, paths := range tc.Components {
		if len(paths) > 0 {
			kinds = append(kinds, kind)
		}
	}
	slices.Sort(kinds)
	return kinds
}

func unsupportedKinds(entry targetcontracts.Entry, graph PackageGraph, tc TargetComponents) []string {
	supportedPortable := setOf(entry.PortableComponentKinds)
	var unsupported []string
	if len(graph.Portable.Paths("skills")) > 0 && !supportedPortable["skills"] {
		unsupported = append(unsupported, "skills")
	}
	if graph.Portable.MCP != nil && !supportedPortable["mcp_servers"] {
		unsupported = append(unsupported, "mcp_servers")
	}
	supportedNative := setOf(entry.TargetComponentKinds)
	for _, kind := range DiscoveredTargetKinds(tc) {
		if !supportedNative[kind] {
			unsupported = append(unsupported, kind)
		}
	}
	slices.Sort(unsupported)
	return slices.Compact(unsupported)
}

func targetFiles(tc TargetComponents) []string {
	var out []string
	for _, path := range tc.Docs {
		if strings.TrimSpace(path) != "" {
			out = append(out, path)
		}
	}
	for _, paths := range tc.Components {
		out = append(out, paths...)
	}
	slices.Sort(out)
	return out
}

func displayName(manifest Manifest, tc TargetComponents) string {
	return manifest.Name
}

func defaultClaudeHooks(entrypoint string) []byte {
	type hookCommand struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type hookEntry struct {
		Hooks []hookCommand `json:"hooks"`
	}
	hooks := map[string][]hookEntry{}
	for _, name := range stableClaudeHookNames() {
		hooks[name] = []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: entrypoint + " " + name}}}}
	}
	body, _ := marshalJSON(map[string]any{"hooks": hooks})
	return body
}

func stableClaudeHookNames() []string {
	return []string{
		"Stop",
		"PreToolUse",
		"UserPromptSubmit",
	}
}

func claudeHookNames() []string {
	return []string{
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
	}
}

func marshalJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func mustJSON(v any) []byte {
	body, _ := marshalJSON(v)
	return body
}

func mustYAML(v any) []byte {
	body, _ := yaml.Marshal(v)
	return body
}

func addSourceFiles(set map[string]struct{}, files []string) {
	for _, rel := range files {
		set[rel] = struct{}{}
	}
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, filepath.ToSlash(key))
	}
	slices.Sort(out)
	return out
}

func normalizeManifest(m *Manifest) {
	pluginmodel.NormalizeManifest(m)
}

func normalizeLauncher(l *Launcher) {
	pluginmodel.NormalizeLauncher(l)
}

func normalizeTarget(target string) string {
	return pluginmodel.NormalizeTarget(target)
}

func normalizeRuntime(runtime string) string {
	return pluginmodel.NormalizeRuntime(runtime)
}

func defaultName(root string) string {
	name := filepath.Base(filepath.Clean(root))
	if err := scaffold.ValidateProjectName(name); err == nil {
		return name
	}
	return "plugin"
}

func requireLauncherEntrypoint(graph PackageGraph) (string, error) {
	if graph.Launcher == nil {
		return "", fmt.Errorf("required launcher missing: %s", LauncherFileName)
	}
	if strings.TrimSpace(graph.Launcher.Entrypoint) == "" {
		return "", fmt.Errorf("invalid %s: entrypoint required", LauncherFileName)
	}
	return graph.Launcher.Entrypoint, nil
}

func appendWarning(seen map[string]struct{}, warnings *[]Warning, warning Warning) {
	key := string(warning.Kind) + ":" + warning.Path
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*warnings = append(*warnings, warning)
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func collisionSafeSlug(name string, used map[string]int) string {
	slug := slugify(name)
	if slug == "" {
		slug = "item"
	}
	if used[slug] == 0 {
		used[slug] = 1
		return slug
	}
	index := used[slug]
	used[slug] = index + 1
	return fmt.Sprintf("%s-%d", slug, index)
}

func geminiExtraContextArtifactPath(rel string) string {
	base := filepath.ToSlash(filepath.Join("targets", "gemini", "contexts"))
	trimmed, err := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(rel))
	if err != nil {
		return filepath.ToSlash(filepath.Join("contexts", filepath.Base(rel)))
	}
	return filepath.ToSlash(filepath.Join("contexts", trimmed))
}

func slugify(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastHyphen := false
	for _, r := range name {
		switch {
		case unicode.IsLower(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastHyphen = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == '.' || r == '/':
			if b.Len() == 0 || lastHyphen {
				continue
			}
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cleanRelativeRef(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}
