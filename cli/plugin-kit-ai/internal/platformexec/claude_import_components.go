package platformexec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func importClaudeComponentRefs(root, kind, dstRoot string, overridden bool, refs []string) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	if !overridden {
		if !fileExists(filepath.Join(root, kind)) {
			return nil, nil, nil
		}
		refs = []string{kind}
	}
	if len(refs) == 0 {
		return nil, nil, nil
	}
	artifacts, err := copyArtifactsFromRefs(root, refs, dstRoot)
	if err != nil {
		return nil, nil, err
	}
	if !overridden {
		return artifacts, nil, nil
	}
	return artifacts, []pluginmodel.Warning{{
		Kind:    pluginmodel.WarningFidelity,
		Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
		Message: fmt.Sprintf("custom Claude %s paths were normalized into canonical package-standard layout", kind),
	}}, nil
}

func importClaudePortableSkills(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	if !manifest.SkillsOverride {
		if !fileExists(filepath.Join(root, "skills")) {
			return nil, nil, nil
		}
		artifacts, err := copyArtifactsFromRefs(root, []string{"skills"}, "skills")
		if err != nil {
			return nil, nil, err
		}
		return artifacts, nil, nil
	}
	if len(manifest.SkillsRefs) == 0 {
		return nil, nil, nil
	}
	artifacts, err := copyArtifactsFromRefs(root, manifest.SkillsRefs, "skills")
	if err != nil {
		return nil, nil, err
	}
	return artifacts, []pluginmodel.Warning{{
		Kind:    pluginmodel.WarningFidelity,
		Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
		Message: "custom Claude skills paths were normalized into canonical package-standard layout",
	}}, nil
}

func importClaudeStructuredDoc(root, rootPath, targetPath string, inlineProvided bool, inline map[string]any, inlineLabel string) ([]pluginmodel.Artifact, string, error) {
	if body, err := os.ReadFile(filepath.Join(root, rootPath)); err == nil {
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, "", nil
	} else if !os.IsNotExist(err) {
		return nil, "", err
	}
	if inlineProvided {
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: mustJSON(inline)}}, fmt.Sprintf("%s was normalized into %s", inlineLabel, filepath.ToSlash(targetPath)), nil
	}
	return nil, "", nil
}

func importClaudeLSP(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	const targetPath = "targets/claude/lsp.json"
	if fileExists(filepath.Join(root, ".lsp.json")) {
		body, err := os.ReadFile(filepath.Join(root, ".lsp.json"))
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, nil, nil
	}
	if !manifest.LSPOverride {
		return nil, nil, nil
	}
	switch {
	case manifest.InlineLSP != nil:
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: mustJSON(manifest.InlineLSP)}}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "inline Claude lspServers were normalized into targets/claude/lsp.json",
		}}, nil
	case len(manifest.LSPRefs) == 1:
		ref := cleanRelativeRef(manifest.LSPRefs[0])
		body, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude lspServers path was normalized into targets/claude/lsp.json",
		}}, nil
	case len(manifest.LSPRefs) > 1:
		body, err := mergeClaudeObjectRefs(root, manifest.LSPRefs, "Claude lspServers")
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{{RelPath: targetPath, Content: body}}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude lspServers path array was normalized into canonical package-standard layout",
		}}, nil
	default:
		return nil, nil, nil
	}
}

func importClaudeMCP(root string, manifest importedClaudePluginManifest) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	if fileExists(filepath.Join(root, ".mcp.json")) || !manifest.MCPOverride {
		return nil, nil, nil
	}
	switch {
	case manifest.InlineMCP != nil:
		artifact, err := importedPortableMCPArtifact("claude", manifest.InlineMCP)
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{artifact}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "inline Claude mcpServers were normalized into src/mcp/servers.yaml",
		}}, nil
	case len(manifest.MCPRefs) == 1:
		ref := cleanRelativeRef(manifest.MCPRefs[0])
		body, err := os.ReadFile(filepath.Join(root, ref))
		if err != nil {
			return nil, nil, err
		}
		doc, err := decodeJSONObject(body, "Claude mcpServers")
		if err != nil {
			return nil, nil, err
		}
		artifact, err := importedPortableMCPArtifact("claude", doc)
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{artifact}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude mcpServers path was normalized into src/mcp/servers.yaml",
		}}, nil
	case len(manifest.MCPRefs) > 1:
		body, err := mergeClaudeObjectRefs(root, manifest.MCPRefs, "Claude mcpServers")
		if err != nil {
			return nil, nil, err
		}
		doc, err := decodeJSONObject(body, "Claude mcpServers")
		if err != nil {
			return nil, nil, err
		}
		artifact, err := importedPortableMCPArtifact("claude", doc)
		if err != nil {
			return nil, nil, err
		}
		return []pluginmodel.Artifact{artifact}, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(".claude-plugin", "plugin.json")),
			Message: "custom Claude mcpServers path array was normalized into canonical package-standard layout",
		}}, nil
	default:
		return nil, nil, nil
	}
}
