package platformexec

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func validateGeminiSettings(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	seenNames := map[string]string{}
	seenEnvVars := map[string]string{}
	for _, rel := range rels {
		body, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		var setting geminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		if message := validateGeminiSettingMap(raw, setting); message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s: %s", rel, message),
			})
			continue
		}
		if prev, ok := seenNames[strings.ToLower(strings.TrimSpace(setting.Name))]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates setting name %q already declared in %s", rel, setting.Name, prev),
			})
		} else {
			seenNames[strings.ToLower(strings.TrimSpace(setting.Name))] = rel
		}
		if prev, ok := seenEnvVars[strings.ToLower(strings.TrimSpace(setting.EnvVar))]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates env_var %q already declared in %s", rel, setting.EnvVar, prev),
			})
		} else {
			seenEnvVars[strings.ToLower(strings.TrimSpace(setting.EnvVar))] = rel
		}
	}
	return diagnostics
}

func validateGeminiThemes(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	seenNames := map[string]string{}
	for _, rel := range rels {
		_, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		name, _ := raw["name"].(string)
		if message := validateGeminiThemeMap(rel, raw); message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s: %s", rel, message),
			})
			continue
		}
		name = strings.TrimSpace(name)
		if prev, ok := seenNames[strings.ToLower(name)]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s duplicates theme name %q already declared in %s", rel, name, prev),
			})
			continue
		}
		seenNames[strings.ToLower(name)] = rel
	}
	return diagnostics
}
