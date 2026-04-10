package platformexec

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiGeneratedExtension(
	root string,
	graph pluginmodel.PackageGraph,
	state pluginmodel.TargetState,
	meta geminiPackageMeta,
) ([]Diagnostic, error) {
	extension, ok, diagnostics := readGeminiGeneratedExtension(root)
	if !ok {
		return diagnostics, nil
	}
	extensionDiagnostics, err := validateGeminiExtensionContract(root, graph, state, meta, extension)
	if err != nil {
		return nil, err
	}
	return append(diagnostics, extensionDiagnostics...), nil
}

func readGeminiGeneratedExtension(root string) (importedGeminiExtension, bool, []Diagnostic) {
	extension, ok, err := readImportedGeminiExtension(root)
	if err != nil {
		return importedGeminiExtension{}, false, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json is invalid: %v", err),
		}}
	}
	if !ok {
		return importedGeminiExtension{}, false, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json is not readable",
		}}
	}
	return extension, true, nil
}
