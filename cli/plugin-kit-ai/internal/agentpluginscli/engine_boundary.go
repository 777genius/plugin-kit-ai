package agentpluginscli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

// GroupLifecycle is the narrow install-engine boundary required to turn the
// CLI's combined preflight into one durable operation group. Until the install
// engine supplies this API, the CLI can reuse one package and preflight every
// target, but the existing per-target use cases cannot provide group rollback.
type GroupLifecycle interface {
	AddGroup(context.Context, AddGroupInput) ([]usecase.AddResult, error)
	UpdateGroup(context.Context, AddGroupInput) ([]usecase.AddResult, error)
	RepairGroup(context.Context, AddGroupInput) ([]usecase.AddResult, error)
	RemoveGroup(context.Context, RemoveGroupInput) ([]usecase.RemoveResult, error)
}

type AddGroupInput struct {
	OperationID string
	Inputs      []usecase.AddInput
}

type RemoveGroupInput struct {
	OperationID string
	Inputs      []usecase.RemoveInput
}

// SourceSwitcher is the narrow install-engine boundary needed by the public
// switch command. The CLI owns selection, source acquisition, validation and
// output; the install domain owns its cross-binding transaction and recovery.
type SourceSwitcher interface {
	Switch(context.Context, SwitchInput) (SwitchResult, error)
}

type SwitchInput struct {
	Installation domain.Installation
	Package      domain.PackageEnvelope
	DryRun       bool
	Confirmed    bool
}

type SwitchResult struct {
	OperationID string               `json:"operation_id,omitempty"`
	Status      string               `json:"status"`
	Source      string               `json:"source"`
	Revision    string               `json:"revision,omitempty"`
	TreeDigest  string               `json:"tree_digest,omitempty"`
	Targets     []SwitchTargetResult `json:"targets"`
	NextActions []string             `json:"next_actions,omitempty"`
}

type SwitchTargetResult struct {
	Target     string   `json:"target"`
	Status     string   `json:"status"`
	NextAction string   `json:"next_action,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

// DataPurger is separate because permanent PLUGIN_DATA deletion requires an
// ownership-checked all-or-nothing preflight that the CLI must not emulate.
type DataPurger interface {
	PurgeData(context.Context, PurgeDataInput) (PurgeDataResult, error)
}

type PurgeDataInput struct {
	Installation domain.Installation
	Targets      []domain.ClientID
	DryRun       bool
	Confirmed    bool
}

type PurgeDataResult struct {
	OperationID string   `json:"operation_id,omitempty"`
	Status      string   `json:"status"`
	Targets     []string `json:"targets,omitempty"`
	Purged      []string `json:"purged_receipts,omitempty"`
	NextActions []string `json:"next_actions,omitempty"`
}

func newSwitchCommand(app App, opts *options) *cobra.Command {
	var destination string
	command := &cobra.Command{
		Use:   "switch <name-or-installation-id> --to <distribution-or-exact-source>",
		Short: "Move a complete Agent Plugin installation to a reviewed source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			if strings.TrimSpace(opts.target) != "" {
				return fmt.Errorf("switch always moves the complete installation; do not pass --target")
			}
			if strings.TrimSpace(destination) == "" {
				return fmt.Errorf("switch requires --to with a qualified distribution or exact source")
			}
			return runSwitch(cmd.Context(), cmd, app, opts, args[0], destination)
		},
	}
	command.Flags().StringVar(&destination, "to", "", "qualified distribution ID or exact immutable/local source")
	_ = command.MarkFlagRequired("to")
	return command
}

func runSwitch(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector, source string) error {
	if app.SourceSwitcher == nil {
		return fmt.Errorf("switch requires the install engine source-switch transaction API")
	}
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	loaded, err := app.loadPackage(ctx, source)
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	if loaded.envelope.Manifest.Name != installation.DeclaredName {
		return fmt.Errorf("switch source manifest name %q does not match installed product %q", loaded.envelope.Manifest.Name, installation.DeclaredName)
	}
	input := SwitchInput{Installation: installation, Package: loaded.envelope, DryRun: true, Confirmed: false}
	planned, err := app.SourceSwitcher.Switch(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun {
		return renderSwitchResult(cmd.OutOrStdout(), opts.format, planned)
	}
	if opts.format == "human" {
		if err := renderSwitchResult(cmd.OutOrStdout(), opts.format, planned); err != nil {
			return err
		}
	}
	input.DryRun = false
	input.Confirmed = true
	result, err := app.SourceSwitcher.Switch(ctx, input)
	if renderErr := renderSwitchResult(cmd.OutOrStdout(), opts.format, result); renderErr != nil && err == nil {
		err = renderErr
	}
	return err
}

func renderSwitchResult(writer io.Writer, format string, result SwitchResult) error {
	if format == "json" {
		return writeJSONOutput(writer, "switch", result)
	}
	_, _ = fmt.Fprintf(writer, "Switch: %s\n", result.Status)
	for _, target := range result.Targets {
		_, _ = fmt.Fprintf(writer, "  %s: %s\n", target.Target, target.Status)
		if target.NextAction != "" {
			_, _ = fmt.Fprintf(writer, "    Next: %s\n", target.NextAction)
		}
	}
	return nil
}

func runPurgeData(ctx context.Context, cmd *cobra.Command, app App, opts *options, installation domain.Installation) error {
	if app.DataPurger == nil {
		return fmt.Errorf("--purge-data requires the install engine ownership-checked data purge API")
	}
	targets, err := parseTargetOption(opts.target)
	if err != nil {
		return err
	}
	if len(targets) == 0 && hasMaterializedBindings(installation, opts.scope) {
		return fmt.Errorf("removing active bindings with --purge-data requires an explicit --target list")
	}
	input := PurgeDataInput{Installation: installation, Targets: targets, DryRun: true, Confirmed: false}
	planned, err := app.DataPurger.PurgeData(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun {
		return renderPurgeDataResult(cmd.OutOrStdout(), opts.format, planned)
	}
	if opts.format == "human" {
		if err := renderPurgeDataResult(cmd.OutOrStdout(), opts.format, planned); err != nil {
			return err
		}
	}
	input.DryRun = false
	input.Confirmed = true
	result, err := app.DataPurger.PurgeData(ctx, input)
	if renderErr := renderPurgeDataResult(cmd.OutOrStdout(), opts.format, result); renderErr != nil && err == nil {
		err = renderErr
	}
	return err
}

func hasMaterializedBindings(installation domain.Installation, scope string) bool {
	for _, binding := range installation.Clients {
		if binding.Scope == scope && binding.Materialization != domain.MaterializationAbsent {
			return true
		}
	}
	return false
}

func renderPurgeDataResult(writer io.Writer, format string, result PurgeDataResult) error {
	if format == "json" {
		return writeJSONOutput(writer, "remove", result)
	}
	_, err := fmt.Fprintf(writer, "Data purge: %s\n", result.Status)
	return err
}
