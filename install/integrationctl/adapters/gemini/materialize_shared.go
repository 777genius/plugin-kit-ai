package gemini

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
	"gopkg.in/yaml.v3"
)

type settingDoc struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	EnvVar      string `yaml:"env_var"`
	Sensitive   bool   `yaml:"sensitive"`
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
