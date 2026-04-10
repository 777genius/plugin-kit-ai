package main

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
	"github.com/spf13/cobra"
)

type initCommandRunner interface {
	Run(app.InitOptions) (string, error)
}

type initFlagState struct {
	template              string
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

Start with the job you want to solve:

Connect an online service:
  Use --template online-service for hosted integrations like Notion, Stripe, Cloudflare, or Vercel.
  This starter creates an MCP-first repo with shared authored source under src/ and no launcher code.

Connect a local tool:
  Use --template local-tool for local MCP-backed tools like Docker Hub, Chrome DevTools, or HubSpot Developer.
  This starter creates an MCP-first repo with local command wiring under src/ and no launcher code.

Build custom plugin logic - Advanced:
  Use --template custom-logic when you need launcher-backed code, hooks, or your own runtime behavior.
  This path is more powerful and more engineering-heavy than the first two starters.
  Plain init remains as a legacy compatibility path for the older codex-runtime plus Go starter.

Already have native config:
  Use plugin-kit-ai import to bring current Claude/Codex/Gemini/OpenCode/Cursor native files into the package-standard authored layout.
  init is for creating a new package-standard project, not for preserving native files as the authored source of truth.

Public flags:
  --template   Recommended start: "online-service", "local-tool", or "custom-logic".
  --platform   Advanced override: "codex-runtime" (default), "codex-package", "claude", "gemini", "opencode", "cursor", or "cursor-workspace".
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
	cmd.Flags().StringVar(&flags.template, "template", "", `recommended start ("online-service", "local-tool", or "custom-logic")`)
	cmd.Flags().StringVar(&flags.platform, "platform", "codex-runtime", `target lane ("codex-runtime", "codex-package", "claude", "gemini", "opencode", "cursor", or "cursor-workspace")`)
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
		strings.EqualFold(strings.TrimSpace(flags.platform), "cursor") ||
		strings.EqualFold(strings.TrimSpace(flags.platform), "cursor-workspace")) &&
		!cmd.Flags().Changed("runtime") {
		runtime = ""
	}
	opts := app.InitOptions{
		ProjectName:           name,
		Template:              flags.template,
		Platform:              flags.platform,
		PlatformExplicit:      cmd.Flags().Changed("platform"),
		Runtime:               runtime,
		RuntimeExplicit:       cmd.Flags().Changed("runtime"),
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
	templateName := strings.TrimSpace(opts.Template)
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

	if templateName == scaffold.InitTemplateOnlineService || templateName == scaffold.InitTemplateLocalTool {
		lines = append(lines,
			"  plugin-kit-ai inspect . --authoring",
			"  plugin-kit-ai generate .",
			"  plugin-kit-ai generate --check .",
		)
		if opts.PlatformExplicit && strings.TrimSpace(opts.Platform) != "" {
			lines = append(lines, fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", strings.TrimSpace(opts.Platform)))
		} else {
			lines = append(lines, "  plugin-kit-ai validate . --platform claude --strict")
		}
		lines = append(lines, "  See src/README.md for the first run")
		return strings.Join(lines, "\n") + "\n"
	}

	if templateName == scaffold.InitTemplateCustomLogic {
		lines = append(lines,
			"  plugin-kit-ai inspect . --authoring",
		)
	}

	if platform == "gemini" && strings.TrimSpace(opts.Runtime) == "go" {
		if opts.Extras {
			lines = append(lines, "  Portable MCP starter: src/mcp/servers.yaml")
		}
		lines = append(lines,
			"  go test ./...",
			"  plugin-kit-ai generate .",
			"  plugin-kit-ai generate --check .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  plugin-kit-ai inspect . --target gemini",
			"  plugin-kit-ai capabilities --mode runtime --platform gemini",
			"  make test-gemini-runtime",
			"  gemini extensions link .",
			"  make test-gemini-runtime-live",
			"  See README.md for Gemini runtime steps",
		)
		return strings.Join(lines, "\n") + "\n"
	}

	if platform == "gemini" || platform == "codex-package" || platform == "opencode" || platform == "cursor" || platform == "cursor-workspace" {
		if opts.Extras {
			lines = append(lines, "  Portable MCP starter: src/mcp/servers.yaml")
		}
		lines = append(lines,
			"  plugin-kit-ai generate .",
			"  plugin-kit-ai generate --check .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See src/README.md for the full first run",
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
			"  See src/README.md for the full first run",
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
			"  See src/README.md for the full first run",
		)
	case "shell":
		lines = append(lines,
			"  plugin-kit-ai doctor .",
			"  plugin-kit-ai bootstrap .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			fmt.Sprintf("  %s", initTestCommand(platform)),
			fmt.Sprintf("  %s", initDevCommand(platform)),
			"  See src/README.md for the full first run",
		)
	default:
		lines = append(lines,
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			fmt.Sprintf("  %s", initTestCommand(platform)),
			fmt.Sprintf("  %s", initDevCommand(platform)),
			"  See src/README.md for SDK setup and first-run steps",
		)
	}

	if templateName == scaffold.InitTemplateCustomLogic {
		lines = append(lines, "  Advanced path: start with src/README.md, then grow into deeper runtime and hook details only when you need them.")
	} else if templateName == "" {
		lines = append(lines, "  Legacy compatibility path. For a new online service or local tool repo, start with --template online-service or --template local-tool instead.")
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
