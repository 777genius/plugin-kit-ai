package codexmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

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
