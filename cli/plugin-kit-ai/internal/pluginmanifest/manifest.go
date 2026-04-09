package pluginmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexconfig"
	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/geminimanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/publishschema"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
	"github.com/777genius/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

const (
	FileName         = pluginmodel.FileName
	LauncherFileName = pluginmodel.LauncherFileName
	APIVersionV1     = pluginmodel.APIVersionV1
)

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
	NativeDocPaths      map[string]string         `json:"native_doc_paths,omitempty"`
	NativeSurfaces      []targetcontracts.Surface `json:"native_surfaces,omitempty"`
	NativeSurfaceTiers  map[string]string         `json:"native_surface_tiers,omitempty"`
	ManagedArtifacts    []string                  `json:"managed_artifacts"`
	UnsupportedKinds    []string                  `json:"unsupported_kinds,omitempty"`
}

type InspectLayout struct {
	AuthoredRoot      string              `json:"authored_root,omitempty"`
	AuthoredInputs    []string            `json:"authored_inputs"`
	BoundaryDocs      []string            `json:"boundary_docs,omitempty"`
	GeneratedGuide    string              `json:"generated_guide,omitempty"`
	GeneratedOutputs  []string            `json:"generated_outputs"`
	GeneratedByTarget map[string][]string `json:"generated_by_target,omitempty"`
}

type Inspection struct {
	Manifest    Manifest               `json:"manifest"`
	Launcher    *Launcher              `json:"launcher,omitempty"`
	Portable    PortableComponents     `json:"portable"`
	Publication publicationmodel.Model `json:"publication"`
	Targets     []InspectTarget        `json:"targets"`
	Layout      InspectLayout          `json:"layout"`
	SourceFiles []string               `json:"source_files"`
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

type importedCodexPluginManifest = codexmanifest.ImportedPluginManifest

type importedCodexNativeConfig = codexconfig.ImportedConfig

type importedCodexTargetMeta struct {
	ModelHint string `yaml:"model_hint,omitempty"`
}

type importedGeminiTargetMeta = geminimanifest.PackageMeta
type importedGeminiExtension = geminimanifest.ImportedExtension

type authoredLayout struct {
	RootRel string
}

func (l authoredLayout) IsCanonical() bool {
	return filepath.ToSlash(strings.TrimSpace(l.RootRel)) == pluginmodel.SourceDirName
}

func (l authoredLayout) Path(rel string) string {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" {
		return filepath.ToSlash(strings.TrimSpace(l.RootRel))
	}
	if strings.TrimSpace(l.RootRel) == "" {
		return rel
	}
	return filepath.ToSlash(filepath.Join(l.RootRel, rel))
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
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return Manifest{}, nil, err
	}
	body, err := os.ReadFile(filepath.Join(root, layout.Path(FileName)))
	if err != nil {
		return Manifest{}, nil, err
	}
	return Analyze(body)
}

func LoadLauncherWithWarnings(root string) (Launcher, []Warning, error) {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return Launcher{}, nil, err
	}
	body, err := os.ReadFile(filepath.Join(root, layout.Path(LauncherFileName)))
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
		if _, oldShape := rawTargets.(map[string]any); oldShape {
			return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: targets must be a YAML sequence")
		}
	}
	if _, ok := raw["runtime"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: runtime moved to %s", LauncherFileName)
	}
	if _, ok := raw["entrypoint"]; ok {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml format: entrypoint moved to %s", LauncherFileName)
	}
	if rawFormat, hasFormat := raw["format"]; hasFormat && strings.TrimSpace(fmt.Sprint(rawFormat)) != "" {
		return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml field: format")
	}
	if apiVersion, hasAPIVersion := raw["api_version"]; hasAPIVersion {
		if strings.TrimSpace(fmt.Sprint(apiVersion)) != APIVersionV1 {
			return Manifest{}, nil, fmt.Errorf("unsupported plugin.yaml api_version %q: expected %q", strings.TrimSpace(fmt.Sprint(apiVersion)), APIVersionV1)
		}
	}
	if err := validateSchema(body, FileName, manifestSchema(), true); err != nil {
		return Manifest{}, nil, err
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
	if err := validateSchema(body, LauncherFileName, launcherSchema(), false); err != nil {
		return Launcher{}, nil, err
	}
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
	return pluginmodel.ValidateGeminiExtensionName(name)
}

