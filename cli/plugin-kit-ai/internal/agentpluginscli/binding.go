package agentpluginscli

import (
	"context"
	"fmt"
	"io"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

func newRebindCommand(app App, opts *options) *cobra.Command {
	return newBindingChangeCommand(app, opts, usecase.BindingChangeRebind)
}

func newMigrateFormatCommand(app App, opts *options) *cobra.Command {
	return newBindingChangeCommand(app, opts, usecase.BindingChangeMigrateFormat)
}

func newBindingChangeCommand(app App, opts *options, mode usecase.BindingChangeMode) *cobra.Command {
	commandName := "rebind"
	short := "Review and change the source bound to a removed Agent Plugin"
	if mode == usecase.BindingChangeMigrateFormat {
		commandName = "migrate-format"
		short = "Review and migrate a removed legacy binding to Agent Plugins 1.0"
	}
	return &cobra.Command{
		Use:   commandName + " <name-or-installation-id> <new-source>",
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			return runBindingChange(cmd.Context(), cmd, app, opts, mode, args[0], args[1])
		},
	}
}

func runBindingChange(
	ctx context.Context,
	cmd *cobra.Command,
	app App,
	opts *options,
	mode usecase.BindingChangeMode,
	selector, source string,
) error {
	commandName := bindingCommandName(mode)
	if !opts.dryRun && !app.Terminal && !opts.yes {
		return fmt.Errorf("non-interactive %s requires --yes", commandName)
	}
	writeProgress(app, opts.format, "Resolving and validating the proposed Agent Plugin binding...")
	loaded, err := app.loadPackage(ctx, source)
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	service := usecase.Service{
		StateStore: app.StateStore,
		Lock:       app.MutationLock,
		Kernel:     transaction.Kernel{StateStore: app.StateStore, Directory: app.Directory},
	}
	input := usecase.BindingChangeInput{Selector: selector, Envelope: loaded.envelope}
	planned, err := executeBindingChange(ctx, service, mode, input)
	if err != nil {
		return err
	}
	if opts.dryRun || planned.NoChange {
		return renderBindingChange(cmd.OutOrStdout(), opts.format, commandName, planned, opts.dryRun)
	}
	if !planned.Plan.CanApply {
		if renderErr := renderBindingChange(cmd.OutOrStdout(), opts.format, commandName, planned, false); renderErr != nil {
			return renderErr
		}
		return fmt.Errorf("binding change is blocked; remove all listed targets first")
	}
	if opts.format == "human" {
		renderHumanBindingPlan(cmd.OutOrStdout(), planned.Plan)
	}
	confirmed := opts.yes
	if !confirmed && opts.format == "human" && app.Terminal {
		confirmed, err = promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Apply this binding change? [y/N]")
		if err != nil {
			return err
		}
	}
	if !confirmed {
		if opts.format == "json" {
			return renderBindingChange(cmd.OutOrStdout(), opts.format, commandName, planned, false)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
		return nil
	}
	writeProgress(app, opts.format, "Committing the reviewed binding change...")
	input.Confirmed = true
	result, changeErr := executeBindingChange(ctx, service, mode, input)
	if renderErr := renderBindingChange(cmd.OutOrStdout(), opts.format, commandName, result, false); renderErr != nil && changeErr == nil {
		changeErr = renderErr
	}
	return changeErr
}

func executeBindingChange(ctx context.Context, service usecase.Service, mode usecase.BindingChangeMode, input usecase.BindingChangeInput) (usecase.BindingChangeResult, error) {
	if mode == usecase.BindingChangeMigrateFormat {
		return service.MigrateFormat(ctx, input)
	}
	return service.Rebind(ctx, input)
}

func bindingCommandName(mode usecase.BindingChangeMode) string {
	if mode == usecase.BindingChangeMigrateFormat {
		return "migrate-format"
	}
	return "rebind"
}

func renderBindingChange(writer io.Writer, format, commandName string, result usecase.BindingChangeResult, dryRun bool) error {
	data := struct {
		DryRun bool                        `json:"dry_run"`
		Result usecase.BindingChangeResult `json:"result"`
	}{DryRun: dryRun, Result: result}
	if format == "json" {
		return writeJSONOutput(writer, commandName, data)
	}
	if dryRun || !result.Plan.CanApply {
		renderHumanBindingPlan(writer, result.Plan)
	}
	if !result.Plan.CanApply {
		_, _ = fmt.Fprintln(writer, "Blocked. Remove every listed target and native object first.")
		return nil
	}
	if result.NoChange {
		_, _ = fmt.Fprintln(writer, "Binding already matches. No changes made.")
		return nil
	}
	if result.Mutated {
		_, _ = fmt.Fprintln(writer, "Binding updated. Reinstall targets explicitly with agentplugins add.")
	}
	return nil
}

func renderHumanBindingPlan(writer io.Writer, plan usecase.BindingChangePlan) {
	_, _ = fmt.Fprintf(writer, "Plugin: %s -> %s\n", plan.OldName, plan.NewName)
	_, _ = fmt.Fprintf(writer, "Format: %s -> %s\n", plan.OldFormat.FormatID, plan.NewFormat.FormatID)
	_, _ = fmt.Fprintf(writer, "Source: %s -> %s\n", provenanceLabel(plan.OldSource), provenanceLabel(plan.NewSource))
	_, _ = fmt.Fprintln(writer, "PLUGIN_DATA: not transferred")
	for _, target := range plan.Targets {
		_, _ = fmt.Fprintf(writer, "  %s/%s: %s\n", target.ClientID, target.Scope, target.Decision)
	}
	for _, blocker := range plan.Blockers {
		_, _ = fmt.Fprintf(writer, "  Blocker: %s\n", blocker)
	}
}

func provenanceLabel(source usecase.ProvenanceSummary) string {
	if source.Repository != "" {
		value := source.Repository
		if source.PackageSubpath != "" {
			value += "//" + source.PackageSubpath
		}
		return value
	}
	return source.Kind
}
