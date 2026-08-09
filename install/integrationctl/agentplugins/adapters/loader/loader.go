package loader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
)

type Loader struct {
	Registry ports.SchemaRegistry
}

func (loader Loader) Load(ctx context.Context, input domain.LoadInput) (domain.PackageEnvelope, error) {
	if err := ctx.Err(); err != nil {
		return domain.PackageEnvelope{}, err
	}
	if loader.Registry == nil {
		return domain.PackageEnvelope{}, domain.FatalLoad("schema_registry_missing", "plugin.json", "Agent Plugins schema registry is required", nil)
	}
	root, err := filepath.Abs(input.SnapshotRoot)
	if err != nil {
		return domain.PackageEnvelope{}, domain.FatalLoad("snapshot_invalid", "plugin.json", "resolve package snapshot root", err)
	}
	root = filepath.Clean(root)
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return domain.PackageEnvelope{}, domain.FatalLoad("snapshot_invalid", "plugin.json", "package snapshot root must be a real directory", err)
	}
	manifest, diagnostics, manifestDigest, err := loader.loadPluginManifest(filepath.Join(root, "plugin.json"))
	if err != nil {
		return domain.PackageEnvelope{}, err
	}
	mcp, mcpDiagnostics := loader.loadMCP(filepath.Join(root, "mcp.json"))
	diagnostics = append(diagnostics, mcpDiagnostics...)
	skills, invalidSkills, invalidSkillsRoot, skillDiagnostics := loadSkills(filepath.Join(root, "skills"))
	diagnostics = append(diagnostics, skillDiagnostics...)

	inventory := domain.ComponentInventory{
		MCPPresent:        mcp.Present,
		MCPEnabled:        mcp.Enabled,
		MCPServers:        sortedMCPServerNames(mcp.Servers),
		Skills:            sortedSkillNames(skills),
		InvalidSkills:     invalidSkills,
		InvalidSkillsRoot: invalidSkillsRoot,
		Extensions:        sortedRawKeys(manifest.Extensions),
	}
	for name := range mcp.InvalidServer {
		inventory.InvalidMCPServer = append(inventory.InvalidMCPServer, name)
	}
	sort.Strings(inventory.InvalidMCPServer)

	executableFiles := append([]string(nil), input.ExecutableFiles...)
	sort.Strings(executableFiles)
	return domain.PackageEnvelope{
		LoaderKind:      domain.LoaderKindAgentPlugins,
		FormatID:        domain.FormatIDAgentPluginsV1,
		SchemaURI:       manifest.SchemaURI,
		SchemaVersion:   "1.0.0",
		ManifestSchema:  domain.SchemaIdentity{URI: manifest.SchemaURI, Version: "1.0.0"},
		Manifest:        manifest,
		MCP:             mcp,
		Skills:          skills,
		Inventory:       inventory,
		Diagnostics:     diagnostics,
		Source:          input.Source,
		TreeDigest:      strings.TrimSpace(input.TreeDigest),
		ManifestDigest:  manifestDigest,
		ExecutableFiles: executableFiles,
		SnapshotRoot:    root,
	}, nil
}

func readRegularFile(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("path is not a regular file")
	}
	body, err := os.ReadFile(path)
	return body, true, err
}

func sha256Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sortedRawKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMCPServerNames(values map[string]domain.MCPServer) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSkillNames(values map[string]domain.Skill) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
