package main

import (
	"fmt"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

var initRunner app.InitRunner

var (
	initPlatform string
	initRuntime  string
	initOutput   string
	initForce    bool
	initExtras   bool
)

var initCmd = &cobra.Command{
	Use:   "init [project-name]",
	Short: "Create a plugin-kit-ai project scaffold for Codex or Claude",
	Long: `Creates a platform-specific plugin-kit-ai project scaffold.

Public flags:
  --platform   Supported: "codex" (default) and "claude".
  --runtime    Supported: "go" (default), "python", "node", "shell".
  -o, --output Target directory (default: ./<project-name>).
  -f, --force  Allow writing into a non-empty directory and overwrite generated files.
  --extras     Also emit Makefile, .goreleaser.yml, skills/, commands/ (stretch scaffold).`,
	Args: cobra.ExactArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initPlatform, "platform", "codex", `host CLI ("codex" or "claude")`)
	initCmd.Flags().StringVar(&initRuntime, "runtime", "go", `runtime ("go", "python", "node", or "shell")`)
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "", "output directory (default: ./<project-name>)")
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "overwrite generated files; allow non-empty output directory")
	initCmd.Flags().BoolVar(&initExtras, "extras", false, "include optional scaffold files (runtime-dependent extras plus skills and commands)")
}

func runInit(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	out, err := initRunner.Run(app.InitOptions{
		ProjectName: name,
		Platform:    initPlatform,
		Runtime:     initRuntime,
		OutputDir:   initOutput,
		Force:       initForce,
		Extras:      initExtras,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created plugin %q at %s\n", name, out)
	return nil
}
