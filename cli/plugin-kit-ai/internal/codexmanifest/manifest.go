package codexmanifest

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	SkillsRef     = "./skills/"
	MCPServersRef = "./.mcp.json"
	AppsRef       = "./.app.json"
)

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
	LegacyAppsRef bool
	Interface     map[string]any
	Extra         map[string]any
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
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	if err := ValidateInterfaceDoc(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func ParseAppManifestDoc(body []byte) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
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

func DecodeImportedPluginManifest(body []byte) (ImportedPluginManifest, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return ImportedPluginManifest{}, err
	}
	out := ImportedPluginManifest{}
	if value, ok := raw["name"].(string); ok {
		out.Name = strings.TrimSpace(value)
	}
	if value, ok := raw["version"].(string); ok {
		out.Version = strings.TrimSpace(value)
	}
	if value, ok := raw["description"].(string); ok {
		out.Description = strings.TrimSpace(value)
	}
	if value, ok := decodeAuthor(raw["author"]); ok {
		out.PackageMeta.Author = value
	}
	if value, ok := raw["homepage"].(string); ok {
		out.PackageMeta.Homepage = strings.TrimSpace(value)
	}
	if value, ok := raw["repository"].(string); ok {
		out.PackageMeta.Repository = strings.TrimSpace(value)
	}
	if value, ok := raw["license"].(string); ok {
		out.PackageMeta.License = strings.TrimSpace(value)
	}
	if values, ok := raw["keywords"].([]any); ok {
		out.PackageMeta.Keywords = normalizeJSONStrings(values)
	}
	if value, ok := raw["skills"].(string); ok {
		out.SkillsPath = strings.TrimSpace(value)
	}
	if value, ok := raw["mcpServers"].(string); ok {
		out.MCPServersRef = strings.TrimSpace(value)
	}
	if value, ok := raw["apps"].(string); ok {
		out.AppsRef = strings.TrimSpace(value)
	}
	if values, ok := raw["apps"].([]any); ok && len(out.AppsRef) == 0 {
		if appRef, ok := decodeLegacyAppsRef(values); ok {
			out.AppsRef = appRef
			out.LegacyAppsRef = true
		}
	}
	if value, ok := raw["interface"].(map[string]any); ok {
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

func decodeAuthor(value any) (*Author, bool) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, false
		}
		return &Author{Name: text}, true
	case map[string]any:
		author := &Author{}
		if item, ok := typed["name"].(string); ok {
			author.Name = item
		}
		if item, ok := typed["email"].(string); ok {
			author.Email = item
		}
		if item, ok := typed["url"].(string); ok {
			author.URL = item
		}
		author.Normalize()
		if author.Empty() {
			return nil, false
		}
		return author, true
	default:
		return nil, false
	}
}

func decodeLegacyAppsRef(values []any) (string, bool) {
	items := normalizeJSONStrings(values)
	if len(items) != 1 {
		return "", false
	}
	return items[0], true
}

func normalizeJSONStrings(values []any) []string {
	out := make([]string, 0, len(values))
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
