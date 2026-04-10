package platformexec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
)

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
