package platformexec

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type importedClaudeHooksFile struct {
	Hooks map[string][]importedClaudeHookEntry `json:"hooks"`
}

type importedClaudeHookEntry struct {
	Hooks []importedClaudeHookCommand `json:"hooks"`
}

type importedClaudeHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func parseClaudeHooks(body []byte) (importedClaudeHooksFile, error) {
	var hooks importedClaudeHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		return importedClaudeHooksFile{}, err
	}
	return hooks, nil
}

func trimClaudeHookCommand(command, hookName string) (string, bool) {
	command = strings.TrimSpace(command)
	suffix := " " + strings.TrimSpace(hookName)
	if !strings.HasSuffix(command, suffix) {
		return "", false
	}
	entrypoint := strings.TrimSpace(strings.TrimSuffix(command, suffix))
	if entrypoint == "" {
		return "", false
	}
	return entrypoint, true
}

func inferClaudeEntrypoint(body []byte) (string, bool) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return "", false
	}
	for hookName, entries := range hooks.Hooks {
		for _, entry := range entries {
			for _, command := range entry.Hooks {
				if command.Type != "command" {
					continue
				}
				if entrypoint, ok := trimClaudeHookCommand(command.Command, hookName); ok {
					return entrypoint, true
				}
			}
		}
	}
	return "", false
}

func validateClaudeHookEntrypoints(body []byte, entrypoint string) ([]string, error) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for hookName, entries := range hooks.Hooks {
		expected := strings.TrimSpace(entrypoint) + " " + strings.TrimSpace(hookName)
		foundCommand := false
		for _, entry := range entries {
			for _, command := range entry.Hooks {
				if strings.TrimSpace(command.Type) != "command" {
					continue
				}
				foundCommand = true
				if strings.TrimSpace(command.Command) != expected {
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q uses %q; expected %q from launcher.yaml entrypoint", hookName, command.Command, expected))
				}
			}
		}
		if !foundCommand {
			mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q declares no command hooks; expected %q", hookName, expected))
		}
	}
	return mismatches, nil
}

func defaultClaudeHooks(entrypoint string) []byte {
	type hookCommand struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type hookEntry struct {
		Hooks []hookCommand `json:"hooks"`
	}
	hooks := map[string][]hookEntry{}
	for _, name := range stableClaudeHookNames() {
		hooks[name] = []hookEntry{{Hooks: []hookCommand{{Type: "command", Command: entrypoint + " " + name}}}}
	}
	body, _ := marshalJSON(map[string]any{"hooks": hooks})
	return body
}

func stableClaudeHookNames() []string {
	return []string{"Stop", "PreToolUse", "UserPromptSubmit"}
}

func stableGeminiHookNames() []string {
	return []string{
		"SessionStart",
		"SessionEnd",
		"BeforeModel",
		"AfterModel",
		"BeforeToolSelection",
		"BeforeAgent",
		"AfterAgent",
		"BeforeTool",
		"AfterTool",
	}
}

func geminiInvocationAlias(hookName string) string {
	switch strings.TrimSpace(hookName) {
	case "SessionStart":
		return "GeminiSessionStart"
	case "SessionEnd":
		return "GeminiSessionEnd"
	case "BeforeModel":
		return "GeminiBeforeModel"
	case "AfterModel":
		return "GeminiAfterModel"
	case "BeforeToolSelection":
		return "GeminiBeforeToolSelection"
	case "BeforeAgent":
		return "GeminiBeforeAgent"
	case "AfterAgent":
		return "GeminiAfterAgent"
	case "BeforeTool":
		return "GeminiBeforeTool"
	case "AfterTool":
		return "GeminiAfterTool"
	default:
		return ""
	}
}

type geminiHooksFile struct {
	Hooks map[string][]geminiHookGroup `json:"hooks"`
}

type geminiHookGroup struct {
	Matcher string                `json:"matcher,omitempty"`
	Hooks   []importedHookCommand `json:"hooks"`
}

type importedHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func parseGeminiHooks(body []byte) (geminiHooksFile, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return geminiHooksFile{}, err
	}
	hooksValue, ok := raw["hooks"]
	if !ok {
		return geminiHooksFile{}, fmt.Errorf("Gemini hooks file must define a top-level hooks object")
	}
	if _, ok := hooksValue.(map[string]any); !ok {
		return geminiHooksFile{}, fmt.Errorf("Gemini hooks file must define a top-level hooks object")
	}
	var hooks geminiHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		return geminiHooksFile{}, err
	}
	if hooks.Hooks == nil {
		return geminiHooksFile{}, fmt.Errorf("Gemini hooks file must define a top-level hooks object")
	}
	return hooks, nil
}

func trimGeminiHookCommand(command, invocation string) (string, bool) {
	command = strings.TrimSpace(command)
	suffix := " " + strings.TrimSpace(invocation)
	if !strings.HasSuffix(command, suffix) {
		return "", false
	}
	entrypoint := strings.TrimSpace(strings.TrimSuffix(command, suffix))
	if entrypoint == "" {
		return "", false
	}
	return normalizeGeminiHookEntrypoint(entrypoint), true
}

