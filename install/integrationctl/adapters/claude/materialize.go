package claude

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (a Adapter) syncManagedMarketplace(ctx context.Context, manifest domain.IntegrationManifest, sourceRoot string) (string, string, string, error) {
	root := filepath.Clean(sourceRoot)
	managedRoot := managedMarketplaceRoot(a.userHome(), manifest.IntegrationID)
	parent := filepath.Dir(managedRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", "", domain.NewError(domain.ErrMutationApply, "prepare Claude managed marketplace parent", err)
	}
	tmpRoot, err := os.MkdirTemp(parent, manifest.IntegrationID+".tmp-*")
	if err != nil {
		return "", "", "", domain.NewError(domain.ErrMutationApply, "create Claude managed marketplace temp root", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpRoot)
		}
	}()
	marketplaceName := managedMarketplaceName(manifest.IntegrationID)
	pluginDir := filepath.Join(tmpRoot, "plugins", manifest.IntegrationID)
	if fileExists(filepath.Join(root, "src", "plugin.yaml")) {
		if err := a.materializeAuthoredClaudeSource(ctx, manifest, root, pluginDir); err != nil {
			return "", "", "", err
		}
	} else if fileExists(filepath.Join(root, ".claude-plugin", "plugin.json")) {
		if err := copyNativeClaudePackage(root, pluginDir); err != nil {
			return "", "", "", err
		}
	} else {
		if err := a.materializeAuthoredClaudeSource(ctx, manifest, root, pluginDir); err != nil {
			return "", "", "", err
		}
	}
	if err := writeClaudeMarketplace(filepath.Join(tmpRoot, ".claude-plugin", "marketplace.json"), marketplaceName, manifest, "./plugins/"+manifest.IntegrationID); err != nil {
		return "", "", "", err
	}
	if err := os.RemoveAll(managedRoot); err != nil && !os.IsNotExist(err) {
		return "", "", "", domain.NewError(domain.ErrMutationApply, "replace Claude managed marketplace root", err)
	}
	if err := os.Rename(tmpRoot, managedRoot); err != nil {
		return "", "", "", domain.NewError(domain.ErrMutationApply, "activate Claude managed marketplace root", err)
	}
	cleanup = false
	return managedRoot, marketplaceName, manifest.IntegrationID + "@" + marketplaceName, nil
}

func copyNativeClaudePackage(sourceRoot, destRoot string) error {
	for _, path := range []string{
		filepath.Join(sourceRoot, ".claude-plugin"),
		filepath.Join(sourceRoot, ".mcp.json"),
		filepath.Join(sourceRoot, "settings.json"),
		filepath.Join(sourceRoot, ".lsp.json"),
		filepath.Join(sourceRoot, "hooks"),
		filepath.Join(sourceRoot, "skills"),
		filepath.Join(sourceRoot, "commands"),
		filepath.Join(sourceRoot, "agents"),
	} {
		if _, err := copyPathIfExists(path, filepath.Join(destRoot, filepath.Base(path))); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy native Claude package", err)
		}
	}
	return nil
}

func (a Adapter) materializeAuthoredClaudeSource(ctx context.Context, manifest domain.IntegrationManifest, sourceRoot, destRoot string) error {
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Claude materialized package root", err)
	}
	doc := map[string]any{
		"name":        manifest.IntegrationID,
		"version":     manifest.Version,
		"description": manifest.Description,
	}
	if hasSkills, err := copyPathIfExists(filepath.Join(sourceRoot, "src", "skills"), filepath.Join(destRoot, "skills")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Claude skills", err)
	} else if hasSkills {
		doc["skills"] = "./skills/"
	}
	if hasAgents, err := copyPathIfExists(filepath.Join(sourceRoot, "src", "targets", "claude", "agents"), filepath.Join(destRoot, "agents")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Claude agents", err)
	} else if hasAgents {
		doc["agents"] = "./agents/"
	}
	if fileExists(filepath.Join(sourceRoot, "src", "targets", "claude", "settings.json")) {
		if err := copyFile(filepath.Join(sourceRoot, "src", "targets", "claude", "settings.json"), filepath.Join(destRoot, "settings.json")); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Claude settings", err)
		}
	}
	if fileExists(filepath.Join(sourceRoot, "src", "targets", "claude", "lsp.json")) {
		if err := copyFile(filepath.Join(sourceRoot, "src", "targets", "claude", "lsp.json"), filepath.Join(destRoot, ".lsp.json")); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Claude LSP config", err)
		}
	}
	if fileExists(filepath.Join(sourceRoot, "src", "targets", "claude", "user-config.json")) {
		raw, err := readJSONObject(filepath.Join(sourceRoot, "src", "targets", "claude", "user-config.json"))
		if err != nil {
			return err
		}
		doc["userConfig"] = raw
	}
	if hasHooks, err := copyPathIfExists(filepath.Join(sourceRoot, "src", "targets", "claude", "hooks"), filepath.Join(destRoot, "hooks")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Claude hooks", err)
	} else if !hasHooks && fileExists(filepath.Join(sourceRoot, "src", "launcher.yaml")) {
		return domain.NewError(domain.ErrMutationApply, "Claude authored source with src/launcher.yaml requires generated native hooks or authored src/targets/claude/hooks", nil)
	}
	if _, err := copyPathIfExists(filepath.Join(sourceRoot, "src", "targets", "claude", "commands"), filepath.Join(destRoot, "commands")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Claude commands", err)
	}
	if mcp, err := a.renderClaudeMCP(ctx, sourceRoot); err != nil {
		return err
	} else if len(mcp) > 0 {
		body, err := marshalJSON(mcp)
		if err != nil {
			return domain.NewError(domain.ErrMutationApply, "marshal Claude MCP config", err)
		}
		if err := os.WriteFile(filepath.Join(destRoot, ".mcp.json"), body, 0o644); err != nil {
			return domain.NewError(domain.ErrMutationApply, "write Claude MCP config", err)
		}
		doc["mcpServers"] = "./.mcp.json"
	}
	if err := mergeClaudeManifestExtra(doc, filepath.Join(sourceRoot, "src", "targets", "claude", "manifest.extra.json")); err != nil {
		return err
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Claude plugin manifest", err)
	}
	if err := os.MkdirAll(filepath.Join(destRoot, ".claude-plugin"), 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Claude plugin manifest dir", err)
	}
	if err := os.WriteFile(filepath.Join(destRoot, ".claude-plugin", "plugin.json"), body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Claude plugin manifest", err)
	}
	return nil
}

