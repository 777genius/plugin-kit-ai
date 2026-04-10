package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
)

func renderInspectText(report pluginmanifest.Inspection) string {
	var out strings.Builder
	_, _ = fmt.Fprintf(&out, "package %s %s\n", report.Manifest.Name, report.Manifest.Version)
	_, _ = fmt.Fprintf(&out, "targets: %s\n", strings.Join(report.Manifest.Targets, ", "))
	if authoredRoot := strings.TrimSpace(report.Layout.AuthoredRoot); authoredRoot != "" {
		_, _ = fmt.Fprintf(&out, "layout: authored_root=%s", authoredRoot)
		if len(report.Layout.BoundaryDocs) > 0 {
			_, _ = fmt.Fprintf(&out, " boundary_docs=%s", strings.Join(report.Layout.BoundaryDocs, ","))
		}
		if generatedGuide := strings.TrimSpace(report.Layout.GeneratedGuide); generatedGuide != "" {
			_, _ = fmt.Fprintf(&out, " generated_guide=%s", generatedGuide)
		}
		_, _ = fmt.Fprintln(&out)
	}
	_, _ = fmt.Fprintf(&out, "portable: skills=%d mcp=%t\n", len(report.Portable.Paths("skills")), report.Portable.MCP != nil)
	_, _ = fmt.Fprintf(&out, "publication: api_version=%s packages=%d channels=%d\n", report.Publication.Core.APIVersion, len(report.Publication.Packages), len(report.Publication.Channels))
	if report.Launcher != nil {
		_, _ = fmt.Fprintf(&out, "launcher: runtime=%s entrypoint=%s\n", report.Launcher.Runtime, report.Launcher.Entrypoint)
	}
	renderInspectLayoutSection(&out, report)
	renderInspectPublicationSection(&out, report)
	renderInspectTargetsSection(&out, report)
	return out.String()
}

func renderInspectLayoutSection(out *strings.Builder, report pluginmanifest.Inspection) {
	if len(report.Layout.AuthoredInputs) > 0 {
		_, _ = fmt.Fprintln(out, "authored_inputs:")
		for _, path := range report.Layout.AuthoredInputs {
			_, _ = fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	if len(report.Layout.GeneratedOutputs) > 0 {
		_, _ = fmt.Fprintln(out, "generated_outputs:")
		for _, path := range report.Layout.GeneratedOutputs {
			_, _ = fmt.Fprintf(out, "  - %s\n", path)
		}
	}
	if len(report.Layout.GeneratedByTarget) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "generated_by_target:")
	var targetNames []string
	for name := range report.Layout.GeneratedByTarget {
		targetNames = append(targetNames, name)
	}
	slices.Sort(targetNames)
	for _, name := range targetNames {
		_, _ = fmt.Fprintf(out, "  %s:\n", name)
		for _, path := range report.Layout.GeneratedByTarget[name] {
			_, _ = fmt.Fprintf(out, "    - %s\n", path)
		}
	}
}

func renderInspectPublicationSection(out *strings.Builder, report pluginmanifest.Inspection) {
	for _, channel := range report.Publication.Channels {
		_, _ = fmt.Fprintf(out, "  channel[%s]: path=%s targets=%s",
			channel.Family,
			channel.Path,
			strings.Join(channel.PackageTargets, ","),
		)
		if details := inspectChannelDetails(channel.Details); details != "" {
			_, _ = fmt.Fprintf(out, " details=%s", details)
		}
		_, _ = fmt.Fprintln(out)
	}
	for _, pkg := range report.Publication.Packages {
		_, _ = fmt.Fprintf(out, "  publish[%s]: family=%s channels=%s inputs=%d managed=%d\n",
			pkg.Target,
			pkg.PackageFamily,
			strings.Join(pkg.ChannelFamilies, ","),
			len(pkg.AuthoredInputs),
			len(pkg.ManagedArtifacts),
		)
	}
}

func renderInspectTargetsSection(out *strings.Builder, report pluginmanifest.Inspection) {
	for _, target := range report.Targets {
		_, _ = fmt.Fprintf(out, "- %s: class=%s production=%s runtime=%s native=%s managed=%s\n",
			target.Target,
			target.TargetClass,
			target.ProductionClass,
			target.RuntimeContract,
			strings.Join(target.TargetNativeKinds, ","),
			strings.Join(target.ManagedArtifacts, ","),
		)
		if docs := inspectTargetDocs(target); len(docs) > 0 {
			_, _ = fmt.Fprintf(out, "  docs=%s\n", strings.Join(docs, ","))
		}
		if len(target.UnsupportedKinds) > 0 {
			_, _ = fmt.Fprintf(out, "  unsupported=%s\n", strings.Join(target.UnsupportedKinds, ","))
		}
		if len(target.NativeSurfaces) > 0 {
			var tiers []string
			for _, surface := range target.NativeSurfaces {
				tiers = append(tiers, surface.Kind+"="+surface.Tier)
			}
			_, _ = fmt.Fprintf(out, "  surfaces=%s\n", strings.Join(tiers, ","))
		}
		for _, advice := range inspectTargetAdvice(report, target) {
			_, _ = fmt.Fprintf(out, "  %s\n", advice)
		}
	}
}

func inspectTargetDocs(target pluginmanifest.InspectTarget) []string {
	if len(target.NativeDocPaths) == 0 {
		return nil
	}
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
	return docs
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
