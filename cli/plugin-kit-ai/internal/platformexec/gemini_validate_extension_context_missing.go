package platformexec

import (
	"fmt"
	"strings"
)

func validateGeminiUnexpectedContext(extension importedGeminiExtension) []Diagnostic {
	if strings.TrimSpace(extension.Meta.ContextFileName) == "" {
		return nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     "gemini-extension.json",
		Target:   "gemini",
		Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q without an authored primary context", strings.TrimSpace(extension.Meta.ContextFileName)),
	}}
}
