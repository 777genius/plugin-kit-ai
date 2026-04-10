package platformexec

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

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
