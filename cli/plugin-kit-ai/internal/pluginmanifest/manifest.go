package pluginmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/scaffold"
	"gopkg.in/yaml.v3"
)

const (
	SchemaVersion = 1
	FileName      = "plugin.yaml"
)

var supportedTargets = []string{"claude", "codex", "gemini"}

type WarningKind string

const (
	WarningUnknownField    WarningKind = "unknown_field"
	WarningDeprecatedField WarningKind = "deprecated_field"
	WarningIgnoredImport   WarningKind = "ignored_import"
)

type Warning struct {
	Kind    WarningKind
	Path    string
	Message string
}

type PathRef struct {
	Path string `yaml:"path" json:"path"`
}

type Components struct {
	Skills   []PathRef `yaml:"skills"`
	Commands []PathRef `yaml:"commands"`
}

type TargetConfig struct {
	Model string `yaml:"model,omitempty"`
}

type Targets struct {
	Enabled []string      `yaml:"enabled"`
	Codex   *TargetConfig `yaml:"codex,omitempty"`
}

type Manifest struct {
	SchemaVersion int        `yaml:"schema_version"`
	Name          string     `yaml:"name"`
	Version       string     `yaml:"version"`
	Description   string     `yaml:"description"`
	Runtime       string     `yaml:"runtime"`
	Entrypoint    string     `yaml:"entrypoint"`
	Targets       Targets    `yaml:"targets"`
	Components    Components `yaml:"components"`
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

func Parse(body []byte) (Manifest, error) {
	manifest, _, err := Analyze(body)
	return manifest, err
}

func Analyze(body []byte) (Manifest, []Warning, error) {
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
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("invalid plugin.yaml: unsupported schema_version %d", m.SchemaVersion)
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
	if len(m.Targets.Enabled) == 0 {
		return fmt.Errorf("invalid plugin.yaml: targets.enabled must not be empty")
	}
	seen := map[string]struct{}{}
	for _, target := range m.Targets.Enabled {
		target = normalizeTarget(target)
		if !slices.Contains(supportedTargets, target) {
			return fmt.Errorf("invalid plugin.yaml: unsupported target %q", target)
		}
		if _, ok := seen[target]; ok {
			return fmt.Errorf("invalid plugin.yaml: duplicate target %q", target)
		}
		seen[target] = struct{}{}
	}
	for _, ref := range componentRefs(m.Components) {
		if strings.TrimSpace(ref.Path) == "" {
			return fmt.Errorf("invalid plugin.yaml: component paths must not be empty")
		}
	}
	return nil
}

func (m Manifest) EnabledTargets() []string {
	out := make([]string, 0, len(m.Targets.Enabled))
	for _, target := range m.Targets.Enabled {
		out = append(out, normalizeTarget(target))
	}
	return out
}

func (m Manifest) ComponentPaths() []string {
	refs := componentRefs(m.Components)
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Path)
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

func Default(projectName, platform, runtime, description string, extras bool) Manifest {
	platform = normalizeTarget(platform)
	runtime = normalizeRuntime(runtime)
	if strings.TrimSpace(description) == "" {
		description = "plugin-kit-ai plugin"
	}
	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Name:          projectName,
		Version:       "0.1.0",
		Description:   description,
		Runtime:       runtime,
		Entrypoint:    "./bin/" + projectName,
		Targets: Targets{
			Enabled: []string{platform},
		},
		Components: Components{
			Skills:   []PathRef{},
			Commands: []PathRef{},
		},
	}
	if platform == "codex" {
		manifest.Targets.Codex = &TargetConfig{Model: "gpt-5-codex"}
	}
	if extras {
		manifest.Components.Skills = append(manifest.Components.Skills, PathRef{Path: filepath.ToSlash(filepath.Join("skills", projectName, "SKILL.md"))})
		manifest.Components.Commands = append(manifest.Components.Commands, PathRef{Path: filepath.ToSlash(filepath.Join("commands", projectName+".md"))})
	}
	return manifest
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

func Render(root string, target string) (RenderResult, error) {
	manifest, err := Load(root)
	if err != nil {
		return RenderResult{}, err
	}
	selected, err := manifest.SelectedTargets(target)
	if err != nil {
		return RenderResult{}, err
	}
	var artifacts []Artifact
	expected := map[string]struct{}{}
	for _, target := range selected {
		rendered, err := renderTargetArtifacts(manifest, target)
		if err != nil {
			return RenderResult{}, err
		}
		for _, artifact := range rendered {
			artifacts = append(artifacts, artifact)
			expected[artifact.RelPath] = struct{}{}
		}
	}
	var stale []string
	for _, path := range managedPaths(selected) {
		if _, ok := expected[path]; !ok {
			if _, err := os.Stat(filepath.Join(root, path)); err == nil {
				stale = append(stale, path)
			}
		}
	}
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
		if string(body) != string(artifact.Content) {
			drift = append(drift, artifact.RelPath)
		}
	}
	drift = append(drift, result.StalePaths...)
	slices.Sort(drift)
	return slices.Compact(drift), nil
}

