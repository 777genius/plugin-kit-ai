package main

import (
	"fmt"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

var pluginService app.PluginService

var (
	renderTarget string
	renderCheck  bool
)

var renderCmd = &cobra.Command{
	Use:   "render [path]",
	Short: "Compile native target artifacts from the package graph",
	Long: `Compile native target artifacts from the package graph discovered via plugin.yaml and standard directories.

Claude and Codex runtime/package lanes render their managed native artifacts from the package graph.
Gemini rendering is packaging-only: it produces a native extension manifest, but does not imply runtime parity or a production-ready Gemini runtime path.
OpenCode rendering is workspace-config-only: it produces opencode.json plus mirrored skills, commands, agents, and themes, while keeping local JS/TS plugin code for the later OpenCode code-plugin wave.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		out, err := pluginService.Render(app.PluginRenderOptions{
			Root:   root,
			Target: renderTarget,
			Check:  renderCheck,
		})
		if err != nil {
			return err
		}
		if renderCheck {
			if len(out) > 0 {
				return fmt.Errorf("generated artifacts drifted: %v", out)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Generated artifacts are up to date in %s\n", root)
			return nil
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Rendered %d artifact(s) in %s\n", len(out), root)
		return nil
	},
}

func init() {
	renderCmd.Flags().StringVar(&renderTarget, "target", "all", `render target ("all", "claude", "codex-package", "codex-runtime", "gemini", "opencode")`)
	renderCmd.Flags().BoolVar(&renderCheck, "check", false, "fail if generated artifacts are out of date")
}
