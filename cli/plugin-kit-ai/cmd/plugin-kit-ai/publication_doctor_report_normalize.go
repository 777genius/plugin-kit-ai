package main

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/publicationmodel"
)

func warningMessages(warnings []pluginmanifest.Warning) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, warning.Message)
	}
	return out
}

func normalizePublicationModel(model publicationmodel.Model) publicationmodel.Model {
	if model.Packages == nil {
		model.Packages = []publicationmodel.Package{}
	}
	if model.Channels == nil {
		model.Channels = []publicationmodel.Channel{}
	}
	for i := range model.Packages {
		if model.Packages[i].ChannelFamilies == nil {
			model.Packages[i].ChannelFamilies = []string{}
		}
		if model.Packages[i].AuthoredInputs == nil {
			model.Packages[i].AuthoredInputs = []string{}
		}
		if model.Packages[i].ManagedArtifacts == nil {
			model.Packages[i].ManagedArtifacts = []string{}
		}
	}
	for i := range model.Channels {
		if model.Channels[i].PackageTargets == nil {
			model.Channels[i].PackageTargets = []string{}
		}
	}
	return model
}

