package main

import (
	"encoding/json"
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

type publishRunner interface {
	Publish(app.PluginPublishOptions) (app.PluginPublishResult, error)
}

type publishJSONReport struct {
	Format        string            `json:"format"`
	SchemaVersion int               `json:"schema_version"`
	Channel       string            `json:"channel"`
	Target        string            `json:"target"`
	Mode          string            `json:"mode"`
	WorkflowClass string            `json:"workflow_class"`
	Dest          string            `json:"dest,omitempty"`
	PackageRoot   string            `json:"package_root,omitempty"`
	DetailCount   int               `json:"detail_count"`
	Details       map[string]string `json:"details"`
	NextStepCount int               `json:"next_step_count"`
	NextSteps     []string          `json:"next_steps"`
}

var publishCmd = newPublishCmd(pluginService)

func newPublishCmd(runner publishRunner) *cobra.Command {
	var channel string
	var dest string
	var packageRoot string
	var dryRun bool
	var format string
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
			switch format {
			case "json":
				body, err := json.MarshalIndent(buildPublishJSONReport(result), "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", body)
				return nil
			case "text":
			default:
				return fmt.Errorf("unsupported publish output format %q", format)
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
	cmd.Flags().StringVar(&format, "format", "text", `output format ("text" or "json")`)
	_ = cmd.MarkFlagRequired("channel")
	return cmd
}

func buildPublishJSONReport(result app.PluginPublishResult) publishJSONReport {
	details := result.Details
	if details == nil {
		details = map[string]string{}
	}
	nextSteps := result.NextSteps
	if nextSteps == nil {
		nextSteps = []string{}
	}
	return publishJSONReport{
		Format:        "plugin-kit-ai/publish-report",
		SchemaVersion: 1,
		Channel:       result.Channel,
		Target:        result.Target,
		Mode:          result.Mode,
		WorkflowClass: result.WorkflowClass,
		Dest:          result.Dest,
		PackageRoot:   result.PackageRoot,
		DetailCount:   len(details),
		Details:       details,
		NextStepCount: len(nextSteps),
		NextSteps:     nextSteps,
	}
}
