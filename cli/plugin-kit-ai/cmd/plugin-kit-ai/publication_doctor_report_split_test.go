package main

import (
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func TestNormalizePublicationModelInitializesNilSlices(t *testing.T) {
	t.Parallel()
	model := normalizePublicationModel(publicationmodel.Model{
		Packages: []publicationmodel.Package{{}},
		Channels: []publicationmodel.Channel{{}},
	})
	if model.Packages[0].ChannelFamilies == nil || model.Packages[0].AuthoredInputs == nil || model.Packages[0].ManagedArtifacts == nil {
		t.Fatalf("package slices = %+v", model.Packages[0])
	}
	if model.Channels[0].PackageTargets == nil {
		t.Fatalf("channel slices = %+v", model.Channels[0])
	}
}

func TestWarningMessagesProjectsWarningBodies(t *testing.T) {
	t.Parallel()
	got := warningMessages([]pluginmanifest.Warning{{Message: "first"}, {Message: "second"}})
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("warnings = %+v", got)
	}
}
