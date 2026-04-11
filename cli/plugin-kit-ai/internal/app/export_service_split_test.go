package app

import (
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
)

func TestResolveExportServiceInputDefaultsRootAndRequiresPlatform(t *testing.T) {
	t.Parallel()

	root, platform, err := resolveExportServiceInput(PluginExportOptions{Platform: "claude"})
	if err != nil {
		t.Fatalf("resolveExportServiceInput: %v", err)
	}
	if root != "." || platform != "claude" {
		t.Fatalf("root/platform = %q/%q", root, platform)
	}
	if _, _, err := resolveExportServiceInput(PluginExportOptions{}); err == nil {
		t.Fatal("expected missing platform error")
	}
}

func TestBuildExportResultLinesIncludesRuntimeMetadata(t *testing.T) {
	t.Parallel()

	lines := buildExportResultLines(exportServiceContext{
		platform: "claude",
		project:  runtimecheck.Project{Root: "/tmp/out", Runtime: "node"},
	}, exportArchivePlan{
		outputPath: "/tmp/out/demo_claude_node_bundle.tar.gz",
		files:      []string{"src/plugin.yaml"},
		metadata: exportMetadata{
			RuntimeRequirement: "Node.js 20+ installed on the machine running the plugin",
			RuntimeInstallHint: "Go is the recommended path when you want users to avoid installing Node.js before running the plugin",
		},
	})
	text := strings.Join(lines, "\n")
	for _, want := range []string{
		"Runtime requirement: Node.js 20+ installed on the machine running the plugin",
		"Runtime install hint: Go is the recommended path when you want users to avoid installing Node.js before running the plugin",
		"tar -xzf demo_claude_node_bundle.tar.gz",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("lines missing %q:\n%s", want, text)
		}
	}
}
