package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
)

type fakeInitRunner struct {
	gotOpts app.InitOptions
	outDir  string
	err     error
}

func (f *fakeInitRunner) Run(opts app.InitOptions) (string, error) {
	f.gotOpts = opts
	return f.outDir, f.err
}

func executeInitCommand(t *testing.T, cmdArgs ...string) (string, error) {
	t.Helper()
	runner := &fakeInitRunner{outDir: "/tmp/demo plugin"}
	cmd := newInitCmd(runner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(cmdArgs)
	err := cmd.Execute()
	return buf.String(), err
}

func TestInitHelpIncludesScenarioLanesAndDefaults(t *testing.T) {
	t.Parallel()
	output, err := executeInitCommand(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Fast local plugin",
		"Production-ready plugin repo",
		"Already have native config",
		"plugin-kit-ai import",
		`--platform   Supported: "codex-runtime" (default), "codex-package", "claude", and "gemini".`,
		`--runtime    Supported: "go" (default), "python", "node", "shell" for launcher-based targets only.`,
		"--typescript Generate a TypeScript scaffold on top of the node runtime lane",
		"--runtime go remains the default",
		"--platform codex-package",
		"--claude-extended-hooks",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestInitCommandUsesDefaultPlatformAndRuntime(t *testing.T) {
	t.Parallel()
	runner := &fakeInitRunner{outDir: "/tmp/demo plugin"}
	cmd := newInitCmd(runner)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"demo"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if runner.gotOpts.Platform != "codex-runtime" {
		t.Fatalf("platform = %q, want codex-runtime", runner.gotOpts.Platform)
	}
	if runner.gotOpts.Runtime != "go" {
		t.Fatalf("runtime = %q, want go", runner.gotOpts.Runtime)
	}
}

func TestInitSuccessOutputByLane(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		args         []string
		want         []string
		notWant      []string
		wantRuntime  string
		wantPlatform string
	}{
		{
			name:         "codex-runtime-go",
			args:         []string{"demo"},
			wantRuntime:  "go",
			wantPlatform: "codex-runtime",
			want: []string{
				`Created plugin "demo" at /tmp/demo plugin`,
				`cd "/tmp/demo plugin"`,
				"plugin-kit-ai validate . --platform codex-runtime --strict",
				"See README.md for SDK setup and first-run steps",
			},
		},
		{
			name:         "codex-runtime-python",
			args:         []string{"demo", "--runtime", "python"},
			wantRuntime:  "python",
			wantPlatform: "codex-runtime",
			want: []string{
				"plugin-kit-ai bootstrap .",
				"plugin-kit-ai validate . --platform codex-runtime --strict",
				"See README.md for the full first run",
			},
			notWant: []string{
				"production-ready",
			},
		},
		{
			name:         "codex-runtime-node",
			args:         []string{"demo", "--runtime", "node"},
			wantRuntime:  "node",
			wantPlatform: "codex-runtime",
			want: []string{
				"plugin-kit-ai bootstrap .",
				"plugin-kit-ai validate . --platform codex-runtime --strict",
				"See README.md for the full first run",
			},
			notWant: []string{
				"production-ready",
			},
		},
		{
			name:         "codex-package",
			args:         []string{"demo", "--platform", "codex-package"},
			wantRuntime:  "",
			wantPlatform: "codex-package",
			want: []string{
				"plugin-kit-ai render .",
				"plugin-kit-ai validate . --platform codex-package --strict",
				"See README.md for the full first run",
			},
		},
		{
			name:         "gemini-go",
			args:         []string{"demo", "--platform", "gemini"},
			wantRuntime:  "",
			wantPlatform: "gemini",
			want: []string{
				"plugin-kit-ai render .",
				"plugin-kit-ai validate . --platform gemini --strict",
				"See README.md for the full first run",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeInitRunner{outDir: "/tmp/demo plugin"}
			cmd := newInitCmd(runner)
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			output := buf.String()
			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Fatalf("output missing %q:\n%s", want, output)
				}
			}
			for _, unwanted := range tc.notWant {
				if strings.Contains(output, unwanted) {
					t.Fatalf("output unexpectedly contains %q:\n%s", unwanted, output)
				}
			}
			if runner.gotOpts.Runtime != tc.wantRuntime {
				t.Fatalf("runtime = %q, want %q", runner.gotOpts.Runtime, tc.wantRuntime)
			}
			if runner.gotOpts.Platform != tc.wantPlatform {
				t.Fatalf("platform = %q, want %q", runner.gotOpts.Platform, tc.wantPlatform)
			}
		})
	}
}

func TestInitCommandRejectsTypeScriptOutsideNodeRuntime(t *testing.T) {
	t.Parallel()
	cmd := newInitCmd(app.InitRunner{})
	cmd.SetArgs([]string{"demo", "--typescript", "-o", t.TempDir()})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--typescript requires --runtime node") {
		t.Fatalf("err = %v", err)
	}
}
