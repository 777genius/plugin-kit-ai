package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type cursorAdapter struct{}

const (
	cursorPluginManifestPath = ".cursor-plugin/plugin.json"
	cursorPluginMCPRef       = "./.mcp.json"
)

func (cursorAdapter) ID() string { return "cursor" }

func (cursorAdapter) DetectNative(root string) bool {
	return fileExists(filepath.Join(root, ".cursor-plugin", "plugin.json"))
}

func (cursorAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	targetDir := filepath.Join(root, pluginmodel.SourceDirName, "targets", "cursor")
	entries, err := os.ReadDir(targetDir)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return err
	case len(entries) == 0:
		return nil
	default:
		return fmt.Errorf("target cursor does not support src/targets/cursor/... in phase 1: use src/skills/** and src/mcp/servers.yaml, or move repo-local Cursor config to src/targets/cursor-workspace/...")
	}
}

func (cursorAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	manifest, ok, err := readImportedCursorPluginManifest(root)
	if err != nil {
		return ImportResult{}, err
	}
	if !ok {
		return ImportResult{}, fmt.Errorf("Cursor plugin import requires %s", cursorPluginManifestPath)
	}
	if name := stringMapField(manifest, "name"); name != "" {
		result.Manifest.Name = name
	}
	if version := stringMapField(manifest, "version"); version != "" {
		result.Manifest.Version = version
	}
	if description := stringMapField(manifest, "description"); description != "" {
		result.Manifest.Description = description
	}
	if servers, ok, err := importedCursorPluginMCP(root, manifest); err != nil {
		return ImportResult{}, err
	} else if ok {
		artifact, err := importedPortableMCPArtifact("cursor", servers)
		if err != nil {
			return ImportResult{}, err
		}
		result.Artifacts = append(result.Artifacts, artifact)
	}
	skillArtifacts, err := copyArtifactDirs(root, artifactDir{src: "skills", dst: "skills"})
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, skillArtifacts...)
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (cursorAdapter) Generate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	doc := map[string]any{
		"name":        graph.Manifest.Name,
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	var artifacts []pluginmodel.Artifact
	if graph.Portable.MCP != nil {
		doc["mcpServers"] = cursorPluginMCPRef
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "cursor")
		if err != nil {
			return nil, err
		}
		body, err := marshalJSON(projected)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: ".mcp.json",
			Content: body,
		})
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, pluginmodel.Artifact{
		RelPath: cursorPluginManifestPath,
		Content: body,
	})
	skillArtifacts, err := renderPortableSkills(root, graph.Portable.Paths("skills"), "skills")
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, skillArtifacts...)
	return compactArtifacts(artifacts), nil
}

func (cursorAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	return nil, nil
}

func (cursorAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	manifest, ok, err := readImportedCursorPluginManifest(root)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s is invalid: %v", cursorPluginManifestPath, err),
		}}, nil
	}
	if !ok {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s is not readable", cursorPluginManifestPath),
		}}, nil
	}
	if got := stringMapField(manifest, "name"); got != strings.TrimSpace(graph.Manifest.Name) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s sets name %q; expected %q from plugin.yaml", cursorPluginManifestPath, got, strings.TrimSpace(graph.Manifest.Name)),
		})
	}
	if got := stringMapField(manifest, "version"); got != strings.TrimSpace(graph.Manifest.Version) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s sets version %q; expected %q from plugin.yaml", cursorPluginManifestPath, got, strings.TrimSpace(graph.Manifest.Version)),
		})
	}
	if got := stringMapField(manifest, "description"); got != strings.TrimSpace(graph.Manifest.Description) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s sets description %q; expected %q from plugin.yaml", cursorPluginManifestPath, got, strings.TrimSpace(graph.Manifest.Description)),
		})
	}
	if graph.Portable.MCP != nil {
		ref, ok := manifest["mcpServers"]
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     cursorPluginManifestPath,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor plugin manifest %s must reference %q when portable MCP is authored", cursorPluginManifestPath, cursorPluginMCPRef),
			})
			return diagnostics, nil
		}
		refText, ok := ref.(string)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     cursorPluginManifestPath,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor plugin manifest %s field %q must be the string %q", cursorPluginManifestPath, "mcpServers", cursorPluginMCPRef),
			})
			return diagnostics, nil
		}
		if strings.TrimSpace(refText) != cursorPluginMCPRef {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     cursorPluginManifestPath,
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor plugin manifest %s must use %q for mcpServers when portable MCP is present", cursorPluginManifestPath, cursorPluginMCPRef),
			})
		}
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "cursor")
		if err != nil {
			return nil, err
		}
		rendered, ok, err := readCursorMCPServers(filepath.Join(root, ".mcp.json"), ".mcp.json")
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     ".mcp.json",
				Target:   "cursor",
				Message:  fmt.Sprintf("Cursor plugin MCP manifest .mcp.json is invalid: %v", err),
			})
			return diagnostics, nil
		}
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     ".mcp.json",
				Target:   "cursor",
				Message:  "Cursor plugin MCP manifest .mcp.json is not readable",
			})
			return diagnostics, nil
		}
		if !jsonDocumentsEqual(projected, rendered) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     ".mcp.json",
				Target:   "cursor",
				Message:  "Cursor plugin MCP manifest .mcp.json does not match authored portable MCP projection",
			})
		}
	} else if _, ok := manifest["mcpServers"]; ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     cursorPluginManifestPath,
			Target:   "cursor",
			Message:  fmt.Sprintf("Cursor plugin manifest %s may not define mcpServers when portable MCP is absent", cursorPluginManifestPath),
		})
	}
	return diagnostics, nil
}

func readImportedCursorPluginManifest(root string) (map[string]any, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, ".cursor-plugin", "plugin.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	doc, err := decodeJSONObject(body, "Cursor plugin manifest .cursor-plugin/plugin.json")
	if err != nil {
		return nil, false, err
	}
	return doc, true, nil
}

func importedCursorPluginMCP(root string, manifest map[string]any) (map[string]any, bool, error) {
	value, ok := manifest["mcpServers"]
	if !ok {
		if servers, ok, err := readCursorMCPServers(filepath.Join(root, ".mcp.json"), ".mcp.json"); err != nil {
			return nil, false, err
		} else if ok {
			return servers, true, nil
		}
		return nil, false, nil
	}
	switch typed := value.(type) {
	case nil:
		return nil, false, nil
	case string:
		ref := strings.TrimSpace(typed)
		switch ref {
		case "", cursorPluginMCPRef, ".mcp.json":
		default:
			return nil, false, fmt.Errorf("unsupported Cursor plugin mcpServers ref %q: use %q", ref, cursorPluginMCPRef)
		}
		return readCursorMCPServers(filepath.Join(root, ".mcp.json"), ".mcp.json")
	case map[string]any:
		return typed, true, nil
	default:
		return nil, false, fmt.Errorf("Cursor plugin field %q must be a string ref or object", "mcpServers")
	}
}

func stringMapField(doc map[string]any, key string) string {
	value, ok := doc[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
