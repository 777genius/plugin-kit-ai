package scaffold

func filesFor(platform, runtime string, extras, typescript bool) []TemplateFile {
	if runtime == RuntimeGo {
		def := generatedPlatforms[platform]
		return def.Files
	}

	files := []TemplateFile{
		{Path: "plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
	}
	if platform != "gemini" && platform != "codex-package" {
		files = append(files, TemplateFile{Path: "launcher.yaml", Template: "launcher.yaml.tmpl", Extra: false})
	}

	switch platform {
	case "claude":
		files = append(files,
			TemplateFile{Path: "targets/claude/hooks/hooks.json", Template: "targets.claude.hooks.json.tmpl", Extra: false},
			TemplateFile{Path: "targets/claude/settings.json", Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: "targets/claude/lsp.json", Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: "targets/claude/user-config.json", Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: "targets/claude/manifest.extra.json", Template: "empty.json.tmpl", Extra: true},
			TemplateFile{Path: "README.md", Template: "README.executable.md.tmpl", Extra: false},
		)
	case "codex-runtime":
		files = append(files,
			TemplateFile{Path: "targets/codex-runtime/package.yaml", Template: "targets.codex-runtime.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: "README.md", Template: "codex-runtime.README.executable.md.tmpl", Extra: false},
		)
	case "codex-package":
		files = append(files,
			TemplateFile{Path: "targets/codex-package/package.yaml", Template: "targets.codex-package.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: "README.md", Template: "codex-package.README.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
			)
		}
		return files
	case "gemini":
		files = append(files,
			TemplateFile{Path: "targets/gemini/package.yaml", Template: "targets.gemini.package.yaml.tmpl", Extra: false},
			TemplateFile{Path: "targets/gemini/contexts/GEMINI.md", Template: "gemini.GEMINI.md.tmpl", Extra: false},
			TemplateFile{Path: "README.md", Template: "gemini.README.md.tmpl", Extra: false},
		)
		if extras {
			files = append(files,
				TemplateFile{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
			)
		}
		return files
	}

	switch runtime {
	case RuntimePython:
		files = append(files,
			TemplateFile{Path: "src/main.py", Template: "python.main.py.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}", Template: "python.launcher.sh.tmpl", Extra: false},
			TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "python.launcher.cmd.tmpl", Extra: false},
		)
	case RuntimeNode:
		if typescript {
			files = append(files,
				TemplateFile{Path: "src/main.ts", Template: "node.main.ts.tmpl", Extra: false},
				TemplateFile{Path: "tsconfig.json", Template: "node.tsconfig.json.tmpl", Extra: false},
				TemplateFile{Path: "package.json", Template: "node.package.json.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}", Template: "node.launcher.sh.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "node.launcher.cmd.tmpl", Extra: false},
			)
		} else {
			files = append(files,
				TemplateFile{Path: "src/main.mjs", Template: "node.main.mjs.tmpl", Extra: false},
				TemplateFile{Path: "package.json", Template: "node.package.json.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}", Template: "node.launcher.sh.tmpl", Extra: false},
				TemplateFile{Path: "bin/{{.ProjectName}}.cmd", Template: "node.launcher.cmd.tmpl", Extra: false},
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
			TemplateFile{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
		)
	}

	return files
}
