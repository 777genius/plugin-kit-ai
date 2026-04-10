package claude

import (
	"context"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

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
