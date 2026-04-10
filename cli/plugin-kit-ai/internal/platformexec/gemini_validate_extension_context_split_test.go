package platformexec

import (
	"strings"
	"testing"
)

func TestValidateGeminiContextFileNameProjectionRejectsMismatch(t *testing.T) {
	t.Parallel()

	diagnostics := validateGeminiContextFileNameProjection(geminiContextSelection{ArtifactName: "GEMINI.md"}, importedGeminiExtension{
		Meta: geminiPackageMeta{ContextFileName: "ALT.md"},
	})
	if text := diagnosticsText(diagnostics); !strings.Contains(text, `sets contextFileName "ALT.md"; expected "GEMINI.md"`) {
		t.Fatalf("diagnostics missing context projection failure:\n%s", text)
	}
}

func TestValidateGeminiContextFileReadableRejectsMissingPrimaryContext(t *testing.T) {
	t.Parallel()

	diagnostics := validateGeminiContextFileReadable(t.TempDir(), geminiContextSelection{ArtifactName: "GEMINI.md"})
	if text := diagnosticsText(diagnostics); !strings.Contains(text, "Gemini primary context file GEMINI.md is not readable") {
		t.Fatalf("diagnostics missing missing-context failure:\n%s", text)
	}
}
