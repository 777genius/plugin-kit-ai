package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func geminiContextArtifacts(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (string, pluginmodel.Artifact, []pluginmodel.Artifact, bool, error) {
	selected, ok, err := selectGeminiPrimaryContext(graph, state, meta)
	if err != nil {
		return "", pluginmodel.Artifact{}, nil, false, err
	}
	if !ok {
		return "", pluginmodel.Artifact{}, nil, false, nil
	}
	body, err := os.ReadFile(filepath.Join(root, selected.SourcePath))
	if err != nil {
		return "", pluginmodel.Artifact{}, nil, false, err
	}
	primary := pluginmodel.Artifact{RelPath: selected.ArtifactName, Content: body}
	var extra []pluginmodel.Artifact
	for _, rel := range state.ComponentPaths("contexts") {
		if rel == selected.SourcePath {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", pluginmodel.Artifact{}, nil, false, err
		}
		extra = append(extra, pluginmodel.Artifact{
			RelPath: geminiExtraContextArtifactPath(rel),
			Content: body,
		})
	}
	slices.SortFunc(extra, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return selected.ArtifactName, primary, extra, true, nil
}

func selectGeminiPrimaryContext(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (geminiContextSelection, bool, error) {
	candidates := geminiContextCandidates(graph, state)
	selected := strings.TrimSpace(meta.ContextFileName)
	if selected != "" {
		matches := candidatesByArtifactName(candidates, selected)
		switch len(matches) {
		case 0:
			return geminiContextSelection{}, false, fmt.Errorf("gemini context_file_name %q does not resolve to a Gemini-native context source", selected)
		case 1:
			return matches[0], true, nil
		default:
			return geminiContextSelection{}, false, fmt.Errorf("gemini context_file_name %q is ambiguous across multiple context sources", selected)
		}
	}
	fallback := candidatesByArtifactName(candidates, "GEMINI.md")
	switch len(fallback) {
	case 1:
		return fallback[0], true, nil
	case 0:
		if len(candidates) == 0 {
			return geminiContextSelection{}, false, nil
		}
		if len(candidates) == 1 {
			return candidates[0], true, nil
		}
		return geminiContextSelection{}, false, fmt.Errorf("gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly")
	default:
		return geminiContextSelection{}, false, fmt.Errorf("gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly")
	}
}

func geminiContextCandidates(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) []geminiContextSelection {
	var out []geminiContextSelection
	seen := map[string]struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		artifactName := filepath.Base(rel)
		if artifactName == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, geminiContextSelection{ArtifactName: artifactName, SourcePath: rel})
	}
	slices.SortFunc(out, func(a, b geminiContextSelection) int {
		if cmp := strings.Compare(a.ArtifactName, b.ArtifactName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.SourcePath, b.SourcePath)
	})
	return out
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
		matches := geminiContextMatches(graph, state, selected)
		switch len(matches) {
		case 0:
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     state.DocPath("package_metadata"),
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini context_file_name %q does not resolve to a Gemini-native context source", selected),
			}}
		case 1:
			return nil
		default:
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     state.DocPath("package_metadata"),
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini context_file_name %q is ambiguous across multiple context sources", selected),
			}}
		}
	}
	geminiMD := geminiContextMatches(graph, state, "GEMINI.md")
	if len(geminiMD) > 1 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     "contexts",
			Target:   "gemini",
			Message:  "Gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly",
		}}
	}
	if len(geminiMD) == 1 || len(candidates) <= 1 {
		return nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeManifestInvalid,
		Path:     "contexts",
		Target:   "gemini",
		Message:  "Gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly",
	}}
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
