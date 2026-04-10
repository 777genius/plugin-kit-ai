package platformexec

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiExtensionContextContract(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta, extension importedGeminiExtension) ([]Diagnostic, error) {
	expected, ok, err := resolveGeminiExpectedContext(graph, state, meta)
	if err != nil {
		return nil, err
	}
	if ok {
		return validateGeminiExpectedContext(root, expected, extension), nil
	}
	return validateGeminiUnexpectedContext(extension), nil
}
