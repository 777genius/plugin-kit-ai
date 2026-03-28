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

	"github.com/pelletier/go-toml/v2"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/scaffold"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/targetcontracts"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/platformmeta"
	"gopkg.in/yaml.v3"
)

const (
	FileName         = "plugin.yaml"
	LauncherFileName = "launcher.yaml"
	FormatMarker     = "plugin-kit-ai/package"
)

var geminiExtensionNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type WarningKind string

const (
	WarningUnknownField  WarningKind = "unknown_field"
	WarningIgnoredImport WarningKind = "ignored_import"
	WarningFidelity      WarningKind = "fidelity"
)

type Warning struct {
	Kind    WarningKind
	Path    string
	Message string
}

type Manifest struct {
	Format      string   `yaml:"format" json:"format"`
	Name        string   `yaml:"name" json:"name"`
	Version     string   `yaml:"version" json:"version"`
	Description string   `yaml:"description" json:"description"`
	Targets     []string `yaml:"targets" json:"targets"`
}

type Launcher struct {
	Runtime    string `yaml:"runtime" json:"runtime"`
	Entrypoint string `yaml:"entrypoint" json:"entrypoint"`
}

type PortableMCP struct {
	Path    string
	Servers map[string]any
}

type PortableComponents struct {
	Items map[string][]string
	MCP   *PortableMCP
}

type NativeDocFormat string

const (
	NativeDocFormatJSON NativeDocFormat = "json"
	NativeDocFormatTOML NativeDocFormat = "toml"
)

type NativeExtraDoc struct {
	Path   string
	Format NativeDocFormat
	Raw    []byte
	Fields map[string]any
}

type CodexTargetMeta struct {
	ModelHint string `yaml:"model_hint,omitempty"`
}

type GeminiTargetMeta struct {
	ContextFileName string   `yaml:"context_file_name,omitempty"`
	ExcludeTools    []string `yaml:"exclude_tools,omitempty"`
	MigratedTo      string   `yaml:"migrated_to,omitempty"`
	PlanDirectory   string   `yaml:"plan_directory,omitempty"`
}

type TargetComponents struct {
	Target     string
	Docs       map[string]string
	Components map[string][]string
	Codex      CodexTargetMeta
	Gemini     GeminiTargetMeta
}

type GeminiSetting struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	EnvVar      string `yaml:"env_var" json:"envVar"`
	Sensitive   bool   `yaml:"sensitive" json:"sensitive"`
}

type PackageGraph struct {
	Manifest    Manifest
	Launcher    *Launcher
	Portable    PortableComponents
	Targets     map[string]TargetComponents
	SourceFiles []string
}

func newPortableComponents() PortableComponents {
	return PortableComponents{Items: map[string][]string{}}
}

func (p *PortableComponents) Add(kind string, paths ...string) {
	if p.Items == nil {
		p.Items = map[string][]string{}
	}
	p.Items[kind] = append(p.Items[kind], paths...)
}

func (p PortableComponents) Paths(kind string) []string {
	return append([]string(nil), p.Items[kind]...)
}

func (p PortableComponents) Kinds() []string {
	out := make([]string, 0, len(p.Items))
	for kind, paths := range p.Items {
		if len(paths) == 0 {
			continue
		}
		out = append(out, kind)
	}
	slices.Sort(out)
	return out
}

func newTargetComponents(target string) TargetComponents {
	return TargetComponents{
		Target:     target,
		Docs:       map[string]string{},
		Components: map[string][]string{},
	}
}

func (tc *TargetComponents) SetDoc(kind, path string) {
	if tc.Docs == nil {
		tc.Docs = map[string]string{}
	}
	tc.Docs[kind] = filepath.ToSlash(path)
}

func (tc TargetComponents) DocPath(kind string) string {
	return strings.TrimSpace(tc.Docs[kind])
}

func (tc *TargetComponents) AddComponent(kind string, paths ...string) {
	if tc.Components == nil {
		tc.Components = map[string][]string{}
	}
	tc.Components[kind] = append(tc.Components[kind], paths...)
}

func (tc TargetComponents) ComponentPaths(kind string) []string {
	return append([]string(nil), tc.Components[kind]...)
}

