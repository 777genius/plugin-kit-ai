package main

import (
	"fmt"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

var (
	importFrom  string
	importForce bool
)

var importCmd = &cobra.Command{
	Use:   "import [path]",
	Short: "Import current native target artifacts into the package standard layout",
	Long: `Import an existing native plugin into the package standard layout.

Claude import maps native plugin artifacts into the package-standard layout.
Codex import can materialize the official package lane, the local runtime lane, or both from current native artifacts.
Gemini import is packaging-only in the current contract: it backfills manifest metadata, but does not promote Gemini to a production-ready runtime target.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		warnings, err := pluginService.Import(app.PluginImportOptions{
			Root:  root,
			From:  importFrom,
			Force: importForce,
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
	importCmd.Flags().StringVar(&importFrom, "from", "", `source platform ("claude", "codex", "codex-package", "codex-runtime", "gemini")`)
	importCmd.Flags().BoolVarP(&importForce, "force", "f", false, "overwrite plugin.yaml if it already exists")
}
