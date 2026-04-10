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
		return validateGeminiExpectedContextContract(root, expected, extension), nil
	}
	return validateGeminiUnexpectedContextContract(extension), nil
}

func validateGeminiExpectedContextContract(root string, expected geminiContextSelection, extension importedGeminiExtension) []Diagnostic {
	return validateGeminiExpectedContext(root, expected, extension)
}

func validateGeminiUnexpectedContextContract(extension importedGeminiExtension) []Diagnostic {
	return validateGeminiUnexpectedContext(extension)
}
