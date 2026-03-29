package pluginmodel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/scaffold"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
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

type TargetState struct {
	Target     string
	Docs       map[string]string
	Components map[string][]string
}

type PackageGraph struct {
	Manifest    Manifest
	Launcher    *Launcher
	Portable    PortableComponents
	Targets     map[string]TargetState
	SourceFiles []string
}

type Artifact struct {
	RelPath string
	Content []byte
}

func NewPortableComponents() PortableComponents {
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

func NewTargetState(target string) TargetState {
	return TargetState{
		Target:     target,
		Docs:       map[string]string{},
		Components: map[string][]string{},
	}
}

func (tc *TargetState) SetDoc(kind, path string) {
	if tc.Docs == nil {
		tc.Docs = map[string]string{}
	}
	tc.Docs[kind] = filepath.ToSlash(path)
}

func (tc TargetState) DocPath(kind string) string {
	return strings.TrimSpace(tc.Docs[kind])
}

func (tc *TargetState) AddComponent(kind string, paths ...string) {
	if tc.Components == nil {
		tc.Components = map[string][]string{}
	}
	tc.Components[kind] = append(tc.Components[kind], paths...)
}

func (tc TargetState) ComponentPaths(kind string) []string {
	return append([]string(nil), tc.Components[kind]...)
}

func NormalizeManifest(m *Manifest) {
	m.Format = strings.TrimSpace(m.Format)
	if m.Format == "" {
		m.Format = FormatMarker
	}
	m.Name = strings.TrimSpace(m.Name)
	m.Version = strings.TrimSpace(m.Version)
	m.Description = strings.TrimSpace(m.Description)
	for i, target := range m.Targets {
		m.Targets[i] = NormalizeTarget(target)
	}
	slices.Sort(m.Targets)
	m.Targets = slices.Compact(m.Targets)
}

func NormalizeLauncher(l *Launcher) {
	l.Runtime = NormalizeRuntime(l.Runtime)
	l.Entrypoint = strings.TrimSpace(l.Entrypoint)
}

func NormalizeTarget(target string) string {
	return strings.ToLower(strings.TrimSpace(target))
}

func NormalizeRuntime(runtime string) string {
	return strings.ToLower(strings.TrimSpace(runtime))
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
		target = NormalizeTarget(target)
		if target == "codex" {
			return fmt.Errorf("invalid plugin.yaml: target %q was split; use %q for the official plugin bundle and/or %q for repo-local notify integration", target, "codex-package", "codex-runtime")
		}
		if !slices.Contains(supportedTargets, target) {
			return fmt.Errorf("invalid plugin.yaml: unsupported target %q", target)
		}
		if _, ok := seen[target]; ok {
			return fmt.Errorf("invalid plugin.yaml: duplicate target %q", target)
		}
		seen[target] = struct{}{}
	}
	if _, ok := seen["gemini"]; ok && !geminiExtensionNameRe.MatchString(strings.TrimSpace(m.Name)) {
		return fmt.Errorf("invalid plugin.yaml: invalid Gemini extension name %q: use lowercase letters, digits, and hyphens only", strings.TrimSpace(m.Name))
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

func (m Manifest) EnabledTargets() []string {
	out := make([]string, 0, len(m.Targets))
	for _, target := range m.Targets {
		out = append(out, NormalizeTarget(target))
	}
	return out
}

func (m Manifest) SelectedTargets(target string) ([]string, error) {
	target = NormalizeTarget(target)
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

func LoadNativeExtraDoc(root, rel string, format NativeDocFormat) (NativeExtraDoc, error) {
	if strings.TrimSpace(rel) == "" {
		return NativeExtraDoc{}, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return NativeExtraDoc{}, err
	}
	fields, err := ParseNativeExtraDocFields(body, format)
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

func ParseNativeExtraDocFields(body []byte, format NativeDocFormat) (map[string]any, error) {
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

func MergeNativeExtraObject(base map[string]any, doc NativeExtraDoc, label string, managedPaths []string) error {
	if len(doc.Fields) == 0 {
		return nil
	}
	if err := ValidateNativeExtraDocConflicts(doc, label, managedPaths); err != nil {
		return err
	}
	mergeExtraObject(base, doc.Fields)
	return nil
}

func TrimmedExtraBody(doc NativeExtraDoc) []byte {
	return bytes.TrimSpace(doc.Raw)
}

func IsCanonicalCodexNotify(notify []string) bool {
	return len(notify) == 2 && strings.TrimSpace(notify[0]) != "" && strings.TrimSpace(notify[1]) == "notify"
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
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

func asStringMap(value any) (map[string]any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return typed, true
}

func joinPath(prefix, key string) string {
	key = strings.TrimSpace(key)
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
