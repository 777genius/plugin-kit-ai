package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/spf13/cobra"
)

var inspectTarget string
var inspectFormat string

type inspectRunner interface {
	Inspect(app.PluginInspectOptions) (pluginmanifest.Inspection, []pluginmanifest.Warning, error)
}

var inspectCmd = newInspectCmd(pluginService)

func newInspectCmd(runner inspectRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect [path]",
		Short: "Inspect the discovered package graph and target coverage",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			report, warnings, err := runner.Inspect(app.PluginInspectOptions{
				Root:   root,
				Target: inspectTarget,
			})
			if err != nil {
				return err
			}
			for _, warning := range warnings {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
			}
			switch strings.ToLower(strings.TrimSpace(inspectFormat)) {
			case "", "text":
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "package %s %s\n", report.Manifest.Name, report.Manifest.Version)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "targets: %s\n", strings.Join(report.Manifest.Targets, ", "))
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "portable: skills=%d mcp=%t\n", len(report.Portable.Paths("skills")), report.Portable.MCP != nil)
				if report.Launcher != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "launcher: runtime=%s entrypoint=%s\n", report.Launcher.Runtime, report.Launcher.Entrypoint)
				}
				for _, target := range report.Targets {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s: class=%s production=%s runtime=%s native=%s managed=%s\n",
						target.Target,
						target.TargetClass,
						target.ProductionClass,
						target.RuntimeContract,
						strings.Join(target.TargetNativeKinds, ","),
						strings.Join(target.ManagedArtifacts, ","),
					)
					if len(target.UnsupportedKinds) > 0 {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  unsupported=%s\n", strings.Join(target.UnsupportedKinds, ","))
					}
					if len(target.NativeSurfaces) > 0 {
						var tiers []string
						for _, surface := range target.NativeSurfaces {
							tiers = append(tiers, surface.Kind+"="+surface.Tier)
						}
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  surfaces=%s\n", strings.Join(tiers, ","))
					}
					for _, advice := range inspectTargetAdvice(report, target) {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", advice)
					}
				}
				return nil
			case "json":
				out, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			default:
				return fmt.Errorf("unsupported format %q (use text or json)", inspectFormat)
			}
		},
	}
	cmd.Flags().StringVar(&inspectTarget, "target", "all", `inspect target ("all", "claude", "codex-package", "codex-runtime", "gemini", "opencode", or "cursor")`)
	cmd.Flags().StringVar(&inspectFormat, "format", "text", "output format: text or json")
	return cmd
}

func inspectTargetAdvice(report pluginmanifest.Inspection, target pluginmanifest.InspectTarget) []string {
	if target.Target != "gemini" {
		return nil
	}
	if report.Launcher == nil {
		return []string{
			"next=render --check + validate --strict keep the packaging lane honest; add --runtime go when you want the Gemini production runtime",
		}
	}
	return []string{
		"next=go test ./...; plugin-kit-ai render --check .; plugin-kit-ai validate . --platform gemini --strict; gemini extensions link .",
		"runtime_gate=make test-gemini-runtime",
		"live_runtime_gate=make test-gemini-runtime-live",
	}
}