func Default(projectName, platform, runtime, description string, _ bool) Manifest {
	platform = normalizeTarget(platform)
	if strings.TrimSpace(description) == "" {
		description = "plugin-kit-ai plugin"
	}
	return Manifest{
		APIVersion:  APIVersionV1,
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
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return err
	}
	normalizeManifest(&manifest)
	if err := manifest.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(FileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", FileName)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal plugin.yaml: %w", err)
	}
	return os.WriteFile(full, body, 0o644)
}

func SaveLauncher(root string, launcher Launcher, force bool) error {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return err
	}
	normalizeLauncher(&launcher)
	if err := launcher.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(LauncherFileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", LauncherFileName)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
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
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return PackageGraph{}, nil, err
	}
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
	if err := validateRemovedPortableInputs(root, layout, manifest.EnabledTargets()); err != nil {
		return PackageGraph{}, warnings, err
	}
	sourceSet := map[string]struct{}{layout.Path(FileName): {}}
	if launcher != nil {
		sourceSet[layout.Path(LauncherFileName)] = struct{}{}
	}
	if fileExists(filepath.Join(root, layout.Path("README.md"))) {
		sourceSet[layout.Path("README.md")] = struct{}{}
	}
	publication, err := publishschema.DiscoverInLayout(root, layout.Path(""))
	if err != nil {
		return PackageGraph{}, warnings, err
	}
	if err := publication.ValidateTargets(manifest.EnabledTargets()); err != nil {
		return PackageGraph{}, warnings, err
	}
	addSourceFiles(sourceSet, publication.Paths())

	skillPaths := discoverFiles(root, layout.Path(filepath.Join("skills")), func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	graph.Portable.Add("skills", skillPaths...)
	addSourceFiles(sourceSet, skillPaths)

	if mcpDoc, ok, err := discoverMCP(root, layout); err != nil {
		return PackageGraph{}, warnings, err
	} else if ok {
		graph.Portable.MCP = mcpDoc
		sourceSet[mcpDoc.Path] = struct{}{}
	}

	for _, target := range manifest.EnabledTargets() {
		state, err := discoverTarget(root, layout, target)
		if err != nil {
			return PackageGraph{}, warnings, err
		}
		graph.Targets[target] = state
		addSourceFiles(sourceSet, targetFiles(state))
	}

	graph.SourceFiles = sortedKeys(sourceSet)
	return graph, warnings, nil
}

func validateRemovedPortableInputs(root string, layout authoredLayout, targets []string) error {
	if removedPortableInputExists(root, layout, "agents") && !looksLikeManagedAgentsOutput(root, layout, targets) {
		return errors.New(rootAgentsMigrationMessage(targets))
	}
	if removedPortableInputExists(root, layout, "contexts") && !looksLikeManagedContextsOutput(root, layout, targets) {
		return errors.New(rootContextsMigrationMessage(targets))
	}
	return nil
}

func removedPortableInputExists(root string, layout authoredLayout, rel string) bool {
	candidates := []string{filepath.ToSlash(rel)}
	if canonical := layout.Path(rel); canonical != rel {
		candidates = append(candidates, filepath.ToSlash(canonical))
	}
	for _, candidate := range candidates {
		if authoredInputExists(root, candidate) {
			return true
		}
	}
	return false
}

func looksLikeManagedAgentsOutput(root string, layout authoredLayout, targets []string) bool {
	targetSet := setOf(targets)
	if !targetSet["claude"] {
		return false
	}
	return len(discoverFiles(root, layout.Path(filepath.Join("targets", "claude", "agents")), nil)) > 0
}

func looksLikeManagedContextsOutput(root string, layout authoredLayout, targets []string) bool {
	targetSet := setOf(targets)
	if targetSet["gemini"] && len(discoverFiles(root, layout.Path(filepath.Join("targets", "gemini", "contexts")), nil)) > 0 {
		return true
	}
	if targetSet["codex-runtime"] && len(discoverFiles(root, layout.Path(filepath.Join("targets", "codex-runtime", "contexts")), nil)) > 0 {
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
		layout, lerr := detectAuthoredLayout(root)
		if lerr != nil {
			return nil, lerr
		}
		return nil, fmt.Errorf("required launcher missing: %s", layout.Path(LauncherFileName))
	}
	return nil, err
}

