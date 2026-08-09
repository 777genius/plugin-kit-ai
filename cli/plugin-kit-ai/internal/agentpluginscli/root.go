package agentpluginscli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func NewRoot(app App) *cobra.Command {
	opts := &options{}
	root := &cobra.Command{
		Use:           "agentplugins",
		Short:         "Install and manage Agent Plugins 1.0 packages across AI clients",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(app.input())
	root.SetOut(app.output())
	root.SetErr(app.errorOutput())
	flags := root.PersistentFlags()
	flags.StringVar(&opts.target, "target", "", "target client: codex, chatgpt, cursor, copilot, vscode, or kiro")
	flags.StringVar(&opts.scope, "scope", "user", "installation scope: user or project")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "show the exact plan without changes")
	flags.BoolVar(&opts.yes, "yes", false, "confirm this selected target without installing everywhere")
	_ = flags.MarkHidden("yes")
	flags.StringVar(&opts.format, "format", "human", "output format: human or json")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable color output")

	root.AddCommand(newAddCommand(app, opts))
	root.AddCommand(newUpdateCommand(app, opts))
	root.AddCommand(newRepairCommand(app, opts))
	root.AddCommand(newRemoveCommand(app, opts))
	root.AddCommand(newMigrateFormatCommand(app, opts))
	root.AddCommand(newMigrateStateCommand(app, opts))
	root.AddCommand(newRebindCommand(app, opts))
	root.AddCommand(newListCommand(app, opts))
	root.AddCommand(newInfoCommand(app, opts))
	root.AddCommand(newDoctorCommand(app, opts))
	root.AddCommand(newVersionCommand(app, opts))
	return root
}

func validateCommonOptions(opts *options) error {
	if opts.format != "human" && opts.format != "json" {
		return fmt.Errorf("--format must be human or json")
	}
	if opts.scope != "user" && opts.scope != "project" {
		return fmt.Errorf("--scope must be user or project")
	}
	if strings.EqualFold(strings.TrimSpace(opts.target), "all") {
		return fmt.Errorf("--target all is not supported; choose clients explicitly")
	}
	return nil
}