type InspectTarget struct {
	Target            string   `json:"target"`
	TargetClass       string   `json:"target_class"`
	TargetNoun        string   `json:"target_noun,omitempty"`
	ProductionClass   string   `json:"production_class"`
	RuntimeContract   string   `json:"runtime_contract"`
	InstallModel      string   `json:"install_model,omitempty"`
	DevModel          string   `json:"dev_model,omitempty"`
	ActivationModel   string   `json:"activation_model,omitempty"`
	NativeRoot        string   `json:"native_root,omitempty"`
	PortableKinds     []string `json:"portable_kinds"`
	TargetNativeKinds []string `json:"target_native_kinds"`
	ManagedArtifacts  []string `json:"managed_artifacts"`
	UnsupportedKinds  []string `json:"unsupported_kinds,omitempty"`
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

type Artifact struct {
	RelPath string
	Content []byte
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

type importedCodexPluginManifest struct {
	Name          string
	Version       string
	Description   string
	SkillsPath    string
	MCPServersRef string
	Extra         map[string]any
}

type importedCodexNativeConfig struct {
	Model  string
	Notify []string
	Extra  map[string]any
}

type importedGeminiExtension struct {
	Name        string
	Version     string
	Description string
	Meta        GeminiTargetMeta
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

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.Format) != FormatMarker {
		return fmt.Errorf("invalid plugin.yaml: format must be %q", FormatMarker)
	}
	if err := scaffold.ValidateProjectName(m.Name); err != nil {
		return fmt.Errorf("invalid plugin.yaml: %w", err)
	}
	if strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("invalid plugin.yaml: version required")
	}
	if strings.TrimSpace(m.Description) == "" {
		return fmt.Errorf("invalid plugin.yaml: description required")
	}
	if len(m.Targets) == 0 {
		return fmt.Errorf("invalid plugin.yaml: targets must not be empty")
	}
	seen := map[string]struct{}{}
	supportedTargets := platformmeta.IDs()
	for _, target := range m.Targets {
		target = normalizeTarget(target)
		if !slices.Contains(supportedTargets, target) {
			return fmt.Errorf("invalid plugin.yaml: unsupported target %q", target)
		}
		if _, ok := seen[target]; ok {
			return fmt.Errorf("invalid plugin.yaml: duplicate target %q", target)
		}
		seen[target] = struct{}{}
	}
	if _, ok := seen["gemini"]; ok {
		if err := ValidateGeminiExtensionName(m.Name); err != nil {
			return fmt.Errorf("invalid plugin.yaml: %w", err)
		}
	}
	return nil
}

func (l Launcher) Validate() error {
	if _, ok := scaffold.LookupRuntime(l.Runtime); !ok {
		return fmt.Errorf("invalid %s: unsupported runtime %q", LauncherFileName, l.Runtime)
	}
	if strings.TrimSpace(l.Entrypoint) == "" {
		return fmt.Errorf("invalid %s: entrypoint required", LauncherFileName)
	}
	return nil
}

func ValidateGeminiExtensionName(name string) error {
	name = strings.TrimSpace(name)
	if !geminiExtensionNameRe.MatchString(name) {
		return fmt.Errorf("invalid Gemini extension name %q: use lowercase letters, digits, and hyphens only", name)
	}
	return nil
}

func (m Manifest) EnabledTargets() []string {
	out := make([]string, 0, len(m.Targets))
	for _, target := range m.Targets {
		out = append(out, normalizeTarget(target))
	}
	return out
}

