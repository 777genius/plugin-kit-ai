package app

import "github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"

type PluginRenderOptions struct {
	Root   string
	Target string
	Check  bool
}

type PluginImportOptions struct {
	Root             string
	From             string
	Force            bool
	IncludeUserScope bool
}

type PluginNormalizeOptions struct {
	Root  string
	Force bool
}

type PluginInspectOptions struct {
	Root   string
	Target string
}

type PluginService struct{}

func (PluginService) Render(opts PluginRenderOptions) ([]string, error) {
	if opts.Check {
		return pluginmanifest.Drift(opts.Root, opts.Target)
	}
	result, err := pluginmanifest.Render(opts.Root, opts.Target)
	if err != nil {
		return nil, err
	}
	if err := pluginmanifest.WriteArtifacts(opts.Root, result.Artifacts); err != nil {
		return nil, err
	}
	if err := pluginmanifest.RemoveArtifacts(opts.Root, result.StalePaths); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(result.Artifacts))
	for _, artifact := range result.Artifacts {
		out = append(out, artifact.RelPath)
	}
	return out, nil
}

func (PluginService) Import(opts PluginImportOptions) ([]pluginmanifest.Warning, error) {
	_, warnings, err := pluginmanifest.Import(opts.Root, opts.From, opts.Force, opts.IncludeUserScope)
	if err != nil {
		return warnings, err
	}
	return warnings, nil
}

func (PluginService) Normalize(opts PluginNormalizeOptions) ([]pluginmanifest.Warning, error) {
	return pluginmanifest.Normalize(opts.Root, opts.Force)
}

func (PluginService) Inspect(opts PluginInspectOptions) (pluginmanifest.Inspection, []pluginmanifest.Warning, error) {
	return pluginmanifest.Inspect(opts.Root, opts.Target)
}
