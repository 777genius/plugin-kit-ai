package pluginmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/scaffold"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/targetcontracts"
	"gopkg.in/yaml.v3"
)

const (
	FileName     = "plugin.yaml"
	FormatMarker = "plugin-kit-ai/package"
)

var supportedTargets = []string{"claude", "codex", "gemini"}

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
	Runtime     string   `yaml:"runtime" json:"runtime"`
	Entrypoint  string   `yaml:"entrypoint" json:"entrypoint"`
	Targets     []string `yaml:"targets" json:"targets"`
}

type PortableMCP struct {
	Path    string
	Servers map[string]any
}

type PortableComponents struct {
	Skills   []string
	Agents   []string
	Contexts []string
	MCP      *PortableMCP
}

type TargetPackage struct {
	DisplayName     string `yaml:"display_name,omitempty"`
	ModelHint       string `yaml:"model_hint,omitempty"`
	ContextFileName string `yaml:"context_file_name,omitempty"`
}

type TargetComponents struct {
	Target      string
	PackagePath string
	Package     TargetPackage
	Hooks       []string
	Commands    []string
	Policies    []string
	Themes      []string
	Settings    []string
	Contexts    []string
	Opaque      []string
}

type PackageGraph struct {
	Manifest    Manifest
	Portable    PortableComponents
	Targets     map[string]TargetComponents
	SourceFiles []string
}

type InspectTarget struct {
	Target            string   `json:"target"`
	TargetClass       string   `json:"target_class"`
	ProductionClass   string   `json:"production_class"`
	RuntimeContract   string   `json:"runtime_contract"`
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

func Load(root string) (Manifest, error) {
	manifest, _, err := LoadWithWarnings(root)
	return manifest, err
}

func LoadWithWarnings(root string) (Manifest, []Warning, error) {
	body, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		return Manifest{}, nil, err
	}
	return Analyze(body)
}

