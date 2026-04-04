package main

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

type publishRunner interface {
	Publish(app.PluginPublishOptions) (app.PluginPublishResult, error)
}

var publishCmd = newPublishCmd(pluginService)

func newPublishCmd(runner publishRunner) *cobra.Command {
	var channel string
	var dest string
	var packageRoot string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "publish [path]",
		Short: "Publish a package target through a bounded channel workflow",
		Long: `Publish a package target through a bounded channel-family workflow.

This first-class publish entrypoint is intentionally bounded to documented channel flows:
- codex-marketplace
- claude-marketplace
- gemini-gallery (dry-run plan only)

Codex and Claude materialize a safe local marketplace root.
Gemini stays repository/release rooted, so publish only supports --dry-run planning there instead of a local marketplace materialization path.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			result, err := runner.Publish(app.PluginPublishOptions{
				Root:        root,
				Channel:     channel,
				Dest:        dest,
				PackageRoot: packageRoot,
				DryRun:      dryRun,
			})
			if err != nil {
				return err
			}
			for _, line := range result.Lines {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&channel, "channel", "", `publish channel ("codex-marketplace", "claude-marketplace", or "gemini-gallery")`)
	cmd.Flags().StringVar(&dest, "dest", "", "destination marketplace root directory for local Codex/Claude marketplace flows")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the materialized publish result without writing changes")
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}
