package validate

import "strings"

var generatedRules = map[string]Rule{
	"claude": {
		Platform: "claude",
		RequiredFiles: []string{
			"go.mod",
			"README.md",
			"launcher.yaml",
			".claude-plugin/plugin.json",
			"hooks/hooks.json",
		},
		ForbiddenFiles: []string{
			"AGENTS.md",
			".codex/config.toml",
		},
		BuildTargets: []string{
			"./...",
		},
	},
	"codex": {
		Platform: "codex",
		RequiredFiles: []string{
			"go.mod",
			"README.md",
			"launcher.yaml",
			"AGENTS.md",
			".codex/config.toml",
		},
		ForbiddenFiles: []string{
			".claude-plugin/plugin.json",
			"hooks/hooks.json",
		},
		BuildTargets: []string{
			"./...",
		},
	},
	"gemini": {
		Platform: "gemini",
		RequiredFiles: []string{
			"plugin.yaml",
			"targets/gemini/package.yaml",
		},
		ForbiddenFiles: []string{},
		BuildTargets: []string{
			"./...",
		},
	},
}

func LookupRule(name string) (Rule, bool) {
	r, ok := generatedRules[normalizePlatform(name)]
	return r, ok
}

func normalizePlatform(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "codex"
	}
	return name
}
