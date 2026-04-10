package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func TestPublicationDoctorInputDefaultsRootToDot(t *testing.T) {
	t.Parallel()

	got := publicationDoctorInput(publicationDoctorFlags{target: "all", format: "text"}, nil)
	if got.root != "." {
		t.Fatalf("root = %q", got.root)
	}
}

func TestNormalizedPublicationDoctorFormatRejectsUnknownValues(t *testing.T) {
	t.Parallel()

	if got := normalizedPublicationDoctorFormat("yaml"); got != "invalid" {
		t.Fatalf("format = %q", got)
	}
}

func TestWritePublicationDoctorWarningsPrefixesMessages(t *testing.T) {
	t.Parallel()

	cmd := newPublicationDoctorCmd(fakeInspectRunner{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	writePublicationDoctorWarnings(cmd, []pluginmanifest.Warning{{Message: "demo warning"}})
	if !strings.Contains(buf.String(), "Warning: demo warning") {
		t.Fatalf("output = %q", buf.String())
	}
}
