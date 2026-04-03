package geminimanifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type PackageMeta struct {
	ContextFileName string   `yaml:"context_file_name,omitempty"`
	ExcludeTools    []string `yaml:"exclude_tools,omitempty"`
	MigratedTo      string   `yaml:"migrated_to,omitempty"`
	PlanDirectory   string   `yaml:"plan_directory,omitempty"`
}

type ImportedExtension struct {
	Name        string
	Version     string
	Description string
	Meta        PackageMeta
	MCPServers  map[string]any
	Settings    []any
	Themes      []any
	Extra       map[string]any
}

func DecodeImportedExtension(body []byte) (ImportedExtension, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return ImportedExtension{}, err
	}
	return decodeImportedExtensionObject(raw)
}

func ReadImportedExtension(root string) (ImportedExtension, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, "gemini-extension.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return ImportedExtension{}, false, nil
		}
		return ImportedExtension{}, false, err
	}
	data, err := DecodeImportedExtension(body)
	if err != nil {
		return ImportedExtension{}, false, err
	}
	return data, true, nil
}

func decodeImportedExtensionObject(raw map[string]any) (ImportedExtension, error) {
	out := ImportedExtension{}

	name, err := optionalNonEmptyStringField(raw, "name")
	if err != nil {
		return ImportedExtension{}, err
	}
	out.Name = name

	version, err := optionalNonEmptyStringField(raw, "version")
	if err != nil {
		return ImportedExtension{}, err
	}
	out.Version = version

	description, err := optionalNonEmptyStringField(raw, "description")
	if err != nil {
		return ImportedExtension{}, err
	}
	out.Description = description

	contextFileName, err := optionalNonEmptyStringField(raw, "contextFileName")
	if err != nil {
		return ImportedExtension{}, err
	}
	out.Meta.ContextFileName = contextFileName

	migratedTo, err := optionalNonEmptyStringField(raw, "migratedTo")
	if err != nil {
		return ImportedExtension{}, err
	}
	out.Meta.MigratedTo = migratedTo

	if servers, ok := raw["mcpServers"]; ok {
		value, ok := servers.(map[string]any)
		if !ok {
			return ImportedExtension{}, fmt.Errorf("Gemini extension field %q must be a JSON object", "mcpServers")
		}
		if len(value) > 0 {
			out.MCPServers = value
		}
	}

	if values, ok := raw["excludeTools"]; ok {
		items, err := stringArrayField(values, "excludeTools")
		if err != nil {
			return ImportedExtension{}, err
		}
		out.Meta.ExcludeTools = items
	}

	if planValue, ok := raw["plan"]; ok {
		plan, ok := planValue.(map[string]any)
		if !ok {
			return ImportedExtension{}, fmt.Errorf("Gemini extension field %q must be a JSON object", "plan")
		}
		if directory, ok := plan["directory"]; ok {
			text, ok := directory.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return ImportedExtension{}, fmt.Errorf("Gemini extension field %q must be a non-empty string when provided", "plan.directory")
			}
			out.Meta.PlanDirectory = text
			delete(plan, "directory")
		}
		if len(plan) == 0 {
			delete(raw, "plan")
		} else {
			raw["plan"] = plan
		}
	}

	if values, ok := raw["settings"]; ok {
		items, err := objectArrayField(values, "settings")
		if err != nil {
			return ImportedExtension{}, err
		}
		for i, item := range items {
			doc, _ := item.(map[string]any)
			if err := validateImportedSettingObject(doc); err != nil {
				return ImportedExtension{}, fmt.Errorf("Gemini extension field %q contains an invalid object at index %d: %w", "settings", i, err)
			}
		}
		out.Settings = items
	}

	if values, ok := raw["themes"]; ok {
		items, err := objectArrayField(values, "themes")
		if err != nil {
			return ImportedExtension{}, err
		}
		for i, item := range items {
			doc, _ := item.(map[string]any)
			if err := validateImportedThemeObject(doc); err != nil {
				return ImportedExtension{}, fmt.Errorf("Gemini extension field %q contains an invalid object at index %d: %w", "themes", i, err)
			}
		}
		out.Themes = items
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

func optionalNonEmptyStringField(raw map[string]any, key string) (string, error) {
	value, ok := raw[key]
	if !ok || value == nil {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("Gemini extension field %q must be a string", key)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	return text, nil
}

func stringArrayField(value any, key string) ([]string, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("Gemini extension field %q must be an array of strings", key)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("Gemini extension field %q must contain non-empty strings (invalid entry at index %d)", key, i)
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, nil
}

func objectArrayField(value any, key string) ([]any, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("Gemini extension field %q must be an array of JSON objects", key)
	}
	out := make([]any, 0, len(raw))
	for i, item := range raw {
		if _, ok := item.(map[string]any); !ok {
			return nil, fmt.Errorf("Gemini extension field %q must contain JSON objects (invalid entry at index %d)", key, i)
		}
		out = append(out, item)
	}
	return out, nil
}

var importedSettingEnvVarRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateImportedSettingObject(doc map[string]any) error {
	name, _ := doc["name"].(string)
	description, _ := doc["description"].(string)
	envVar, _ := doc["envVar"].(string)
	_, hasSensitive := doc["sensitive"]
	if strings.TrimSpace(name) == "" || strings.TrimSpace(description) == "" || strings.TrimSpace(envVar) == "" || !hasSensitive {
		return fmt.Errorf("settings objects require non-empty string name, description, envVar, and boolean sensitive")
	}
	if _, ok := doc["sensitive"].(bool); !ok {
		return fmt.Errorf("settings objects require non-empty string name, description, envVar, and boolean sensitive")
	}
	if !importedSettingEnvVarRe.MatchString(strings.TrimSpace(envVar)) {
		return fmt.Errorf("settings envVar %q must be a valid environment variable name", envVar)
	}
	return nil
}

func validateImportedThemeObject(doc map[string]any) error {
	name, _ := doc["name"].(string)
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("themes objects require non-empty string name")
	}
	if len(doc) <= 1 {
		return fmt.Errorf("themes objects require at least one theme token besides name")
	}
	return nil
}
