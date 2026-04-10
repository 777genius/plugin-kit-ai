package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (geminiAdapter) DetectNative(root string) bool {
	return fileExists(filepath.Join(root, "gemini-extension.json"))
}

func (geminiAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	if rel := state.DocPath("package_metadata"); strings.TrimSpace(rel) != "" {
		if _, ok, err := readYAMLDoc[geminiPackageMeta](root, rel); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		} else if !ok {
			return nil
		}
	}
	for _, rel := range state.ComponentPaths("hooks") {
		expectedSuffix := filepath.ToSlash(filepath.Join("targets", "gemini", "hooks", "hooks.json"))
		if !strings.HasSuffix(filepath.ToSlash(rel), expectedSuffix) {
			return fmt.Errorf("unsupported Gemini hooks layout: use only %s", filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, expectedSuffix)))
		}
	}
	for _, rel := range append(append([]string{}, state.ComponentPaths("settings")...), state.ComponentPaths("themes")...) {
		if !geminiYAMLFileRe.MatchString(rel) {
			kind := "theme"
			if strings.Contains(rel, "/settings/") {
				kind = "setting"
			}
			return fmt.Errorf("unsupported Gemini %s file %s: use .yaml or .yml", kind, rel)
		}
	}
	return nil
}

func (geminiAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	hooksBody, hookBodyErr := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	copied, err := copySingleArtifactIfExists(root, filepath.Join("hooks", "hooks.json"), filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "hooks", "hooks.json"))
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, copied...)
	if hookBodyErr == nil {
		if _, err := parseGeminiHooks(hooksBody); err != nil {
			return ImportResult{}, fmt.Errorf("parse %s: %w", filepath.ToSlash(filepath.Join("hooks", "hooks.json")), err)
		}
		if entrypoint, ok := inferGeminiEntrypoint(hooksBody); ok {
			if result.Launcher == nil {
				result.Launcher = &pluginmodel.Launcher{
					Runtime:    "go",
					Entrypoint: entrypoint,
				}
			} else {
				result.Launcher.Entrypoint = entrypoint
			}
		}
	}
	copied, err = copyArtifactDirs(root,
		artifactDir{src: "skills", dst: "skills"},
		artifactDir{src: "commands", dst: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "commands")},
		artifactDir{src: "policies", dst: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "policies")},
		artifactDir{src: "contexts", dst: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "contexts")},
	)
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, copied...)

	data, ok, err := readImportedGeminiExtension(root)
	if err != nil {
		return ImportResult{}, err
	}
	if ok {
		if strings.TrimSpace(data.Name) != "" {
			result.Manifest.Name = data.Name
		}
		if strings.TrimSpace(data.Version) != "" {
			result.Manifest.Version = data.Version
		}
		if strings.TrimSpace(data.Description) != "" {
			result.Manifest.Description = data.Description
		}
		if len(data.MCPServers) > 0 {
			artifact, err := importedPortableMCPArtifact("gemini", data.MCPServers)
			if err != nil {
				return ImportResult{}, err
			}
			result.Artifacts = append(result.Artifacts, artifact)
		}
		if body := importedGeminiPackageYAML(data.Meta); len(body) > 0 {
			result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "package.yaml"), Content: body})
		}
		result.Artifacts = append(result.Artifacts, importedGeminiSettingsArtifacts(data.Settings)...)
		result.Artifacts = append(result.Artifacts, importedGeminiThemeArtifacts(data.Themes)...)
		if len(data.Extra) > 0 {
			result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "manifest.extra.json"), Content: mustJSON(data.Extra)})
			result.Warnings = append(result.Warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "manifest.extra.json")),
				Message: "preserved additional Gemini manifest fields under targets/gemini/manifest.extra.json",
			})
		}
		if contextName := importedGeminiPrimaryContextName(root, data.Meta); contextName != "" {
			contextArtifacts, err := copySingleArtifactIfExists(root, contextName, filepath.Join(pluginmodel.SourceDirName, "targets", "gemini", "contexts", filepath.Base(contextName)))
			if err != nil {
				return ImportResult{}, err
			}
			result.Artifacts = append(result.Artifacts, contextArtifacts...)
		}
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func importedGeminiPackageYAML(meta geminiPackageMeta) []byte {
	if len(meta.ExcludeTools) == 0 &&
		strings.TrimSpace(meta.ContextFileName) == "" &&
		strings.TrimSpace(meta.PlanDirectory) == "" {
		return nil
	}
	return mustYAML(meta)
}

func importedGeminiPrimaryContextName(root string, meta geminiPackageMeta) string {
	if strings.TrimSpace(meta.ContextFileName) != "" {
		return filepath.Base(strings.TrimSpace(meta.ContextFileName))
	}
	if fileExists(filepath.Join(root, "GEMINI.md")) {
		return "GEMINI.md"
	}
	return ""
}
