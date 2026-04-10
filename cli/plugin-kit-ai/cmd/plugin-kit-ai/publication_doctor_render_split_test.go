package main

import (
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
)

func TestPublicationDoctorTextLinesAppendsLocalRootLines(t *testing.T) {
	t.Parallel()

	got := publicationDoctorTextLines(publicationDiagnosis{
		Lines: []string{"status"},
	}, &app.PluginPublicationVerifyRootResult{
		Lines: []string{"local-root"},
	})
	if len(got) != 2 || got[0] != "status" || got[1] != "local-root" {
		t.Fatalf("lines = %+v", got)
	}
}
