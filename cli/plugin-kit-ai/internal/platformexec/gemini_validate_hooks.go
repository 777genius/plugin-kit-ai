package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiGeneratedHooks(root string, graph pluginmodel.PackageGraph, authoredHookPaths []string) []Diagnostic {
	const generatedHooksPath = "hooks/hooks.json"
	var diagnostics []Diagnostic
	body, err := os.ReadFile(filepath.Join(root, generatedHooksPath))
	if len(authoredHookPaths) > 0 {
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json is not readable",
			})
			return diagnostics
		}
		renderedHooks, err := parseGeminiHooks(body)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini generated hooks file %s is invalid: %v", generatedHooksPath, err),
			})
			return diagnostics
		}
		authoredBody, readErr := os.ReadFile(filepath.Join(root, authoredHookPaths[0]))
		if readErr != nil {
			return diagnostics
		}
		authoredHooks, parseErr := parseGeminiHooks(authoredBody)
		if parseErr != nil {
			return diagnostics
		}
		if !jsonDocumentsEqual(authoredHooks, renderedHooks) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json does not match authored targets/gemini/hooks/hooks.json",
			})
		}
		return diagnostics
	}
	if !geminiUsesGeneratedHooks(graph, pluginmodel.TargetState{Target: "gemini"}) {
		if err == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json may not exist when no authored hooks or generated launcher hooks are expected",
			})
		}
		return diagnostics
	}
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  "Gemini generated hooks/hooks.json is not readable",
		})
		return diagnostics
	}
	renderedHooks, err := parseGeminiHooks(body)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini generated hooks file %s is invalid: %v", generatedHooksPath, err),
		})
		return diagnostics
	}
	expectedHooks, err := parseGeminiHooks(defaultGeminiHooks(strings.TrimSpace(graph.Launcher.Entrypoint)))
	if err != nil {
		return diagnostics
	}
	if !jsonDocumentsEqual(expectedHooks, renderedHooks) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  "Gemini generated hooks/hooks.json does not match the managed launcher-derived hooks projection",
		})
	}
	return diagnostics
}

func validateGeminiHookFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini JSON asset %s is not readable: %v", rel, err),
			})
			continue
		}
		var discard map[string]any
		if err := json.Unmarshal(body, &discard); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		hooks, ok := discard["hooks"].(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s must define a top-level hooks object", rel),
			})
			continue
		}
		if hooks == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s must define a top-level hooks object", rel),
			})
		}
	}
	return diagnostics
}

func validateGeminiHookEntrypointConsistency(root string, rels []string, entrypoint string) []Diagnostic {
	if strings.TrimSpace(entrypoint) == "" {
		return nil
	}
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		mismatches, err := validateGeminiHookEntrypoints(body, entrypoint)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		for _, msg := range mismatches {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeEntrypointMismatch,
				Path:     rel,
				Target:   "gemini",
				Message:  msg,
			})
		}
	}
	return diagnostics
}
