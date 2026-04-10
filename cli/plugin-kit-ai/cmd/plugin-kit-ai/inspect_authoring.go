package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func renderInspectAuthoring(report pluginmanifest.Inspection) string {
	lines := []string{
		fmt.Sprintf("Plugin repo: %s %s", report.Manifest.Name, report.Manifest.Version),
		fmt.Sprintf("This repo is set up to %s.", inspectAuthoringPath(report)),
	}

	if authoredRoot := strings.TrimSpace(report.Layout.AuthoredRoot); authoredRoot != "" {
		lines = append(lines, fmt.Sprintf("Editable source lives under %s.", authoredRoot))
	}

	if len(report.Layout.AuthoredInputs) > 0 {
		lines = append(lines, "", "Edit these files:")
		for _, path := range report.Layout.AuthoredInputs {
			lines = append(lines, "  - "+path)
		}
	}

	guides, outputs := authoredGeneratedOutputs(report)
	if len(guides) > 0 {
		lines = append(lines, "", "Managed guidance files:")
		for _, path := range guides {
			lines = append(lines, "  - "+path)
		}
	}
	if len(outputs) > 0 {
		lines = append(lines, "", "Generated target outputs:")
		for _, path := range outputs {
			lines = append(lines, "  - "+path)
		}
	}

	if len(report.Manifest.Targets) > 0 {
		lines = append(lines, "", "Supported outputs:")
		for _, target := range report.Manifest.Targets {
			lines = append(lines, "  - "+inspectAuthoringTargetLabel(target))
		}
	}

	lines = append(lines, "", "Next commands:")
	for _, command := range inspectAuthoringNextCommands(report) {
		lines = append(lines, "  - "+command)
	}

	return strings.Join(lines, "\n") + "\n"
}

func inspectAuthoringPath(report pluginmanifest.Inspection) string {
	if report.Launcher != nil {
		return "build custom plugin logic"
	}
	if report.Portable.MCP != nil && report.Portable.MCP.File != nil {
		hasRemote := false
		hasLocal := false
		for _, server := range report.Portable.MCP.File.Servers {
			switch strings.TrimSpace(server.Type) {
			case "remote":
				hasRemote = true
			case "stdio":
				hasLocal = true
			}
		}
		switch {
		case hasRemote && !hasLocal:
			return "connect an online service"
		case hasLocal && !hasRemote:
			return "connect a local tool"
		case hasLocal && hasRemote:
			return "connect online services and local tools"
		}
	}
	return "manage generated plugin outputs from one authored source"
}

func authoredGeneratedOutputs(report pluginmanifest.Inspection) ([]string, []string) {
	seen := map[string]struct{}{}
	guides := make([]string, 0, len(report.Layout.GeneratedOutputs)+len(report.Layout.BoundaryDocs)+1)
	for _, path := range report.Layout.GeneratedOutputs {
		switch path {
		case "README.md", "CLAUDE.md", "AGENTS.md", "GENERATED.md":
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			guides = append(guides, path)
		}
	}
	for _, path := range report.Layout.BoundaryDocs {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		guides = append(guides, path)
	}
	if generatedGuide := strings.TrimSpace(report.Layout.GeneratedGuide); generatedGuide != "" {
		if _, ok := seen[generatedGuide]; !ok {
			seen[generatedGuide] = struct{}{}
			guides = append(guides, generatedGuide)
		}
	}
	outputs := make([]string, 0, len(report.Layout.GeneratedOutputs))
	for _, path := range report.Layout.GeneratedOutputs {
		if _, ok := seen[path]; ok {
			continue
		}
		outputs = append(outputs, path)
	}
	if len(guides) == 0 && len(outputs) == 0 {
		return nil, nil
	}
	return orderAuthoringGuideFiles(slices.Compact(guides)), slices.Compact(outputs)
}

func inspectAuthoringNextCommands(report pluginmanifest.Inspection) []string {
	commands := []string{"plugin-kit-ai generate ."}
	if report.Launcher == nil {
		commands = append(commands, "plugin-kit-ai generate --check .")
	}
	commands = append(commands, fmt.Sprintf("plugin-kit-ai validate . --platform %s --strict", inspectAuthoringPrimaryTarget(report)))
	if len(report.Manifest.Targets) > 1 {
		commands = append(commands, "Then validate any other outputs you plan to ship.")
	}
	return commands
}

func inspectAuthoringPrimaryTarget(report pluginmanifest.Inspection) string {
	if len(report.Manifest.Targets) == 0 {
		return "claude"
	}
	return report.Manifest.Targets[0]
}

func inspectAuthoringTargetLabel(target string) string {
	switch strings.TrimSpace(target) {
	case "claude":
		return "Claude (claude)"
	case "codex-package":
		return "Codex package (codex-package)"
	case "codex-runtime":
		return "Codex runtime (codex-runtime)"
	case "gemini":
		return "Gemini extension (gemini)"
	case "opencode":
		return "OpenCode (opencode)"
	case "cursor":
		return "Cursor plugin (cursor)"
	case "cursor-workspace":
		return "Cursor workspace (cursor-workspace)"
	default:
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			return target
		}
		return strings.ToUpper(trimmed[:1]) + trimmed[1:] + " (" + trimmed + ")"
	}
}

func orderAuthoringGuideFiles(paths []string) []string {
	preferred := []string{"README.md", "CLAUDE.md", "AGENTS.md", "GENERATED.md"}
	seen := make(map[string]struct{}, len(paths))
	ordered := make([]string, 0, len(paths))
	for _, want := range preferred {
		for _, path := range paths {
			if path != want {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			ordered = append(ordered, path)
		}
	}
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		ordered = append(ordered, path)
	}
	return ordered
}
