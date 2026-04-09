package pluginmanifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/platformexec"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
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

type preparedImport struct {
	Manifest      Manifest
	Launcher      *Launcher
	Artifacts     []Artifact
	Warnings      []Warning
	ImportSource  string
	DetectedKinds []string
	DroppedKinds  []string
}

func importPackage(root string, from string, force bool, includeUserScope bool) (Manifest, []Warning, error) {
	if _, err := detectAuthoredLayout(root); err != nil && !os.IsNotExist(err) {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return Manifest{}, nil, err
		}
	}
	if fileExists(filepath.Join(root, ".plugin-kit-ai", "project.toml")) {
		return Manifest{}, nil, fmt.Errorf("unsupported project format for import: .plugin-kit-ai/project.toml is not supported; rewrite the project into the package standard layout")
	}
	prepared, err := prepareImportFromRoot(root, from, includeUserScope)
	if err != nil {
		return Manifest{}, prepared.Warnings, err
	}
	if err := writePreparedImport(root, prepared, force); err != nil {
		return prepared.Manifest, prepared.Warnings, err
	}
	return prepared.Manifest, prepared.Warnings, nil
}

func prepareImportFromRoot(root string, from string, includeUserScope bool) (preparedImport, error) {
	explicitFrom := strings.TrimSpace(from) != ""
	from = normalizeTarget(from)
	matches := platformexec.DetectImport(root)
	detectedKinds := make([]string, 0, len(matches))
	for _, match := range matches {
		detectedKinds = append(detectedKinds, match.ID())
	}
	if from == "" {
		switch {
		case len(matches) == 0:
			from = ""
		case len(matches) == 1:
			from = matches[0].ID()
		default:
			var ids []string
			for _, match := range matches {
				ids = append(ids, match.ID())
			}
			return preparedImport{}, fmt.Errorf("ambiguous import source: detected multiple native layouts (%s); pass --from explicitly", strings.Join(ids, ", "))
		}
	}
	if explicitFrom && from == "codex" {
		return preparedImport{}, fmt.Errorf("unsupported import source %q", from)
	}
	if !isSupportedImportSource(from) {
		return preparedImport{}, fmt.Errorf("unsupported import source %q", from)
	}
	adapter, ok := platformexec.Lookup(from)
	if !ok {
		return preparedImport{}, fmt.Errorf("unsupported import source %q", from)
	}
	seed := platformexec.ImportSeed{
		Manifest:         defaultManifest(defaultName(root), from, inferRuntime(root), "plugin-kit-ai plugin"),
		Explicit:         explicitFrom,
		IncludeUserScope: includeUserScope,
	}
	if requiresLauncherForTarget(from) {
		launcher := defaultLauncher(defaultName(root), inferRuntime(root))
		seed.Launcher = &launcher
	}
	imported, err := adapter.Import(root, seed)
	if err != nil {
		return preparedImport{}, err
	}
	artifacts := append([]Artifact{}, imported.Artifacts...)
	if mcpArtifacts, err := importedPortableMCPArtifacts(root); err != nil {
		return preparedImport{Warnings: imported.Warnings}, err
	} else {
		artifacts = append(artifacts, mcpArtifacts...)
	}
	if fileExists(filepath.Join(root, ".mcp.json")) {
		imported.Warnings = append(imported.Warnings, Warning{
			Kind:    WarningFidelity,
			Path:    ".mcp.json",
			Message: "portable MCP will be preserved under src/mcp/servers.yaml",
		})
	}
	return preparedImport{
		Manifest:      imported.Manifest,
		Launcher:      imported.Launcher,
		Artifacts:     artifacts,
		Warnings:      imported.Warnings,
		ImportSource:  from,
		DetectedKinds: detectedKinds,
		DroppedKinds:  uniqueSortedKinds(imported.DroppedKinds),
	}, nil
}

func writePreparedImport(root string, prepared preparedImport, force bool) error {
	layout := authoredLayout{RootRel: pluginmodel.SourceDirName}
	if err := saveManifestWithLayout(root, layout, prepared.Manifest, force); err != nil {
		return err
	}
	if prepared.Launcher != nil {
		if err := saveLauncherWithLayout(root, layout, *prepared.Launcher, force); err != nil {
			return err
		}
	}
	artifacts := prefixAuthoredArtifacts(prepared.Artifacts, layout)
	return writeArtifacts(root, artifacts)
}

func isSupportedImportSource(from string) bool {
	return slices.Contains(platformmeta.IDs(), from)
}

func inferClaudeEntrypoint(body []byte) (string, bool) {
	hooks, err := parseClaudeHooks(body)
	if err != nil {
		return "", false
	}
	for _, hookName := range claudeHookNames() {
		for _, entry := range hooks.Hooks[hookName] {
			for _, command := range entry.Hooks {
				if command.Type != "command" {
					continue
				}
				entrypoint, ok := trimClaudeHookCommand(command.Command, hookName)
				if ok {
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
		expected := entrypoint + " " + hookName
		foundCommand := false
		for _, entry := range entries {
			for _, command := range entry.Hooks {
				foundCommand = true
				if command.Type != "command" {
					mismatches = append(mismatches, fmt.Sprintf("entrypoint mismatch: Claude hook %q uses type %q; expected command %q", hookName, command.Type, expected))
					continue
				}
				if command.Command != expected {
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

func claudeHookNames() []string {
	return []string{
		"SessionStart",
		"SessionEnd",
		"Notification",
		"PostToolUse",
		"PostToolUseFailure",
		"PermissionRequest",
		"SubagentStart",
		"SubagentStop",
		"PreCompact",
		"Setup",
		"Stop",
		"PreToolUse",
		"TeammateIdle",
		"TaskCompleted",
		"UserPromptSubmit",
		"ConfigChange",
		"WorktreeCreate",
		"WorktreeRemove",
	}
}

func inferRuntime(root string) string {
	switch {
	case fileExists(filepath.Join(root, "go.mod")):
		return "go"
	case fileExists(filepath.Join(root, "src", "main.py")):
		return "python"
	case fileExists(filepath.Join(root, "src", "main.mjs")):
		return "node"
	case fileExists(filepath.Join(root, "scripts", "main.sh")):
		return "shell"
	default:
		return "go"
	}
}

func importedPortableMCPArtifacts(root string) ([]Artifact, error) {
	body, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse portable MCP .mcp.json: %w", err)
	}
	normalized, err := pluginmodel.ImportedPortableMCPYAML("", doc)
	if err != nil {
		return nil, err
	}
	return []Artifact{{RelPath: filepath.Join("mcp", "servers.yaml"), Content: normalized}}, nil
}

func prefixAuthoredArtifacts(artifacts []Artifact, layout authoredLayout) []Artifact {
	if strings.TrimSpace(layout.RootRel) == "" {
		return artifacts
	}
	out := make([]Artifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		rel := filepath.ToSlash(filepath.Clean(artifact.RelPath))
		prefix := filepath.ToSlash(layout.RootRel)
		if rel != prefix && !strings.HasPrefix(rel, prefix+"/") {
			artifact.RelPath = layout.Path(rel)
		} else {
			artifact.RelPath = rel
		}
		out = append(out, artifact)
	}
	return out
}
