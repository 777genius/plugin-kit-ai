package filesystem

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/adapters/frontmatter"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/domain"
	"gopkg.in/yaml.v3"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

type InitTemplate string

const (
	TemplateGoCommand  InitTemplate = "go-command"
	TemplateCLIWrapper InitTemplate = "cli-wrapper"
	TemplateDocsOnly   InitTemplate = "docs-only"
)

type TemplateData struct {
	SkillName            string
	Description          string
	Command              string
	CommandLine          string
	Runtime              string
	AllowedTools         []string
	CompatibilitySummary []string
	ExecutionNotes       []string
	Body                 string
}

type Repository struct{}

func (Repository) LoadSkill(root, name string) (domain.SkillDocument, error) {
	full := filepath.Join(root, "skills", name, "SKILL.md")
	body, err := os.ReadFile(full)
	if err != nil {
		return domain.SkillDocument{}, err
	}
	return frontmatter.Parse(body)
}

func (Repository) Discover(root string) ([]string, error) {
	skillsRoot := filepath.Join(root, "skills")
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(skillsRoot, entry.Name(), "SKILL.md")); err == nil {
			out = append(out, entry.Name())
		}
	}
	return out, nil
}

func (Repository) InitSkill(root string, data TemplateData, templateName InitTemplate, force bool) error {
	skillRoot := filepath.Join(root, "skills", data.SkillName)
	if err := os.MkdirAll(skillRoot, 0o755); err != nil {
		return err
	}
	var files []struct {
		path string
		tpl  string
	}
	switch templateName {
	case TemplateGoCommand:
		files = []struct {
			path string
			tpl  string
		}{
			{path: filepath.Join("skills", data.SkillName, "SKILL.md"), tpl: "skill.go-command.md.tmpl"},
			{path: filepath.Join("cmd", data.SkillName, "main.go"), tpl: "skill.go-command.main.go.tmpl"},
			{path: filepath.Join("cmd", data.SkillName, "main_test.go"), tpl: "skill.go-command.test.go.tmpl"},
		}
	case TemplateCLIWrapper:
		files = []struct {
			path string
			tpl  string
		}{
			{path: filepath.Join("skills", data.SkillName, "SKILL.md"), tpl: "skill.cli-wrapper.md.tmpl"},
			{path: filepath.Join("skills", data.SkillName, "scripts", ".keep"), tpl: ""},
		}
	case TemplateDocsOnly:
		files = []struct {
			path string
			tpl  string
		}{
			{path: filepath.Join("skills", data.SkillName, "SKILL.md"), tpl: "skill.docs-only.md.tmpl"},
			{path: filepath.Join("skills", data.SkillName, "references", ".keep"), tpl: ""},
		}
	default:
		return fmt.Errorf("unknown skill template %q", templateName)
	}
	for _, file := range files {
		if err := writeOne(root, file.path, file.tpl, data, force); err != nil {
			return err
		}
	}
	return nil
}

func (Repository) WriteArtifacts(root string, artifacts []domain.Artifact) error {
	for _, artifact := range artifacts {
		full := filepath.Join(root, artifact.RelPath)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, artifact.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (Repository) RemoveArtifacts(root string, relPaths []string) error {
	for _, relPath := range relPaths {
		full := filepath.Join(root, relPath)
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := pruneEmptyParents(root, filepath.Dir(full)); err != nil {
			return err
		}
	}
	return nil
}

func (Repository) ListManagedArtifacts(root string, selectedTargets map[string]struct{}) ([]string, error) {
	seen := make(map[string]struct{})
	if _, ok := selectedTargets["claude"]; ok {
		if err := walkManagedFiles(filepath.Join(root, "generated", "skills", "claude"), func(relPath string) {
			seen[filepath.ToSlash(relPath)] = struct{}{}
		}); err != nil {
			return nil, err
		}
	}
	if _, ok := selectedTargets["codex"]; ok {
		if err := walkManagedFiles(filepath.Join(root, "generated", "skills", "codex"), func(relPath string) {
			seen[filepath.ToSlash(relPath)] = struct{}{}
		}); err != nil {
			return nil, err
		}
	}
	if err := walkGeneratedCommandDocs(filepath.Join(root, "commands"), func(relPath string) {
		seen[filepath.ToSlash(relPath)] = struct{}{}
	}); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for relPath := range seen {
		out = append(out, filepath.FromSlash(relPath))
	}
	sort.Strings(out)
	return out, nil
}

func RenderTemplate(name string, data TemplateData) ([]byte, error) {
	raw, err := tmplFS.ReadFile(filepath.Join("templates", name))
	if err != nil {
		return nil, err
	}
	tpl, err := template.New(name).Funcs(template.FuncMap{
		"yamlString": yamlString,
	}).Parse(string(raw))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeOne(root, relPath, templateName string, data TemplateData, force bool) error {
	full := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(full); err == nil && !force {
		return fmt.Errorf("refusing to overwrite existing file %s (use --force)", relPath)
	}
	if templateName == "" {
		return os.WriteFile(full, nil, 0o644)
	}
	body, err := RenderTemplate(templateName, data)
	if err != nil {
		return err
	}
	return os.WriteFile(full, body, 0o644)
}

func CommandLine(spec domain.SkillSpec) string {
	command := strings.TrimSpace(spec.Command)
	if len(spec.Args) == 0 {
		return command
	}
	parts := []string{command}
	for _, arg := range spec.Args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func quoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"`$&|;<>*?()[]{}\\") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func yamlString(v string) string {
	body, err := yaml.Marshal(v)
	if err != nil {
		return `""`
	}
	return strings.TrimSpace(string(body))
}

func walkManagedFiles(root string, add func(relPath string)) error {
	base := filepath.Dir(filepath.Dir(filepath.Dir(root)))
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		add(relPath)
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func walkGeneratedCommandDocs(root string, add func(relPath string)) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		full := filepath.Join(root, entry.Name())
		body, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		text := string(body)
		if !strings.Contains(text, "This file is generated from `skills/") || !strings.Contains(text, "Regenerate with `plugin-kit-ai skills render`.") {
			continue
		}
		add(filepath.Join("commands", entry.Name()))
	}
	return nil
}

func pruneEmptyParents(root, dir string) error {
	root = filepath.Clean(root)
	dir = filepath.Clean(dir)
	for dir != root && dir != "." && dir != string(filepath.Separator) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				dir = filepath.Dir(dir)
				continue
			}
			return err
		}
		if len(entries) > 0 {
			return nil
		}
		if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil
		}
		dir = parent
	}
	return nil
}
