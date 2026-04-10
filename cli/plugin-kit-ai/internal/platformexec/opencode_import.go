package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type opencodeImportedState struct {
	plugins         []opencodePluginRef
	pluginsProvided bool
	mcp             map[string]any
	defaultAgent    string
	defaultAgentSet bool
	instructions    []string
	instructionsSet bool
	permission      any
	permissionSet   bool
	extra           map[string]any
	artifacts       map[string]pluginmodel.Artifact
	warnings        []pluginmodel.Warning
	hasInput        bool
}

type opencodeImportSource struct {
	dir       string
	display   string
	warnOnUse bool
	warnPath  string
	warnMsg   string
}

type opencodeScopeConfig struct {
	root              string
	displayConfigRoot string
	workspaceRoot     string
	workspaceDisplay  string
}

func (opencodeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	state := opencodeImportedState{
		artifacts: map[string]pluginmodel.Artifact{},
	}

	if seed.IncludeUserScope {
		home, err := os.UserHomeDir()
		if err != nil {
			return ImportResult{}, fmt.Errorf("resolve user home for OpenCode import: %w", err)
		}
		for _, reject := range []struct {
			full    string
			display string
		}{
			{full: filepath.Join(home, ".agents", "skills"), display: filepath.ToSlash(filepath.Join("~", ".agents", "skills"))},
			{full: filepath.Join(home, ".claude", "skills"), display: filepath.ToSlash(filepath.Join("~", ".claude", "skills"))},
		} {
			if err := rejectOpenCodeCompatSkillRoot(reject.full, reject.display); err != nil {
				return ImportResult{}, err
			}
		}
		globalRoot := filepath.Join(home, ".config", "opencode")
		if err := importOpenCodeScope(&state, opencodeScopeConfig{
			root:              globalRoot,
			displayConfigRoot: filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
			workspaceRoot:     globalRoot,
			workspaceDisplay:  filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
		}); err != nil {
			return ImportResult{}, err
		}
	}

	for _, reject := range []struct {
		full    string
		display string
	}{
		{full: filepath.Join(root, ".agents", "skills"), display: filepath.ToSlash(filepath.Join(".agents", "skills"))},
		{full: filepath.Join(root, ".claude", "skills"), display: filepath.ToSlash(filepath.Join(".claude", "skills"))},
	} {
		if err := rejectOpenCodeCompatSkillRoot(reject.full, reject.display); err != nil {
			return ImportResult{}, err
		}
	}
	if err := importOpenCodeScope(&state, opencodeScopeConfig{
		root:              root,
		displayConfigRoot: "",
		workspaceRoot:     filepath.Join(root, ".opencode"),
		workspaceDisplay:  ".opencode",
	}); err != nil {
		return ImportResult{}, err
	}
	if !state.hasInput {
		return ImportResult{}, fmt.Errorf("OpenCode import requires opencode.json, opencode.jsonc, supported workspace directories, or --include-user-scope with OpenCode native sources")
	}

	artifacts := []pluginmodel.Artifact{{
		RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "package.yaml"),
		Content: mustYAML(opencodePackageMeta{Plugins: append([]opencodePluginRef(nil), state.plugins...)}),
	}}
	if len(state.mcp) > 0 {
		artifact, err := importedPortableMCPArtifact("opencode", state.mcp)
		if err != nil {
			return ImportResult{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	if state.defaultAgentSet {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "default_agent.txt"),
			Content: append([]byte(state.defaultAgent), '\n'),
		})
	}
	if state.instructionsSet {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "instructions.yaml"),
			Content: mustYAML(state.instructions),
		})
	}
	if state.permissionSet {
		body, err := marshalJSON(state.permission)
		if err != nil {
			return ImportResult{}, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "permission.json"),
			Content: body,
		})
	}
	if len(state.extra) > 0 {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "config.extra.json"),
			Content: mustJSON(state.extra),
		})
	}
	for _, rel := range sortedArtifactKeys(state.artifacts) {
		artifacts = append(artifacts, state.artifacts[rel])
	}
	return ImportResult{
		Manifest:  seed.Manifest,
		Launcher:  nil,
		Artifacts: compactArtifacts(artifacts),
		Warnings:  state.warnings,
	}, nil
}

