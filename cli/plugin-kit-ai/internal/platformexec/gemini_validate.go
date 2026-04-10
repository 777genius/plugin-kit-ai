package platformexec

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (geminiAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	meta, _, err := readYAMLDoc[geminiPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	hookPaths := state.ComponentPaths("hooks")
	diagnostics, err := validateGeminiAuthoredSurfaces(root, graph, state, meta, hookPaths)
	if err != nil {
		return nil, err
	}
	generatedDiagnostics, err := validateGeminiGeneratedExtension(root, graph, state, meta)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, generatedDiagnostics...)
	return diagnostics, nil
}