func Parse(body []byte) (Manifest, error) {
	manifest, _, err := Analyze(body)
	return manifest, err
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
	if _, ok := scaffold.LookupRuntime(m.Runtime); !ok {
		return fmt.Errorf("invalid plugin.yaml: unsupported runtime %q", m.Runtime)
	}
	if strings.TrimSpace(m.Entrypoint) == "" {
		return fmt.Errorf("invalid plugin.yaml: entrypoint required")
	}
	if len(m.Targets) == 0 {
		return fmt.Errorf("invalid plugin.yaml: targets must not be empty")
	}
	seen := map[string]struct{}{}
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
	runtime = normalizeRuntime(runtime)
	if strings.TrimSpace(description) == "" {
		description = "plugin-kit-ai plugin"
	}
	return Manifest{
		Format:      FormatMarker,
		Name:        projectName,
		Version:     "0.1.0",
		Description: description,
		Runtime:     runtime,
		Entrypoint:  "./bin/" + projectName,
		Targets:     []string{platform},
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

func Normalize(root string, force bool) ([]Warning, error) {
	manifest, warnings, err := LoadWithWarnings(root)
	if err != nil {
		return nil, err
	}
	if err := Save(root, manifest, force); err != nil {
		return warnings, err
	}
	return warnings, nil
}

func Discover(root string) (PackageGraph, []Warning, error) {
	manifest, warnings, err := LoadWithWarnings(root)
	if err != nil {
		return PackageGraph{}, nil, err
	}
	graph := PackageGraph{
		Manifest: manifest,
		Targets:  make(map[string]TargetComponents, len(manifest.Targets)),
	}
	sourceSet := map[string]struct{}{FileName: {}}

	graph.Portable.Skills = discoverFiles(root, filepath.Join("skills"), func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	addSourceFiles(sourceSet, graph.Portable.Skills)

	graph.Portable.Agents = discoverFiles(root, filepath.Join("agents"), func(rel string) bool {
		return strings.HasSuffix(rel, ".md")
	})
	addSourceFiles(sourceSet, graph.Portable.Agents)

	graph.Portable.Contexts = discoverFiles(root, filepath.Join("contexts"), nil)
	addSourceFiles(sourceSet, graph.Portable.Contexts)

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
			ProductionClass:   entry.ProductionClass,
			RuntimeContract:   entry.RuntimeContract,
			PortableKinds:     entry.PortableComponentKinds,
			TargetNativeKinds: discoveredNativeKinds(tc),
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

func Import(root string, from string) (Manifest, []Warning, error) {
	if fileExists(filepath.Join(root, ".plugin-kit-ai", "project.toml")) {
		return Manifest{}, nil, fmt.Errorf("unsupported project format for import: .plugin-kit-ai/project.toml is not supported; rewrite the project into the package standard layout")
	}
	from = normalizeTarget(from)
	if from == "" {
		from = inferNativePlatform(root)
	}
	if !slices.Contains(supportedTargets, from) {
		return Manifest{}, nil, fmt.Errorf("unsupported import source %q", from)
	}
	manifest, warnings, err := importManifest(root, from)
	if err != nil {
		return Manifest{}, nil, err
	}
	if err := materializeImportedLayout(root, from, manifest); err != nil {
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
	manifest := map[string]any{
		"name":        displayName(graph.Manifest, tc),
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if len(graph.Portable.Skills) > 0 {
		manifest["skills"] = "./skills/"
	}
	if len(graph.Portable.Agents) > 0 {
		manifest["agents"] = "./agents/"
	}
	if graph.Portable.MCP != nil {
		manifest["mcpServers"] = "./.mcp.json"
	}
	pluginJSON, err := marshalJSON(manifest)
	if err != nil {
		return nil, err
	}
	artifacts := []Artifact{{RelPath: filepath.Join(".claude-plugin", "plugin.json"), Content: pluginJSON}}
	if graph.Portable.MCP != nil {
		mcpJSON, err := marshalJSON(graph.Portable.MCP.Servers)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, Artifact{RelPath: ".mcp.json", Content: mcpJSON})
	}
	if len(tc.Hooks) > 0 {
		copied, err := copyArtifacts(root, filepath.Join("targets", "claude", "hooks"), "hooks")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	} else {
		artifacts = append(artifacts, Artifact{
			RelPath: filepath.Join("hooks", "hooks.json"),
			Content: defaultClaudeHooks(graph.Manifest.Entrypoint),
		})
	}
	copiedKinds := []struct {
		src string
		dst string
	}{
		{src: filepath.Join("targets", "claude", "commands"), dst: "commands"},
		{src: filepath.Join("targets", "claude", "contexts"), dst: "contexts"},
	}
	for _, item := range copiedKinds {
		copied, err := copyArtifacts(root, item.src, item.dst)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	}
	return artifacts, nil
}

func renderCodex(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	manifest := map[string]any{
		"name":        displayName(graph.Manifest, tc),
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if len(graph.Portable.Skills) > 0 {
		manifest["skills"] = "./skills/"
	}
	if graph.Portable.MCP != nil {
		manifest["mcpServers"] = "./.mcp.json"
	}
	pluginJSON, err := marshalJSON(manifest)
	if err != nil {
		return nil, err
	}
	model := tc.Package.ModelHint
	if strings.TrimSpace(model) == "" {
		model = "gpt-5-codex"
	}
	var config bytes.Buffer
	config.WriteString("# Generated by plugin-kit-ai. DO NOT EDIT.\n")
	config.WriteString(fmt.Sprintf("model = %q\n", model))
	config.WriteString(fmt.Sprintf("notify = [%q, %q]\n", graph.Manifest.Entrypoint, "notify"))
	artifacts := []Artifact{
		{RelPath: filepath.Join(".codex-plugin", "plugin.json"), Content: pluginJSON},
		{RelPath: filepath.Join(".codex", "config.toml"), Content: config.Bytes()},
	}
	if graph.Portable.MCP != nil {
		mcpJSON, err := marshalJSON(graph.Portable.MCP.Servers)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, Artifact{RelPath: ".mcp.json", Content: mcpJSON})
	}
	for _, item := range []struct {
		src string
		dst string
	}{
		{src: filepath.Join("targets", "codex", "commands"), dst: "commands"},
		{src: filepath.Join("targets", "codex", "contexts"), dst: "contexts"},
	} {
		copied, err := copyArtifacts(root, item.src, item.dst)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	}
	return artifacts, nil
}

func renderGemini(root string, graph PackageGraph, tc TargetComponents) ([]Artifact, error) {
	manifest := map[string]any{
		"name":        displayName(graph.Manifest, tc),
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if graph.Portable.MCP != nil {
		manifest["mcpServers"] = graph.Portable.MCP.Servers
	}

	artifacts := []Artifact{}
	if contextName, contextArtifact, ok, err := geminiContextArtifact(root, graph, tc); err != nil {
		return nil, err
	} else if ok {
		manifest["contextFileName"] = contextName
		artifacts = append(artifacts, contextArtifact)
	}

	manifestJSON, err := marshalJSON(manifest)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, Artifact{RelPath: "gemini-extension.json", Content: manifestJSON})

	for _, item := range []struct {
		src string
		dst string
	}{
		{src: filepath.Join("targets", "gemini", "hooks"), dst: "hooks"},
		{src: filepath.Join("targets", "gemini", "commands"), dst: "commands"},
		{src: filepath.Join("targets", "gemini", "policies"), dst: "policies"},
		{src: filepath.Join("targets", "gemini", "themes"), dst: "themes"},
		{src: filepath.Join("targets", "gemini", "settings"), dst: "settings"},
		{src: filepath.Join("targets", "gemini", "contexts"), dst: "contexts"},
	} {
		copied, err := copyArtifacts(root, item.src, item.dst)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	}
	return artifacts, nil
}

func geminiContextArtifact(root string, graph PackageGraph, tc TargetComponents) (string, Artifact, bool, error) {
	name, ok := geminiContextArtifactName(graph, tc)
	if !ok {
		return "", Artifact{}, false, nil
	}
	rel, ok := geminiContextSourcePath(graph, tc, name)
	if !ok {
		return "", Artifact{}, false, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return "", Artifact{}, false, err
	}
	return name, Artifact{RelPath: name, Content: body}, true, nil
}

func geminiContextArtifactName(graph PackageGraph, tc TargetComponents) (string, bool) {
	selected := strings.TrimSpace(tc.Package.ContextFileName)
	if selected != "" {
		if _, ok := geminiContextSourcePath(graph, tc, selected); ok {
			return selected, true
		}
	}
	candidates := append([]string{}, tc.Contexts...)
	candidates = append(candidates, graph.Portable.Contexts...)
	if len(candidates) == 0 {
		return "", false
	}
	slices.Sort(candidates)
	return filepath.Base(candidates[0]), true
}

func geminiContextSourcePath(graph PackageGraph, tc TargetComponents, name string) (string, bool) {
	for _, rel := range append(append([]string{}, tc.Contexts...), graph.Portable.Contexts...) {
		if filepath.Base(rel) == name {
			return rel, true
		}
	}
	return "", false
}

func discoverTarget(root string, target string) (TargetComponents, error) {
	tc := TargetComponents{Target: target}
	packagePath := filepath.Join("targets", target, "package.yaml")
	if body, err := os.ReadFile(filepath.Join(root, packagePath)); err == nil {
		tc.PackagePath = filepath.ToSlash(packagePath)
		if err := yaml.Unmarshal(body, &tc.Package); err != nil {
			return TargetComponents{}, fmt.Errorf("parse %s: %w", packagePath, err)
		}
	}
	tc.Hooks = discoverFiles(root, filepath.Join("targets", target, "hooks"), nil)
	tc.Commands = discoverFiles(root, filepath.Join("targets", target, "commands"), nil)
	tc.Policies = discoverFiles(root, filepath.Join("targets", target, "policies"), nil)
	tc.Themes = discoverFiles(root, filepath.Join("targets", target, "themes"), nil)
	tc.Settings = discoverFiles(root, filepath.Join("targets", target, "settings"), nil)
	tc.Contexts = discoverFiles(root, filepath.Join("targets", target, "contexts"), nil)
	return tc, nil
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
		"runtime":     {},
		"entrypoint":  {},
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

func importManifest(root, from string) (Manifest, []Warning, error) {
	warnings := []Warning{}
	manifest := Default(defaultName(root), from, inferRuntime(root), "plugin-kit-ai plugin", false)
	enrichFromNative(root, &manifest, from, &warnings)
	return manifest, warnings, nil
}

func enrichFromNative(root string, manifest *Manifest, from string, warnings *[]Warning) {
	switch from {
	case "claude":
		loadClaudeMetadata(root, manifest)
	case "codex":
		loadCodexMetadata(root, manifest)
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

func materializeImportedLayout(root, from string, manifest Manifest) error {
	if fileExists(filepath.Join(root, ".mcp.json")) {
		body, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
		if err != nil {
			return err
		}
		if err := writeImportedFile(root, filepath.Join("mcp", "servers.json"), body); err != nil {
			return err
		}
	}
	switch from {
	case "claude":
		if fileExists(filepath.Join(root, "hooks", "hooks.json")) {
			body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
			if err != nil {
				return err
			}
			if err := writeImportedFile(root, filepath.Join("targets", "claude", "hooks", "hooks.json"), body); err != nil {
				return err
			}
		}
	case "codex":
		model := importCodexModel(root)
		if strings.TrimSpace(model) != "" {
			body, err := yaml.Marshal(TargetPackage{ModelHint: model})
			if err != nil {
				return err
			}
			if err := writeImportedFile(root, filepath.Join("targets", "codex", "package.yaml"), body); err != nil {
				return err
			}
		}
	case "gemini":
		if fileExists(filepath.Join(root, "hooks", "hooks.json")) {
			body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
			if err != nil {
				return err
			}
			if err := writeImportedFile(root, filepath.Join("targets", "gemini", "hooks", "hooks.json"), body); err != nil {
				return err
			}
		}
		for _, kind := range []string{"commands", "policies", "themes", "settings"} {
			if err := copyTreeIfExists(root, kind, filepath.Join("targets", "gemini", kind)); err != nil {
				return err
			}
		}
		if fileExists(filepath.Join(root, "GEMINI.md")) {
			body, err := os.ReadFile(filepath.Join(root, "GEMINI.md"))
			if err != nil {
				return err
			}
			if err := writeImportedFile(root, filepath.Join("targets", "gemini", "contexts", "GEMINI.md"), body); err != nil {
				return err
			}
			body, err = yaml.Marshal(TargetPackage{ContextFileName: "GEMINI.md"})
			if err != nil {
				return err
			}
			if err := writeImportedFile(root, filepath.Join("targets", "gemini", "package.yaml"), body); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadClaudeMetadata(root string, manifest *Manifest) {
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
		for _, hook := range claudeHookNames() {
			token := `"command": "`
			text := string(body)
			idx := strings.Index(text, token)
			if idx < 0 {
				continue
			}
			rest := text[idx+len(token):]
			end := strings.Index(rest, " "+hook+`"`)
			if end > 0 {
				manifest.Entrypoint = rest[:end]
				break
			}
		}
	}
}

func loadCodexMetadata(root string, manifest *Manifest) {
	type meta struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if body, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json")); err == nil {
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
	if model := importCodexModel(root); strings.TrimSpace(model) != "" {
		_ = writeImportedFile(root, filepath.Join("targets", "codex", "package.yaml"), mustYAML(TargetPackage{ModelHint: model}))
	}
	if body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml")); err == nil {
		text := string(body)
		if idx := strings.Index(text, `notify = ["`); idx >= 0 {
			rest := text[idx+len(`notify = ["`):]
			if end := strings.Index(rest, `", "notify"]`); end >= 0 {
				manifest.Entrypoint = rest[:end]
			}
		}
	}
}

func loadGeminiMetadata(root string, manifest *Manifest) {
	type meta struct {
		Name            string         `json:"name"`
		Version         string         `json:"version"`
		Description     string         `json:"description"`
		ContextFileName string         `json:"contextFileName"`
		MCPServers      map[string]any `json:"mcpServers"`
	}
	if body, err := os.ReadFile(filepath.Join(root, "gemini-extension.json")); err == nil {
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
			if len(m.MCPServers) > 0 {
				_ = writeImportedFile(root, filepath.Join("mcp", "servers.json"), mustJSON(m.MCPServers))
			}
			if strings.TrimSpace(m.ContextFileName) != "" {
				_ = writeImportedFile(root, filepath.Join("targets", "gemini", "package.yaml"), mustYAML(TargetPackage{ContextFileName: m.ContextFileName}))
			}
		}
	}
}

func importCodexModel(root string) string {
	body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		return ""
	}
	text := string(body)
	if idx := strings.Index(text, `model = "`); idx >= 0 {
		rest := text[idx+len(`model = "`):]
		if end := strings.Index(rest, `"`); end >= 0 {
			return rest[:end]
		}
	}
	return ""
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

func expectedManagedPaths(graph PackageGraph, selected []string) []string {
	seen := map[string]struct{}{}
	for _, target := range selected {
		if entry, ok := targetcontracts.Lookup(target); ok {
			for _, path := range entry.ManagedArtifacts {
				if strings.Contains(path, "**") {
					continue
				}
				seen[path] = struct{}{}
			}
		}
		tc := graph.Targets[target]
		switch target {
		case "claude":
			addManagedCopies(seen, tc.Hooks, filepath.Join("targets", "claude", "hooks"), "hooks")
			addManagedCopies(seen, tc.Commands, filepath.Join("targets", "claude", "commands"), "commands")
			addManagedCopies(seen, tc.Contexts, filepath.Join("targets", "claude", "contexts"), "contexts")
		case "codex":
			addManagedCopies(seen, tc.Commands, filepath.Join("targets", "codex", "commands"), "commands")
			addManagedCopies(seen, tc.Contexts, filepath.Join("targets", "codex", "contexts"), "contexts")
		case "gemini":
			addManagedCopies(seen, tc.Hooks, filepath.Join("targets", "gemini", "hooks"), "hooks")
			addManagedCopies(seen, tc.Commands, filepath.Join("targets", "gemini", "commands"), "commands")
			addManagedCopies(seen, tc.Policies, filepath.Join("targets", "gemini", "policies"), "policies")
			addManagedCopies(seen, tc.Themes, filepath.Join("targets", "gemini", "themes"), "themes")
			addManagedCopies(seen, tc.Settings, filepath.Join("targets", "gemini", "settings"), "settings")
			addManagedCopies(seen, tc.Contexts, filepath.Join("targets", "gemini", "contexts"), "contexts")
			if name, ok := geminiContextArtifactName(graph, tc); ok {
				seen[name] = struct{}{}
			}
		}
		if graph.Portable.MCP != nil && (target == "claude" || target == "codex") {
			seen[".mcp.json"] = struct{}{}
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

func discoveredNativeKinds(tc TargetComponents) []string {
	var kinds []string
	if tc.PackagePath != "" {
		kinds = append(kinds, "package_metadata")
	}
	if len(tc.Hooks) > 0 {
		kinds = append(kinds, "hooks")
	}
	if len(tc.Commands) > 0 {
		kinds = append(kinds, "commands")
	}
	if len(tc.Policies) > 0 {
		kinds = append(kinds, "policies")
	}
	if len(tc.Themes) > 0 {
		kinds = append(kinds, "themes")
	}
	if len(tc.Settings) > 0 {
		kinds = append(kinds, "settings")
	}
	if len(tc.Contexts) > 0 {
		kinds = append(kinds, "contexts")
	}
	return kinds
}

func unsupportedKinds(entry targetcontracts.Entry, graph PackageGraph, tc TargetComponents) []string {
	supportedPortable := setOf(entry.PortableComponentKinds)
	var unsupported []string
	if len(graph.Portable.Skills) > 0 && !supportedPortable["skills"] {
		unsupported = append(unsupported, "skills")
	}
	if graph.Portable.MCP != nil && !supportedPortable["mcp_servers"] {
		unsupported = append(unsupported, "mcp_servers")
	}
	if len(graph.Portable.Agents) > 0 && !supportedPortable["agents"] {
		unsupported = append(unsupported, "agents")
	}
	supportedNative := setOf(entry.TargetComponentKinds)
	for _, kind := range discoveredNativeKinds(tc) {
		if !supportedNative[kind] {
			unsupported = append(unsupported, kind)
		}
	}
	slices.Sort(unsupported)
	return slices.Compact(unsupported)
}

func targetFiles(tc TargetComponents) []string {
	var out []string
	if tc.PackagePath != "" {
		out = append(out, tc.PackagePath)
	}
	out = append(out, tc.Hooks...)
	out = append(out, tc.Commands...)
	out = append(out, tc.Policies...)
	out = append(out, tc.Themes...)
	out = append(out, tc.Settings...)
	out = append(out, tc.Contexts...)
	return out
}

func displayName(manifest Manifest, tc TargetComponents) string {
	if strings.TrimSpace(tc.Package.DisplayName) != "" {
		return tc.Package.DisplayName
	}
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
	for _, name := range claudeHookNames() {
		hooks[name] = []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: entrypoint + " " + name}}}}
	}
	body, _ := marshalJSON(map[string]any{"hooks": hooks})
	return body
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

func copyTreeIfExists(root, srcRel, dstRel string) error {
	full := filepath.Join(root, srcRel)
	if _, err := os.Stat(full); err != nil {
		return nil
	}
	return filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
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
		return writeImportedFile(root, filepath.Join(dstRel, rel), body)
	})
}

func writeImportedFile(root, rel string, body []byte) error {
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if fileExists(full) {
		return nil
	}
	return os.WriteFile(full, body, 0o644)
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
	m.Runtime = normalizeRuntime(m.Runtime)
	m.Entrypoint = strings.TrimSpace(m.Entrypoint)
	for i, target := range m.Targets {
		m.Targets[i] = normalizeTarget(target)
	}
	slices.Sort(m.Targets)
	m.Targets = slices.Compact(m.Targets)
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
