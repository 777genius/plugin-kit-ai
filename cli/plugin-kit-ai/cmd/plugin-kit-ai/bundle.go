package main

import (
	"fmt"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

type bundleInstallRunner interface {
	BundleInstall(app.PluginBundleInstallOptions) (app.PluginBundleInstallResult, error)
}

var (
	bundleInstallDest  string
	bundleInstallForce bool
)

var bundleCmd = newBundleCmd(pluginService)

func newBundleCmd(runner bundleInstallRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bundle",
		Short: "Bundle tooling for exported interpreted-runtime handoff archives",
	}
	cmd.AddCommand(newBundleInstallCmd(runner))
	return cmd
}

func newBundleInstallCmd(runner bundleInstallRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install <bundle.tar.gz>",
		Short: "Install a local exported Python/Node bundle into a destination directory",
		Long: `Install a local .tar.gz bundle created by plugin-kit-ai export into a destination directory.

This stable local handoff surface only supports local exported Python/Node bundles for codex-runtime or claude.
It unpacks bundle contents safely, prints next steps, and does not extend the binary-only plugin-kit-ai install flow.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := runner.BundleInstall(app.PluginBundleInstallOptions{
				Archive: args[0],
				Dest:    bundleInstallDest,
				Force:   bundleInstallForce,
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
	cmd.Flags().StringVar(&bundleInstallDest, "dest", "", "destination directory for unpacked bundle contents")
	cmd.Flags().BoolVarP(&bundleInstallForce, "force", "f", false, "overwrite an existing destination directory")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}
