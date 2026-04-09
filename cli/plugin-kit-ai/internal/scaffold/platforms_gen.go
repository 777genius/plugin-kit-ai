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
			{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "src/launcher.yaml", Template: "launcher.yaml.tmpl", Extra: false},
			{Path: "src/targets/claude/hooks/hooks.json", Template: "targets.claude.hooks.json.tmpl", Extra: false},
			{Path: "src/targets/claude/settings.json", Template: "empty.json.tmpl", Extra: true},
			{Path: "src/targets/claude/lsp.json", Template: "empty.json.tmpl", Extra: true},
			{Path: "src/targets/claude/user-config.json", Template: "empty.json.tmpl", Extra: true},
			{Path: "src/targets/claude/manifest.extra.json", Template: "empty.json.tmpl", Extra: true},
			{Path: "src/README.md", Template: "README.md.tmpl", Extra: false},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
			{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
			{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
			{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
		},
	},
	"codex-package": {
		Name: "codex-package",
		Files: []TemplateFile{
			{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "src/README.md", Template: "codex-package.README.md.tmpl", Extra: false},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
			{Path: "src/targets/codex-package/package.yaml", Template: "targets.codex-package.package.yaml.tmpl", Extra: true},
			{Path: "src/mcp/servers.yaml", Template: "mcp.servers.yaml.tmpl", Extra: true},
			{Path: "src/targets/codex-package/interface.json", Template: "codex-package.interface.json.tmpl", Extra: true},
			{Path: "src/targets/codex-package/manifest.extra.json", Template: "empty.json.tmpl", Extra: true},
			{Path: "src/targets/codex-package/app.json", Template: "empty.json.tmpl", Extra: true},
			{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
		},
	},
	"codex-runtime": {
		Name: "codex-runtime",
		Files: []TemplateFile{
			{Path: "go.mod", Template: "codex.go.mod.tmpl", Extra: false},
			{Path: "cmd/{{.ProjectName}}/main.go", Template: "codex.main.go.tmpl", Extra: false},
			{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "src/launcher.yaml", Template: "launcher.yaml.tmpl", Extra: false},
			{Path: "src/targets/codex-runtime/package.yaml", Template: "targets.codex-runtime.package.yaml.tmpl", Extra: false},
			{Path: "src/README.md", Template: "codex-runtime.README.md.tmpl", Extra: false},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
			{Path: "src/targets/codex-runtime/config.extra.toml", Template: "empty.toml.tmpl", Extra: true},
			{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
			{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
		},
	},
	"cursor": {
		Name: "cursor",
		Files: []TemplateFile{
			{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "src/README.md", Template: "cursor.README.md.tmpl", Extra: false},
			{Path: "src/targets/cursor/rules/project.mdc", Template: "cursor.rule.mdc.tmpl", Extra: false},
			{Path: "src/targets/cursor/AGENTS.md", Template: "cursor.AGENTS.md.tmpl", Extra: true},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
		},
	},
	"gemini": {
		Name: "gemini",
		Files: []TemplateFile{
			{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "src/targets/gemini/package.yaml", Template: "targets.gemini.package.yaml.tmpl", Extra: false},
			{Path: "src/targets/gemini/contexts/GEMINI.md", Template: "gemini.GEMINI.md.tmpl", Extra: false},
			{Path: "src/README.md", Template: "gemini.README.md.tmpl", Extra: false},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
			{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
			{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
			{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
		},
	},
	"opencode": {
		Name: "opencode",
		Files: []TemplateFile{
			{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
			{Path: "src/targets/opencode/package.yaml", Template: "targets.opencode.package.yaml.tmpl", Extra: false},
			{Path: "src/README.md", Template: "opencode.README.md.tmpl", Extra: false},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
			{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "opencode.SKILL.md.tmpl", Extra: true},
			{Path: "src/targets/opencode/config.extra.json", Template: "opencode.config.extra.json.tmpl", Extra: true},
			{Path: "src/targets/opencode/default_agent.txt", Template: "opencode.default_agent.txt.tmpl", Extra: true},
			{Path: "src/targets/opencode/instructions.yaml", Template: "opencode.instructions.yaml.tmpl", Extra: true},
			{Path: "src/targets/opencode/permission.json", Template: "opencode.permission.json.tmpl", Extra: true},
			{Path: "src/targets/opencode/commands/{{.ProjectName}}.md", Template: "opencode.command.md.tmpl", Extra: true},
			{Path: "src/targets/opencode/agents/{{.ProjectName}}.md", Template: "opencode.agent.md.tmpl", Extra: true},
			{Path: "src/targets/opencode/themes/{{.ProjectName}}.json", Template: "opencode.theme.json.tmpl", Extra: true},
			{Path: "src/targets/opencode/tools/{{.ProjectName}}.ts", Template: "opencode.tool.ts.tmpl", Extra: true},
			{Path: "src/targets/opencode/plugins/example.js", Template: "opencode.plugin.js.tmpl", Extra: true},
			{Path: "src/targets/opencode/package.json", Template: "opencode.package.json.tmpl", Extra: true},
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
		return "codex-runtime"
	}
	return name
}
