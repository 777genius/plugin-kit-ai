package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
)

// projectClaude renders the portable envelope into Claude Code's official
// skills-directory plugin layout. The active directory is discovered in place
// as <manifest-name>@skills-dir, so no mutable marketplace cache is involved.
func projectClaude(root string, envelope domain.PackageEnvelope, plan domain.DeliveryPlan, dataPath string) error {
	manifest := map[string]any{"name": envelope.Manifest.Name}
	copyString := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			manifest[key] = value
		}
	}
	copyString("version", envelope.Manifest.Version)
	copyString("description", envelope.Manifest.Description)
	copyString("homepage", envelope.Manifest.Homepage)
	copyString("repository", envelope.Manifest.Repository)
	copyString("license", envelope.Manifest.License)
	if envelope.Manifest.Author != nil {
		manifest["author"] = envelope.Manifest.Author
	}
	if len(envelope.Manifest.Keywords) > 0 {
		manifest["keywords"] = envelope.Manifest.Keywords
	}
	if hasSupported(plan, domain.ComponentSkill) {
		manifest["skills"] = "./skills/"
	}

	serverNames := supportedMCPNames(plan)
	if len(serverNames) > 0 {
		manifest["mcpServers"] = "./.mcp.json"
	}
	if err := writeJSON(filepath.Join(root, ".claude-plugin", "plugin.json"), manifest); err != nil {
		return fmt.Errorf("write Claude Code plugin manifest: %w", err)
	}
	if err := projectClaudeMCP(root, envelope, serverNames, plan.ActivePath, dataPath); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(root, "mcp.json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove portable MCP source after Claude projection: %w", err)
	}
	return nil
}

func projectClaudeMCP(root string, envelope domain.PackageEnvelope, serverNames []string, pluginRoot, dataPath string) error {
	path := filepath.Join(root, ".mcp.json")
	if len(serverNames) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove empty Claude MCP projection: %w", err)
		}
		return nil
	}
	servers := make(map[string]map[string]any, len(serverNames))
	for _, name := range serverNames {
		server := envelope.MCP.Servers[name]
		config := cloneObject(server.Decoded)
		switch server.Type {
		case "stdio":
			delete(config, "type")
			if err := applyStdioDataContract(config, pluginRoot, dataPath); err != nil {
				return fmt.Errorf("project Claude stdio MCP server %s: %w", name, err)
			}
		case "streamable-http":
			config["type"] = "http"
		case "sse":
			config["type"] = "sse"
		default:
			continue
		}
		servers[name] = config
	}
	return writeJSON(path, servers)
}
