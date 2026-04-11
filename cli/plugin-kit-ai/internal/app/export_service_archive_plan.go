package app

import (
	"slices"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func buildExportArchiveFiles(ctx exportServiceContext, output string) ([]string, string, error) {
	generated, err := pluginmanifest.Generate(ctx.root, ctx.platform)
	if err != nil {
		return nil, "", err
	}

	files, err := exportFileList(ctx.root, ctx.graph, ctx.project, generated.Artifacts)
	if err != nil {
		return nil, "", err
	}
	outputPath := exportOutputPath(ctx.root, ctx.graph.Manifest.Name, ctx.platform, ctx.graph.Launcher.Runtime, output)
	if rel, ok := relWithinRoot(ctx.root, outputPath); ok {
		files = slices.DeleteFunc(files, func(path string) bool { return path == rel })
	}
	return files, outputPath, nil
}
