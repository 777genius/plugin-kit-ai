package codex

import (
	"context"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (a Adapter) renderCodexMCP(ctx context.Context, sourceRoot string) (map[string]any, error) {
	loader := portablemcp.Loader{FS: a.fs()}
	loaded, err := loader.LoadForTarget(ctx, sourceRoot, domain.TargetCodex)
	if err != nil {
		if derr, ok := err.(*domain.Error); ok && derr.Code == domain.ErrManifestLoad {
			message := strings.ToLower(derr.Message)
			if strings.Contains(message, "portable mcp file not found") {
				return nil, nil
			}
			if strings.Contains(message, "does not define any servers for codex") {
				loaded, err = loader.LoadForTarget(ctx, sourceRoot, domain.TargetID("codex-package"))
				if err == nil {
					return renderCodexLoadedMCP(loaded)
				}
			}
		}
		return nil, err
	}
	return renderCodexLoadedMCP(loaded)
}

func renderCodexLoadedMCP(loaded portablemcp.Loaded) (map[string]any, error) {
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
			item := map[string]any{"command": server.Stdio.Command}
			if len(server.Stdio.Args) > 0 {
				item["args"] = append([]string(nil), server.Stdio.Args...)
			}
			if len(server.Stdio.Env) > 0 {
				item["env"] = server.Stdio.Env
			}
			out[alias] = item
		case "remote":
			item := map[string]any{"url": server.Remote.URL}
			switch strings.ToLower(strings.TrimSpace(server.Remote.Protocol)) {
			case "streamable_http":
				item["type"] = "http"
			default:
				item["type"] = "sse"
			}
			if len(server.Remote.Headers) > 0 {
				item["headers"] = server.Remote.Headers
			}
			out[alias] = item
		default:
			return nil, domain.NewError(domain.ErrMutationApply, "unsupported Codex portable MCP server type "+server.Type, nil)
		}
	}
	return out, nil
}
