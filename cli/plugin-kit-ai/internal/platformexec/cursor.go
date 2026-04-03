package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type cursorAdapter struct{}

func (cursorAdapter) ID() string { return "cursor" }

func (cursorAdapter) DetectNative(root string) bool {
	if fileExists(filepath.Join(root, ".cursor", "mcp.json")) {
		return true
	}
	return len(discoverFiles(root, filepath.Join(".cursor", "rules"), nil)) > 0
}

func (cursorAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	for _, rel := range state.ComponentPaths("rules") {
		if strings.ToLower(filepath.Ext(rel)) != ".mdc" {
			return fmt.Errorf("unsupported Cursor rule file %s: use .mdc", rel)
		}
	}
	return nil
}

func (cursorAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	if seed.IncludeUserScope {
		return ImportResult{}, fmt.Errorf("Cursor import does not support --include-user-scope yet; global ~/.cursor/mcp.json is deferred in the current contract")
	}
	result := ImportResult{Manifest: seed.Manifest}
	var hasCursorState bool

	if body, err := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json")); err == nil {
		doc, err := decodeJSONObject(body, "Cursor MCP config .cursor/mcp.json")
		if err != nil {
			return ImportResult{}, err
		}
		artifact, err := importedPortableMCPArtifact("cursor", doc)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, artifact)
		hasCursorState = true
	} else if !os.IsNotExist(err) {
		return ImportResult{}, err
	}

	ruleArtifacts, err := importCursorRuleArtifacts(root)
	if err != nil {
		return ImportResult{}, err
	}
	if len(ruleArtifacts) > 0 {
		result.Artifacts = append(result.Artifacts, ruleArtifacts...)
		hasCursorState = true
	}

	if _, err := os.Stat(filepath.Join(root, ".cursorrules")); err == nil {
		return ImportResult{}, fmt.Errorf("unsupported Cursor native path .cursorrules: use .cursor/rules/*.mdc and optional root AGENTS.md")
	} else if !os.IsNotExist(err) {
		return ImportResult{}, err
	}

	if body, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); err == nil {
		if seed.Explicit || hasCursorState {
			result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{
				RelPath: filepath.Join("targets", "cursor", "AGENTS.md"),
				Content: body,
			})
			hasCursorState = true
		}
	} else if !os.IsNotExist(err) {
		return ImportResult{}, err
	}

	if !hasCursorState {
		return ImportResult{}, fmt.Errorf("Cursor import requires .cursor/mcp.json, .cursor/rules/**, or explicit --from cursor with root AGENTS.md")
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (cursorAdapter) Render(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "cursor")
		if err != nil {
			return nil, err
		}
		body, err := marshalJSON(projected)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(".cursor", "mcp.json"),
			Content: body,
		})
	}
	rules, err := copyArtifacts(root, filepath.Join("targets", "cursor", "rules"), filepath.Join(".cursor", "rules"))
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, rules...)
	agents, err := copySingleArtifactIfExists(root, filepath.Join("targets", "cursor", "AGENTS.md"), "AGENTS.md")
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, agents...)
	return compactArtifacts(artifacts), nil
}

func (cursorAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	if strings.TrimSpace(state.DocPath("agents_md")) != "" || fileExists(filepath.Join(root, "AGENTS.md")) {
		return []string{"AGENTS.md"}, nil
	}
	return nil, nil
}

func (cursorAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	if graph.Portable.MCP == nil && len(state.ComponentPaths("rules")) == 0 && strings.TrimSpace(state.DocPath("agents_md")) == "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("targets", "cursor")),
			Target:   "cursor",
			Message:  "Cursor target requires at least one of mcp/servers.yaml, targets/cursor/rules/**, or targets/cursor/AGENTS.md",
		})
	}
	diagnostics = append(diagnostics, validateCursorRuleFiles(root, state.ComponentPaths("rules"))...)
	diagnostics = append(diagnostics, validateCursorAgentsMarkdown(root, state.DocPath("agents_md"))...)
	return diagnostics, nil
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
			return fmt.Errorf("Cursor native import does not support symlinks under .cursor/rules")
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
			return fmt.Errorf("Cursor native import only supports .mdc files under .cursor/rules: %s", filepath.ToSlash(filepath.Join(".cursor", "rules", rel)))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join("targets", "cursor", "rules", rel)),
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
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule file %s must stay within targets/cursor/rules without path traversal", rel),
			})
			continue
		}
		lower := strings.ToLower(clean)
		if prior, ok := seenCaseFolded[lower]; ok && prior != clean {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule files %s and %s collide on case-insensitive filesystems", prior, rel),
			})
		} else {
			seenCaseFolded[lower] = clean
		}
		if strings.ToLower(filepath.Ext(rel)) != ".mdc" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule file %s must use the .mdc extension", rel),
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
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule file %s must not be a symlink", rel),
			})
			continue
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule file %s is not readable: %v", rel, err),
			})
			continue
		}
		if strings.TrimSpace(string(body)) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor rule file %s must not be empty", rel),
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
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor AGENTS markdown %s is not readable: %v", rel, err),
		}}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor AGENTS markdown %s must not be a symlink", rel),
		}}
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor AGENTS markdown %s is not readable: %v", rel, err),
		}}
	}
	if strings.TrimSpace(string(body)) == "" {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor AGENTS markdown %s must not be empty", rel),
		}}
	}
	return nil
}
