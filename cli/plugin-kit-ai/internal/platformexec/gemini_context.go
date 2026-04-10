package platformexec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func geminiContextCandidates(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) []geminiContextSelection {
	out := collectGeminiContextCandidates(state)
	sortGeminiContextCandidates(out)
	return out
}

func collectGeminiContextCandidates(state pluginmodel.TargetState) []geminiContextSelection {
	var out []geminiContextSelection
	seen := map[string]struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		out = appendGeminiContextCandidate(out, seen, rel)
	}
	return out
}

func appendGeminiContextCandidate(out []geminiContextSelection, seen map[string]struct{}, rel string) []geminiContextSelection {
	artifactName := filepath.Base(rel)
	if artifactName == "" {
		return out
	}
	if _, ok := seen[rel]; ok {
		return out
	}
	seen[rel] = struct{}{}
	return append(out, geminiContextSelection{ArtifactName: artifactName, SourcePath: rel})
}

func sortGeminiContextCandidates(out []geminiContextSelection) {
	slices.SortFunc(out, func(a, b geminiContextSelection) int {
		if cmp := strings.Compare(a.ArtifactName, b.ArtifactName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.SourcePath, b.SourcePath)
	})
}

func candidatesByArtifactName(candidates []geminiContextSelection, name string) []geminiContextSelection {
	var out []geminiContextSelection
	for _, candidate := range candidates {
		if candidate.ArtifactName == name {
			out = append(out, candidate)
		}
	}
	return out
}

func validateGeminiContext(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) []Diagnostic {
	selected := strings.TrimSpace(meta.ContextFileName)
	candidates := geminiContextMatches(graph, state, "")
	if selected != "" {
		return validateNamedGeminiContextSelection(state, selected, geminiContextMatches(graph, state, selected))
	}
	return validateDefaultGeminiContextSelection(candidates, geminiContextMatches(graph, state, "GEMINI.md"))
}

func validateNamedGeminiContextSelection(state pluginmodel.TargetState, selected string, matches []string) []Diagnostic {
	switch len(matches) {
	case 0:
		return []Diagnostic{geminiContextManifestDiagnostic(state.DocPath("package_metadata"), fmt.Sprintf("Gemini context_file_name %q does not resolve to a Gemini-native context source", selected))}
	case 1:
		return nil
	default:
		return []Diagnostic{geminiContextManifestDiagnostic(state.DocPath("package_metadata"), fmt.Sprintf("Gemini context_file_name %q is ambiguous across multiple context sources", selected))}
	}
}

func validateDefaultGeminiContextSelection(candidates, geminiMD []string) []Diagnostic {
	if len(geminiMD) > 1 {
		return []Diagnostic{geminiContextManifestDiagnostic("contexts", "Gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly")}
	}
	if len(geminiMD) == 1 || len(candidates) <= 1 {
		return nil
	}
	return []Diagnostic{geminiContextManifestDiagnostic("contexts", "Gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly")}
}

func geminiContextManifestDiagnostic(path, message string) Diagnostic {
	return Diagnostic{
		Severity: SeverityFailure,
		Code:     CodeManifestInvalid,
		Path:     path,
		Target:   "gemini",
		Message:  message,
	}
}

func geminiContextMatches(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, name string) []string {
	var matches []string
	seen := map[string]struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		rel = filepath.ToSlash(rel)
		if name == "" || filepath.Base(rel) == name {
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			matches = append(matches, rel)
		}
	}
	slices.Sort(matches)
	return matches
}
