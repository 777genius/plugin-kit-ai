package platformexec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func importedOpenCodeInlineCommandArtifacts(raw map[string]any, configPath string) ([]pluginmodel.Artifact, map[string]any, []pluginmodel.Warning, error) {
	return importedOpenCodeInlineMarkdownArtifacts("command", raw, configPath, "commands", normalizeInlineOpenCodeCommand)
}

func importedOpenCodeInlineAgentArtifacts(raw map[string]any, configPath string) ([]pluginmodel.Artifact, map[string]any, []pluginmodel.Warning, error) {
	return importedOpenCodeInlineMarkdownArtifacts("agent", raw, configPath, "agents", normalizeInlineOpenCodeAgent)
}

type openCodeInlineNormalizer func(name string, spec map[string]any) (map[string]any, string, bool)

func importedOpenCodeInlineMarkdownArtifacts(field string, raw map[string]any, configPath string, dstKind string, normalize openCodeInlineNormalizer) ([]pluginmodel.Artifact, map[string]any, []pluginmodel.Warning, error) {
	if len(raw) == 0 {
		return nil, nil, nil, nil
	}
	var (
		artifacts []pluginmodel.Artifact
		warnings  []pluginmodel.Warning
		remaining = map[string]any{}
	)
	for name, value := range raw {
		spec, ok := value.(map[string]any)
		if !ok {
			remaining[name] = value
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    configPath,
				Message: fmt.Sprintf("preserved OpenCode inline %s %q in targets/opencode/config.extra.json because it is not representable as targets/opencode/%s/*.md", field, name, dstKind),
			})
			continue
		}
		frontmatter, body, ok := normalize(name, spec)
		if !ok {
			remaining[name] = value
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    configPath,
				Message: fmt.Sprintf("preserved OpenCode inline %s %q in targets/opencode/config.extra.json because it is not representable as targets/opencode/%s/*.md", field, name, dstKind),
			})
			continue
		}
		relPath, ok := canonicalOpenCodeNamedMarkdownPath(dstKind, name)
		if !ok {
			remaining[name] = value
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    configPath,
				Message: fmt.Sprintf("preserved OpenCode inline %s %q in targets/opencode/config.extra.json because its name cannot be normalized into a canonical markdown file path", field, name),
			})
			continue
		}
		content, err := marshalOpenCodeMarkdown(frontmatter, body)
		if err != nil {
			return nil, nil, nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{RelPath: relPath, Content: content})
	}
	return compactArtifacts(artifacts), remaining, warnings, nil
}

func normalizeInlineOpenCodeCommand(name string, spec map[string]any) (map[string]any, string, bool) {
	template, ok := spec["template"].(string)
	if !ok || strings.TrimSpace(template) == "" {
		return nil, "", false
	}
	for key := range spec {
		switch key {
		case "template", "description", "agent", "subtask", "model":
		default:
			return nil, "", false
		}
	}
	frontmatter := map[string]any{}
	if description, ok := spec["description"]; ok {
		text, ok := description.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["description"] = strings.TrimSpace(text)
	}
	if agent, ok := spec["agent"]; ok {
		text, ok := agent.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["agent"] = strings.TrimSpace(text)
	}
	if subtask, ok := spec["subtask"]; ok {
		flag, ok := subtask.(bool)
		if !ok {
			return nil, "", false
		}
		frontmatter["subtask"] = flag
	}
	if model, ok := spec["model"]; ok {
		text, ok := model.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["model"] = strings.TrimSpace(text)
	}
	return frontmatter, strings.TrimSpace(template), true
}

