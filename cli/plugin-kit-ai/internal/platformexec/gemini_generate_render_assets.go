package platformexec

import "github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"

func mergeGeminiManifestAssets(root string, state pluginmodel.TargetState, manifest map[string]any) error {
	settings, err := loadGeminiSettings(root, state.ComponentPaths("settings"))
	if err != nil {
		return err
	}
	if len(settings) > 0 {
		manifest["settings"] = settings
	}
	themes, err := loadGeminiThemes(root, state.ComponentPaths("themes"))
	if err != nil {
		return err
	}
	if len(themes) > 0 {
		manifest["themes"] = themes
	}
	return nil
}

func mergeGeminiManifestContexts(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta, manifest map[string]any) ([]pluginmodel.Artifact, error) {
	if contextName, contextArtifact, extraContexts, ok, err := geminiContextArtifacts(root, graph, state, meta); err != nil {
		return nil, err
	} else if ok {
		manifest["contextFileName"] = contextName
		return append([]pluginmodel.Artifact{contextArtifact}, extraContexts...), nil
	}
	return nil, nil
}

