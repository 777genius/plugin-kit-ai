package platformexec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func newOpenCodeImportedState() opencodeImportedState {
	return opencodeImportedState{
		artifacts: map[string]pluginmodel.Artifact{},
	}
}

func importOpenCodeUserScope(state *opencodeImportedState, seed ImportSeed) error {
	if !seed.IncludeUserScope {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home for OpenCode import: %w", err)
	}
	if err := rejectOpenCodeCompatSkillRoots(openCodeUserCompatSkillRoots(home)); err != nil {
		return err
	}
	globalRoot := filepath.Join(home, ".config", "opencode")
	return importOpenCodeScope(state, opencodeScopeConfig{
		root:              globalRoot,
		displayConfigRoot: filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
		workspaceRoot:     globalRoot,
		workspaceDisplay:  filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
	})
}

func importOpenCodeProjectScope(state *opencodeImportedState, root string) error {
	if err := rejectOpenCodeCompatSkillRoots(openCodeProjectCompatSkillRoots(root)); err != nil {
		return err
	}
	return importOpenCodeScope(state, opencodeScopeConfig{
		root:              root,
		displayConfigRoot: "",
		workspaceRoot:     filepath.Join(root, ".opencode"),
		workspaceDisplay:  ".opencode",
	})
}

func requireOpenCodeImportedInput(state opencodeImportedState) error {
	if state.hasInput {
		return nil
	}
	return fmt.Errorf("OpenCode import requires opencode.json, opencode.jsonc, supported workspace directories, or --include-user-scope with OpenCode native sources")
}

type openCodeCompatSkillRoot struct {
	full    string
	display string
}

func openCodeUserCompatSkillRoots(home string) []openCodeCompatSkillRoot {
	return []openCodeCompatSkillRoot{
		{full: filepath.Join(home, ".agents", "skills"), display: filepath.ToSlash(filepath.Join("~", ".agents", "skills"))},
		{full: filepath.Join(home, ".claude", "skills"), display: filepath.ToSlash(filepath.Join("~", ".claude", "skills"))},
	}
}

func openCodeProjectCompatSkillRoots(root string) []openCodeCompatSkillRoot {
	return []openCodeCompatSkillRoot{
		{full: filepath.Join(root, ".agents", "skills"), display: filepath.ToSlash(filepath.Join(".agents", "skills"))},
		{full: filepath.Join(root, ".claude", "skills"), display: filepath.ToSlash(filepath.Join(".claude", "skills"))},
	}
}

func rejectOpenCodeCompatSkillRoots(roots []openCodeCompatSkillRoot) error {
	for _, reject := range roots {
		if err := rejectOpenCodeCompatSkillRoot(reject.full, reject.display); err != nil {
			return err
		}
	}
	return nil
}