func Inspect(root string, target string) (Inspection, []Warning, error) {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return Inspection{}, nil, err
	}
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
		Launcher:    graph.Launcher,
		Portable:    graph.Portable,
		SourceFiles: cloneStringSlice(graph.SourceFiles),
		Layout: InspectLayout{
			AuthoredRoot:      layout.Path(""),
			AuthoredInputs:    cloneStringSlice(graph.SourceFiles),
			BoundaryDocs:      boundaryDocsForLayout(layout),
			GeneratedGuide:    generatedGuideForLayout(layout),
			GeneratedByTarget: map[string][]string{},
		},
		Publication: publicationmodel.Build(graph, mustDiscoverPublication(root), selected),
	}
	if generatedOutputs, err := generatedArtifactInventory(root, layout, graph, selected); err == nil {
		inspection.Layout.GeneratedOutputs = generatedOutputs
	} else {
		return Inspection{}, warnings, err
	}
	for _, name := range selected {
		entry, ok := targetcontracts.Lookup(name)
		if !ok {
			continue
		}
		tc := graph.Targets[name]
		managedArtifacts, err := generatedArtifactInventory(root, layout, graph, []string{name})
		if err != nil {
			return Inspection{}, warnings, err
		}
		inspection.Layout.GeneratedByTarget[name] = cloneStringSlice(managedArtifacts)
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
			PortableKinds:       cloneStringSlice(entry.PortableComponentKinds),
			TargetNativeKinds:   cloneStringSlice(DiscoveredTargetKinds(tc)),
			NativeDocPaths:      discoveredNativeDocPaths(tc),
			NativeSurfaces:      append([]targetcontracts.Surface(nil), entry.NativeSurfaces...),
			NativeSurfaceTiers:  cloneStringMap(entry.NativeSurfaceTiers),
			ManagedArtifacts:    managedArtifacts,
			UnsupportedKinds:    cloneStringSlice(unsupportedKinds(entry, graph, tc)),
		})
	}
	return inspection, warnings, nil
}

