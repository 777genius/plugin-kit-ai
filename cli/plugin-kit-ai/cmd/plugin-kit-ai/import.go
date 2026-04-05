package main

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

var (
	importFrom             string
	importForce            bool
	importIncludeUserScope bool
)

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import current native target artifacts into the package standard layout",
	Long: `Import an existing native plugin into the package standard layout.

Claude import maps native plugin artifacts into the package-standard layout under src/.
Codex import materializes either the official package lane or the local runtime lane from current native artifacts. Use codex-package or codex-runtime explicitly for the lane you want to preserve.
Gemini import backfills the extension package layout and may preserve an optional launcher-based Go runtime lane when that authored project already uses one. That runtime lane now exposes a production-ready 9-hook surface, but it still does not imply blanket Gemini runtime parity for future hooks beyond the promoted contract.
OpenCode import is workspace-config-only in the current contract: it normalizes project-native JSON/JSONC config, commands, agents, themes, local plugin code, plugin-local package metadata, compatible skill roots, and optional user-scope OpenCode sources into the canonical package-standard layout.
Cursor import is workspace-config-only in the current contract: it normalizes .cursor/mcp.json and .cursor/rules/** into the canonical src-authored layout.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		warnings, err := pluginService.Import(app.PluginImportOptions{
			Root:             root,
			From:             importFrom,
			Force:            importForce,
			IncludeUserScope: importIncludeUserScope,
		})
		if err != nil {
			return err
		}
		for _, warning := range warnings {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Imported %s into the package standard layout\n", root)
		return nil
	},
}

func init() {
	importCmd.Flags().StringVar(&importFrom, "from", "", `source platform ("claude", "codex-package", "codex-runtime", "gemini", "opencode", or "cursor"; omit to auto-detect current native layouts)`)
	importCmd.Flags().BoolVarP(&importForce, "force", "f", false, "overwrite src/plugin.yaml if it already exists")
	importCmd.Flags().BoolVar(&importIncludeUserScope, "include-user-scope", false, "include explicit user-scope native sources when supported by the import target")
}
