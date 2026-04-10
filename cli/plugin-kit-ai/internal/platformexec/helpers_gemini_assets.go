package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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
		if message := validateGeminiSettingMap(raw, setting); message != "" {
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
		_, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			return nil, err
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
		themes = append(themes, normalizeGeminiThemeMap(raw))
	}
	return themes, nil
}

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

func normalizeGeminiThemeMap(raw map[string]any) map[string]any {
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
	return theme
}

func validateGeminiSettingMap(raw map[string]any, setting geminiSetting) string {
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
