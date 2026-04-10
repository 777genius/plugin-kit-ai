package platformexec

import (
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (claudeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	pluginManifest, _, manifestPresent, err := readImportedClaudePluginManifest(root)
	if err != nil {
		return ImportResult{}, err
	}
	if manifestPresent {
		if strings.TrimSpace(pluginManifest.Name) != "" {
			result.Manifest.Name = pluginManifest.Name
		}
		if strings.TrimSpace(pluginManifest.Version) != "" {
			result.Manifest.Version = pluginManifest.Version
		}
		if strings.TrimSpace(pluginManifest.Description) != "" {
			result.Manifest.Description = pluginManifest.Description
		}
	} else {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    ".claude-plugin/plugin.json",
			Message: "native Claude plugin imported without manifest; package-standard defaults were derived from the directory name",
		})
	}
	for _, warning := range pluginManifest.Warnings {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: warning,
		})
	}
	if strings.TrimSpace(pluginManifest.Name) != "" && pluginManifest.Name != seed.Manifest.Name {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "normalized Claude plugin identity into canonical package-standard plugin.yaml",
		})
	}
	if copied, warnings, err := importClaudePortableSkills(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if hookArtifacts, hookBody, warnings, err := importClaudeHooks(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Warnings = append(result.Warnings, warnings...)
		if len(hookBody) > 0 {
			if entrypoint, ok := inferClaudeEntrypoint(hookBody); ok && result.Launcher == nil {
				result.Launcher = &pluginmodel.Launcher{
					Runtime:    "go",
					Entrypoint: entrypoint,
				}
			} else if ok {
				result.Launcher.Entrypoint = entrypoint
			} else {
				result.DroppedKinds = append(result.DroppedKinds, "hooks")
				result.Warnings = append(result.Warnings, pluginmodel.Warning{
					Kind:    pluginmodel.WarningFidelity,
					Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
					Message: "Claude hooks were omitted from canonical package-standard import because their commands do not map to launcher.yaml entrypoint semantics",
				})
				hookArtifacts = nil
			}
		}
		result.Artifacts = append(result.Artifacts, hookArtifacts...)
	}

	if copied, warnings, err := importClaudeComponentRefs(root, "commands", filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "commands"), pluginManifest.CommandsOverride, pluginManifest.CommandsRefs); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if copied, warnings, err := importClaudeComponentRefs(root, "agents", filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "agents"), pluginManifest.AgentsOverride, pluginManifest.AgentsRefs); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}

	if copied, warning, err := importClaudeStructuredDoc(root, "settings.json", filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "settings.json"), manifestPresent && pluginManifest.SettingsProvided, pluginManifest.Settings, "Claude manifest settings"); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		if strings.TrimSpace(warning) != "" {
			result.Warnings = append(result.Warnings, pluginmodel.Warning{Kind: pluginmodel.WarningFidelity, Path: "settings.json", Message: warning})
		}
	}
	if copied, warnings, err := importClaudeLSP(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if pluginManifest.UserConfigProvided {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "user-config.json"),
			Content: mustJSON(pluginManifest.UserConfig),
		})
	}
	if copied, warnings, err := importClaudeMCP(root, pluginManifest); err != nil {
		return ImportResult{}, err
	} else {
		result.Artifacts = append(result.Artifacts, copied...)
		result.Warnings = append(result.Warnings, warnings...)
	}
	if len(pluginManifest.Extra) > 0 {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "manifest.extra.json"),
			Content: mustJSON(pluginManifest.Extra),
		})
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "manifest.extra.json")),
			Message: "preserved unsupported Claude manifest fields under targets/claude/manifest.extra.json",
		})
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}
