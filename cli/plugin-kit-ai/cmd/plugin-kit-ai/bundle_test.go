package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
)

type fakeBundleRunner struct {
	installResult app.PluginBundleInstallResult
	installErr    error
	fetchResult   app.PluginBundleFetchResult
	fetchErr      error
}

func (f fakeBundleRunner) BundleInstall(app.PluginBundleInstallOptions) (app.PluginBundleInstallResult, error) {
	return f.installResult, f.installErr
}

func (f fakeBundleRunner) BundleFetch(_ context.Context, _ app.PluginBundleFetchOptions) (app.PluginBundleFetchResult, error) {
	return f.fetchResult, f.fetchErr
}

func TestBundleInstallHelpIncludesLocalTarballLanguage(t *testing.T) {
	cmd := newBundleCmd(fakeBundleRunner{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"install", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{"local .tar.gz", "Python/Node", "binary-only"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestBundleInstallWritesRunnerOutput(t *testing.T) {
	cmd := newBundleCmd(fakeBundleRunner{
		installResult: app.PluginBundleInstallResult{
			Lines: []string{
				"Bundle: plugin=demo platform=codex-runtime runtime=python manager=requirements.txt (pip)",
				"Installed path: /tmp/demo",
			},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"install", "--dest", "/tmp/demo", "/tmp/demo.tar.gz"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Installed path: /tmp/demo") {
		t.Fatalf("output = %s", output)
	}
}

func TestBundleFetchHelpIncludesURLAndGitHubLanguage(t *testing.T) {
	cmd := newBundleCmd(fakeBundleRunner{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"fetch", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	for _, want := range []string{"HTTPS", "owner/repo", "binary-only"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestBundleFetchWritesRunnerOutput(t *testing.T) {
	cmd := newBundleCmd(fakeBundleRunner{
		fetchResult: app.PluginBundleFetchResult{
			Lines: []string{
				"Bundle source: https://example.com/demo_bundle.tar.gz",
				"Installed path: /tmp/demo",
			},
		},
	})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"fetch", "--url", "https://example.com/demo_bundle.tar.gz", "--dest", "/tmp/demo"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Bundle source: https://example.com/demo_bundle.tar.gz") {
		t.Fatalf("output = %s", output)
	}
}
