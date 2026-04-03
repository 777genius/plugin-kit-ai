package main

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

type initCommandRunner interface {
	Run(app.InitOptions) (string, error)
}

type initFlagState struct {
	platform              string
	runtime               string
	typescript            bool
	runtimePackage        bool
	runtimePackageVersion string
	output                string
	force                 bool
	extras                bool
	claudeExtendedHooks   bool
}

var initRunner app.InitRunner

var initCmd = newInitCmd(initRunner)

var stableRuntimePackageVersionRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

const initLongDescription = `Creates a package-standard plugin-kit-ai project scaffold.

Choose the lane that matches your goal:

Fast local plugin:
  Use --runtime python or --runtime node when repo-local iteration matters more than packaged distribution.
  These are supported executable-runtime paths, not equal production paths.

Production-ready plugin repo:
  Plain init keeps the strongest supported runtime path. --runtime go remains the default, and --platform codex-runtime remains the default target.
  Use --platform claude for Claude hooks, and add --claude-extended-hooks only when you intentionally want the wider runtime-supported subset.
  Use --platform codex-package for the official Codex plugin bundle without local notify/runtime wiring.
  Use --platform opencode for the OpenCode workspace-config lane without launcher/runtime scaffolding.
  Use --platform cursor for the Cursor workspace-config lane without launcher/runtime scaffolding.

Already have native config:
  Use plugin-kit-ai import to bring current Claude/Codex/Gemini/OpenCode/Cursor native files into the package-standard authored layout.
  init is for creating a new package-standard project, not for preserving native files as the authored source of truth.

Public flags:
  --platform   Supported: "codex-runtime" (default), "codex-package", "claude", "gemini", "opencode", and "cursor".
  --runtime    Supported: "go" (default), "python", "node", "shell" for launcher-based targets only.
  --typescript Generate a TypeScript scaffold on top of the node runtime lane (requires --runtime node).
  --runtime-package
               For --runtime python or --runtime node, import the shared plugin-kit-ai-runtime package instead of vendoring the helper file into src/.
  --runtime-package-version
               Pin the generated plugin-kit-ai-runtime dependency version. Required on development builds; released CLIs default to their own stable tag.
  -o, --output Target directory (default: ./<project-name>).
  -f, --force  Allow writing into a non-empty directory and overwrite generated files.
  --extras     Also emit optional release helpers such as Makefile, .goreleaser.yml, portable skills/, and stable Python/Node bundle-release workflow scaffolding where supported.
  --claude-extended-hooks
               For --platform claude, scaffold the full runtime-supported hook set instead of the stable default subset.`

func newInitCmd(runner initCommandRunner) *cobra.Command {
	flags := initFlagState{}
	cmd := &cobra.Command{
		Use:   "init [project-name]",
		Short: "Create a plugin-kit-ai package scaffold",
		Long:  initLongDescription,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, runner, flags, args)
		},
	}
	cmd.Flags().StringVar(&flags.platform, "platform", "codex-runtime", `target lane ("codex-runtime", "codex-package", "claude", "gemini", "opencode", or "cursor")`)
	cmd.Flags().StringVar(&flags.runtime, "runtime", "go", `runtime ("go", "python", "node", or "shell")`)
	cmd.Flags().BoolVar(&flags.typescript, "typescript", false, "generate a TypeScript scaffold on top of the node runtime lane")
	cmd.Flags().BoolVar(&flags.runtimePackage, "runtime-package", false, "for --runtime python or --runtime node, import the shared plugin-kit-ai-runtime package instead of vendoring the helper file")
	cmd.Flags().StringVar(&flags.runtimePackageVersion, "runtime-package-version", "", "pin the generated plugin-kit-ai-runtime dependency version")
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "output directory (default: ./<project-name>)")
	cmd.Flags().BoolVarP(&flags.force, "force", "f", false, "overwrite generated files; allow non-empty output directory")
	cmd.Flags().BoolVar(&flags.extras, "extras", false, "include optional scaffold files (runtime-dependent extras plus skills and commands)")
	cmd.Flags().BoolVar(&flags.claudeExtendedHooks, "claude-extended-hooks", false, "for --platform claude, scaffold the full runtime-supported hook set instead of the stable default subset")
	return cmd
}

func runInit(cmd *cobra.Command, runner initCommandRunner, flags initFlagState, args []string) error {
	name := strings.TrimSpace(args[0])
	runtime := flags.runtime
	if (strings.EqualFold(strings.TrimSpace(flags.platform), "gemini") ||
		strings.EqualFold(strings.TrimSpace(flags.platform), "codex-package") ||
		strings.EqualFold(strings.TrimSpace(flags.platform), "opencode") ||
		strings.EqualFold(strings.TrimSpace(flags.platform), "cursor")) &&
		!cmd.Flags().Changed("runtime") {
		runtime = ""
	}
	opts := app.InitOptions{
		ProjectName:           name,
		Platform:              flags.platform,
		Runtime:               runtime,
		TypeScript:            flags.typescript,
		RuntimePackage:        flags.runtimePackage,
		RuntimePackageVersion: resolveRuntimePackageVersion(flags.runtimePackage, flags.runtimePackageVersion),
		OutputDir:             flags.output,
		Force:                 flags.force,
		Extras:                flags.extras,
		ClaudeExtendedHooks:   flags.claudeExtendedHooks,
	}
	out, err := runner.Run(opts)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), formatInitSuccess(out, opts))
	return nil
}

