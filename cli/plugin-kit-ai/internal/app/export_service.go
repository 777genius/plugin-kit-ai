package app

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
	"slices"
)

func (PluginService) Export(opts PluginExportOptions) (PluginExportResult, error) {
	ctx, err := loadExportServiceContext(opts)
	if err != nil {
		return PluginExportResult{}, err
	}
	plan, err := prepareExportArchivePlan(ctx, opts.Output)
	if err != nil {
		return PluginExportResult{}, err
	}
	if err := writeExportArchive(ctx.root, plan.outputPath, plan.files, plan.metadata); err != nil {
		return PluginExportResult{}, err
	}
	return PluginExportResult{Lines: buildExportResultLines(ctx, plan)}, nil
}

func exportBlockingFailures(failures []validate.Failure) []validate.Failure {
	return slices.DeleteFunc(slices.Clone(failures), func(f validate.Failure) bool {
		return f.Kind == validate.FailureGeneratedContractInvalid
	})
}
