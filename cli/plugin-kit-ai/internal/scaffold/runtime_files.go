package scaffold

func filesFor(platform, runtime string, extras, typescript, sharedRuntimePackage bool) []TemplateFile {
	if platform == "gemini" && runtime == RuntimeGo {
		files := []TemplateFile{
			{Path: "go.mod", Template: "go.mod.tmpl", Extra: false},
			{Path: "cmd/{{.ProjectName}}/main.go", Template: "gemini.main.go.tmpl", Extra: false},
			{Path: authoredPath("plugin.yaml"), Template: "plugin.yaml.tmpl", Extra: false},
			{Path: authoredPath("launcher.yaml"), Template: "launcher.yaml.tmpl", Extra: false},
			{Path: authoredPath("targets/gemini/package.yaml"), Template: "targets.gemini.package.yaml.tmpl", Extra: false},
			{Path: authoredPath("targets/gemini/contexts/GEMINI.md"), Template: "gemini.GEMINI.md.tmpl", Extra: false},
			{Path: authoredPath("targets/gemini/hooks/hooks.json"), Template: "targets.gemini.hooks.json.tmpl", Extra: false},
			{Path: authoredPath("README.md"), Template: "gemini.README.go.md.tmpl", Extra: false},
			{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
			{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
		}
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("mcp/servers.yaml"), Template: "mcp.servers.yaml.tmpl", Extra: true},
				TemplateFile{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
				TemplateFile{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("skills/{{.ProjectName}}/SKILL.md"), Template: "SKILL.md.tmpl", Extra: true},
			)
		}
		return files
	}
	if runtime == RuntimeGo {
		def := generatedPlatforms[platform]
		return def.Files
	}

	files := []TemplateFile{
		{Path: authoredPath("plugin.yaml"), Template: "plugin.yaml.tmpl", Extra: false},
		{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
		{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
	}
	if platform != "gemini" && platform != "codex-package" && platform != "opencode" && platform != "cursor" && platform != "cursor-workspace" {
		files = append(files, TemplateFile{Path: authoredPath("launcher.yaml"), Template: "launcher.yaml.tmpl", Extra: false})
	}

	switch platform {
	case "claude":
		files = append(files,
			TemplateFile{Path: authoredPath("targets/claude/hooks/hooks.json"), Template: "targets.claude.hooks.json.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("targets/claude/settings.json"), Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: authoredPath("targets/claude/lsp.json"), Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: authoredPath("targets/claude/user-config.json"), Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: authoredPath("targets/claude/manifest.extra.json"), Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: authoredPath("README.md"), Template: "README.executable.md.tmpl", Extra: false},
		)
	case "codex-runtime":
		files = append(files,
			TemplateFile{Path: authoredPath("targets/codex-runtime/package.yaml"), Template: "targets.codex-runtime.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("README.md"), Template: "codex-runtime.README.executable.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("targets/codex-runtime/config.extra.toml"), Template: "empty.toml.tmpl", Extra: true},
			)
		}
	case "codex-package":
		files = append(files,
			TemplateFile{Path: authoredPath("README.md"), Template: "codex-package.README.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("targets/codex-package/package.yaml"), Template: "targets.codex-package.package.yaml.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/codex-package/interface.json"), Template: "codex-package.interface.json.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/codex-package/manifest.extra.json"), Template: "empty.json.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/codex-package/app.json"), Template: "empty.json.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("mcp/servers.yaml"), Template: "mcp.servers.yaml.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("skills/{{.ProjectName}}/SKILL.md"), Template: "SKILL.md.tmpl", Extra: true},
			)
		}
		return files
	case "gemini":
		files = append(files,
			TemplateFile{Path: authoredPath("targets/gemini/package.yaml"), Template: "targets.gemini.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("targets/gemini/contexts/GEMINI.md"), Template: "gemini.GEMINI.md.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("README.md"), Template: "gemini.README.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("mcp/servers.yaml"), Template: "mcp.servers.yaml.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("skills/{{.ProjectName}}/SKILL.md"), Template: "SKILL.md.tmpl", Extra: true},
			)
		}
		return files
	case "opencode":
		files = append(files,
			TemplateFile{Path: authoredPath("targets/opencode/package.yaml"), Template: "targets.opencode.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("README.md"), Template: "opencode.README.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("mcp/servers.yaml"), Template: "mcp.servers.yaml.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("skills/{{.ProjectName}}/SKILL.md"), Template: "opencode.SKILL.md.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/opencode/commands/{{.ProjectName}}.md"), Template: "opencode.command.md.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/opencode/agents/{{.ProjectName}}.md"), Template: "opencode.agent.md.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/opencode/themes/{{.ProjectName}}.json"), Template: "opencode.theme.json.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/opencode/tools/{{.ProjectName}}.ts"), Template: "opencode.tool.ts.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/opencode/plugins/example.js"), Template: "opencode.plugin.js.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("targets/opencode/package.json"), Template: "opencode.package.json.tmpl", Extra: true},
			)
		}
		return files
	case "cursor":
		files = append(files,
			TemplateFile{Path: authoredPath("README.md"), Template: "cursor.README.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("mcp/servers.yaml"), Template: "mcp.servers.yaml.tmpl", Extra: true},
				TemplateFile{Path: authoredPath("skills/{{.ProjectName}}/SKILL.md"), Template: "SKILL.md.tmpl", Extra: true},
			)
		}
		return files
	case "cursor-workspace":
		files = append(files,
			TemplateFile{Path: authoredPath("README.md"), Template: "cursor-workspace.README.md.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("targets/cursor-workspace/rules/project.mdc"), Template: "cursor.rule.mdc.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: authoredPath("mcp/servers.yaml"), Template: "mcp.servers.yaml.tmpl", Extra: true},
			)
		}
		return files
	}

	switch runtime {
	case RuntimePython:
		files = append(files,
			TemplateFile{Path: "requirements.txt", Template: "python.requirements.txt.tmpl", Extra: false},
			TemplateFile{Path: authoredPath("main.py"), Template: "python.main.py.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}", Template: "python.launcher.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "python.launcher.cmd.tmpl", Extra: false},
		)
		if !sharedRuntimePackage {
			files = append(files, TemplateFile{Path: authoredPath("plugin_runtime.py"), Template: "python.plugin_runtime.py.tmpl", Extra: false})
		}
		if extras && (platform == "claude" || platform == "codex-runtime") {
			files = append(files,
				TemplateFile{Path: ".github/workflows/bundle-release.yml", Template: "bundle-release.workflow.yml.tmpl", Extra: true},
			)
		}
	case RuntimeNode:
		if typescript {
			files = append(files,
				TemplateFile{Path: authoredPath("main.ts"), Template: "node.main.ts.tmpl", Extra: false},
				TemplateFile{Path: "tsconfig.json", Template: "node.tsconfig.json.tmpl", Extra: false},
				TemplateFile{Path: "package.json", Template: "node.package.json.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}", Template: "node.launcher.sh.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "node.launcher.cmd.tmpl", Extra: false},
			)
			if !sharedRuntimePackage {
				files = append(files, TemplateFile{Path: authoredPath("plugin-runtime.ts"), Template: "node.plugin-runtime.ts.tmpl", Extra: false})
			}
		} else {
			files = append(files,
				TemplateFile{Path: authoredPath("main.mjs"), Template: "node.main.mjs.tmpl", Extra: false},
				TemplateFile{Path: "package.json", Template: "node.package.json.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}", Template: "node.launcher.sh.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "node.launcher.cmd.tmpl", Extra: false},
			)
			if !sharedRuntimePackage {
				files = append(files, TemplateFile{Path: authoredPath("plugin-runtime.mjs"), Template: "node.plugin-runtime.mjs.tmpl", Extra: false})
			}
		}
		if extras && (platform == "claude" || platform == "codex-runtime") {
			files = append(files,
				TemplateFile{Path: ".github/workflows/bundle-release.yml", Template: "bundle-release.workflow.yml.tmpl", Extra: true},
			)
		}
	case RuntimeShell:
		files = append(files,
			TemplateFile{Path: "scripts/main.sh", Template: "shell.main.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}", Template: "shell.launcher.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "shell.launcher.cmd.tmpl", Extra: false},
		)
	}

	if extras && platform != "codex-runtime" {
		files = append(files,
			TemplateFile{Path: authoredPath("skills/{{.ProjectName}}/SKILL.md"), Template: "SKILL.md.tmpl", Extra: true},
		)
	}

	return files
}
