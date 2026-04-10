package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

func newPublicationCmd(runner inspectRunner) *cobra.Command {
	var target string
	var format string
	cmd := &cobra.Command{
		Use:   "publication [path]",
		Short: "Show the publication-oriented package and channel view",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			report, warnings, err := runner.Inspect(app.PluginInspectOptions{
				Root:   root,
				Target: target,
			})
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(format)) {
			case "", "text":
				for _, warning := range warnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "publication %s %s api_version=%s\n",
					report.Publication.Core.Name,
					report.Publication.Core.Version,
					report.Publication.Core.APIVersion,
				)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "packages: %d channels: %d\n",
					len(report.Publication.Packages),
					len(report.Publication.Channels),
				)
				for _, pkg := range report.Publication.Packages {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  package[%s]: family=%s channels=%s inputs=%d managed=%d\n",
						pkg.Target,
						pkg.PackageFamily,
						strings.Join(pkg.ChannelFamilies, ","),
						len(pkg.AuthoredInputs),
						len(pkg.ManagedArtifacts),
					)
				}
				for _, channel := range report.Publication.Channels {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  channel[%s]: path=%s targets=%s",
						channel.Family,
						channel.Path,
						strings.Join(channel.PackageTargets, ","),
					)
					if details := inspectChannelDetails(channel.Details); details != "" {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), " details=%s", details)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				}
				return nil
			case "json":
				out, err := json.MarshalIndent(buildPublicationJSONReport(report, warnings, target), "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			default:
				return fmt.Errorf("unsupported format %q (use text or json)", format)
			}
		},
	}
	cmd.Flags().StringVar(&target, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.AddCommand(newPublicationDoctorCmd(runner))
	if materializer, ok := any(runner).(publicationMaterializeRunner); ok {
		cmd.AddCommand(newPublicationMaterializeCmd(materializer))
		cmd.AddCommand(newPublicationRemoveCmd(materializer))
	}
	return cmd
}

func newPublicationMaterializeCmd(runner publicationMaterializeRunner) *cobra.Command {
	var target string
	var dest string
	var packageRoot string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "materialize [path]",
		Short: "Materialize a safe local marketplace root for Codex or Claude",
		Long: `Create or update a local marketplace root for a single publication-capable package target.

This workflow is intentionally limited to documented local/catalog flows:
- Codex marketplace roots with .agents/plugins/marketplace.json
- Claude marketplace roots with .claude-plugin/marketplace.json

It copies the materialized package bundle under a managed package root, then merges or creates the marketplace catalog artifact.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			result, err := runner.PublicationMaterialize(app.PluginPublicationMaterializeOptions{
				Root:        root,
				Target:      target,
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
	cmd.Flags().StringVar(&target, "target", "", `materialization target ("claude" or "codex-package")`)
	cmd.Flags().StringVar(&dest, "dest", "", "destination marketplace root directory")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the materialized package root and catalog changes without writing them")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}

func newPublicationRemoveCmd(runner publicationMaterializeRunner) *cobra.Command {
	var target string
	var dest string
	var packageRoot string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "remove [path]",
		Short: "Remove a materialized local marketplace package root and catalog entry",
		Long: `Remove a single plugin from a local Codex or Claude marketplace root.

This workflow is intentionally scoped to documented local/catalog flows and is safe to rerun.
It removes the selected package root and prunes the matching plugin entry from the marketplace catalog while preserving the marketplace root itself.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			result, err := runner.PublicationRemove(app.PluginPublicationRemoveOptions{
				Root:        root,
				Target:      target,
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
	cmd.Flags().StringVar(&target, "target", "", `removal target ("claude" or "codex-package")`)
	cmd.Flags().StringVar(&dest, "dest", "", "destination marketplace root directory")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the package root and catalog pruning without writing changes")
	_ = cmd.MarkFlagRequired("target")
	_ = cmd.MarkFlagRequired("dest")
	return cmd
}