func normalizeInlineOpenCodeAgent(name string, spec map[string]any) (map[string]any, string, bool) {
	description, ok := spec["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return nil, "", false
	}
	_, _ = name, spec
	for key := range spec {
		switch key {
		case "description", "mode", "model", "variant", "temperature", "top_p", "tools", "permission", "disable", "hidden", "options", "color", "steps", "maxSteps", "prompt":
		default:
			return nil, "", false
		}
	}
	frontmatter := map[string]any{
		"description": strings.TrimSpace(description),
	}
	if mode, ok := spec["mode"]; ok {
		text, ok := mode.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["mode"] = strings.TrimSpace(text)
	}
	if model, ok := spec["model"]; ok {
		text, ok := model.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["model"] = strings.TrimSpace(text)
	}
	if variant, ok := spec["variant"]; ok {
		text, ok := variant.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["variant"] = strings.TrimSpace(text)
	}
	if temperature, ok := spec["temperature"]; ok {
		switch value := temperature.(type) {
		case float64:
			frontmatter["temperature"] = value
		default:
			return nil, "", false
		}
	}
	if topP, ok := spec["top_p"]; ok {
		switch value := topP.(type) {
		case float64:
			frontmatter["top_p"] = value
		default:
			return nil, "", false
		}
	}
	if tools, ok := spec["tools"]; ok {
		toolMap, ok := tools.(map[string]any)
		if !ok {
			return nil, "", false
		}
		normalizedTools := map[string]any{}
		for key, value := range toolMap {
			flag, ok := value.(bool)
			if !ok || strings.TrimSpace(key) == "" {
				return nil, "", false
			}
			normalizedTools[key] = flag
		}
		if _, exists := frontmatter["permission"]; !exists && len(normalizedTools) > 0 {
			frontmatter["permission"] = map[string]any{"tools": normalizedTools}
		}
	}
	if permission, ok := spec["permission"]; ok {
		if !isOpenCodePermissionValue(permission) {
			return nil, "", false
		}
		frontmatter["permission"] = permission
	}
	if disable, ok := spec["disable"]; ok {
		flag, ok := disable.(bool)
		if !ok {
			return nil, "", false
		}
		frontmatter["disable"] = flag
	}
	if hidden, ok := spec["hidden"]; ok {
		flag, ok := hidden.(bool)
		if !ok {
			return nil, "", false
		}
		frontmatter["hidden"] = flag
	}
	if options, ok := spec["options"]; ok {
		typed, ok := options.(map[string]any)
		if !ok {
			return nil, "", false
		}
		frontmatter["options"] = typed
	}
	if color, ok := spec["color"]; ok {
		text, ok := color.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["color"] = strings.TrimSpace(text)
	}
	if steps, ok := spec["steps"]; ok {
		value, ok := steps.(float64)
		if !ok || value != float64(int(value)) {
			return nil, "", false
		}
		frontmatter["steps"] = int(value)
	}
	if maxSteps, ok := spec["maxSteps"]; ok {
		if _, exists := frontmatter["steps"]; !exists {
			value, ok := maxSteps.(float64)
			if !ok || value != float64(int(value)) {
				return nil, "", false
			}
			frontmatter["steps"] = int(value)
		}
	}
	body := ""
	if prompt, ok := spec["prompt"]; ok {
		text, ok := prompt.(string)
		if !ok {
			return nil, "", false
		}
		if strings.Contains(text, "{file:") {
			return nil, "", false
		}
		body = strings.TrimSpace(text)
	}
	return frontmatter, body, true
}

func marshalOpenCodeMarkdown(frontmatter map[string]any, body string) ([]byte, error) {
	fm := strings.TrimSpace(string(mustYAML(frontmatter)))
	text := "---\n" + fm + "\n---\n"
	if strings.TrimSpace(body) != "" {
		text += "\n" + strings.TrimSpace(body) + "\n"
	}
	return []byte(text), nil
}

func canonicalOpenCodeNamedMarkdownPath(kind, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return "", false
	}
	return filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", kind, name+".md")), true
}

func sortedArtifactKeys(artifacts map[string]pluginmodel.Artifact) []string {
	out := make([]string, 0, len(artifacts))
	for rel := range artifacts {
		out = append(out, rel)
	}
	slices.Sort(out)
	return out
}