func importOpenCodeScope(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	importedConfig, configDisplayPath, warnings, ok, err := readImportedOpenCodeConfigFromDir(cfg.root, cfg.displayConfigRoot)
	if err != nil {
		return err
	}
	state.warnings = append(state.warnings, warnings...)
	if ok {
		commandArtifacts, remainingCommands, commandWarnings, err := importedOpenCodeInlineCommandArtifacts(importedConfig.Commands, configDisplayPath)
		if err != nil {
			return err
		}
		agentArtifacts, remainingAgents, agentWarnings, err := importedOpenCodeInlineAgentArtifacts(importedConfig.Agents, configDisplayPath)
		if err != nil {
			return err
		}
		state.warnings = append(state.warnings, commandWarnings...)
		state.warnings = append(state.warnings, agentWarnings...)
		state.addArtifacts(commandArtifacts...)
		state.addArtifacts(agentArtifacts...)
		if len(remainingCommands) > 0 {
			if importedConfig.Extra == nil {
				importedConfig.Extra = map[string]any{}
			}
			importedConfig.Extra["command"] = remainingCommands
		}
		if len(remainingAgents) > 0 {
			if importedConfig.Extra == nil {
				importedConfig.Extra = map[string]any{}
			}
			importedConfig.Extra["agent"] = remainingAgents
		}
		state.mergeConfig(importedConfig)
		state.hasInput = true
	}

	themeArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "themes"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "themes")),
		},
		filepath.Join("targets", "opencode", "themes"),
		func(rel string) bool { return filepath.Ext(rel) == ".json" },
	)
	if err != nil {
		return err
	}
	state.addArtifacts(themeArtifacts...)
	if len(themeArtifacts) > 0 {
		state.hasInput = true
	}

	toolArtifacts, toolWarnings, err := importOpenCodeToolArtifacts(cfg.workspaceRoot, cfg.workspaceDisplay)
	if err != nil {
		return err
	}
	state.addArtifacts(toolArtifacts...)
	state.warnings = append(state.warnings, toolWarnings...)
	if len(toolArtifacts) > 0 {
		state.hasInput = true
	}

	commandArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "commands"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "commands")),
		},
		filepath.Join("targets", "opencode", "commands"),
		func(rel string) bool { return filepath.Ext(rel) == ".md" },
	)
	if err != nil {
		return err
	}
	state.addArtifacts(commandArtifacts...)
	if len(commandArtifacts) > 0 {
		state.hasInput = true
	}

	agentArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "agents"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "agents")),
		},
		filepath.Join("targets", "opencode", "agents"),
		func(rel string) bool { return filepath.Ext(rel) == ".md" },
	)
	if err != nil {
		return err
	}
	state.addArtifacts(agentArtifacts...)
	if len(agentArtifacts) > 0 {
		state.hasInput = true
	}

	skillArtifacts, _, err := importDirectoryArtifactsWithWarnings([]opencodeImportSource{{
		dir:     filepath.Join(cfg.workspaceRoot, "skills"),
		display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "skills")),
	}}, "skills", func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	if err != nil {
		return err
	}
	state.addArtifacts(skillArtifacts...)
	if len(skillArtifacts) > 0 {
		state.hasInput = true
	}

	pluginsDir := filepath.Join(cfg.workspaceRoot, "plugins")
	pluginArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     pluginsDir,
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "plugins")),
		},
		filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "plugins"),
		nil,
	)
	if err != nil {
		return err
	}
	state.addArtifacts(pluginArtifacts...)
	if len(pluginArtifacts) > 0 {
		state.hasInput = true
	}

	packageJSON := filepath.Join(cfg.workspaceRoot, "package.json")
	if body, err := os.ReadFile(packageJSON); err == nil {
		state.addArtifacts(pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "package.json")),
			Content: body,
		})
		state.hasInput = true
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

