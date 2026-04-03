package codexmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	PluginDir      = ".codex-plugin"
	PluginFileName = "plugin.json"
	AppFileName    = ".app.json"
	MCPFileName    = ".mcp.json"
	SkillsRef      = "./skills/"
	MCPServersRef  = "./.mcp.json"
	AppsRef        = "./.app.json"
)

func PluginManifestPath() string {
	return filepath.ToSlash(filepath.Join(PluginDir, PluginFileName))
}

func AppManifestPath() string {
	return filepath.ToSlash(AppFileName)
}

func MCPManifestPath() string {
	return filepath.ToSlash(MCPFileName)
}

type Author struct {
	Name  string `yaml:"name,omitempty" json:"name,omitempty"`
	Email string `yaml:"email,omitempty" json:"email,omitempty"`
	URL   string `yaml:"url,omitempty" json:"url,omitempty"`
}

type PackageMeta struct {
	Author     *Author  `yaml:"author,omitempty" json:"author,omitempty"`
	Homepage   string   `yaml:"homepage,omitempty" json:"homepage,omitempty"`
	Repository string   `yaml:"repository,omitempty" json:"repository,omitempty"`
	License    string   `yaml:"license,omitempty" json:"license,omitempty"`
	Keywords   []string `yaml:"keywords,omitempty" json:"keywords,omitempty"`
}

type ImportedPluginManifest struct {
	Name          string
	Version       string
	Description   string
	PackageMeta   PackageMeta
	SkillsPath    string
	MCPServersRef string
	AppsRef       string
	Interface     map[string]any
	Extra         map[string]any
}

type PluginDirLayoutError struct {
	Path string
}

func (e *PluginDirLayoutError) Error() string {
	return fmt.Sprintf("Codex plugin directory %s may only contain %s (unexpected %s)", PluginDir, PluginFileName, e.Path)
}

func (a *Author) Normalize() {
	if a == nil {
		return
	}
	a.Name = strings.TrimSpace(a.Name)
	a.Email = strings.TrimSpace(a.Email)
	a.URL = strings.TrimSpace(a.URL)
}

func (a *Author) Empty() bool {
	if a == nil {
		return true
	}
	return strings.TrimSpace(a.Name) == "" &&
		strings.TrimSpace(a.Email) == "" &&
		strings.TrimSpace(a.URL) == ""
}

func (m *PackageMeta) Normalize() {
	if m == nil {
		return
	}
	if m.Author != nil {
		m.Author.Normalize()
		if m.Author.Empty() {
			m.Author = nil
		}
	}
	m.Homepage = strings.TrimSpace(m.Homepage)
	m.Repository = strings.TrimSpace(m.Repository)
	m.License = strings.TrimSpace(m.License)
	m.Keywords = normalizeStrings(m.Keywords)
}

func (m PackageMeta) Empty() bool {
	return m.Author == nil &&
		strings.TrimSpace(m.Homepage) == "" &&
		strings.TrimSpace(m.Repository) == "" &&
		strings.TrimSpace(m.License) == "" &&
		len(m.Keywords) == 0
}

func (m PackageMeta) Apply(doc map[string]any) {
	if doc == nil {
		return
	}
	if m.Author != nil && !m.Author.Empty() {
		author := map[string]any{}
		if strings.TrimSpace(m.Author.Name) != "" {
			author["name"] = m.Author.Name
		}
		if strings.TrimSpace(m.Author.Email) != "" {
			author["email"] = m.Author.Email
		}
		if strings.TrimSpace(m.Author.URL) != "" {
			author["url"] = m.Author.URL
		}
		if len(author) > 0 {
			doc["author"] = author
		}
	}
	if strings.TrimSpace(m.Homepage) != "" {
		doc["homepage"] = m.Homepage
	}
	if strings.TrimSpace(m.Repository) != "" {
		doc["repository"] = m.Repository
	}
	if strings.TrimSpace(m.License) != "" {
		doc["license"] = m.License
	}
	if len(m.Keywords) > 0 {
		doc["keywords"] = append([]string(nil), m.Keywords...)
	}
}

func ParseInterfaceDoc(body []byte) (map[string]any, error) {
	doc, err := parseJSONObjectDoc(body, "Codex interface doc")
	if err != nil {
		return nil, err
	}
	if err := ValidateInterfaceDoc(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func ParseAppManifestDoc(body []byte) (map[string]any, error) {
	return parseJSONObjectDoc(body, "Codex app manifest")
}

func ValidatePluginDirLayout(root string) error {
	entries, err := os.ReadDir(filepath.Join(root, PluginDir))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == PluginFileName {
			continue
		}
		return &PluginDirLayoutError{Path: filepath.ToSlash(filepath.Join(PluginDir, entry.Name()))}
	}
	return nil
}

func ReadImportedPluginManifest(root string) (ImportedPluginManifest, []byte, error) {
	body, err := os.ReadFile(filepath.Join(root, PluginDir, PluginFileName))
	if err != nil {
		return ImportedPluginManifest{}, nil, err
	}
	if err := ValidatePluginDirLayout(root); err != nil {
		return ImportedPluginManifest{}, nil, err
	}
	out, err := DecodeImportedPluginManifest(body)
	if err != nil {
		return ImportedPluginManifest{}, nil, err
	}
	return out, body, nil
}

func UnexpectedBundleSidecars(root string, manifest ImportedPluginManifest) []string {
	var paths []string
	if strings.TrimSpace(manifest.AppsRef) == "" && fileExists(filepath.Join(root, AppFileName)) {
		paths = append(paths, AppManifestPath())
	}
	if strings.TrimSpace(manifest.MCPServersRef) == "" && fileExists(filepath.Join(root, MCPFileName)) {
		paths = append(paths, MCPManifestPath())
	}
	return paths
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func AppManifestEnabled(doc map[string]any) bool {
	return len(doc) > 0
}

func ValidateInterfaceDoc(doc map[string]any) error {
	if doc == nil {
		return nil
	}
	value, ok := doc["defaultPrompt"]
	if !ok {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("interface.defaultPrompt must be an array of strings")
	}
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return fmt.Errorf("interface.defaultPrompt[%d] must be a string", i)
		}
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("interface.defaultPrompt[%d] must not be empty", i)
		}
	}
	return nil
}