func (m Manifest) SelectedTargets(target string) ([]string, error) {
	target = normalizeTarget(target)
	if target == "" || target == "all" {
		return m.EnabledTargets(), nil
	}
	for _, enabled := range m.EnabledTargets() {
		if enabled == target {
			return []string{target}, nil
		}
	}
	return nil, fmt.Errorf("target %q is not enabled in plugin.yaml", target)
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
		Targets:  make(map[string]TargetComponents, len(manifest.Targets)),
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

	agentPaths := discoverFiles(root, filepath.Join("agents"), func(rel string) bool {
		return strings.HasSuffix(rel, ".md")
	})
	graph.Portable.Add("agents", agentPaths...)
	addSourceFiles(sourceSet, agentPaths)

	contextPaths := discoverFiles(root, filepath.Join("contexts"), nil)
	graph.Portable.Add("contexts", contextPaths...)
	addSourceFiles(sourceSet, contextPaths)

	if mcpDoc, ok, err := discoverMCP(root); err != nil {
		return PackageGraph{}, warnings, err
	} else if ok {
		graph.Portable.MCP = mcpDoc
		sourceSet[mcpDoc.Path] = struct{}{}
	}

	for _, target := range manifest.EnabledTargets() {
		tc, err := discoverTarget(root, target)
		if err != nil {
			return PackageGraph{}, warnings, err
		}
		graph.Targets[target] = tc
		addSourceFiles(sourceSet, targetFiles(tc))
	}

	graph.SourceFiles = sortedKeys(sourceSet)
	return graph, warnings, nil
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
			Target:            name,
			TargetClass:       entry.TargetClass,
			TargetNoun:        entry.TargetNoun,
			ProductionClass:   entry.ProductionClass,
			RuntimeContract:   entry.RuntimeContract,
			InstallModel:      entry.InstallModel,
			DevModel:          entry.DevModel,
			ActivationModel:   entry.ActivationModel,
			NativeRoot:        entry.NativeRoot,
			PortableKinds:     entry.PortableComponentKinds,
			TargetNativeKinds: DiscoveredTargetKinds(tc),
			ManagedArtifacts:  expectedManagedPaths(graph, []string{name}),
			UnsupportedKinds:  unsupportedKinds(entry, graph, tc),
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
			if existing, ok := artifactMap[artifact.RelPath]; ok {
				if !bytes.Equal(existing, artifact.Content) {
					return RenderResult{}, fmt.Errorf("conflicting generated artifact %s across targets", artifact.RelPath)
				}
				continue
			}
			artifactMap[artifact.RelPath] = artifact.Content
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
	for _, path := range expectedManagedPaths(graph, selected) {
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
		full := filepath.Join(root, artifact.RelPath)
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
		full := filepath.Join(root, relPath)
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
		body, err := os.ReadFile(filepath.Join(root, artifact.RelPath))
		if err != nil {
			drift = append(drift, artifact.RelPath)
			continue
		}
		if !bytes.Equal(body, artifact.Content) {
			drift = append(drift, artifact.RelPath)
		}
	}
	drift = append(drift, result.StalePaths...)
	slices.Sort(drift)
	return slices.Compact(drift), nil
}

func Import(root string, from string, force bool) (Manifest, []Warning, error) {
	if fileExists(filepath.Join(root, ".plugin-kit-ai", "project.toml")) {
		return Manifest{}, nil, fmt.Errorf("unsupported project format for import: .plugin-kit-ai/project.toml is not supported; rewrite the project into the package standard layout")
	}
	from = normalizeTarget(from)
	if from == "" {
		from = inferNativePlatform(root)
	}
	if !slices.Contains(platformmeta.IDs(), from) {
		return Manifest{}, nil, fmt.Errorf("unsupported import source %q", from)
	}
	manifest, launcher, warnings, err := importManifest(root, from)
	if err != nil {
		return Manifest{}, nil, err
	}
	artifacts, importWarnings, err := importedLayoutArtifacts(root, from)
	if err != nil {
		return Manifest{}, warnings, err
	}
	warnings = append(warnings, importWarnings...)
	if err := Save(root, manifest, force); err != nil {
		return manifest, warnings, err
	}
	if err := SaveLauncher(root, launcher, force); err != nil {
		return manifest, warnings, err
	}
	if err := WriteArtifacts(root, artifacts); err != nil {
		return Manifest{}, warnings, err
	}
	return manifest, warnings, nil
}

func renderTargetArtifacts(root string, graph PackageGraph, target string) ([]Artifact, error) {
	tc := graph.Targets[target]
	switch target {
	case "claude":
		return renderClaude(root, graph, tc)
	case "codex":
		return renderCodex(root, graph, tc)
	case "gemini":
		return renderGemini(root, graph, tc)
	default:
		return nil, fmt.Errorf("unsupported target %q", target)
	}
}

func renderClaude(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	entrypoint, err := requireLauncherEntrypoint(graph)
	if err != nil {
		return nil, err
	}
	artifacts, err := renderManagedPluginArtifacts(managedPluginArtifactOptions{
		Name:          displayName(graph.Manifest, tc),
		Manifest:      graph.Manifest,
		Portable:      graph.Portable,
		IncludeAgents: true,
		RelPath:       filepath.Join(".claude-plugin", "plugin.json"),
	})
	if err != nil {
		return nil, err
	}
	if hookPaths := tc.ComponentPaths("hooks"); len(hookPaths) > 0 {
		copied, err := copyArtifacts(root, filepath.Join("targets", "claude", "hooks"), "hooks")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	} else {
		artifacts = append(artifacts, Artifact{
			RelPath: filepath.Join("hooks", "hooks.json"),
			Content: defaultClaudeHooks(entrypoint),
		})
	}
	copiedKinds := []artifactDir{
		{src: filepath.Join("targets", "claude", "commands"), dst: "commands"},
		{src: filepath.Join("targets", "claude", "contexts"), dst: "contexts"},
	}
	copied, err := copyArtifactDirs(root, copiedKinds...)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, copied...)
	return artifacts, nil
}