func Generate(root string, target string) (RenderResult, error) {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return RenderResult{}, err
	}
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
		generated, err := renderTargetArtifacts(root, graph, name)
		if err != nil {
			return RenderResult{}, err
		}
		for _, artifact := range generated {
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
	publicationArtifacts, err := publicationexec.Generate(graph, mustDiscoverPublication(root), selected)
	if err != nil {
		return RenderResult{}, err
	}
	for _, artifact := range publicationArtifacts {
		relPath := filepath.ToSlash(filepath.Clean(artifact.RelPath))
		if existing, ok := artifactMap[relPath]; ok {
			if !bytes.Equal(existing, artifact.Content) {
				return RenderResult{}, fmt.Errorf("conflicting generated artifact %s across publication channels and targets", relPath)
			}
			continue
		}
		artifactMap[relPath] = artifact.Content
	}
	if layout.IsCanonical() {
		if claudeBoundary, err := buildRootClaudeBoundaryArtifact(layout); err != nil {
			return RenderResult{}, err
		} else if claudeBoundary != nil {
			artifactMap[claudeBoundary.RelPath] = claudeBoundary.Content
		}
		if readme, err := buildRootReadmeArtifact(root, layout, graph.Manifest); err != nil {
			return RenderResult{}, err
		} else if readme != nil {
			artifactMap[readme.RelPath] = readme.Content
		}
		if generatedGuide, err := buildRootGeneratedGuideArtifact(root, layout, graph); err != nil {
			return RenderResult{}, err
		} else if generatedGuide != nil {
			artifactMap[generatedGuide.RelPath] = generatedGuide.Content
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

func discoveredNativeDocPaths(tc TargetComponents) map[string]string {
	if len(tc.Docs) == 0 {
		return nil
	}
	out := make(map[string]string, len(tc.Docs))
	for kind, path := range tc.Docs {
		if strings.TrimSpace(path) == "" {
			continue
		}
		out[kind] = path
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string{}, items...)
}

func Drift(root string, target string) ([]string, error) {
	result, err := Generate(root, target)
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
	if _, err := detectAuthoredLayout(root); err != nil && !os.IsNotExist(err) {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return Manifest{}, nil, err
		}
	}
	if fileExists(filepath.Join(root, ".plugin-kit-ai", "project.toml")) {
		return Manifest{}, nil, fmt.Errorf("unsupported project format for import: .plugin-kit-ai/project.toml is not supported; rewrite the project into the package standard layout")
	}
	explicitFrom := strings.TrimSpace(from) != ""
	from = normalizeTarget(from)
	if from == "" {
		matches := platformexec.DetectImport(root)
		switch {
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
			Message: "portable MCP will be preserved under src/mcp/servers.yaml",
		})
	}
	if err := saveManifestWithLayout(root, authoredLayout{RootRel: pluginmodel.SourceDirName}, imported.Manifest, force); err != nil {
		return imported.Manifest, imported.Warnings, err
	}
	if imported.Launcher != nil {
		if err := saveLauncherWithLayout(root, authoredLayout{RootRel: pluginmodel.SourceDirName}, *imported.Launcher, force); err != nil {
			return imported.Manifest, imported.Warnings, err
		}
	}
	artifacts = prefixAuthoredArtifacts(artifacts, authoredLayout{RootRel: pluginmodel.SourceDirName})
	if err := WriteArtifacts(root, artifacts); err != nil {
		return Manifest{}, imported.Warnings, err
	}
	return imported.Manifest, imported.Warnings, nil
}

func isSupportedImportSource(from string) bool {
	return slices.Contains(platformmeta.IDs(), from)
}

func requiresLauncherForTarget(target string) bool {
	profile, ok := platformmeta.Lookup(target)
	return ok && profile.Launcher.Requirement == platformmeta.LauncherRequired
}

func renderTargetArtifacts(root string, graph PackageGraph, target string) ([]Artifact, error) {
	tc := graph.Targets[target]
	adapter, ok := platformexec.Lookup(target)
	if !ok {
		return nil, fmt.Errorf("unsupported target %q", target)
	}
	return adapter.Generate(root, graph, tc)
}

func discoverTarget(root string, layout authoredLayout, target string) (TargetComponents, error) {
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
		dir := layout.Path(filepath.Join("targets", target, kind))
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

func discoverMCP(root string, layout authoredLayout) (*PortableMCP, bool, error) {
	for _, legacyRel := range []string{"mcp/servers.json", "mcp/servers.yml"} {
		if fileExists(filepath.Join(root, layout.Path(legacyRel))) {
			return nil, false, fmt.Errorf("unsupported portable MCP authored path %s: use src/mcp/servers.yaml", legacyRel)
		}
	}
	for _, rel := range []string{"mcp/servers.yaml"} {
		authoredRel := layout.Path(rel)
		full := filepath.Join(root, authoredRel)
		body, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		parsed, err := pluginmodel.ParsePortableMCP(authoredRel, body)
		if err != nil {
			return nil, false, err
		}
		return &PortableMCP{Path: authoredRel, Servers: parsed.Servers, File: parsed.File}, true, nil
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
	Kind   yaml.Kind
	Scalar scalarKind
	Fields map[string]schemaSpec
	Seq    *schemaSpec
}

type scalarKind int

const (
	scalarAny scalarKind = iota
	scalarString
)

func manifestSchema() schemaSpec {
	return schemaSpec{Kind: yaml.MappingNode, Fields: map[string]schemaSpec{
		"api_version": {Kind: yaml.ScalarNode, Scalar: scalarString},
		"format":      {Kind: yaml.ScalarNode, Scalar: scalarString},
		"name":        {Kind: yaml.ScalarNode, Scalar: scalarString},
		"version":     {Kind: yaml.ScalarNode, Scalar: scalarString},
		"description": {Kind: yaml.ScalarNode, Scalar: scalarString},
		"author": {Kind: yaml.MappingNode, Fields: map[string]schemaSpec{
			"name":  {Kind: yaml.ScalarNode, Scalar: scalarString},
			"email": {Kind: yaml.ScalarNode, Scalar: scalarString},
			"url":   {Kind: yaml.ScalarNode, Scalar: scalarString},
		}},
		"homepage":   {Kind: yaml.ScalarNode, Scalar: scalarString},
		"repository": {Kind: yaml.ScalarNode, Scalar: scalarString},
		"license":    {Kind: yaml.ScalarNode, Scalar: scalarString},
		"keywords":   {Kind: yaml.SequenceNode, Seq: &schemaSpec{Kind: yaml.ScalarNode, Scalar: scalarString}},
		"targets":    {Kind: yaml.SequenceNode, Seq: &schemaSpec{Kind: yaml.ScalarNode, Scalar: scalarString}},
	}}
}

func launcherSchema() schemaSpec {
	return schemaSpec{Kind: yaml.MappingNode, Fields: map[string]schemaSpec{
		"runtime":    {Kind: yaml.ScalarNode, Scalar: scalarString},
		"entrypoint": {Kind: yaml.ScalarNode, Scalar: scalarString},
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

func validateSchema(body []byte, label string, spec schemaSpec, allowUnknown bool) error {
	var doc yaml.Node
	dec := yaml.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&doc); err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	return validateSchemaNode(doc.Content[0], label, spec, allowUnknown)
}

func validateSchemaNode(node *yaml.Node, path string, spec schemaSpec, allowUnknown bool) error {
	if node == nil {
		return nil
	}
	if spec.Kind != 0 && node.Kind != spec.Kind {
		return fmt.Errorf("invalid %s: expected %s", path, describeSchemaKind(spec.Kind))
	}
	switch spec.Kind {
	case yaml.MappingNode:
		seen := map[string]struct{}{}
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			if keyNode.Kind != yaml.ScalarNode || !isStringSchemaScalar(keyNode) {
				return fmt.Errorf("invalid %s: mapping keys must be strings", path)
			}
			key := strings.TrimSpace(keyNode.Value)
			keyPath := joinPath(path, key)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("invalid %s: duplicate field %q", path, key)
			}
			seen[key] = struct{}{}
			child, ok := spec.Fields[key]
			if !ok {
				if allowUnknown {
					continue
				}
				return fmt.Errorf("invalid %s: unknown field %q", path, key)
			}
			if err := validateSchemaNode(valNode, keyPath, child, allowUnknown); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for idx, item := range node.Content {
			if err := validateSchemaNode(item, fmt.Sprintf("%s[%d]", path, idx), *spec.Seq, allowUnknown); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if spec.Scalar == scalarString && !isStringSchemaScalar(node) {
			return fmt.Errorf("invalid %s: expected string", path)
		}
	}
	return nil
}

func isStringSchemaScalar(node *yaml.Node) bool {
	switch node.Tag {
	case "", "!!str", "tag:yaml.org,2002:str", "!!null", "tag:yaml.org,2002:null":
		return true
	default:
		return false
	}
}

func describeSchemaKind(kind yaml.Kind) string {
	switch kind {
	case yaml.MappingNode:
		return "a YAML mapping"
	case yaml.SequenceNode:
		return "a YAML sequence"
	case yaml.ScalarNode:
		return "a YAML scalar"
	default:
		return "a valid YAML value"
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
			Message: "portable MCP will be preserved under src/mcp/servers.yaml",
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

func readImportedGeminiExtension(root string) (importedGeminiExtension, bool, error) {
	return geminimanifest.ReadImportedExtension(root)
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

func readImportedCodexConfig(root string) (importedCodexNativeConfig, []byte, error) {
	return codexconfig.ReadImportedConfig(root)
}

func readImportedCodexPluginManifest(root string) (importedCodexPluginManifest, []byte, error) {
	return codexmanifest.ReadImportedPluginManifest(root)
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
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		layout = authoredLayout{}
	}
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
				sourceRoot := filepath.ToSlash(strings.TrimSpace(spec.SourceRoot))
				if sourceRoot == "" {
					sourceRoot = "skills"
				}
				addManagedCopies(seen, graph.Portable.Paths("skills"), sourceRoot, spec.OutputRoot)
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
	for _, path := range publicationexec.ManagedPaths(mustDiscoverPublication(root), selected) {
		seen[path] = struct{}{}
	}
	if layout.IsCanonical() {
		seen["GENERATED.md"] = struct{}{}
		if fileExists(filepath.Join(root, layout.Path("README.md"))) {
			seen["README.md"] = struct{}{}
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

func mustDiscoverPublication(root string) publishschema.State {
	layout, err := detectAuthoredLayout(root)
	if err != nil {
		return publishschema.State{}
	}
	state, err := publishschema.DiscoverInLayout(root, layout.Path(""))
	if err != nil {
		return publishschema.State{}
	}
	return state
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func authoredInputExists(root, rel string) bool {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return true
	}
	var hasFile bool
	_ = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		hasFile = true
		return io.EOF
	})
	return hasFile
}

func detectAuthoredLayout(root string) (authoredLayout, error) {
	canonical := authoredLayout{RootRel: pluginmodel.SourceDirName}
	if legacyRel := rootLegacyPortableMCPPath(root); legacyRel != "" {
		return authoredLayout{}, fmt.Errorf("unsupported portable MCP authored path %s: use src/mcp/servers.yaml", legacyRel)
	}
	canonicalPresent := authoredLayoutPresent(root, canonical)
	rootPresent := rootAuthoredLayoutPresent(root)
	switch {
	case canonicalPresent && rootPresent:
		return authoredLayout{}, fmt.Errorf("mixed authored layout: keep manual plugin sources only under %s/ and remove root-authored plugin files", pluginmodel.SourceDirName)
	case canonicalPresent:
		return canonical, nil
	case rootPresent:
		return authoredLayout{}, fmt.Errorf("unsupported authored layout: move manual plugin sources into %s/", pluginmodel.SourceDirName)
	default:
		return canonical, nil
	}
}

func rootLegacyPortableMCPPath(root string) string {
	for _, rel := range []string{
		filepath.ToSlash(filepath.Join("mcp", "servers.json")),
		filepath.ToSlash(filepath.Join("mcp", "servers.yml")),
	} {
		if authoredInputExists(root, rel) {
			return rel
		}
	}
	return ""
}

func authoredLayoutPresent(root string, layout authoredLayout) bool {
	for _, rel := range authoredSentinelPaths() {
		if authoredInputExists(root, layout.Path(rel)) {
			return true
		}
	}
	return false
}

func rootAuthoredLayoutPresent(root string) bool {
	for _, rel := range rootAuthoredSentinelPaths() {
		if authoredInputExists(root, rel) {
			return true
		}
	}
	return false
}

func authoredSentinelPaths() []string {
	return []string{
		FileName,
		LauncherFileName,
		filepath.ToSlash(filepath.Join("mcp", "servers.yaml")),
		"skills",
		"targets",
		"publish",
	}
}

func rootAuthoredSentinelPaths() []string {
	return []string{
		FileName,
		LauncherFileName,
		filepath.ToSlash(filepath.Join("mcp", "servers.yaml")),
		filepath.ToSlash(filepath.Join("mcp", "servers.yml")),
		filepath.ToSlash(filepath.Join("mcp", "servers.json")),
		"targets",
		"publish",
	}
}

func saveManifestWithLayout(root string, layout authoredLayout, manifest Manifest, force bool) error {
	normalizeManifest(&manifest)
	if err := manifest.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(FileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", layout.Path(FileName))
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", layout.Path(FileName), err)
	}
	return os.WriteFile(full, body, 0o644)
}

func saveLauncherWithLayout(root string, layout authoredLayout, launcher Launcher, force bool) error {
	normalizeLauncher(&launcher)
	if err := launcher.Validate(); err != nil {
		return err
	}
	full := filepath.Join(root, layout.Path(LauncherFileName))
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", layout.Path(LauncherFileName))
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	body, err := yaml.Marshal(launcher)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", layout.Path(LauncherFileName), err)
	}
	return os.WriteFile(full, body, 0o644)
}

func prefixAuthoredArtifacts(artifacts []Artifact, layout authoredLayout) []Artifact {
	if strings.TrimSpace(layout.RootRel) == "" {
		return artifacts
	}
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		rel := filepath.ToSlash(filepath.Clean(artifact.RelPath))
		prefix := filepath.ToSlash(layout.RootRel)
		if rel != prefix && !strings.HasPrefix(rel, prefix+"/") {
			artifact.RelPath = layout.Path(rel)
		} else {
			artifact.RelPath = rel
		}
		out = append(out, artifact)
	}
	return out
}

func buildRootReadmeArtifact(root string, layout authoredLayout, manifest Manifest) (*Artifact, error) {
	if !layout.IsCanonical() {
		return nil, nil
	}
	authoredReadme := layout.Path("README.md")
	authoredReadmePath := filepath.Join(root, authoredReadme)
	if _, err := os.Stat(authoredReadmePath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	title := strings.TrimSpace(manifest.Name)
	if title == "" {
		title = "plugin"
	}
	var body strings.Builder
	body.WriteString("# ")
	body.WriteString(title)
	body.WriteString("\n\n")
	body.WriteString("This file is generated by `plugin-kit-ai generate`.\n")
	body.WriteString("Do not edit it by hand. Edit files under `src/`, especially [`src/README.md`](./src/README.md).\n\n")
	body.WriteString("Start here:\n\n")
	body.WriteString("- full plugin guide: [`src/README.md`](./src/README.md)\n")
	body.WriteString("- boundary instructions for humans and agents: [`AGENTS.md`](./AGENTS.md)\n")
	body.WriteString("- generated root output inventory: [`GENERATED.md`](./GENERATED.md)\n\n")
	body.WriteString("This plugin root is the native/generated output surface for the supported targets.\n")
	artifact := Artifact{RelPath: "README.md", Content: []byte(body.String())}
	return &artifact, nil
}

func buildRootClaudeBoundaryArtifact(layout authoredLayout) (*Artifact, error) {
	if !layout.IsCanonical() {
		return nil, nil
	}
	body, _, err := scaffold.RenderTemplate("ROOT.CLAUDE.md.tmpl", scaffold.Data{})
	if err != nil {
		return nil, err
	}
	return &Artifact{
		RelPath: "CLAUDE.md",
		Content: body,
	}, nil
}

func stripLeadingMarkdownTitle(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	if len(lines) == 0 {
		return body
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "# ") {
		return strings.TrimLeft(strings.Join(lines[1:], "\n"), "\n")
	}
	return body
}

func buildRootGeneratedGuideArtifact(root string, layout authoredLayout, graph PackageGraph) (*Artifact, error) {
	if !layout.IsCanonical() {
		return nil, nil
	}
	paths, err := generatedArtifactInventory(root, layout, graph, graph.Manifest.EnabledTargets())
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	body.WriteString("# Generated Outputs\n\n")
	body.WriteString("This file is generated by `plugin-kit-ai generate`.\n")
	body.WriteString("Do not edit the paths below by hand. Edit only `src/`, then regenerate.\n\n")
	body.WriteString("This inventory covers the full plugin package across all enabled targets.\n\n")
	body.WriteString("## Boundary Docs\n\n")
	body.WriteString("These committed root docs are guidance files and are not generated outputs:\n\n")
	for _, rel := range boundaryDocsForLayout(layout) {
		body.WriteString("- `")
		body.WriteString(rel)
		body.WriteString("`\n")
	}
	body.WriteString("\n## Managed Generated Outputs\n\n")
	body.WriteString("`README.md` is a generated root entrypoint that points readers to `src/README.md`.\n\n")
	for _, rel := range paths {
		body.WriteString("- `")
		body.WriteString(rel)
		body.WriteString("`\n")
	}
	body.WriteString("\n## Refresh\n\n")
	body.WriteString("```bash\n")
	body.WriteString("plugin-kit-ai normalize .\n")
	body.WriteString("plugin-kit-ai generate .\n")
	body.WriteString("plugin-kit-ai generate --check .\n")
	body.WriteString("```\n")
	return &Artifact{
		RelPath: "GENERATED.md",
		Content: []byte(body.String()),
	}, nil
}

func generatedArtifactInventory(root string, layout authoredLayout, graph PackageGraph, selected []string) ([]string, error) {
	artifactMap := map[string]struct{}{}
	boundarySet := map[string]struct{}{}
	for _, rel := range boundaryDocsForLayout(layout) {
		boundarySet[filepath.ToSlash(filepath.Clean(rel))] = struct{}{}
	}
	for _, target := range selected {
		generated, err := renderTargetArtifacts(root, graph, target)
		if err != nil {
			return nil, err
		}
		for _, artifact := range generated {
			rel := filepath.ToSlash(filepath.Clean(artifact.RelPath))
			if _, skip := boundarySet[rel]; skip {
				continue
			}
			artifactMap[rel] = struct{}{}
		}
	}
	publicationArtifacts, err := publicationexec.Generate(graph, mustDiscoverPublication(root), selected)
	if err != nil {
		return nil, err
	}
	for _, artifact := range publicationArtifacts {
		artifactMap[filepath.ToSlash(filepath.Clean(artifact.RelPath))] = struct{}{}
	}
	if readme, err := buildRootReadmeArtifact(root, layout, graph.Manifest); err != nil {
		return nil, err
	} else if readme != nil {
		artifactMap[readme.RelPath] = struct{}{}
	}
	artifactMap["GENERATED.md"] = struct{}{}
	return sortedKeys(artifactMap), nil
}

func boundaryDocsForLayout(layout authoredLayout) []string {
	if !layout.IsCanonical() {
		return nil
	}
	return []string{"CLAUDE.md", "AGENTS.md"}
}

func generatedGuideForLayout(layout authoredLayout) string {
	if !layout.IsCanonical() {
		return ""
	}
	return "GENERATED.md"
}

func cleanRelativeRef(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "." {
		return ""
	}
	return path
}
