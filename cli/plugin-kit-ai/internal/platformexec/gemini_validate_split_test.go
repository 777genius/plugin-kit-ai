package platformexec

import (
	"strings"
	"testing"
)

func TestValidateGeminiDirNameReportsBasenameMismatch(t *testing.T) {
	t.Parallel()

	diagnostics := validateGeminiDirName(t.TempDir(), "demo-extension")
	if text := diagnosticsText(diagnostics); !strings.Contains(text, "does not match extension name") {
		t.Fatalf("diagnostics missing dir-name mismatch:\n%s", text)
	}
}

func TestReadGeminiGeneratedExtensionReportsUnreadableManifest(t *testing.T) {
	t.Parallel()

	_, ok, diagnostics := readGeminiGeneratedExtension(t.TempDir())
	if ok {
		t.Fatal("expected unreadable extension")
	}
	if text := diagnosticsText(diagnostics); !strings.Contains(text, "is not readable") {
		t.Fatalf("diagnostics missing unreadable manifest failure:\n%s", text)
	}
}
