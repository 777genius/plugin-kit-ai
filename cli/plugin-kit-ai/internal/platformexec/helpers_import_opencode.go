package platformexec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type importedOpenCodeConfig struct {
	Plugins          []opencodePluginRef
	PluginsProvided  bool
	MCP              map[string]any
	MCPProvided      bool
	Commands         map[string]any
	CommandsProvided bool
	Agents           map[string]any
	AgentsProvided   bool
	DefaultAgent     string
	DefaultAgentSet  bool
	Instructions     []string
	InstructionsSet  bool
	Permission       any
	PermissionSet    bool
	Extra            map[string]any
}

func resolveOpenCodeConfigPathInDir(dir string, warningBase string) (string, []pluginmodel.Warning, bool, error) {
	jsonRel := "opencode.json"
	jsoncRel := "opencode.jsonc"
	jsonPath := filepath.Join(dir, jsonRel)
	jsoncPath := filepath.Join(dir, jsoncRel)
	hasJSON := fileExists(jsonPath)
	hasJSONC := fileExists(jsoncPath)
	warnPath := jsoncRel
	if strings.TrimSpace(warningBase) != "" {
		warnPath = filepath.ToSlash(filepath.Join(warningBase, jsoncRel))
	}
	switch {
	case hasJSON && hasJSONC:
		return jsonPath, []pluginmodel.Warning{{
			Kind:    pluginmodel.WarningFidelity,
			Path:    warnPath,
			Message: "ignored opencode.jsonc because opencode.json takes precedence during OpenCode import normalization",
		}}, true, nil
	case hasJSON:
		return jsonPath, nil, true, nil
	case hasJSONC:
		return jsoncPath, nil, true, nil
	default:
		return "", nil, false, nil
	}
}

func resolveOpenCodeConfigPath(root string) (string, []pluginmodel.Warning, bool, error) {
	path, warnings, ok, err := resolveOpenCodeConfigPathInDir(root, "")
	if err != nil || !ok {
		return "", warnings, ok, err
	}
	rel, rerr := filepath.Rel(root, path)
	if rerr != nil {
		return "", nil, false, rerr
	}
	return filepath.ToSlash(rel), warnings, true, nil
}

func readImportedOpenCodeConfigFromFile(path string, displayPath string) (importedOpenCodeConfig, string, []pluginmodel.Warning, bool, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return importedOpenCodeConfig{}, "", nil, false, err
	}
	data, err := decodeImportedOpenCodeConfig(body)
	if err != nil {
		return importedOpenCodeConfig{}, displayPath, nil, false, err
	}
	if strings.TrimSpace(displayPath) == "" {
		displayPath = filepath.ToSlash(path)
	}
	return data, displayPath, nil, true, nil
}