func Import(root string, from string) (Manifest, []Warning, error) {
	from = normalizeTarget(from)
	if from == "" {
		from = inferLegacyPlatform(root)
	}
	if !slices.Contains(supportedTargets, from) {
		return Manifest{}, nil, fmt.Errorf("unsupported import source %q", from)
	}
	warnings := importWarnings(root)
	if legacy, err := loadLegacyProjectManifest(root); err == nil {
		manifest := Default(defaultName(root), legacy.Platform, legacy.Runtime, legacy.Description, hasExtras(root))
		manifest.Entrypoint = legacy.Entrypoint
		enrichFromNative(root, &manifest, from)
		return manifest, warnings, nil
	}
	manifest := Default(defaultName(root), from, inferRuntime(root), "plugin-kit-ai plugin", hasExtras(root))
	enrichFromNative(root, &manifest, from)
	return manifest, warnings, nil
}

func renderTargetArtifacts(manifest Manifest, target string) ([]Artifact, error) {
	target = normalizeTarget(target)
	codexModel := ""
	if manifest.Targets.Codex != nil {
		codexModel = manifest.Targets.Codex.Model
	}
	data := scaffold.Data{
		ProjectName: manifest.Name,
		ModulePath:  scaffold.DefaultModulePath(manifest.Name),
		Description: manifest.Description,
		Version:     manifest.Version,
		Platform:    target,
		Runtime:     manifest.Runtime,
		Entrypoint:  manifest.Entrypoint,
		CodexModel:  codexModel,
		HasSkills:   len(manifest.Components.Skills) > 0,
		HasCommands: len(manifest.Components.Commands) > 0,
	}
	var artifacts []Artifact
	switch target {
	case "claude":
		artifact, err := renderTemplateArtifact(".claude-plugin/plugin.json", "plugin.json.tmpl", data)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
		artifact, err = renderTemplateArtifact("hooks/hooks.json", "hooks.json.tmpl", data)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	case "codex":
		artifact, err := renderTemplateArtifact(filepath.Join(".codex-plugin", "plugin.json"), "codex.plugin.json.tmpl", data)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
		artifact, err = renderTemplateArtifact(filepath.Join(".codex", "config.toml"), "codex.config.toml.tmpl", data)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	case "gemini":
		artifact, err := renderTemplateArtifact("gemini-extension.json", "gemini-extension.json.tmpl", data)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func renderTemplateArtifact(relPath, tpl string, data scaffold.Data) (Artifact, error) {
	body, _, err := scaffold.RenderTemplate(tpl, data)
	if err != nil {
		return Artifact{}, fmt.Errorf("render %s from %s: %w", relPath, tpl, err)
	}
	return Artifact{RelPath: relPath, Content: body}, nil
}

func managedPaths(targets []string) []string {
	seen := map[string]struct{}{}
	for _, target := range targets {
		switch normalizeTarget(target) {
		case "claude":
			seen[filepath.Join(".claude-plugin", "plugin.json")] = struct{}{}
			seen[filepath.Join("hooks", "hooks.json")] = struct{}{}
		case "codex":
			seen[filepath.Join(".codex-plugin", "plugin.json")] = struct{}{}
			seen[filepath.Join(".codex", "config.toml")] = struct{}{}
		case "gemini":
			seen["gemini-extension.json"] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
}

func componentRefs(components Components) []PathRef {
	out := make([]PathRef, 0, len(components.Skills)+len(components.Commands))
	out = append(out, components.Skills...)
	out = append(out, components.Commands...)
	return out
}

func defaultName(root string) string {
	name := filepath.Base(filepath.Clean(root))
	if err := scaffold.ValidateProjectName(name); err == nil {
		return name
	}
	return "plugin"
}

func hasExtras(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "skills")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(root, "commands")); err == nil {
		return true
	}
	return false
}

func normalizeTarget(target string) string {
	return strings.ToLower(strings.TrimSpace(target))
}

func normalizeRuntime(runtime string) string {
	return strings.ToLower(strings.TrimSpace(runtime))
}

func normalizeManifest(m *Manifest) {
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	m.Description = strings.TrimSpace(m.Description)
	m.Runtime = normalizeRuntime(m.Runtime)
	m.Entrypoint = strings.TrimSpace(m.Entrypoint)
	for i, target := range m.Targets.Enabled {
		m.Targets.Enabled[i] = normalizeTarget(target)
	}
	if m.Targets.Codex != nil {
		m.Targets.Codex.Model = strings.TrimSpace(m.Targets.Codex.Model)
		if m.Targets.Codex.Model == "" {
			m.Targets.Codex = nil
		}
	}
	if m.Components.Skills == nil {
		m.Components.Skills = []PathRef{}
	}
	if m.Components.Commands == nil {
		m.Components.Commands = []PathRef{}
	}
}

type legacyProject struct {
	Platform    string
	Runtime     string
	Entrypoint  string
	Description string
}

func loadLegacyProjectManifest(root string) (legacyProject, error) {
	full := filepath.Join(root, ".plugin-kit-ai", "project.toml")
	body, err := os.ReadFile(full)
	if err != nil {
		return legacyProject{}, err
	}
	out := legacyProject{Description: "plugin-kit-ai plugin"}
	lines := strings.Split(string(body), "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "platform":
			out.Platform = trimQuoted(value)
		case "runtime":
			out.Runtime = trimQuoted(value)
		case "entrypoint":
			out.Entrypoint = trimQuoted(value)
		}
	}
	if out.Platform == "" || out.Runtime == "" || out.Entrypoint == "" {
		return legacyProject{}, fmt.Errorf("invalid legacy project manifest")
	}
	return out, nil
}

func trimQuoted(v string) string {
	s, err := strconv.Unquote(v)
	if err != nil {
		return strings.Trim(v, `"`)
	}
	return s
}

func inferLegacyPlatform(root string) string {
	switch {
	case fileExists(filepath.Join(root, ".claude-plugin", "plugin.json")) || fileExists(filepath.Join(root, "hooks", "hooks.json")):
		return "claude"
	case fileExists(filepath.Join(root, ".codex", "config.toml")) || fileExists(filepath.Join(root, "AGENTS.md")):
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

func enrichFromNative(root string, manifest *Manifest, from string) {
	switch from {
	case "claude":
		loadClaudeMetadata(root, manifest)
	case "codex":
		loadCodexMetadata(root, manifest)
	case "gemini":
		loadGeminiMetadata(root, manifest)
	}
	manifest.Components.Skills = discoverSkillRefs(root)
	manifest.Components.Commands = discoverCommandRefs(root)
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
		text := string(body)
		for _, hook := range []string{"SessionStart", "Stop", "Notification"} {
			token := `"command": "`
			idx := strings.Index(text, token)
			if idx >= 0 {
				rest := text[idx+len(token):]
				end := strings.Index(rest, " "+hook+`"`)
				if end > 0 {
					manifest.Entrypoint = rest[:end]
					break
				}
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
	if body, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml")); err == nil {
		text := string(body)
		if idx := strings.Index(text, `notify = ["`); idx >= 0 {
			rest := text[idx+len(`notify = ["`):]
			if end := strings.Index(rest, `", "notify"]`); end >= 0 {
				manifest.Entrypoint = rest[:end]
			}
		}
		if idx := strings.Index(text, `model = "`); idx >= 0 {
			rest := text[idx+len(`model = "`):]
			if end := strings.Index(rest, `"`); end >= 0 {
				if manifest.Targets.Codex == nil {
					manifest.Targets.Codex = &TargetConfig{}
				}
				manifest.Targets.Codex.Model = rest[:end]
			}
		}
	}
}

func loadGeminiMetadata(root string, manifest *Manifest) {
	type meta struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
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
		}
	}
}

func discoverSkillRefs(root string) []PathRef {
	skillsRoot := filepath.Join(root, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		return []PathRef{}
	}
	var refs []PathRef
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join("skills", entry.Name(), "SKILL.md")
		if fileExists(filepath.Join(root, path)) {
			refs = append(refs, PathRef{Path: filepath.ToSlash(path)})
		}
	}
	return refs
}

func discoverCommandRefs(root string) []PathRef {
	entries, err := os.ReadDir(filepath.Join(root, "commands"))
	if err != nil {
		return []PathRef{}
	}
	var refs []PathRef
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		refs = append(refs, PathRef{Path: filepath.ToSlash(filepath.Join("commands", entry.Name()))})
	}
	return refs
}

