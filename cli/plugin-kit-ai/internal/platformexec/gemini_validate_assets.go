package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
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
		nameKey := strings.ToLower(strings.TrimSpace(setting.Name))
		if prev, ok := seenNames[nameKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates setting name %q already declared in %s", rel, setting.Name, prev),
			})
		} else {
			seenNames[nameKey] = rel
		}
		envKey := strings.ToLower(strings.TrimSpace(setting.EnvVar))
		if prev, ok := seenEnvVars[envKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates env_var %q already declared in %s", rel, setting.EnvVar, prev),
			})
		} else {
			seenEnvVars[envKey] = rel
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
		nameKey := strings.ToLower(name)
		if prev, ok := seenNames[nameKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s duplicates theme name %q already declared in %s", rel, name, prev),
			})
			continue
		}
		seenNames[nameKey] = rel
	}
	return diagnostics
}

func validateGeminiPolicies(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini policy file %s is not readable: %v", rel, err),
			})
			continue
		}
		text := string(body)
		for _, key := range []string{"allow", "yolo"} {
			if strings.Contains(text, key+" =") {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityWarning,
					Code:     CodeGeminiPolicyIgnored,
					Path:     rel,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension policies ignore %q at extension tier", key),
				})
			}
		}
	}
	return diagnostics
}

func validateGeminiCommands(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".toml" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s must use the .toml extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s is not readable: %v", rel, err),
			})
			continue
		}
		var discard map[string]any
		if err := toml.Unmarshal(body, &discard); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s is invalid TOML: %v", rel, err),
			})
		}
	}
	return diagnostics
}
