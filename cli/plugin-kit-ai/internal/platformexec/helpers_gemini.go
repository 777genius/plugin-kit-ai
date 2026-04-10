package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/geminimanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"gopkg.in/yaml.v3"
)

type geminiPackageMeta = geminimanifest.PackageMeta

type importedGeminiExtension = geminimanifest.ImportedExtension

func readImportedGeminiExtension(root string) (importedGeminiExtension, bool, error) {
	return geminimanifest.ReadImportedExtension(root)
}

func importedGeminiSettingsArtifacts(values []any) []pluginmodel.Artifact {
	used := map[string]int{}
	var artifacts []pluginmodel.Artifact
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		setting := geminiSetting{}
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
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "settings", filename),
			Content: mustYAML(setting),
		})
	}
	return artifacts
}

func importedGeminiThemeArtifacts(values []any) []pluginmodel.Artifact {
	used := map[string]int{}
	var artifacts []pluginmodel.Artifact
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name, _ := item["name"].(string)
		filename := collisionSafeSlug(name, used) + ".yaml"
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "themes", filename),
			Content: mustYAML(item),
		})
	}
	return artifacts
}

type geminiSetting struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	EnvVar      string `yaml:"env_var" json:"envVar"`
	Sensitive   bool   `yaml:"sensitive" json:"sensitive"`
}

var geminiSettingEnvVarRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var geminiThemeObjectKeys = map[string]struct{}{
	"background": {},
	"text":       {},
	"status":     {},
	"ui":         {},
}

var geminiThemeStringArrayKeys = map[string]struct{}{
	"GradientColors": {},
	"gradient":       {},
}

func collisionSafeSlug(name string, used map[string]int) string {
	base := strings.TrimSpace(strings.ToLower(name))
	if base == "" {
		base = "item"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "item"
	}
	used[slug]++
	if used[slug] == 1 {
		return slug
	}
	return fmt.Sprintf("%s-%d", slug, used[slug])
}

type geminiContextSelection struct {
	ArtifactName string
	SourcePath   string
}

func geminiExtraContextArtifactPath(rel string) string {
	return filepath.ToSlash(filepath.Join("contexts", filepath.Base(rel)))
}

func loadGeminiSettings(root string, rels []string) ([]map[string]any, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	seenNames := map[string]string{}
	seenEnvVars := map[string]string{}
	var settings []map[string]any
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		var raw map[string]any
		if err := yaml.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		var setting geminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			return nil, fmt.Errorf("parse %s: %w", rel, err)
		}
		if message := validateGeminiSettingMap(rel, raw, setting); message != "" {
			return nil, fmt.Errorf("invalid %s: %s", rel, message)
		}
		nameKey := strings.ToLower(strings.TrimSpace(setting.Name))
		if prev, ok := seenNames[nameKey]; ok {
			return nil, fmt.Errorf("invalid %s: Gemini setting name %q duplicates %s", rel, setting.Name, prev)
		}
		seenNames[nameKey] = rel
		envKey := strings.ToLower(strings.TrimSpace(setting.EnvVar))
		if prev, ok := seenEnvVars[envKey]; ok {
			return nil, fmt.Errorf("invalid %s: Gemini setting env_var %q duplicates %s", rel, setting.EnvVar, prev)
		}
		seenEnvVars[envKey] = rel
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
	seenNames := map[string]string{}
	var themes []map[string]any
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
		if message := validateGeminiThemeMap(rel, raw); message != "" {
			return nil, fmt.Errorf("invalid %s: %s", rel, message)
		}
		name = strings.TrimSpace(name)
		nameKey := strings.ToLower(name)
		if prev, ok := seenNames[nameKey]; ok {
			return nil, fmt.Errorf("invalid %s: Gemini theme name %q duplicates %s", rel, name, prev)
		}
		seenNames[nameKey] = rel
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

var geminiYAMLFileRe = regexp.MustCompile(`(?i)\.(yaml|yml)$`)

func readGeminiYAMLMap(root, rel string) ([]byte, map[string]any, error) {
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(body, &raw); err != nil {
		return nil, nil, err
	}
	return body, raw, nil
}

func validateGeminiSettingMap(_ string, raw map[string]any, setting geminiSetting) string {
	_, hasSensitive := raw["sensitive"]
	_, sensitiveIsBool := raw["sensitive"].(bool)
	if strings.TrimSpace(setting.Name) == "" || strings.TrimSpace(setting.Description) == "" || strings.TrimSpace(setting.EnvVar) == "" || !hasSensitive || !sensitiveIsBool {
		return "Gemini settings require string name, description, env_var, and boolean sensitive"
	}
	if !geminiSettingEnvVarRe.MatchString(strings.TrimSpace(setting.EnvVar)) {
		return fmt.Sprintf("Gemini settings require env_var %q to be a valid environment variable name", setting.EnvVar)
	}
	return ""
}

func validateGeminiThemeMap(rel string, raw map[string]any) string {
	name, _ := raw["name"].(string)
	if strings.TrimSpace(name) == "" {
		return "Gemini themes require name"
	}
	if len(raw) <= 1 {
		return "Gemini themes require at least one theme token besides name"
	}
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" || key == "name" || key == "type" {
			continue
		}
		switch {
		case geminiThemeRequiresObject(key):
			if _, ok := value.(map[string]any); !ok {
				return fmt.Sprintf("Gemini theme key %q must be a YAML object", key)
			}
			if message := validateGeminiThemeValue(filepath.ToSlash(filepath.Join(rel, key)), value); message != "" {
				return message
			}
		case geminiThemeRequiresStringArray(key):
			if _, ok := geminiStringSlice(value); !ok {
				return fmt.Sprintf("Gemini theme key %q must be an array of non-empty strings", key)
			}
		default:
			if message := validateGeminiThemeValue(filepath.ToSlash(filepath.Join(rel, key)), value); message != "" {
				return message
			}
		}
	}
	return ""
}

func validateGeminiThemeValue(path string, value any) string {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return fmt.Sprintf("Gemini theme token %q must be a non-empty string", path)
		}
		return ""
	case []any:
		if _, ok := geminiStringSlice(typed); !ok {
			return fmt.Sprintf("Gemini theme token %q must be an array of non-empty strings", path)
		}
		return ""
	case map[string]any:
		if len(typed) == 0 {
			return fmt.Sprintf("Gemini theme object %q may not be empty", path)
		}
		for childKey, childValue := range typed {
			childKey = strings.TrimSpace(childKey)
			if childKey == "" {
				continue
			}
			if geminiThemeRequiresStringArray(childKey) {
				if _, ok := geminiStringSlice(childValue); !ok {
					return fmt.Sprintf("Gemini theme key %q must be an array of non-empty strings", filepath.ToSlash(filepath.Join(path, childKey)))
				}
				continue
			}
			if message := validateGeminiThemeValue(filepath.ToSlash(filepath.Join(path, childKey)), childValue); message != "" {
				return message
			}
		}
		return ""
	default:
		return fmt.Sprintf("Gemini theme token %q must be a non-empty string, string array, or object", path)
	}
}

func geminiThemeRequiresObject(key string) bool {
	_, ok := geminiThemeObjectKeys[key]
	return ok
}

func geminiThemeRequiresStringArray(key string) bool {
	_, ok := geminiThemeStringArrayKeys[key]
	return ok
}