func renderCodex(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	entrypoint, err := requireLauncherEntrypoint(graph)
	if err != nil {
		return nil, err
	}
	manifestExtra, err := loadNativeExtraDoc(root, tc.DocPath("manifest_extra"), NativeDocFormatJSON)
	if err != nil {
		return nil, err
	}
	artifacts, err := renderManagedPluginArtifacts(managedPluginArtifactOptions{
		Name:          graph.Manifest.Name,
		Manifest:      graph.Manifest,
		Portable:      graph.Portable,
		IncludeAgents: false,
		RelPath:       filepath.Join(".codex-plugin", "plugin.json"),
		Extra:         manifestExtra,
		Label:         "codex manifest.extra.json",
		ManagedPaths:  []string{"name", "version", "description", "skills", "mcpServers"},
	})
	if err != nil {
		return nil, err
	}
	model := tc.Codex.ModelHint
	if strings.TrimSpace(model) == "" {
		model = scaffold.DefaultCodexModel
	}
	configExtra, err := loadNativeExtraDoc(root, tc.DocPath("config_extra"), NativeDocFormatTOML)
	if err != nil {
		return nil, err
	}
	if err := ValidateNativeExtraDocConflicts(configExtra, "codex config.extra.toml", []string{"model", "notify"}); err != nil {
		return nil, err
	}
	var config bytes.Buffer
	config.WriteString("# Generated by plugin-kit-ai. DO NOT EDIT.\n")
	config.WriteString(fmt.Sprintf("model = %q\n", model))
	config.WriteString(fmt.Sprintf("notify = [%q, %q]\n", entrypoint, "notify"))
	if extraBody := trimmedExtraBody(configExtra); len(extraBody) > 0 {
		config.WriteByte('\n')
		config.Write(extraBody)
		config.WriteByte('\n')
	}
	artifacts = append(artifacts, Artifact{RelPath: filepath.Join(".codex", "config.toml"), Content: config.Bytes()})
	copiedKinds := []artifactDir{
		{src: filepath.Join("targets", "codex", "commands"), dst: "commands"},
		{src: filepath.Join("targets", "codex", "contexts"), dst: "contexts"},
	}
	copied, err := copyArtifactDirs(root, copiedKinds...)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, copied...)
	return artifacts, nil
}

func renderGemini(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	manifest := map[string]any{
		"name":        graph.Manifest.Name,
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if graph.Portable.MCP != nil {
		manifest["mcpServers"] = graph.Portable.MCP.Servers
	}
	artifacts := []Artifact{}
	if len(tc.Gemini.ExcludeTools) > 0 {
		manifest["excludeTools"] = append([]string(nil), tc.Gemini.ExcludeTools...)
	}
	if strings.TrimSpace(tc.Gemini.MigratedTo) != "" {
		manifest["migratedTo"] = tc.Gemini.MigratedTo
	}
	if strings.TrimSpace(tc.Gemini.PlanDirectory) != "" {
		manifest["plan"] = map[string]any{"directory": tc.Gemini.PlanDirectory}
	}
	settings, err := loadGeminiSettings(root, tc.ComponentPaths("settings"))
	if err != nil {
		return nil, err
	}
	if len(settings) > 0 {
		manifest["settings"] = settings
	}
	themes, err := loadGeminiThemes(root, tc.ComponentPaths("themes"))
	if err != nil {
		return nil, err
	}
	if len(themes) > 0 {
		manifest["themes"] = themes
	}
	if contextName, contextArtifact, extraContexts, ok, err := geminiContextArtifacts(root, graph, tc); err != nil {
		return nil, err
	} else if ok {
		manifest["contextFileName"] = contextName
		artifacts = append(artifacts, contextArtifact)
		artifacts = append(artifacts, extraContexts...)
	}
	if extra, err := loadNativeExtraDoc(root, tc.DocPath("manifest_extra"), NativeDocFormatJSON); err != nil {
		return nil, err
	} else if err := mergeNativeExtraObject(manifest, extra, "gemini manifest.extra.json", []string{
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
	}); err != nil {
		return nil, err
	}

	manifestJSON, err := marshalJSON(manifest)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{RelPath: "gemini-extension.json", Content: manifestJSON})

	copiedKinds := []artifactDir{
		{src: filepath.Join("targets", "gemini", "hooks"), dst: "hooks"},
		{src: filepath.Join("targets", "gemini", "commands"), dst: "commands"},
		{src: filepath.Join("targets", "gemini", "policies"), dst: "policies"},
	}
	copied, err := copyArtifactDirs(root, copiedKinds...)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, copied...)
	return artifacts, nil
}

