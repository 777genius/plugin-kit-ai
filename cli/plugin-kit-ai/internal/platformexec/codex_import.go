package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/codexmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/pelletier/go-toml/v2"
)

func (codexPackageAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{Manifest: seed.Manifest, Launcher: nil}
	pluginManifest, _, err := readImportedCodexPluginManifest(root)
	if err != nil {
		return ImportResult{}, err
	}
	if unexpected := codexmanifest.UnexpectedBundleSidecars(root, pluginManifest); len(unexpected) > 0 {
		return ImportResult{}, fmt.Errorf("Codex package bundle contains unexpected sidecar artifacts without matching plugin.json refs: %s", strings.Join(unexpected, ", "))
	}
	result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "package.yaml"),
		Content: mustYAML(pluginManifest.PackageMeta),
	})
	if strings.TrimSpace(pluginManifest.Name) != "" {
		result.Manifest.Name = pluginManifest.Name
	}
	if strings.TrimSpace(pluginManifest.Version) != "" {
		result.Manifest.Version = pluginManifest.Version
	}
	if strings.TrimSpace(pluginManifest.Description) != "" {
		result.Manifest.Description = pluginManifest.Description
	}

	extra := cloneStringMap(pluginManifest.Extra)
	if pluginManifest.Interface != nil {
		body, err := marshalJSON(pluginManifest.Interface)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "interface.json"),
			Content: body,
		})
	}
	if ref := strings.TrimSpace(pluginManifest.SkillsPath); ref != "" {
		copied, err := copyArtifactsFromRefs(root, []string{ref}, "skills")
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, copied...)
	}
	if ref := strings.TrimSpace(pluginManifest.AppsRef); ref != "" {
		refPath, err := resolveRelativeRef(root, ref)
		if err != nil {
			return ImportResult{}, err
		}
		appBody, err := os.ReadFile(filepath.Join(root, refPath))
		if err != nil {
			return ImportResult{}, err
		}
		if _, err := codexmanifest.ParseAppManifestDoc(appBody); err != nil {
			return ImportResult{}, fmt.Errorf("parse %s: %w", filepath.ToSlash(refPath), err)
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "app.json"),
			Content: append([]byte(nil), appBody...),
		})
		if ref != codexmanifest.AppsRef {
			result.Warnings = append(result.Warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join(".codex-plugin", "plugin.json")),
				Message: "normalized Codex plugin apps path to the managed ./.app.json location",
			})
		}
	}
	if ref := strings.TrimSpace(pluginManifest.MCPServersRef); ref != "" {
		refPath, err := resolveRelativeRef(root, ref)
		if err != nil {
			return ImportResult{}, err
		}
		body, err := os.ReadFile(filepath.Join(root, refPath))
		if err != nil {
			return ImportResult{}, err
		}
		servers, err := decodeJSONObject(body, fmt.Sprintf("Codex MCP manifest %s", filepath.ToSlash(refPath)))
		if err != nil {
			return ImportResult{}, err
		}
		artifact, err := importedPortableMCPArtifact("codex-package", servers)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, artifact)
	}
	if len(extra) > 0 {
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-package", "manifest.extra.json"),
			Content: mustJSON(extra),
		})
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
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (codexRuntimeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	config, _, err := readImportedCodexConfig(root)
	if err != nil {
		return ImportResult{}, err
	}
	meta := codexRuntimeMeta{}
	if strings.TrimSpace(config.Model) != "" {
		meta.ModelHint = config.Model
	}
	result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
		RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-runtime", "package.yaml"),
		Content: mustYAML(meta),
	})
	if len(config.Extra) > 0 {
		body, err := toml.Marshal(config.Extra)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-runtime", "config.extra.toml"),
			Content: body,
		})
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "codex-runtime", "config.extra.toml")),
			Message: "preserved unsupported Codex config fields under targets/codex-runtime/config.extra.toml",
		})
	}
	if len(config.Notify) > 0 && result.Launcher != nil && strings.TrimSpace(config.Notify[0]) != "" {
		result.Launcher.Entrypoint = strings.TrimSpace(config.Notify[0])
	}
	if len(config.Notify) > 0 && !pluginmodel.IsCanonicalCodexNotify(config.Notify) {
		result.Warnings = append(result.Warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".codex", "config.toml")),
			Message: "normalized Codex notify argv to the managed [entrypoint, \"notify\"] shape",
		})
	}
	copied, err := copyArtifactDirs(root,
		artifactDir{src: "commands", dst: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-runtime", "commands")},
		artifactDir{src: "contexts", dst: filepath.Join(pluginmodel.SourceDirName, "targets", "codex-runtime", "contexts")},
	)
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, copied...)
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}