func formatInitSuccess(outDir string, opts app.InitOptions) string {
	platform := strings.TrimSpace(opts.Platform)
	if platform == "" {
		platform = "codex-runtime"
	}
	runtime := strings.TrimSpace(opts.Runtime)
	if runtime == "" {
		runtime = "go"
	}

	lines := []string{
		fmt.Sprintf("Created plugin %q at %s", opts.ProjectName, outDir),
		"Next:",
		fmt.Sprintf("  cd %s", strconv.Quote(outDir)),
	}

	if platform == "gemini" && strings.TrimSpace(opts.Runtime) == "go" {
		if opts.Extras {
			lines = append(lines, "  Portable MCP starter: mcp/servers.yaml")
		}
		lines = append(lines,
			"  go test ./...",
			"  plugin-kit-ai render .",
			"  plugin-kit-ai render --check .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  gemini extensions link .",
			"  See README.md for Gemini beta runtime steps",
		)
		return strings.Join(lines, "\n") + "\n"
	}

	if platform == "gemini" || platform == "codex-package" || platform == "opencode" || platform == "cursor" {
		if opts.Extras {
			lines = append(lines, "  Portable MCP starter: mcp/servers.yaml")
		}
		lines = append(lines,
			"  plugin-kit-ai render .",
			"  plugin-kit-ai render --check .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See README.md for the full first run",
		)
		return strings.Join(lines, "\n") + "\n"
	}

	switch runtime {
	case "python":
		if opts.RuntimePackage && strings.TrimSpace(opts.RuntimePackageVersion) != "" {
			lines = append(lines, fmt.Sprintf("  Shared helper dependency: plugin-kit-ai-runtime@%s", opts.RuntimePackageVersion))
		}
		lines = append(lines,
			"  plugin-kit-ai doctor .",
			"  plugin-kit-ai bootstrap .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			fmt.Sprintf("  %s", initTestCommand(platform)),
			fmt.Sprintf("  %s", initDevCommand(platform)),
			"  See README.md for the full first run",
		)
	case "node":
		if opts.RuntimePackage && strings.TrimSpace(opts.RuntimePackageVersion) != "" {
			lines = append(lines, fmt.Sprintf("  Shared helper dependency: plugin-kit-ai-runtime@%s", opts.RuntimePackageVersion))
		}
		lines = append(lines,
			"  plugin-kit-ai doctor .",
			"  plugin-kit-ai bootstrap .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			fmt.Sprintf("  %s", initTestCommand(platform)),
			fmt.Sprintf("  %s", initDevCommand(platform)),
			"  See README.md for the full first run",
		)
	case "shell":
		lines = append(lines,
			"  plugin-kit-ai doctor .",
			"  plugin-kit-ai bootstrap .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			fmt.Sprintf("  %s", initTestCommand(platform)),
			fmt.Sprintf("  %s", initDevCommand(platform)),
			"  See README.md for the full first run",
		)
	default:
		lines = append(lines,
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			fmt.Sprintf("  %s", initTestCommand(platform)),
			fmt.Sprintf("  %s", initDevCommand(platform)),
			"  See README.md for SDK setup and first-run steps",
		)
	}

	return strings.Join(lines, "\n") + "\n"
}

func initTestCommand(platform string) string {
	switch strings.TrimSpace(platform) {
	case "claude":
		return "plugin-kit-ai test . --platform claude --all"
	case "codex-runtime":
		return "plugin-kit-ai test . --platform codex-runtime --event Notify"
	default:
		return "plugin-kit-ai test ."
	}
}

func initDevCommand(platform string) string {
	switch strings.TrimSpace(platform) {
	case "claude":
		return "plugin-kit-ai dev . --platform claude --event Stop"
	case "codex-runtime":
		return "plugin-kit-ai dev . --platform codex-runtime --event Notify"
	default:
		return "plugin-kit-ai dev ."
	}
}

func resolveRuntimePackageVersion(enabled bool, explicit string) string {
	if !enabled {
		return strings.TrimSpace(explicit)
	}
	if version := normalizeStableRuntimePackageVersion(explicit); version != "" {
		return version
	}
	if version := normalizeStableRuntimePackageVersion(version); version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if version := normalizeStableRuntimePackageVersion(bi.Main.Version); version != "" {
			return version
		}
	}
	return strings.TrimSpace(explicit)
}

func normalizeStableRuntimePackageVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "(devel)" || version == "devel" {
		return ""
	}
	if !stableRuntimePackageVersionRe.MatchString(version) {
		return ""
	}
	return strings.TrimPrefix(version, "v")
}