func (a Adapter) renderClaudeMCP(ctx context.Context, sourceRoot string) (map[string]any, error) {
	loader := portablemcp.Loader{FS: a.fs()}
	loaded, err := loader.LoadForTarget(ctx, sourceRoot, domain.TargetClaude)
	if err != nil {
		if derr, ok := err.(*domain.Error); ok && derr.Code == domain.ErrManifestLoad && strings.Contains(strings.ToLower(derr.Message), "portable mcp file not found") {
			return nil, nil
		}
		return nil, err
	}
	out := make(map[string]any, len(loaded.Servers))
	aliases := make([]string, 0, len(loaded.Servers))
	for alias := range loaded.Servers {
		aliases = append(aliases, alias)
	}
	slices.Sort(aliases)
	for _, alias := range aliases {
		server := loaded.Servers[alias]
		switch server.Type {
		case "stdio":
			doc := map[string]any{"command": server.Stdio.Command}
			if len(server.Stdio.Args) > 0 {
				doc["args"] = append([]string(nil), server.Stdio.Args...)
			}
			if len(server.Stdio.Env) > 0 {
				doc["env"] = server.Stdio.Env
			}
			out[alias] = doc
		case "remote":
			doc := map[string]any{"url": server.Remote.URL}
			switch strings.ToLower(strings.TrimSpace(server.Remote.Protocol)) {
			case "streamable_http":
				doc["type"] = "http"
			default:
				doc["type"] = "sse"
			}
			if len(server.Remote.Headers) > 0 {
				doc["headers"] = server.Remote.Headers
			}
			out[alias] = doc
		default:
			return nil, domain.NewError(domain.ErrMutationApply, "unsupported Claude portable MCP server type "+server.Type, nil)
		}
	}
	return out, nil
}

func writeClaudeMarketplace(path, marketplaceName string, manifest domain.IntegrationManifest, sourceRoot string) error {
	body, err := marshalJSON(map[string]any{
		"name": marketplaceName,
		"owner": map[string]any{
			"name": "plugin-kit-ai",
		},
		"plugins": []map[string]any{
			{
				"name":        manifest.IntegrationID,
				"source":      sourceRoot,
				"description": manifest.Description,
				"version":     manifest.Version,
			},
		},
	})
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Claude marketplace catalog", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Claude marketplace catalog dir", err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Claude marketplace catalog", err)
	}
	return nil
}

func mergeClaudeManifestExtra(doc map[string]any, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return domain.NewError(domain.ErrMutationApply, "read Claude manifest.extra.json", err)
	}
	var extra map[string]any
	if err := json.Unmarshal(body, &extra); err != nil {
		return domain.NewError(domain.ErrMutationApply, "parse Claude manifest.extra.json", err)
	}
	for key, value := range extra {
		switch strings.TrimSpace(key) {
		case "", "name", "version", "description", "mcpServers", "userConfig", "skills", "agents":
			return domain.NewError(domain.ErrMutationApply, "Claude manifest.extra.json may not override managed key "+key, nil)
		default:
			doc[key] = value
		}
	}
	return nil
}

func readJSONObject(path string) (map[string]any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, domain.NewError(domain.ErrMutationApply, "read JSON object", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, domain.NewError(domain.ErrMutationApply, "parse JSON object", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	return doc, nil
}

func copyPathIfExists(src, dest string) (bool, error) {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.IsDir() {
		return true, filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dest, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			return copyFile(path, target)
		})
	}
	return true, copyFile(src, dest)
}

func copyFile(src, dest string) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, 0o644)
}

func marshalJSON(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
