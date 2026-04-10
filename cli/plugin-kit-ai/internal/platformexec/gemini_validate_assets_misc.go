package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

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
