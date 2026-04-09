package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
)

type cursorWorkspaceAdapter struct{}

const removedCursorRulesFileName = "." + "cursor" + "rules"
const (
	cursorAgentsSectionStart = "<!-- plugin-kit-ai:cursor-agents:start -->"
	cursorAgentsSectionEnd   = "<!-- plugin-kit-ai:cursor-agents:end -->"
)

func (cursorWorkspaceAdapter) ID() string { return "cursor-workspace" }

func (cursorWorkspaceAdapter) DetectNative(root string) bool {
	if fileExists(filepath.Join(root, ".cursor", "mcp.json")) {
		return true
	}
	return len(discoverFiles(root, filepath.Join(".cursor", "rules"), nil)) > 0
}

func (cursorWorkspaceAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	for _, rel := range state.ComponentPaths("rules") {
		if strings.ToLower(filepath.Ext(rel)) != ".mdc" {
			return fmt.Errorf("unsupported Cursor rule file %s: use .mdc", rel)
		}
	}
	return nil
}

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

func (cursorWorkspaceAdapter) Generate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "cursor-workspace")
		if err != nil {
			return nil, err
		}
		body, err := marshalJSON(map[string]any{
			"mcpServers": projected,
		})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(".cursor", "mcp.json"),
			Content: body,
		})
	}
	rules, err := copyArtifacts(root, authoredComponentDir(state, "rules", filepath.Join("targets", "cursor-workspace", "rules")), filepath.Join(".cursor", "rules"))
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, rules...)
	agentsContent, err := renderCursorRootAgents(root, state)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, pluginmodel.Artifact{
		RelPath: "AGENTS.md",
		Content: agentsContent,
	})
	return compactArtifacts(artifacts), nil
}

func (cursorWorkspaceAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	return []string{"AGENTS.md"}, nil
}

func (cursorWorkspaceAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	if graph.Portable.MCP == nil && len(state.ComponentPaths("rules")) == 0 && strings.TrimSpace(state.DocPath("agents_markdown")) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("targets", "cursor-workspace")),
			Target:   "cursor-workspace",
			Message:  "Cursor workspace target requires at least one of src/mcp/servers.yaml, src/targets/cursor-workspace/rules/**, or src/targets/cursor-workspace/AGENTS.md",
		})
	}
	diagnostics = append(diagnostics, validateCursorRuleFiles(root, state.ComponentPaths("rules"))...)
	diagnostics = append(diagnostics, validateCursorAgentsMarkdown(root, state.DocPath("agents_markdown"))...)
	return diagnostics, nil
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

func renderCursorRootAgents(root string, state pluginmodel.TargetState) ([]byte, error) {
	body, _, err := scaffold.RenderTemplate("ROOT.AGENTS.md.tmpl", scaffold.Data{Platform: "cursor-workspace"})
	if err != nil {
		return nil, err
	}
	rel := strings.TrimSpace(state.DocPath("agents_markdown"))
	if rel == "" {
		return body, nil
	}
	authored, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(string(authored))
	if content == "" {
		return body, nil
	}
	merged := strings.TrimRight(string(body), "\n") + "\n\n" + cursorAgentsSectionStart + "\n" + content + "\n" + cursorAgentsSectionEnd + "\n"
	return []byte(merged), nil
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
	if strings.TrimSpace(content) == "" {
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

func extractCursorManagedAgentsSection(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	start := strings.Index(body, cursorAgentsSectionStart)
	end := strings.Index(body, cursorAgentsSectionEnd)
	if start < 0 || end < 0 || end <= start {
		return ""
	}
	start += len(cursorAgentsSectionStart)
	return strings.TrimSpace(body[start:end])
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

func validateCursorRuleFiles(root string, rels []string) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var diagnostics []Diagnostic
	seenCaseFolded := map[string]string{}
	for _, rel := range rels {
		clean := filepath.ToSlash(filepath.Clean(rel))
		if clean != rel || strings.Contains(clean, "..") {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule file %s must stay within src/targets/cursor-workspace/rules without path traversal", rel),
			})
			continue
		}
		lower := strings.ToLower(clean)
		if prior, ok := seenCaseFolded[lower]; ok && prior != clean {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule files %s and %s collide on case-insensitive filesystems", prior, rel),
			})
		} else {
			seenCaseFolded[lower] = clean
		}
		if strings.ToLower(filepath.Ext(rel)) != ".mdc" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule file %s must use the .mdc extension", rel),
			})
			continue
		}
		fullPath := filepath.Join(root, rel)
		info, err := os.Lstat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule file %s must not be a symlink", rel),
			})
			continue
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule file %s is not readable: %v", rel, err),
			})
			continue
		}
		if strings.TrimSpace(string(body)) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor-workspace",
				Message:  fmt.Sprintf("Cursor workspace rule file %s must not be empty", rel),
			})
		}
	}
	return diagnostics
}

func validateCursorAgentsMarkdown(root string, rel string) []Diagnostic {
	if strings.TrimSpace(rel) == "" {
		return nil
	}
	full := filepath.Join(root, rel)
	info, err := os.Lstat(full)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor-workspace",
			Message:  fmt.Sprintf("Cursor workspace AGENTS markdown %s is not readable: %v", rel, err),
		}}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor-workspace",
			Message:  fmt.Sprintf("Cursor workspace AGENTS markdown %s must not be a symlink", rel),
		}}
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor-workspace",
			Message:  fmt.Sprintf("Cursor workspace AGENTS markdown %s is not readable: %v", rel, err),
		}}
	}
	if strings.TrimSpace(string(body)) == "" {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor-workspace",
			Message:  fmt.Sprintf("Cursor workspace AGENTS markdown %s must not be empty", rel),
		}}
	}
	return nil
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
