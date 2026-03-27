package validate

import "strings"

var generatedRules = map[string]Rule{
	"claude": {
		Platform: "claude",
		RequiredFiles: []string{
			"go.mod",
			"README.md",
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