func parseJSONObjectDoc(body []byte, label string) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s must be valid JSON: %w", label, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%s must contain a single JSON object", label)
		}
		return nil, fmt.Errorf("%s must be valid JSON: %w", label, err)
	}
	doc, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func DecodeImportedPluginManifest(body []byte) (ImportedPluginManifest, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return ImportedPluginManifest{}, err
	}
	out := ImportedPluginManifest{}
	if value, ok, err := decodeJSONStringField(raw, "name"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.Name = value
	}
	if value, ok, err := decodeJSONStringField(raw, "version"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.Version = value
	}
	if value, ok, err := decodeJSONStringField(raw, "description"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.Description = value
	}
	if value, ok, err := decodeAuthorField(raw); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.PackageMeta.Author = value
	}
	if value, ok, err := decodeJSONStringField(raw, "homepage"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.PackageMeta.Homepage = value
	}
	if value, ok, err := decodeJSONStringField(raw, "repository"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.PackageMeta.Repository = value
	}
	if value, ok, err := decodeJSONStringField(raw, "license"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.PackageMeta.License = value
	}
	if values, ok, err := decodeJSONStringArrayField(raw, "keywords"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.PackageMeta.Keywords = values
	}
	if value, ok, err := decodeJSONStringField(raw, "skills"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.SkillsPath = value
	}
	if value, ok, err := decodeJSONStringField(raw, "mcpServers"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.MCPServersRef = value
	}
	if value, ok, err := decodeJSONStringField(raw, "apps"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		out.AppsRef = value
	}
	if value, ok, err := decodeJSONObjectField(raw, "interface"); err != nil {
		return ImportedPluginManifest{}, err
	} else if ok {
		if err := ValidateInterfaceDoc(value); err != nil {
			return ImportedPluginManifest{}, err
		}
		out.Interface = value
	}

	delete(raw, "name")
	delete(raw, "version")
	delete(raw, "description")
	delete(raw, "author")
	delete(raw, "homepage")
	delete(raw, "repository")
	delete(raw, "license")
	delete(raw, "keywords")
	delete(raw, "skills")
	delete(raw, "mcpServers")
	delete(raw, "apps")
	delete(raw, "interface")

	out.PackageMeta.Normalize()
	if len(raw) > 0 {
		out.Extra = raw
	}
	return out, nil
}

func decodeAuthorField(raw map[string]any) (*Author, bool, error) {
	value, ok := raw["author"]
	if !ok || value == nil {
		return nil, false, nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("Codex plugin author must be a JSON object")
	}
	author := &Author{}
	if item, ok, err := decodeJSONStringMapField(typed, "name"); err != nil {
		return nil, false, fmt.Errorf("Codex plugin author.name must be a string")
	} else if ok {
		author.Name = item
	}
	if item, ok, err := decodeJSONStringMapField(typed, "email"); err != nil {
		return nil, false, fmt.Errorf("Codex plugin author.email must be a string")
	} else if ok {
		author.Email = item
	}
	if item, ok, err := decodeJSONStringMapField(typed, "url"); err != nil {
		return nil, false, fmt.Errorf("Codex plugin author.url must be a string")
	} else if ok {
		author.URL = item
	}
	author.Normalize()
	if author.Empty() {
		return nil, false, nil
	}
	return author, true, nil
}

func decodeJSONStringField(raw map[string]any, field string) (string, bool, error) {
	value, ok := raw[field]
	if !ok || value == nil {
		return "", false, nil
	}
	typed, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("Codex plugin %s must be a string", field)
	}
	return strings.TrimSpace(typed), true, nil
}

func decodeJSONStringArrayField(raw map[string]any, field string) ([]string, bool, error) {
	value, ok := raw[field]
	if !ok || value == nil {
		return nil, false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, false, fmt.Errorf("Codex plugin %s must be an array of strings", field)
	}
	out := make([]string, 0, len(items))
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false, fmt.Errorf("Codex plugin %s[%d] must be a string", field, i)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out, true, nil
}

func decodeJSONObjectField(raw map[string]any, field string) (map[string]any, bool, error) {
	value, ok := raw[field]
	if !ok || value == nil {
		return nil, false, nil
	}
	doc, ok := value.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("Codex plugin %s must be a JSON object", field)
	}
	return doc, true, nil
}

func decodeJSONStringMapField(raw map[string]any, field string) (string, bool, error) {
	value, ok := raw[field]
	if !ok || value == nil {
		return "", false, nil
	}
	typed, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string", field)
	}
	return strings.TrimSpace(typed), true, nil
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
