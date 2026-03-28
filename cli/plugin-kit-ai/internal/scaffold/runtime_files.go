package scaffold

func filesFor(platform, runtime string, extras bool) []TemplateFile {
	if runtime == RuntimeGo {
		def := generatedPlatforms[platform]
		return def.Files
	}

	files := []TemplateFile{
		{Path: "plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
		{Path: "launcher.yaml", Template: "launcher.yaml.tmpl", Extra: false},
	}

	switch platform {
	case "claude":
		files = append(files,
			TemplateFile{Path: "targets/claude/hooks/hooks.json", Template: "targets.claude.hooks.json.tmpl", Extra: false},
			TemplateFile{Path: "README.md", Template: "README.executable.md.tmpl", Extra: false},
		)
	case "codex":
		files = append(files,
			TemplateFile{Path: "targets/codex/package.yaml", Template: "targets.codex.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: "AGENTS.md", Template: "codex.AGENTS.executable.md.tmpl", Extra: false},
			TemplateFile{Path: "README.md", Template: "codex.README.executable.md.tmpl", Extra: false},
		)
	case "gemini":
		files = append(files,
			TemplateFile{Path: "targets/gemini/package.yaml", Template: "targets.gemini.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: "contexts/GEMINI.md", Template: "gemini.GEMINI.md.tmpl", Extra: false},
			TemplateFile{Path: "README.md", Template: "gemini.README.executable.md.tmpl", Extra: false},
		)
	}

	switch runtime {
	case RuntimePython:
		files = append(files,
			TemplateFile{Path: "src/main.py", Template: "python.main.py.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}", Template: "python.launcher.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "python.launcher.cmd.tmpl", Extra: false},
		)
	case RuntimeNode:
		files = append(files,
			TemplateFile{Path: "src/main.mjs", Template: "node.main.mjs.tmpl", Extra: false},
			TemplateFile{Path: "package.json", Template: "node.package.json.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}", Template: "node.launcher.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "node.launcher.cmd.tmpl", Extra: false},
		)
	case RuntimeShell:
		files = append(files,
			TemplateFile{Path: "scripts/main.sh", Template: "shell.main.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}", Template: "shell.launcher.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "shell.launcher.cmd.tmpl", Extra: false},
		)
	}

	if extras {
		files = append(files,
			TemplateFile{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
		)
	}

	return files
}
