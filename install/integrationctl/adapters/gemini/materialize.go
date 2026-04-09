package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"gopkg.in/yaml.v3"
)

type packageMeta struct {
	ContextFileName string   `yaml:"context_file_name,omitempty"`
	ExcludeTools    []string `yaml:"exclude_tools,omitempty"`
	MigratedTo      string   `yaml:"migrated_to,omitempty"`
	PlanDirectory   string   `yaml:"plan_directory,omitempty"`
}

type settingDoc struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	EnvVar      string `yaml:"env_var"`
	Sensitive   bool   `yaml:"sensitive"`
}

func (a Adapter) copyNativeGeminiPackage(sourceRoot, destRoot string) error {
	if err := copyFile(filepath.Join(sourceRoot, "gemini-extension.json"), filepath.Join(destRoot, "gemini-extension.json")); err != nil {
		return domain.NewError(domain.ErrMutationApply, "copy Gemini manifest", err)
	}
	manifestBody, err := os.ReadFile(filepath.Join(sourceRoot, "gemini-extension.json"))
	if err != nil {
		return domain.NewError(domain.ErrMutationApply, "read Gemini manifest", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBody, &manifest); err != nil {
		return domain.NewError(domain.ErrMutationApply, "parse Gemini manifest", err)
	}
	contextName, _ := manifest["contextFileName"].(string)
	contextName = strings.TrimSpace(filepath.Base(contextName))
	if contextName == "" && fileExists(filepath.Join(sourceRoot, "GEMINI.md")) {
		contextName = "GEMINI.md"
	}
	if contextName != "" && fileExists(filepath.Join(sourceRoot, contextName)) {
		if err := copyFile(filepath.Join(sourceRoot, contextName), filepath.Join(destRoot, contextName)); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Gemini primary context", err)
		}
	}
	for _, dir := range []string{"commands", "contexts", "hooks", "policies", "skills", "agents"} {
		if err := copyDirIfExists(filepath.Join(sourceRoot, dir), filepath.Join(destRoot, dir)); err != nil {
			return domain.NewError(domain.ErrMutationApply, "copy Gemini package directory "+dir, err)
		}
	}
	return nil
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
	if strings.TrimSpace(meta.MigratedTo) != "" {
		doc["migratedTo"] = strings.TrimSpace(meta.MigratedTo)
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

func (a Adapter) loadProjectedMCP(ctx context.Context, sourceRoot string) (map[string]any, error) {
	loader := portablemcp.Loader{FS: a.fs()}
	loaded, err := loader.LoadForTarget(ctx, sourceRoot, domain.TargetGemini)
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
			doc := map[string]any{
				"command": server.Stdio.Command,
			}
			if len(server.Stdio.Args) > 0 {
				args := make([]string, 0, len(server.Stdio.Args))
				for _, arg := range server.Stdio.Args {
					args = append(args, strings.ReplaceAll(arg, "${package.root}", "${extensionPath}"))
				}
				doc["args"] = args
			}
			if len(server.Stdio.Env) > 0 {
				env := map[string]string{}
				for key, value := range server.Stdio.Env {
					env[key] = strings.ReplaceAll(value, "${package.root}", "${extensionPath}")
				}
				doc["env"] = env
			}
			out[alias] = doc
		case "remote":
			doc := map[string]any{}
			switch strings.ToLower(strings.TrimSpace(server.Remote.Protocol)) {
			case "streamable_http":
				doc["httpUrl"] = server.Remote.URL
			default:
				doc["url"] = server.Remote.URL
			}
			if len(server.Remote.Headers) > 0 {
				doc["headers"] = server.Remote.Headers
			}
			out[alias] = doc
		default:
			return nil, domain.NewError(domain.ErrMutationApply, "unsupported Gemini portable MCP server type "+server.Type, nil)
		}
	}
	return out, nil
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
	meta.MigratedTo = strings.TrimSpace(meta.MigratedTo)
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

func loadSettings(root string) ([]map[string]any, error) {
	rels, err := discoverFiles(root)
	if err != nil {
		return nil, domain.NewError(domain.ErrMutationApply, "discover Gemini settings", err)
	}
	if len(rels) == 0 {
		return nil, nil
	}
	seenNames := map[string]string{}
	seenEnv := map[string]string{}
	out := make([]map[string]any, 0, len(rels))
	for _, rel := range rels {
		if !isYAMLPath(rel) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "read Gemini setting", err)
		}
		var doc settingDoc
		if err := yaml.Unmarshal(body, &doc); err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "parse Gemini setting", err)
		}
		doc.Name = strings.TrimSpace(doc.Name)
		doc.Description = strings.TrimSpace(doc.Description)
		doc.EnvVar = strings.TrimSpace(doc.EnvVar)
		if doc.Name == "" || doc.Description == "" || doc.EnvVar == "" {
			return nil, domain.NewError(domain.ErrMutationApply, "Gemini settings require non-empty name, description, and env_var", nil)
		}
		nameKey := strings.ToLower(doc.Name)
		if prev, ok := seenNames[nameKey]; ok {
			return nil, domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Gemini setting name %q duplicates %s", doc.Name, prev), nil)
		}
		envKey := strings.ToLower(doc.EnvVar)
		if prev, ok := seenEnv[envKey]; ok {
			return nil, domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Gemini setting env_var %q duplicates %s", doc.EnvVar, prev), nil)
		}
		seenNames[nameKey] = rel
		seenEnv[envKey] = rel
		out = append(out, map[string]any{
			"name":        doc.Name,
			"description": doc.Description,
			"envVar":      doc.EnvVar,
			"sensitive":   doc.Sensitive,
		})
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func loadThemes(root string) ([]map[string]any, error) {
	rels, err := discoverFiles(root)
	if err != nil {
		return nil, domain.NewError(domain.ErrMutationApply, "discover Gemini themes", err)
	}
	if len(rels) == 0 {
		return nil, nil
	}
	seenNames := map[string]string{}
	out := make([]map[string]any, 0, len(rels))
	for _, rel := range rels {
		if !isYAMLPath(rel) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "read Gemini theme", err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(body, &doc); err != nil {
			return nil, domain.NewError(domain.ErrMutationApply, "parse Gemini theme", err)
		}
		if doc == nil {
			doc = map[string]any{}
		}
		name, _ := doc["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, domain.NewError(domain.ErrMutationApply, "Gemini themes require a non-empty name", nil)
		}
		if prev, ok := seenNames[strings.ToLower(name)]; ok {
			return nil, domain.NewError(domain.ErrMutationApply, fmt.Sprintf("Gemini theme name %q duplicates %s", name, prev), nil)
		}
		seenNames[strings.ToLower(name)] = rel
		out = append(out, doc)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
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
		case "", "name", "version", "description", "mcpServers", "contextFileName", "excludeTools", "migratedTo", "settings", "themes":
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

func discoverFiles(root string) ([]string, error) {
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(out)
	return out, nil
}

func copyDirIfExists(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
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

func isYAMLPath(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func marshalJSON(value any) ([]byte, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}