func normalizeGeminiHookEntrypoint(entrypoint string) string {
	entrypoint = strings.TrimSpace(entrypoint)
	switch {
	case strings.HasPrefix(entrypoint, "${extensionPath}${/}"):
		rel := strings.TrimPrefix(entrypoint, "${extensionPath}${/}")
		rel = strings.ReplaceAll(rel, "${/}", "/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return "./"
		}
		return "./" + rel
	case strings.HasPrefix(entrypoint, "${extensionPath}/"):
		rel := strings.TrimPrefix(entrypoint, "${extensionPath}/")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return "./"
		}
		return "./" + rel
	default:
		return entrypoint
	}
}

func geminiHookEntrypointForExtension(entrypoint string) string {
	entrypoint = strings.TrimSpace(entrypoint)
	if entrypoint == "" {
		return ""
	}
	if strings.HasPrefix(entrypoint, "${extensionPath}") {
		return entrypoint
	}
	if strings.HasPrefix(entrypoint, "./") {
		rel := strings.TrimPrefix(entrypoint, "./")
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return "${extensionPath}"
		}
		return "${extensionPath}${/}" + strings.ReplaceAll(rel, "/", "${/}")
	}
	return entrypoint
}

func geminiHookCommandCandidates(entrypoint, invocation string) []string {
	candidates := []string{}
	seen := map[string]struct{}{}
	for _, base := range []string{
		strings.TrimSpace(entrypoint),
		geminiHookEntrypointForExtension(entrypoint),
	} {
		base = strings.TrimSpace(base)
		if base == "" {
			continue
		}
		command := base + " " + strings.TrimSpace(invocation)
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		candidates = append(candidates, command)
	}
	return candidates
}

func inferGeminiEntrypoint(body []byte) (string, bool) {
	hooks, err := parseGeminiHooks(body)
	if err != nil {
		return "", false
	}
	for _, hookName := range stableGeminiHookNames() {
		invocation := geminiInvocationAlias(hookName)
		for _, entry := range hooks.Hooks[hookName] {
			for _, command := range entry.Hooks {
				if strings.TrimSpace(command.Type) != "command" {
					continue
				}
				if entrypoint, ok := trimGeminiHookCommand(command.Command, invocation); ok {
					return entrypoint, true
				}
			}
		}
	}
	return "", false
}

func defaultGeminiHooks(entrypoint string) []byte {
	type hookCommand struct {
		Name    string `json:"name,omitempty"`
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type hookEntry struct {
		Matcher string        `json:"matcher,omitempty"`
		Hooks   []hookCommand `json:"hooks"`
	}
	hooks := map[string][]hookEntry{}
	for _, name := range stableGeminiHookNames() {
		invocation := geminiInvocationAlias(name)
		commands := geminiHookCommandCandidates(entrypoint, invocation)
		command := strings.TrimSpace(entrypoint) + " " + invocation
		if len(commands) > 0 {
			command = commands[len(commands)-1]
		}
		hooks[name] = []hookEntry{{
			Matcher: "*",
			Hooks: []hookCommand{{
				Type:    "command",
				Command: command,
			}},
		}}
	}
	body, _ := marshalJSON(map[string]any{"hooks": hooks})
	return body
}

func validateGeminiHookEntrypoints(body []byte, entrypoint string) ([]string, error) {
	hooks, err := parseGeminiHooks(body)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for _, hookName := range stableGeminiHookNames() {
		matcher := "*"
		invocation := geminiInvocationAlias(hookName)
		entries := hooks.Hooks[hookName]
		expectedCommands := geminiHookCommandCandidates(entrypoint, invocation)
		expected := strings.TrimSpace(entrypoint) + " " + invocation
		if len(expectedCommands) > 0 {
			expected = expectedCommands[len(expectedCommands)-1]
		}
		if len(entries) == 0 {
			mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Gemini hook %q is missing; expected %q", hookName, expected))
			continue
		}
		foundCommand := false
		for _, entry := range entries {
			if strings.TrimSpace(entry.Matcher) != matcher {
				mismatches = append(mismatches, fmt.Sprintf("matcher mismatch: Gemini hook %q uses %q; expected %q", hookName, entry.Matcher, matcher))
			}
			for _, command := range entry.Hooks {
				if strings.TrimSpace(command.Type) != "command" {
					continue
				}
				foundCommand = true
				if !slices.Contains(expectedCommands, strings.TrimSpace(command.Command)) {
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Gemini hook %q uses %q; expected %q from launcher.yaml entrypoint", hookName, command.Command, expected))
				}
			}
		}
		if !foundCommand {
			mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Gemini hook %q declares no command hooks; expected %q", hookName, expected))
		}
	}
	return mismatches, nil
}