type geminiContextSelection struct {
	ArtifactName string
	SourcePath   string
}

func geminiContextArtifacts(root string, graph PackageGraph, tc TargetComponents) (string, Artifact, []Artifact, bool, error) {
	selected, ok, err := selectGeminiPrimaryContext(graph, tc)
	if err != nil {
		return "", Artifact{}, nil, false, err
	}
	if !ok {
		return "", Artifact{}, nil, false, nil
	}
	body, err := os.ReadFile(filepath.Join(root, selected.SourcePath))
	if err != nil {
		return "", Artifact{}, nil, false, err
	}
	primary := Artifact{RelPath: selected.ArtifactName, Content: body}
	var extra []Artifact
	for _, rel := range tc.ComponentPaths("contexts") {
		if rel == selected.SourcePath {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", Artifact{}, nil, false, err
		}
		extra = append(extra, Artifact{
			RelPath: geminiExtraContextArtifactPath(rel),
			Content: body,
		})
	}
	slices.SortFunc(extra, func(a, b Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return selected.ArtifactName, primary, extra, true, nil
}

func selectGeminiPrimaryContext(graph PackageGraph, tc TargetComponents) (geminiContextSelection, bool, error) {
	candidates := geminiContextCandidates(graph, tc)
	selected := strings.TrimSpace(tc.Gemini.ContextFileName)
	if selected != "" {
		matches := candidatesByArtifactName(candidates, selected)
		switch len(matches) {
		case 0:
			return geminiContextSelection{}, false, fmt.Errorf("gemini context_file_name %q does not resolve to a shared or Gemini-native context source", selected)
		case 1:
			return matches[0], true, nil
		default:
			return geminiContextSelection{}, false, fmt.Errorf("gemini context_file_name %q is ambiguous across multiple context sources", selected)
		}
	}
	fallback := candidatesByArtifactName(candidates, "GEMINI.md")
	switch len(fallback) {
	case 1:
		return fallback[0], true, nil
	case 0:
		if len(candidates) == 0 {
			return geminiContextSelection{}, false, nil
		}
		if len(candidates) == 1 {
			return candidates[0], true, nil
		}
		return geminiContextSelection{}, false, fmt.Errorf("gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly")
	default:
		return geminiContextSelection{}, false, fmt.Errorf("gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly")
	}
}

func geminiContextCandidates(graph PackageGraph, tc TargetComponents) []geminiContextSelection {
	var out []geminiContextSelection
	seen := map[string]struct{}{}
	for _, rel := range append(append([]string{}, tc.ComponentPaths("contexts")...), graph.Portable.Paths("contexts")...) {
		key := filepath.ToSlash(rel)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, geminiContextSelection{
			ArtifactName: filepath.Base(rel),
			SourcePath:   key,
		})
	}
	slices.SortFunc(out, func(a, b geminiContextSelection) int {
		if cmp := strings.Compare(a.ArtifactName, b.ArtifactName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.SourcePath, b.SourcePath)
	})
	return out
}

func candidatesByArtifactName(candidates []geminiContextSelection, name string) []geminiContextSelection {
	var out []geminiContextSelection
	for _, candidate := range candidates {
		if candidate.ArtifactName == name {
			out = append(out, candidate)
		}
	}
	return out
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
	tc := newTargetComponents(target)
	packagePath := filepath.Join("targets", target, "package.yaml")
	if body, err := os.ReadFile(filepath.Join(root, packagePath)); err == nil {
		tc.SetDoc("package_metadata", packagePath)
		switch target {
		case "codex":
			if err := yaml.Unmarshal(body, &tc.Codex); err != nil {
				return TargetComponents{}, fmt.Errorf("parse %s: %w", packagePath, err)
			}
		case "gemini":
			if err := yaml.Unmarshal(body, &tc.Gemini); err != nil {
				return TargetComponents{}, fmt.Errorf("parse %s: %w", packagePath, err)
			}
		default:
			var discard map[string]any
			if err := yaml.Unmarshal(body, &discard); err != nil {
				return TargetComponents{}, fmt.Errorf("parse %s: %w", packagePath, err)
			}
		}
	}
	if target == "gemini" {
		hookFiles := discoverFiles(root, filepath.Join("targets", target, "hooks"), nil)
		for _, rel := range hookFiles {
			if rel != filepath.ToSlash(filepath.Join("targets", "gemini", "hooks", "hooks.json")) {
				return TargetComponents{}, fmt.Errorf("unsupported Gemini hooks layout: use only targets/gemini/hooks/hooks.json")
			}
		}
		tc.AddComponent("hooks", hookFiles...)
	} else {
		tc.AddComponent("hooks", discoverFiles(root, filepath.Join("targets", target, "hooks"), nil)...)
	}
	tc.AddComponent("commands", discoverFiles(root, filepath.Join("targets", target, "commands"), nil)...)
	tc.AddComponent("policies", discoverFiles(root, filepath.Join("targets", target, "policies"), nil)...)
	if target == "gemini" {
		themes, err := discoverGeminiYAMLFiles(root, filepath.Join("targets", target, "themes"), "theme")
		if err != nil {
			return TargetComponents{}, err
		}
		settings, err := discoverGeminiYAMLFiles(root, filepath.Join("targets", target, "settings"), "setting")
		if err != nil {
			return TargetComponents{}, err
		}
		tc.AddComponent("themes", themes...)
		tc.AddComponent("settings", settings...)
	} else {
		tc.AddComponent("themes", discoverFiles(root, filepath.Join("targets", target, "themes"), nil)...)
		tc.AddComponent("settings", discoverFiles(root, filepath.Join("targets", target, "settings"), nil)...)
	}
	tc.AddComponent("contexts", discoverFiles(root, filepath.Join("targets", target, "contexts"), nil)...)
	if extraPath := optionalTargetDocPath(root, target, "manifest.extra.json"); extraPath != "" {
		tc.SetDoc("manifest_extra", extraPath)
	}
	if target == "codex" {
		if extraPath := optionalTargetDocPath(root, target, "config.extra.toml"); extraPath != "" {
			tc.SetDoc("config_extra", extraPath)
		}
	}
	return tc, nil
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
	for _, rel := range []string{"mcp/servers.yaml", "mcp/servers.yml", "mcp/servers.json"} {
		full := filepath.Join(root, rel)
		body, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		servers := map[string]any{}
		if strings.HasSuffix(rel, ".json") {
			if err := json.Unmarshal(body, &servers); err != nil {
				return nil, false, fmt.Errorf("parse %s: %w", rel, err)
			}
		} else {
			if err := yaml.Unmarshal(body, &servers); err != nil {
				return nil, false, fmt.Errorf("parse %s: %w", rel, err)
			}
		}
		if nested, ok := servers["servers"].(map[string]any); ok {
			servers = nested
		}
		return &PortableMCP{Path: rel, Servers: servers}, true, nil
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
			Message: "portable MCP will be preserved under mcp/servers.json",
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

func importedLayoutArtifacts(root, from string) ([]Artifact, []Warning, error) {
	var artifacts []Artifact
	var warnings []Warning
	mcpArtifacts, err := importedPortableMCPArtifacts(root)
	if err != nil {
		return nil, nil, err
	}
	artifacts = append(artifacts, mcpArtifacts...)
	switch from {
	case "claude":
		copied, err := copySingleArtifactIfExists(root, filepath.Join("hooks", "hooks.json"), filepath.Join("targets", "claude", "hooks", "hooks.json"))
		if err != nil {
			return nil, nil, err
		}
		artifacts = append(artifacts, copied...)
	case "codex":
		copied, codexWarnings, err := importedCodexArtifacts(root)
		if err != nil {
			return nil, nil, err
		}
		artifacts = append(artifacts, copied...)
		warnings = append(warnings, codexWarnings...)
	case "gemini":
		copied, geminiWarnings, err := importedGeminiArtifacts(root)
		if err != nil {
			return nil, nil, err
		}
		artifacts = append(artifacts, copied...)
		warnings = append(warnings, geminiWarnings...)
	}
	slices.SortFunc(artifacts, func(a, b Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return compactArtifacts(artifacts), warnings, nil
}

func importedCodexArtifacts(root string) ([]Artifact, []Warning, error) {
	var artifacts []Artifact
	var warnings []Warning
	config, _, err := readImportedCodexConfig(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, err
		}
	} else {
		if strings.TrimSpace(config.Model) != "" {
			body, err := yaml.Marshal(CodexTargetMeta{ModelHint: config.Model})
			if err != nil {
				return nil, nil, err
			}
			artifacts = append(artifacts, Artifact{RelPath: filepath.Join("targets", "codex", "package.yaml"), Content: body})
		}
		if len(config.Extra) > 0 {
			body, err := toml.Marshal(config.Extra)
			if err != nil {
				return nil, nil, err
			}
			artifacts = append(artifacts, Artifact{RelPath: filepath.Join("targets", "codex", "config.extra.toml"), Content: body})
			warnings = append(warnings, Warning{
				Kind:    WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join("targets", "codex", "config.extra.toml")),
				Message: "preserved unsupported Codex config fields under targets/codex/config.extra.toml",
			})
		}
		if len(config.Notify) > 0 && !isCanonicalCodexNotify(config.Notify) {
			warnings = append(warnings, Warning{
				Kind:    WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
				Message: "normalized Codex notify argv to the managed [entrypoint, \"notify\"] shape",
			})
		}
	}
	pluginManifest, _, err := readImportedCodexPluginManifest(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, nil, err
		}
	} else {
		if len(pluginManifest.Extra) > 0 {
			artifacts = append(artifacts, Artifact{RelPath: filepath.Join("targets", "codex", "manifest.extra.json"), Content: mustJSON(pluginManifest.Extra)})
			warnings = append(warnings, Warning{
				Kind:    WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join("targets", "codex", "manifest.extra.json")),
				Message: "preserved unsupported Codex plugin manifest fields under targets/codex/manifest.extra.json",
			})
		}
		if strings.TrimSpace(pluginManifest.SkillsPath) != "" && strings.TrimSpace(pluginManifest.SkillsPath) != "./skills/" {
			warnings = append(warnings, Warning{
				Kind:    WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Message: "normalized Codex plugin skills path to the managed ./skills/ location",
			})
		}
		if strings.TrimSpace(pluginManifest.MCPServersRef) != "" && strings.TrimSpace(pluginManifest.MCPServersRef) != "./.mcp.json" {
			warnings = append(warnings, Warning{
				Kind:    WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Message: "normalized Codex plugin mcpServers path to the managed ./.mcp.json location",
			})
		}
	}
	return compactArtifacts(artifacts), warnings, nil
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

func importedGeminiPrimaryContextName(root string, meta GeminiTargetMeta) string {
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
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return importedCodexPluginManifest{}, nil, err
	}
	out := importedCodexPluginManifest{}
	if value, ok := raw["name"].(string); ok {
		out.Name = strings.TrimSpace(value)
	}
	if value, ok := raw["version"].(string); ok {
		out.Version = strings.TrimSpace(value)
	}
	if value, ok := raw["description"].(string); ok {
		out.Description = strings.TrimSpace(value)
	}
	if value, ok := raw["skills"].(string); ok {
		out.SkillsPath = strings.TrimSpace(value)
	}
	if value, ok := raw["mcpServers"].(string); ok {
		out.MCPServersRef = strings.TrimSpace(value)
	}
	delete(raw, "name")
	delete(raw, "version")
	delete(raw, "description")
	delete(raw, "skills")
	delete(raw, "mcpServers")
	if len(raw) > 0 {
		out.Extra = raw
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
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q uses %q; expected %q from plugin.yaml entrypoint", hookName, command.Command, expected))
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
		mcpJSON, err := marshalJSON(opts.Portable.MCP.Servers)
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
	return copySingleArtifactIfExists(root, ".mcp.json", filepath.Join("mcp", "servers.json"))
}

func importedGeminiArtifacts(root string) ([]Artifact, []Warning, error) {
	var artifacts []Artifact
	var warnings []Warning
	copied, err := copySingleArtifactIfExists(root, filepath.Join("hooks", "hooks.json"), filepath.Join("targets", "gemini", "hooks", "hooks.json"))
	if err != nil {
		return nil, nil, err
	}
	artifacts = append(artifacts, copied...)
	copied, err = copyArtifactDirs(root,
		artifactDir{src: "commands", dst: filepath.Join("targets", "gemini", "commands")},
		artifactDir{src: "policies", dst: filepath.Join("targets", "gemini", "policies")},
	)
	if err != nil {
		return nil, nil, err
	}
	artifacts = append(artifacts, copied...)

	data, ok, err := readImportedGeminiExtension(root)
	if err != nil {
		return nil, nil, err
	}
	if ok {
		if len(data.MCPServers) > 0 {
			artifacts = append(artifacts, Artifact{RelPath: filepath.Join("mcp", "servers.json"), Content: mustJSON(data.MCPServers)})
		}
		if body, ok := importedGeminiPackageYAML(data.Meta); ok {
			artifacts = append(artifacts, Artifact{RelPath: filepath.Join("targets", "gemini", "package.yaml"), Content: body})
		}
		artifacts = append(artifacts, importedGeminiSettingsArtifacts(data.Settings)...)
		artifacts = append(artifacts, importedGeminiThemeArtifacts(data.Themes)...)
		if len(data.Extra) > 0 {
			artifacts = append(artifacts, Artifact{RelPath: filepath.Join("targets", "gemini", "manifest.extra.json"), Content: mustJSON(data.Extra)})
			warnings = append(warnings, Warning{
				Kind:    WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join("targets", "gemini", "manifest.extra.json")),
				Message: "preserved unsupported Gemini manifest fields under targets/gemini/manifest.extra.json",
			})
		}
		if contextName := importedGeminiPrimaryContextName(root, data.Meta); contextName != "" {
			contextArtifacts, err := copySingleArtifactIfExists(root, contextName, filepath.Join("targets", "gemini", "contexts", filepath.Base(contextName)))
			if err != nil {
				return nil, nil, err
			}
			artifacts = append(artifacts, contextArtifacts...)
		}
	}
	return compactArtifacts(artifacts), warnings, nil
}

func importedGeminiPackageYAML(meta GeminiTargetMeta) ([]byte, bool) {
	if len(meta.ExcludeTools) == 0 &&
		strings.TrimSpace(meta.ContextFileName) == "" &&
		strings.TrimSpace(meta.MigratedTo) == "" &&
		strings.TrimSpace(meta.PlanDirectory) == "" {
		return nil, false
	}
	return mustYAML(meta), true
}

func importedGeminiSettingsArtifacts(values []any) []Artifact {
	used := map[string]int{}
	var artifacts []Artifact
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		setting := GeminiSetting{}
		if name, ok := item["name"].(string); ok {
			setting.Name = name
		}
		if description, ok := item["description"].(string); ok {
			setting.Description = description
		}
		if envVar, ok := item["envVar"].(string); ok {
			setting.EnvVar = envVar
		}
		if sensitive, ok := item["sensitive"].(bool); ok {
			setting.Sensitive = sensitive
		}
		filename := collisionSafeSlug(setting.Name, used) + ".yaml"
		artifacts = append(artifacts, Artifact{
			RelPath: filepath.Join("targets", "gemini", "settings", filename),
			Content: mustYAML(setting),
		})
	}
	return artifacts
}

