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

var publishCmd = newPublishCmd(pluginService)

func newPublishCmd(runner publishRunner) *cobra.Command {
	var channel string
	var dest string
	var packageRoot string
	var dryRun bool
	var all bool
	var format string
	cmd := &cobra.Command{
		Use:   "publish [path]",
		Short: "Publish a package target through a bounded channel workflow",
		Long: `Publish a package target through a bounded channel-family workflow.

This first-class publish entrypoint is intentionally bounded to documented channel flows:
- codex-marketplace
- claude-marketplace
- gemini-gallery (dry-run plan only)
- all authored channels (dry-run plan only)

Codex and Claude materialize a safe local marketplace root.
Gemini stays repository/release rooted, so publish only supports --dry-run planning there instead of a local marketplace materialization path.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			if all && channel != "" {
				return fmt.Errorf("publish --all cannot be combined with --channel")
			}
			if !all && channel == "" {
				return fmt.Errorf("publish requires --channel unless --all is set")
			}
			if all && !dryRun {
				return fmt.Errorf("publish --all currently supports only --dry-run planning")
			}
			result, err := runner.Publish(app.PluginPublishOptions{
				Root:        root,
				Channel:     channel,
				Dest:        dest,
				PackageRoot: packageRoot,
				DryRun:      dryRun,
				All:         all,
			})
			if err != nil {
				return err
			}
			switch format {
			case "json":
				body, err := json.MarshalIndent(buildPublishJSONPayload(result), "", "  ")
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
	cmd.Flags().BoolVar(&all, "all", false, "plan across all authored publication channels (dry-run only)")
	cmd.Flags().StringVar(&dest, "dest", "", "destination marketplace root directory for local Codex/Claude marketplace flows")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview the materialized publish result without writing changes")
	cmd.Flags().StringVar(&format, "format", "text", `output format ("text" or "json")`)
	return cmd
}

func buildPublishJSONPayload(result app.PluginPublishResult) map[string]any {
	details := result.Details
	if details == nil {
		details = map[string]string{}
	}
	issues := result.Issues
	if issues == nil {
		issues = []app.PluginPublishIssue{}
	}
	warnings := result.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	nextSteps := result.NextSteps
	if nextSteps == nil {
		nextSteps = []string{}
	}
	channels := buildPublishJSONPayloads(result.Channels)
	payload := map[string]any{
		"format":          "plugin-kit-ai/publish-report",
		"schema_version":  1,
		"ready":           result.Ready,
		"status":          result.Status,
		"mode":            result.Mode,
		"workflow_class":  result.WorkflowClass,
		"detail_count":    len(details),
		"details":         details,
		"issue_count":     len(issues),
		"issues":          append([]app.PluginPublishIssue{}, issues...),
		"next_step_count": len(nextSteps),
		"next_steps":      nextSteps,
	}
	if result.Channel != "" {
		payload["channel"] = result.Channel
	}
	if result.Target != "" {
		payload["target"] = result.Target
	}
	if result.Dest != "" {
		payload["dest"] = result.Dest
	}
	if result.PackageRoot != "" {
		payload["package_root"] = result.PackageRoot
	}
	if result.WorkflowClass == "multi_channel_plan" || len(warnings) > 0 {
		payload["warning_count"] = len(warnings)
		payload["warnings"] = append([]string(nil), warnings...)
	}
	if result.WorkflowClass == "multi_channel_plan" || len(channels) > 0 {
		payload["channel_count"] = len(channels)
		payload["channels"] = channels
	}
	return payload
}

func buildPublishJSONPayloads(results []app.PluginPublishResult) []map[string]any {
	if len(results) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(results))
	for _, result := range results {
		out = append(out, buildPublishJSONPayload(result))
	}
	return out
}