func rejectOpenCodeCompatSkillRoot(full, display string) error {
	if _, err := os.Stat(full); err == nil {
		return fmt.Errorf("unsupported OpenCode native skill path %s: use skills/**", display)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *opencodeImportedState) addArtifacts(artifacts ...pluginmodel.Artifact) {
	for _, artifact := range artifacts {
		s.artifacts[artifact.RelPath] = artifact
	}
}

func (s *opencodeImportedState) mergeConfig(config importedOpenCodeConfig) {
	if config.PluginsProvided {
		s.pluginsProvided = true
		s.plugins = append([]opencodePluginRef(nil), config.Plugins...)
	}
	if config.MCPProvided {
		if s.mcp == nil {
			s.mcp = map[string]any{}
		}
		mergeOpenCodeObject(s.mcp, config.MCP)
	}
	if len(config.Extra) > 0 {
		if s.extra == nil {
			s.extra = map[string]any{}
		}
		mergeOpenCodeObject(s.extra, config.Extra)
	}
	if config.DefaultAgentSet {
		s.defaultAgent = config.DefaultAgent
		s.defaultAgentSet = true
	}
	if config.InstructionsSet {
		s.instructions = append([]string(nil), config.Instructions...)
		s.instructionsSet = true
	}
	if config.PermissionSet {
		s.permission = config.Permission
		s.permissionSet = true
	}
}

func readImportedOpenCodeConfigFromDir(root string, displayBase string) (importedOpenCodeConfig, string, []pluginmodel.Warning, bool, error) {
	path, warnings, ok, err := resolveOpenCodeConfigPathInDir(root, displayBase)
	if err != nil || !ok {
		return importedOpenCodeConfig{}, "", warnings, ok, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return importedOpenCodeConfig{}, "", warnings, false, err
	}
	data, err := decodeImportedOpenCodeConfig(body)
	if err != nil {
		return importedOpenCodeConfig{}, "", warnings, false, err
	}
	displayPath := filepath.Base(path)
	if strings.TrimSpace(displayBase) != "" {
		displayPath = filepath.ToSlash(filepath.Join(displayBase, filepath.Base(path)))
	}
	return data, displayPath, warnings, true, nil
}

func importDirectoryArtifacts(source opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, error) {
	artifacts, _, err := importDirectoryArtifactsWithWarnings([]opencodeImportSource{source}, dstRoot, keep)
	return artifacts, err
}

func importDirectoryArtifactsWithWarnings(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	var warnings []pluginmodel.Warning
	for _, source := range sources {
		full := source.dir
		if _, err := os.Stat(full); err != nil {
			continue
		}
		var used bool
		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(full, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if keep != nil && !keep(rel) {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			artifacts[filepath.ToSlash(filepath.Join(dstRoot, rel))] = pluginmodel.Artifact{
				RelPath: filepath.ToSlash(filepath.Join(dstRoot, rel)),
				Content: body,
			}
			used = true
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		if source.warnOnUse && used {
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    source.warnPath,
				Message: source.warnMsg,
			})
		}
	}
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out, warnings, nil
}

func importOpenCodeToolArtifacts(workspaceRoot, workspaceDisplay string) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	legacyDir := filepath.Join(workspaceRoot, "tool")
	if _, err := os.Stat(legacyDir); err == nil {
		return nil, nil, fmt.Errorf("unsupported OpenCode native path %s: use %s", filepath.ToSlash(filepath.Join(workspaceDisplay, "tool")), filepath.ToSlash(filepath.Join(workspaceDisplay, "tools")))
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	sources := []opencodeImportSource{
		{
			dir:     filepath.Join(workspaceRoot, "tools"),
			display: filepath.ToSlash(filepath.Join(workspaceDisplay, "tools")),
		},
	}
	return importDirectoryArtifactsRejectingSymlinks(sources, filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "tools"), nil)
}

func importDirectoryArtifactsRejectingSymlinks(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	for _, source := range sources {
		full := source.dir
		if _, err := os.Stat(full); err != nil {
			continue
		}
		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("OpenCode native import does not support symlinks under %s", source.display)
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(full, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if keep != nil && !keep(rel) {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			dst := filepath.ToSlash(filepath.Join(dstRoot, rel))
			artifacts[dst] = pluginmodel.Artifact{RelPath: dst, Content: body}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out, nil, nil
}

func mergeOpenCodeObject(dst, src map[string]any) {
	if len(src) == 0 {
		return
	}
	for key, value := range src {
		existing, hasExisting := dst[key].(map[string]any)
		incoming, incomingIsMap := value.(map[string]any)
		if hasExisting && incomingIsMap {
			mergeOpenCodeObject(existing, incoming)
			dst[key] = existing
			continue
		}
		dst[key] = value
	}
}

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