func importedGeminiThemeArtifacts(values []any) []Artifact {
	used := map[string]int{}
	var artifacts []Artifact
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		filename := collisionSafeSlug(name, used) + ".yaml"
		artifacts = append(artifacts, Artifact{
			RelPath: filepath.Join("targets", "gemini", "themes", filename),
			Content: mustYAML(item),
		})
	}
	return artifacts
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

func expectedManagedPaths(graph PackageGraph, selected []string) []string {
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
			case platformmeta.ManagedArtifactMirror:
				addManagedCopies(seen, tc.ComponentPaths(spec.ComponentKind), spec.SourceRoot, spec.OutputRoot)
			case platformmeta.ManagedArtifactSelectedContext:
				if spec.ContextMode != platformmeta.ContextStrategyGeminiPrimaryRoot {
					continue
				}
				for _, rel := range tc.ComponentPaths(spec.ComponentKind) {
					seen[geminiExtraContextArtifactPath(rel)] = struct{}{}
				}
				if selectedContext, ok, err := selectGeminiPrimaryContext(graph, tc); err == nil && ok {
					delete(seen, geminiExtraContextArtifactPath(selectedContext.SourcePath))
					seen[selectedContext.ArtifactName] = struct{}{}
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
	if len(graph.Portable.Paths("agents")) > 0 && !supportedPortable["agents"] {
		unsupported = append(unsupported, "agents")
	}
	if len(graph.Portable.Paths("contexts")) > 0 && !supportedPortable["contexts"] {
		unsupported = append(unsupported, "contexts")
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
	m.Format = strings.TrimSpace(m.Format)
	if m.Format == "" {
		m.Format = FormatMarker
	}
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	m.Description = strings.TrimSpace(m.Description)
	for i, target := range m.Targets {
		m.Targets[i] = normalizeTarget(target)
	}
	slices.Sort(m.Targets)
	m.Targets = slices.Compact(m.Targets)
}

func normalizeLauncher(l *Launcher) {
	l.Runtime = normalizeRuntime(l.Runtime)
	l.Entrypoint = strings.TrimSpace(l.Entrypoint)
}

func normalizeTarget(target string) string {
	return strings.ToLower(strings.TrimSpace(target))
}

func normalizeRuntime(runtime string) string {
	return strings.ToLower(strings.TrimSpace(runtime))
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
