package codex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"gopkg.in/yaml.v3"
)

func (a Adapter) materializeAuthoredCodexSource(ctx context.Context, manifest domain.IntegrationManifest, sourceRoot, destRoot string) error {
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Codex materialized package root", err)
	}
	doc := map[string]any{
		"name":        manifest.IntegrationID,
		"version":     manifest.Version,
		"description": manifest.Description,
	}
	if err := mergeCodexPackageMeta(doc, filepath.Join(sourceRoot, "src", "targets", "codex-package", "package.yaml")); err != nil {
		return err
	}
	if hasSkills, err := copyPathIfExists(filepath.Join(sourceRoot, "src", "skills"), filepath.Join(destRoot, "skills")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Codex skills", err)
	} else if hasSkills {
		doc["skills"] = "./skills/"
	}
	if fileExists(filepath.Join(sourceRoot, "src", "targets", "codex-package", "interface.json")) {
		value, err := readAnyJSON(filepath.Join(sourceRoot, "src", "targets", "codex-package", "interface.json"))
		if err != nil {
			return err
		}
		doc["interface"] = value
	}
	if fileExists(filepath.Join(sourceRoot, "src", "targets", "codex-package", "app.json")) {
		if err := copyFile(filepath.Join(sourceRoot, "src", "targets", "codex-package", "app.json"), filepath.Join(destRoot, ".app.json")); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Codex app manifest", err)
		}
		doc["apps"] = "./.app.json"
	}
	if mcp, err := a.renderCodexMCP(ctx, sourceRoot); err != nil {
		return err
	} else if len(mcp) > 0 {
		body, err := marshalJSON(mcp)
		if err != nil {
			return domain.NewError(domain.ErrMutationApply, "marshal Codex MCP config", err)
		}
		if err := os.WriteFile(filepath.Join(destRoot, ".mcp.json"), body, 0o644); err != nil {
			return domain.NewError(domain.ErrMutationApply, "write Codex MCP config", err)
		}
		doc["mcpServers"] = "./.mcp.json"
	}
	if err := mergeCodexManifestExtra(doc, filepath.Join(sourceRoot, "src", "targets", "codex-package", "manifest.extra.json")); err != nil {
		return err
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Codex plugin manifest", err)
	}
	if err := os.MkdirAll(filepath.Join(destRoot, ".codex-plugin"), 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Codex plugin manifest dir", err)
	}
	if err := os.WriteFile(filepath.Join(destRoot, ".codex-plugin", "plugin.json"), body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Codex plugin manifest", err)
	}
	return nil
}

func mergeCodexPackageMeta(doc map[string]any, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return domain.NewError(domain.ErrMutationApply, "read Codex package.yaml", err)
	}
	var meta packageMeta
	if err := yaml.Unmarshal(body, &meta); err != nil {
		return domain.NewError(domain.ErrMutationApply, "parse Codex package.yaml", err)
	}
	if meta.Author != nil {
		authorDoc := map[string]any{}
		if strings.TrimSpace(meta.Author.Name) != "" {
			authorDoc["name"] = strings.TrimSpace(meta.Author.Name)
		}
		if strings.TrimSpace(meta.Author.Email) != "" {
			authorDoc["email"] = strings.TrimSpace(meta.Author.Email)
		}
		if strings.TrimSpace(meta.Author.URL) != "" {
			authorDoc["url"] = strings.TrimSpace(meta.Author.URL)
		}
		if len(authorDoc) > 0 {
			doc["author"] = authorDoc
		}
	}
	if strings.TrimSpace(meta.Homepage) != "" {
		doc["homepage"] = strings.TrimSpace(meta.Homepage)
	}
	if strings.TrimSpace(meta.Repository) != "" {
		doc["repository"] = strings.TrimSpace(meta.Repository)
	}
	if strings.TrimSpace(meta.License) != "" {
		doc["license"] = strings.TrimSpace(meta.License)
	}
	if len(meta.Keywords) > 0 {
		keywords := make([]string, 0, len(meta.Keywords))
		for _, item := range meta.Keywords {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			keywords = append(keywords, item)
		}
		if len(keywords) > 0 {
			doc["keywords"] = keywords
		}
	}
	return nil
}

func mergeCodexManifestExtra(doc map[string]any, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return domain.NewError(domain.ErrMutationApply, "read Codex manifest.extra.json", err)
	}
	var extra map[string]any
	if err := json.Unmarshal(body, &extra); err != nil {
		return domain.NewError(domain.ErrMutationApply, "parse Codex manifest.extra.json", err)
	}
	for key, value := range extra {
		switch strings.TrimSpace(key) {
		case "", "name", "version", "description", "author", "homepage", "repository", "license", "keywords", "interface", "apps", "mcpServers", "skills":
			return domain.NewError(domain.ErrMutationApply, "Codex manifest.extra.json may not override managed key "+key, nil)
		default:
			doc[key] = value
		}
	}
	return nil
}

func readAnyJSON(path string) (any, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, domain.NewError(domain.ErrMutationApply, "read JSON document", err)
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, domain.NewError(domain.ErrMutationApply, "parse JSON document", err)
	}
	return doc, nil
}

func marshalJSON(doc any) ([]byte, error) {
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
