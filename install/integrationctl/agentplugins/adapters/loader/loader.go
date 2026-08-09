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
	if err := rejectDiscoverableHooks(root); err != nil {
		return domain.PackageEnvelope{}, domain.FatalLoad(
			"official_hooks_unsupported", "hooks/hooks.json",
			"lifecycle hooks are auto-discovered by official clients but are not modeled by agentplugins v0.1; remove the hooks directory before installation", err,
		)
	}
	manifestPath := filepath.Join(root, "plugin.json")
	_, portable, probeErr := readRegularFile(manifestPath)
	if probeErr != nil {
		return domain.PackageEnvelope{}, domain.FatalLoad("plugin_manifest_read_failed", "plugin.json", "read root plugin.json", probeErr)
	}
	formatID := domain.FormatIDAgentPluginsV1
	schemaVersion := "1.0.0"
	mcpPath := filepath.Join(root, "mcp.json")
	appPath := filepath.Join(root, ".app.json")
	skillsPath := filepath.Join(root, "skills")
	appDeclared, acceptUndeclaredApp := false, true
	mcpDeclared := true

	var manifest domain.PluginManifest
	var diagnostics []domain.Diagnostic
	var manifestDigest string
	if portable {
		manifest, diagnostics, manifestDigest, err = loader.loadPluginManifest(manifestPath)
	} else {
		formatID = domain.FormatIDOpenAIPlugin
		schemaVersion = ""
		var components openAIComponentPaths
		manifest, components, diagnostics, manifestDigest, err = loader.loadOpenAIPluginManifest(filepath.Join(root, ".codex-plugin", "plugin.json"))
		mcpDeclared = components.MCP
		appDeclared = components.App
		acceptUndeclaredApp = false
		if !components.Skills {
			skillsPath = ""
		}
	}
	if err != nil {
		return domain.PackageEnvelope{}, err
	}

	var mcp domain.MCPComponent
	var mcpDiagnostics []domain.Diagnostic
	if portable {
		mcp, mcpDiagnostics = loader.loadMCP(mcpPath)
	} else {
		mcpPath = filepath.Join(root, ".mcp.json")
		mcp, mcpDiagnostics = loader.loadOpenAIMCP(mcpPath, mcpDeclared)
	}
	diagnostics = append(diagnostics, mcpDiagnostics...)
	app, appDiagnostics := loader.loadApp(appPath, appDeclared, acceptUndeclaredApp)
	diagnostics = append(diagnostics, appDiagnostics...)
	var skills map[string]domain.Skill
	var invalidSkills []string
	var invalidSkillsRoot bool
	var skillDiagnostics []domain.Diagnostic
	if skillsPath != "" {
		skills, invalidSkills, invalidSkillsRoot, skillDiagnostics = loadSkills(skillsPath)
	}
	diagnostics = append(diagnostics, skillDiagnostics...)

	inventory := domain.ComponentInventory{
		MCPPresent:        mcp.Present,
		MCPEnabled:        mcp.Enabled,
		MCPServers:        sortedMCPServerNames(mcp.Servers),
		AppPresent:        app.Present,
		AppBindings:       sortedAppBindingNames(app.Bindings),
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
		FormatID:        formatID,
		SchemaURI:       manifest.SchemaURI,
		SchemaVersion:   schemaVersion,
		ManifestSchema:  domain.SchemaIdentity{URI: manifest.SchemaURI, Version: schemaVersion},
		Manifest:        manifest,
		MCP:             mcp,
		App:             app,
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

func rejectDiscoverableHooks(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("inspect package root for auto-discovered hooks: %w", err)
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "hooks") {
			return fmt.Errorf("package root contains auto-discoverable hooks path %q", entry.Name())
		}
	}
	return nil
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

func sortedAppBindingNames(values map[string]domain.AppBinding) []string {
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