func importWarnings(root string) []Warning {
	var warnings []Warning
	if fileExists(filepath.Join(root, ".mcp.json")) {
		warnings = append(warnings, Warning{
			Kind:    WarningIgnoredImport,
			Path:    ".mcp.json",
			Message: "ignored unsupported import asset: .mcp.json",
		})
	}
	if fileExists(filepath.Join(root, "agents")) {
		warnings = append(warnings, Warning{
			Kind:    WarningIgnoredImport,
			Path:    "agents",
			Message: "ignored unsupported import asset: agents",
		})
	}
	if fileExists(filepath.Join(root, "hooks")) && !fileExists(filepath.Join(root, FileName)) {
		warnings = append(warnings, Warning{
			Kind:    WarningIgnoredImport,
			Path:    "hooks",
			Message: "ignored plugin.yaml v1 import asset: hooks",
		})
	}
	return warnings
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
	Fields     map[string]schemaSpec
	Seq        *schemaSpec
	Deprecated bool
}

func manifestSchema() schemaSpec {
	pathRef := schemaSpec{Fields: map[string]schemaSpec{
		"path": {},
	}}
	return schemaSpec{Fields: map[string]schemaSpec{
		"schema_version": {},
		"name":           {},
		"version":        {},
		"description":    {},
		"runtime":        {},
		"entrypoint":     {},
		"targets": {Fields: map[string]schemaSpec{
			"enabled": {Seq: &schemaSpec{}},
			"codex": {Fields: map[string]schemaSpec{
				"model": {},
			}},
			"claude": {Deprecated: true},
			"gemini": {Deprecated: true},
		}},
		"components": {Fields: map[string]schemaSpec{
			"skills":   {Seq: &pathRef},
			"commands": {Seq: &pathRef},
			"agents":   {Deprecated: true},
			"hooks":    {Deprecated: true},
			"mcp":      {Deprecated: true},
		}},
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
			if child.Deprecated {
				appendWarning(seen, warnings, Warning{
					Kind:    WarningDeprecatedField,
					Path:    keyPath,
					Message: "deprecated plugin.yaml field ignored: " + keyPath,
				})
				continue
			}
			walkNode(valNode, keyPath, child, seen, warnings)
		}
		return
	}
	if spec.Seq != nil && node.Kind == yaml.SequenceNode {
		for idx, item := range node.Content {
			itemPath := fmt.Sprintf("%s[%d]", path, idx)
			walkNode(item, itemPath, *spec.Seq, seen, warnings)
		}
	}
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
