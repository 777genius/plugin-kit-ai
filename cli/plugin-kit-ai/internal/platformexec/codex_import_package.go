package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (codexPackageAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{Manifest: seed.Manifest, Launcher: nil}
	pluginManifest, _, err := readImportedCodexPluginManifest(root)
	if err != nil {
		return ImportResult{}, err
	}
	if err := validateImportedCodexPackageBundle(root, pluginManifest); err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "package.yaml"),
		Content: mustYAML(pluginManifest.PackageMeta),
	})
	result.Manifest = mergeImportedCodexPackageManifest(result.Manifest, pluginManifest)
	extra := cloneStringMap(pluginManifest.Extra)
	if err := appendImportedCodexPackageArtifacts(&result, root, pluginManifest, extra); err != nil {
		return ImportResult{}, err
	}
	appendImportedCodexPackageWarnings(&result, root, pluginManifest, extra)
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func validateImportedCodexPackageBundle(root string, pluginManifest importedCodexPluginManifest) error {
	if unexpected := codexmanifest.UnexpectedBundleSidecars(root, pluginManifest); len(unexpected) > 0 {
		return fmt.Errorf("Codex package bundle contains unexpected sidecar artifacts without matching plugin.json refs: %s", strings.Join(unexpected, ", "))
	}
	return nil
}

func mergeImportedCodexPackageManifest(seed pluginmodel.Manifest, pluginManifest importedCodexPluginManifest) pluginmodel.Manifest {
	if strings.TrimSpace(pluginManifest.Name) != "" {
		seed.Name = pluginManifest.Name
	}
	if strings.TrimSpace(pluginManifest.Version) != "" {
		seed.Version = pluginManifest.Version
	}
	if strings.TrimSpace(pluginManifest.Description) != "" {
		seed.Description = pluginManifest.Description
	}
	return seed
}

func appendImportedCodexPackageArtifacts(result *ImportResult, root string, pluginManifest importedCodexPluginManifest, extra map[string]any) error {
	if pluginManifest.Interface != nil {
		body, err := marshalJSON(pluginManifest.Interface)
		if err != nil {
			return err
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "interface.json"),
			Content: body,
		})
	}
	if err := appendImportedCodexSkillsArtifacts(result, root, pluginManifest); err != nil {
		return err
	}
	if err := appendImportedCodexAppsArtifact(result, root, pluginManifest); err != nil {
		return err
	}
	if err := appendImportedCodexMCPArtifact(result, root, pluginManifest); err != nil {
		return err
	}
	if len(extra) > 0 {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "manifest.extra.json"),
			Content: mustJSON(extra),
		})
	}
	return nil
}

func appendImportedCodexSkillsArtifacts(result *ImportResult, root string, pluginManifest importedCodexPluginManifest) error {
	if ref := strings.TrimSpace(pluginManifest.SkillsPath); ref != "" {
		copied, err := copyArtifactsFromRefs(root, []string{ref}, "skills")
		if err != nil {
			return err
		}
		result.Artifacts = append(result.Artifacts, copied...)
	}
	return nil
}

func appendImportedCodexAppsArtifact(result *ImportResult, root string, pluginManifest importedCodexPluginManifest) error {
	ref := strings.TrimSpace(pluginManifest.AppsRef)
	if ref == "" {
		return nil
	}
	refPath, err := resolveRelativeRef(root, ref)
	if err != nil {
		return err
	}
	appBody, err := os.ReadFile(filepath.Join(root, refPath))
	if err != nil {
		return err
	}
	if _, err := codexmanifest.ParseAppManifestDoc(appBody); err != nil {
		return fmt.Errorf("parse %s: %w", filepath.ToSlash(refPath), err)
	}
	result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "app.json"),
		Content: append([]byte(nil), appBody...),
	})
	return nil
}

func appendImportedCodexMCPArtifact(result *ImportResult, root string, pluginManifest importedCodexPluginManifest) error {
	ref := strings.TrimSpace(pluginManifest.MCPServersRef)
	if ref == "" {
		return nil
	}
	refPath, err := resolveRelativeRef(root, ref)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(filepath.Join(root, refPath))
	if err != nil {
		return err
	}
	servers, err := decodeJSONObject(body, fmt.Sprintf("Codex MCP manifest %s", filepath.ToSlash(refPath)))
	if err != nil {
		return err
	}
	artifact, err := importedPortableMCPArtifact("codex-package", servers)
	if err != nil {
		return err
	}
	result.Artifacts = append(result.Artifacts, artifact)
	return nil
}

func appendImportedCodexPackageWarnings(result *ImportResult, root string, pluginManifest importedCodexPluginManifest, extra map[string]any) {
	if len(extra) > 0 {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "manifest.extra.json")),
			Message: "preserved unsupported Codex plugin manifest fields under targets/codex-package/manifest.extra.json",
		})
	}
	if strings.TrimSpace(pluginManifest.SkillsPath) != "" && strings.TrimSpace(pluginManifest.SkillsPath) != codexmanifest.SkillsRef {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Message: "normalized Codex plugin skills path to the managed ./skills/ location",
		})
	}
	if strings.TrimSpace(pluginManifest.AppsRef) != "" && strings.TrimSpace(pluginManifest.AppsRef) != codexmanifest.AppsRef {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Message: "normalized Codex plugin apps path to the managed ./.app.json location",
		})
	}
	if strings.TrimSpace(pluginManifest.MCPServersRef) != "" && strings.TrimSpace(pluginManifest.MCPServersRef) != codexmanifest.MCPServersRef {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
			Message: "normalized Codex plugin mcpServers path to the managed ./.mcp.json location",
		})
	}
	if fileExists(filepath.Join(root, "agents")) {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningIgnoredImport,
			Path:    "agents",
			Message: "ignored unsupported import asset: agents",
		})
	}
}
