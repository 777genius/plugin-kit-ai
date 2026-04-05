package main

import (
	"encoding/json"
	"fmt"
	"slices"
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
				if authoredRoot := strings.TrimSpace(report.Layout.AuthoredRoot); authoredRoot != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "layout: authored_root=%s", authoredRoot)
					if len(report.Layout.BoundaryDocs) > 0 {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), " boundary_docs=%s", strings.Join(report.Layout.BoundaryDocs, ","))
					}
					if generatedGuide := strings.TrimSpace(report.Layout.GeneratedGuide); generatedGuide != "" {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), " generated_guide=%s", generatedGuide)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "portable: skills=%d mcp=%t\n", len(report.Portable.Paths("skills")), report.Portable.MCP != nil)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "publication: api_version=%s packages=%d channels=%d\n", report.Publication.Core.APIVersion, len(report.Publication.Packages), len(report.Publication.Channels))
				if report.Launcher != nil {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "launcher: runtime=%s entrypoint=%s\n", report.Launcher.Runtime, report.Launcher.Entrypoint)
				}
				if len(report.Layout.AuthoredInputs) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "authored_inputs:")
					for _, path := range report.Layout.AuthoredInputs {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", path)
					}
				}
				if len(report.Layout.GeneratedOutputs) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "generated_outputs:")
					for _, path := range report.Layout.GeneratedOutputs {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", path)
					}
				}
				if len(report.Layout.GeneratedByTarget) > 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "generated_by_target:")
					var targetNames []string
					for name := range report.Layout.GeneratedByTarget {
						targetNames = append(targetNames, name)
					}
					slices.Sort(targetNames)
					for _, name := range targetNames {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s:\n", name)
						for _, path := range report.Layout.GeneratedByTarget[name] {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    - %s\n", path)
						}
					}
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
				for _, pkg := range report.Publication.Packages {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  publish[%s]: family=%s channels=%s inputs=%d managed=%d\n",
						pkg.Target,
						pkg.PackageFamily,
						strings.Join(pkg.ChannelFamilies, ","),
						len(pkg.AuthoredInputs),
						len(pkg.ManagedArtifacts),
					)
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
					if len(target.NativeDocPaths) > 0 {
						var docs []string
						for _, kind := range target.TargetNativeKinds {
							if path := strings.TrimSpace(target.NativeDocPaths[kind]); path != "" {
								docs = append(docs, kind+"="+path)
							}
						}
						var remainingKinds []string
						for kind := range target.NativeDocPaths {
							remainingKinds = append(remainingKinds, kind)
						}
						slices.Sort(remainingKinds)
						for _, kind := range remainingKinds {
							path := target.NativeDocPaths[kind]
							if strings.TrimSpace(path) == "" || containsInspectDoc(docs, kind) {
								continue
							}
							docs = append(docs, kind+"="+path)
						}
						if len(docs) > 0 {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  docs=%s\n", strings.Join(docs, ","))
						}
					}
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

func inspectChannelDetails(details map[string]string) string {
	if len(details) == 0 {
		return ""
	}
	keys := make([]string, 0, len(details))
	for key, value := range details {
		if strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if len(keys) == 0 {
		return ""
	}
	items := make([]string, 0, len(keys))
	for _, key := range keys {
		items = append(items, key+"="+details[key])
	}
	return strings.Join(items, ",")
}

func containsInspectDoc(items []string, kind string) bool {
	prefix := kind + "="
	for _, item := range items {
		if strings.HasPrefix(item, prefix) {
			return true
		}
	}
	return false
}

func inspectTargetAdvice(report pluginmanifest.Inspection, target pluginmanifest.InspectTarget) []string {
	if target.Target != "gemini" {
		return nil
	}
	if report.Launcher == nil {
		return []string{
			"next=generate --check + validate --strict keep the packaging lane honest; add --runtime go when you want the Gemini production-ready 9-hook runtime",
		}
	}
	return []string{
		"next=go test ./...; plugin-kit-ai generate --check .; plugin-kit-ai validate . --platform gemini --strict; gemini extensions link .",
		"runtime_gate=make test-gemini-runtime",
		"live_runtime_gate=make test-gemini-runtime-live",
	}
}
