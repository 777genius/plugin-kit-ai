package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

type initCommandRunner interface {
	Run(app.InitOptions) (string, error)
}

type initFlagState struct {
	platform            string
	runtime             string
	output              string
	force               bool
	extras              bool
	claudeExtendedHooks bool
}

var initRunner app.InitRunner

var initCmd = newInitCmd(initRunner)

const initLongDescription = `Creates a package-standard plugin-kit-ai project scaffold.

Choose the lane that matches your goal:

Fast local plugin:
  Use --runtime python or --runtime node when repo-local iteration matters more than packaged distribution.
  These are supported executable-runtime paths, not equal production paths.

Production-ready plugin repo:
  Plain init keeps the strongest supported path. --runtime go remains the default, and --platform codex remains the default target.
  Use --platform claude for Claude hooks, and add --claude-extended-hooks only when you intentionally want the wider runtime-supported subset.

Already have native config:
  Use plugin-kit-ai import to migrate current Claude/Codex/Gemini native files into the package-standard authored layout.
  init is for creating a new package-standard project, not for preserving native files as the authored source of truth.

Public flags:
  --platform   Supported: "codex" (default), "claude", and "gemini".
  --runtime    Supported: "go" (default), "python", "node", "shell".
  -o, --output Target directory (default: ./<project-name>).
  -f, --force  Allow writing into a non-empty directory and overwrite generated files.
  --extras     Also emit Makefile, .goreleaser.yml, and portable skills/ (stretch scaffold).
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
	cmd.Flags().StringVar(&flags.platform, "platform", "codex", `target CLI ("codex", "claude", or "gemini")`)
	cmd.Flags().StringVar(&flags.runtime, "runtime", "go", `runtime ("go", "python", "node", or "shell")`)
	cmd.Flags().StringVarP(&flags.output, "output", "o", "", "output directory (default: ./<project-name>)")
	cmd.Flags().BoolVarP(&flags.force, "force", "f", false, "overwrite generated files; allow non-empty output directory")
	cmd.Flags().BoolVar(&flags.extras, "extras", false, "include optional scaffold files (runtime-dependent extras plus skills and commands)")
	cmd.Flags().BoolVar(&flags.claudeExtendedHooks, "claude-extended-hooks", false, "for --platform claude, scaffold the full runtime-supported hook set instead of the stable default subset")
	return cmd
}

func runInit(cmd *cobra.Command, runner initCommandRunner, flags initFlagState, args []string) error {
	name := strings.TrimSpace(args[0])
	opts := app.InitOptions{
		ProjectName:         name,
		Platform:            flags.platform,
		Runtime:             flags.runtime,
		OutputDir:           flags.output,
		Force:               flags.force,
		Extras:              flags.extras,
		ClaudeExtendedHooks: flags.claudeExtendedHooks,
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
		platform = "codex"
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

	if platform == "gemini" {
		if runtime == "python" {
			lines = append(lines, "  Create a project .venv, then run:")
		} else if runtime == "node" {
			lines = append(lines, "  npm install")
		}
		lines = append(lines,
			"  plugin-kit-ai render .",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See README.md for the full first run",
		)
		return strings.Join(lines, "\n") + "\n"
	}

	switch runtime {
	case "python":
		lines = append(lines,
			"  Create a project .venv, then run:",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See README.md for the full first run",
		)
	case "node":
		lines = append(lines,
			"  npm install",
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See README.md for the full first run",
		)
	case "shell":
		lines = append(lines,
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See README.md for the full first run",
		)
	default:
		lines = append(lines,
			fmt.Sprintf("  plugin-kit-ai validate . --platform %s --strict", platform),
			"  See README.md for SDK setup and first-run steps",
		)
	}

	return strings.Join(lines, "\n") + "\n"
}
