package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (cursorWorkspaceAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{Manifest: seed.Manifest}
	var hasCursorState bool
	mergedServers := map[string]any{}

	if seed.IncludeUserScope {
		home, err := os.UserHomeDir()
		if err != nil {
			return ImportResult{}, fmt.Errorf("resolve user home for Cursor import: %w", err)
		}
		if imported, ok, err := readCursorMCPServers(filepath.Join(home, ".cursor", "mcp.json"), filepath.ToSlash(filepath.Join("~", ".cursor", "mcp.json"))); err != nil {
			return ImportResult{}, err
		} else if ok {
			mergeOpenCodeObject(mergedServers, imported)
			hasCursorState = true
		}
	}

	if imported, ok, err := readCursorMCPServers(filepath.Join(root, ".cursor", "mcp.json"), ".cursor/mcp.json"); err != nil {
		return ImportResult{}, err
	} else if ok {
		mergeOpenCodeObject(mergedServers, imported)
		hasCursorState = true
	}
	if len(mergedServers) > 0 {
		artifact, err := importedPortableMCPArtifact("cursor-workspace", mergedServers)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, artifact)
	}

	ruleArtifacts, err := importCursorRuleArtifacts(root)
	if err != nil {
		return ImportResult{}, err
	}
	if len(ruleArtifacts) > 0 {
		result.Artifacts = append(result.Artifacts, ruleArtifacts...)
		hasCursorState = true
	}

	if _, err := os.Stat(filepath.Join(root, removedCursorRulesFileName)); err == nil {
		return ImportResult{}, fmt.Errorf("unsupported Cursor repo-root rules file: use .cursor/rules/*.mdc")
	} else if !os.IsNotExist(err) {
		return ImportResult{}, err
	}

	if agentsArtifact, ok, err := importCursorAgentsArtifact(root); err != nil {
		return ImportResult{}, err
	} else if ok {
		result.Artifacts = append(result.Artifacts, agentsArtifact)
		hasCursorState = true
	}

	if !hasCursorState {
		return ImportResult{}, fmt.Errorf("Cursor import requires .cursor/mcp.json, .cursor/rules/**, root AGENTS.md, or --include-user-scope with ~/.cursor/mcp.json")
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func readCursorMCPServers(path string, label string) (map[string]any, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	doc, err := decodeJSONObject(body, "Cursor MCP config "+label)
	if err != nil {
		return nil, false, err
	}
	servers, err := cursorMCPServersFromDocument(doc)
	if err != nil {
		return nil, false, err
	}
	return servers, true, nil
}

func importCursorAgentsArtifact(root string) (pluginmodel.Artifact, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return pluginmodel.Artifact{}, false, nil
		}
		return pluginmodel.Artifact{}, false, err
	}
	content := extractCursorManagedAgentsSection(string(body))
	if strings.TrimSpace(content) == "" &&
		!strings.Contains(string(body), "<!-- plugin-kit-ai:begin managed-guidance -->") &&
		!strings.Contains(string(body), "<!-- plugin-kit-ai:end managed-guidance -->") {
		content = strings.TrimSpace(string(body))
	}
	if strings.TrimSpace(content) == "" {
		return pluginmodel.Artifact{}, false, nil
	}
	return pluginmodel.Artifact{
		RelPath: filepath.ToSlash(filepath.Join("targets", "cursor-workspace", "AGENTS.md")),
		Content: append([]byte(content), '\n'),
	}, true, nil
}

func importCursorRuleArtifacts(root string) ([]pluginmodel.Artifact, error) {
	full := filepath.Join(root, ".cursor", "rules")
	if _, err := os.Stat(full); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var artifacts []pluginmodel.Artifact
	err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("cursor native import does not support symlinks under .cursor/rules")
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(full, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.ToLower(filepath.Ext(rel)) != ".mdc" {
			return fmt.Errorf("cursor native import only supports .mdc files under .cursor/rules: %s", filepath.ToSlash(filepath.Join(".cursor", "rules", rel)))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join("targets", "cursor-workspace", "rules", rel)),
			Content: body,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(artifacts, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return artifacts, nil
}

func cursorMCPServersFromDocument(doc map[string]any) (map[string]any, error) {
	if value, ok := doc["mcpServers"]; ok {
		servers, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Cursor MCP config .cursor/mcp.json field %q must be a JSON object", "mcpServers")
		}
		if servers == nil {
			return map[string]any{}, nil
		}
		return servers, nil
	}
	return doc, nil
}
