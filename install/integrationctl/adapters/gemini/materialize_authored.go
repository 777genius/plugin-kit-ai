package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"gopkg.in/yaml.v3"
)

type packageMeta struct {
	ContextFileName string   `yaml:"context_file_name,omitempty"`
	ExcludeTools    []string `yaml:"exclude_tools,omitempty"`
	PlanDirectory   string   `yaml:"plan_directory,omitempty"`
}

func (a Adapter) materializeAuthoredGeminiSource(ctx context.Context, manifest domain.IntegrationManifest, sourceRoot, destRoot string) error {
	meta, err := loadPackageMeta(filepath.Join(sourceRoot, "src", "targets", "gemini", "package.yaml"))
	if err != nil {
		return err
	}
	doc := map[string]any{
		"name":        manifest.IntegrationID,
		"version":     manifest.Version,
		"description": manifest.Description,
	}
	if len(meta.ExcludeTools) > 0 {
		doc["excludeTools"] = append([]string(nil), meta.ExcludeTools...)
	}
	if strings.TrimSpace(meta.PlanDirectory) != "" {
		doc["plan"] = map[string]any{"directory": strings.TrimSpace(meta.PlanDirectory)}
	}
	if contextName, err := materializeAuthoredContexts(sourceRoot, destRoot, meta); err != nil {
		return err
	} else if contextName != "" {
		doc["contextFileName"] = contextName
	}
	if settings, err := loadSettings(filepath.Join(sourceRoot, "src", "targets", "gemini", "settings")); err != nil {
		return err
	} else if len(settings) > 0 {
		doc["settings"] = settings
	}
	if themes, err := loadThemes(filepath.Join(sourceRoot, "src", "targets", "gemini", "themes")); err != nil {
		return err
	} else if len(themes) > 0 {
		doc["themes"] = themes
	}
	if mcp, err := a.loadProjectedMCP(ctx, sourceRoot); err != nil {
		return err
	} else if len(mcp) > 0 {
		doc["mcpServers"] = mcp
	}
	if err := mergeManifestExtra(doc, filepath.Join(sourceRoot, "src", "targets", "gemini", "manifest.extra.json")); err != nil {
		return err
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "marshal Gemini manifest", err)
	}
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return domain.NewError(domain.ErrMutationApply, "prepare Gemini materialized root", err)
	}
	if err := os.WriteFile(filepath.Join(destRoot, "gemini-extension.json"), body, 0o644); err != nil {
		return domain.NewError(domain.ErrMutationApply, "write Gemini materialized manifest", err)
	}
	for _, pair := range [][2]string{
		{filepath.Join(sourceRoot, "src", "targets", "gemini", "commands"), filepath.Join(destRoot, "commands")},
		{filepath.Join(sourceRoot, "src", "targets", "gemini", "policies"), filepath.Join(destRoot, "policies")},
		{filepath.Join(sourceRoot, "src", "targets", "gemini", "agents"), filepath.Join(destRoot, "agents")},
		{filepath.Join(sourceRoot, "src", "skills"), filepath.Join(destRoot, "skills")},
	} {
		if err := copyDirIfExists(pair[0], pair[1]); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Gemini authored directory", err)
		}
	}
	hooksSrc := filepath.Join(sourceRoot, "src", "targets", "gemini", "hooks", "hooks.json")
	if fileExists(hooksSrc) {
		if err := copyFile(hooksSrc, filepath.Join(destRoot, "hooks", "hooks.json")); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Gemini hooks", err)
		}
	}
	return nil
}

func loadPackageMeta(path string) (packageMeta, error) {
	var meta packageMeta
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return meta, nil
		}
		return meta, domain.NewError(domain.ErrMutationApply, "read Gemini package metadata", err)
	}
	if err := yaml.Unmarshal(body, &meta); err != nil {
		return meta, domain.NewError(domain.ErrMutationApply, "parse Gemini package metadata", err)
	}
	meta.ContextFileName = strings.TrimSpace(meta.ContextFileName)
	meta.PlanDirectory = strings.TrimSpace(meta.PlanDirectory)
	return meta, nil
}

func materializeAuthoredContexts(sourceRoot, destRoot string, meta packageMeta) (string, error) {
	contextsRoot := filepath.Join(sourceRoot, "src", "targets", "gemini", "contexts")
	candidates, err := discoverFiles(contextsRoot)
	if err != nil {
		return "", domain.NewError(domain.ErrMutationApply, "discover Gemini contexts", err)
	}
	if len(candidates) == 0 {
		return "", nil
	}
	selected, ok, err := selectPrimaryContext(candidates, meta.ContextFileName)
	if err != nil {
		return "", domain.NewError(domain.ErrMutationApply, "select Gemini primary context", err)
	}
	if !ok {
		return "", nil
	}
	for _, rel := range candidates {
		src := filepath.Join(contextsRoot, rel)
		name := filepath.Base(rel)
		dest := filepath.Join(destRoot, "contexts", name)
		if rel == selected {
			dest = filepath.Join(destRoot, name)
		}
		if err := copyFile(src, dest); err != nil {
			return "", err
		}
	}
	return filepath.Base(selected), nil
}

func selectPrimaryContext(candidates []string, configured string) (string, bool, error) {
	configured = strings.TrimSpace(filepath.Base(configured))
	if configured != "" {
		var matches []string
		for _, candidate := range candidates {
			if filepath.Base(candidate) == configured {
				matches = append(matches, candidate)
			}
		}
		switch len(matches) {
		case 0:
			return "", false, fmt.Errorf("context_file_name %q does not resolve to a Gemini-native context source", configured)
		case 1:
			return matches[0], true, nil
		default:
			return "", false, fmt.Errorf("context_file_name %q is ambiguous across multiple context sources", configured)
		}
	}
	var gemini []string
	for _, candidate := range candidates {
		if filepath.Base(candidate) == "GEMINI.md" {
			gemini = append(gemini, candidate)
		}
	}
	switch len(gemini) {
	case 1:
		return gemini[0], true, nil
	case 0:
		if len(candidates) == 1 {
			return candidates[0], true, nil
		}
		if len(candidates) == 0 {
			return "", false, nil
		}
		return "", false, fmt.Errorf("primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly")
	default:
		return "", false, fmt.Errorf("primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly")
	}
}

func mergeManifestExtra(doc map[string]any, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return domain.NewError(domain.ErrMutationApply, "read Gemini manifest.extra.json", err)
	}
	var extra map[string]any
	if err := json.Unmarshal(body, &extra); err != nil {
		return domain.NewError(domain.ErrMutationApply, "parse Gemini manifest.extra.json", err)
	}
	for key, value := range extra {
		switch strings.TrimSpace(key) {
		case "", "name", "version", "description", "mcpServers", "contextFileName", "excludeTools", "settings", "themes":
			return domain.NewError(domain.ErrMutationApply, "Gemini manifest.extra.json may not override managed key "+key, nil)
		case "plan":
			raw, ok := value.(map[string]any)
			if !ok {
				return domain.NewError(domain.ErrMutationApply, "Gemini manifest.extra.json field plan must be an object", nil)
			}
			plan, _ := doc["plan"].(map[string]any)
			if plan == nil {
				plan = map[string]any{}
			}
			for childKey, childValue := range raw {
				if strings.TrimSpace(childKey) == "directory" {
					return domain.NewError(domain.ErrMutationApply, "Gemini manifest.extra.json may not override managed key plan.directory", nil)
				}
				plan[childKey] = childValue
			}
			doc["plan"] = plan
		default:
			doc[key] = value
		}
	}
	return nil
}
