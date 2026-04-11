package app

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
)

type exportServiceContext struct {
	root     string
	platform string
	graph    pluginmanifest.PackageGraph
	project  runtimecheck.Project
}

func loadExportServiceContext(opts PluginExportOptions) (exportServiceContext, error) {
	input, err := resolveExportServiceInput(opts)
	if err != nil {
		return exportServiceContext{}, err
	}
	graph, err := loadExportServiceGraph(input.root, input.platform)
	if err != nil {
		return exportServiceContext{}, err
	}
	project, err := loadReadyExportProject(input.root, input.platform, graph.Launcher)
	if err != nil {
		return exportServiceContext{}, err
	}
	return exportServiceContext{
		root:     input.root,
		platform: input.platform,
		graph:    graph,
		project:  project,
	}, nil
}
