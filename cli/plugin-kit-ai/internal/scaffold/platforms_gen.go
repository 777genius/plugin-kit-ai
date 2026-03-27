package scaffold

import (
	"strings"
)

var generatedPlatforms = map[string]PlatformDefinition{
	"claude": {
		Name: "claude",
		Files: []TemplateFile{
			{Path: "go.mod", Template: "go.mod.tmpl", Extra: false},
			{Path: "cmd/{{.ProjectName}}/main.go", Template: "main.go.tmpl", Extra: false},
			{Path: "plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "README.md", Template: "README.md.tmpl", Extra: false},
			{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
			{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
			{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
			{Path: "commands/{{.ProjectName}}.md", Template: "command.md.tmpl", Extra: true},
		},
	},
	"codex": {
		Name: "codex",
		Files: []TemplateFile{
			{Path: "go.mod", Template: "codex.go.mod.tmpl", Extra: false},
			{Path: "cmd/{{.ProjectName}}/main.go", Template: "codex.main.go.tmpl", Extra: false},
			{Path: "plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "codex.AGENTS.md.tmpl", Extra: false},
			{Path: "README.md", Template: "codex.README.md.tmpl", Extra: false},
			{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
			{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
			{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
			{Path: "commands/{{.ProjectName}}.md", Template: "command.md.tmpl", Extra: true},
		},
	},
}

func LookupPlatform(name string) (PlatformDefinition, bool) {
	p, ok := generatedPlatforms[normalizePlatform(name)]
	return p, ok
}

func normalizePlatform(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "codex"
	}
	return name
}
