package platformexec

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiRenderReady(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) error {
	diagnostics, err := validateGeminiRenderReadyDiagnostics(root, graph, state, meta)
	if err != nil {
		return err
	}
	if failure, ok := firstDiagnosticMessage(diagnostics, SeverityFailure); ok {
		return fmt.Errorf(failure)
	}
	return nil
}

func firstDiagnosticMessage(diagnostics []Diagnostic, severity DiagnosticSeverity) (string, bool) {
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severity {
			return diagnostic.Message, true
		}
	}
	return "", false
}
