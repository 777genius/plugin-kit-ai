package main

import (
	"context"
	"fmt"
	"os/signal"
	"strings"
	"syscall"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/install/integrationctl"
	"github.com/spf13/cobra"
)

var integrationsRunner = app.NewIntegrationsRunner(nil)

var (
	integrationTargets         []string
	integrationScope           string
	integrationAutoUpdate      bool
	integrationAdoptNewTargets string
	integrationAllowPre        bool
	integrationDryRun          bool
	integrationUpdateAll       bool
)

var integrationsCmd = &cobra.Command{
	Use:   "integrations",
	Short: "Foundation lifecycle commands for multi-agent integration management",
}

func runIntegrationsList(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		report, err := integrationsRunner.Controller.List(ctx)
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, report)
		return nil
}

var integrationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed integrations from local state",
	RunE:  runIntegrationsList,
}

func runIntegrationsDoctor(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		report, err := integrationsRunner.Controller.Doctor(ctx)
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, report)
		return nil
}

var integrationsDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect integration state and open lifecycle journals",
	RunE:  runIntegrationsDoctor,
}

func runIntegrationsAdd(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		result, err := integrationsRunner.Controller.Add(ctx, integrationctl.AddParams{
			Source:          args[0],
			Targets:         integrationctl.NormalizeTargets(integrationTargets),
			Scope:           strings.TrimSpace(integrationScope),
			AutoUpdate:      boolPtr(integrationAutoUpdate),
			AdoptNewTargets: strings.TrimSpace(integrationAdoptNewTargets),
			AllowPrerelease: boolPtr(integrationAllowPre),
			DryRun:          integrationDryRun,
		})
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, result.Report)
		return nil
}

func newIntegrationsAddCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE:  runIntegrationsAdd,
	}
	cmd.Flags().StringSliceVar(&integrationTargets, "target", nil, "limit planning to one or more targets")
	cmd.Flags().StringVar(&integrationScope, "scope", "user", "scope intent for the planned installation")
	cmd.Flags().BoolVar(&integrationAutoUpdate, "auto-update", true, "desired auto-update policy")
	cmd.Flags().StringVar(&integrationAdoptNewTargets, "adopt-new-targets", "manual", "policy for newly supported targets: manual or auto")
	cmd.Flags().BoolVar(&integrationAllowPre, "pre", false, "allow prerelease updates")
	cmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")
	return cmd
}

var integrationsAddCmd = newIntegrationsAddCommand(
	"add <source>",
	"Plan installation of an integration across supported agent targets",
)

func validateIntegrationsUpdateArgs(cmd *cobra.Command, args []string) error {
	if integrationUpdateAll {
		if len(args) != 0 {
			return fmt.Errorf("update --all does not accept a name")
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("update requires exactly one integration name unless --all is set")
	}
	return nil
}

func runIntegrationsUpdate(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		name := ""
		if len(args) == 1 {
			name = args[0]
		}
		result, err := integrationsRunner.Controller.Update(ctx, integrationctl.UpdateParams{
			Name:   name,
			All:    integrationUpdateAll,
			DryRun: integrationDryRun,
		})
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, result.Report)
		return nil
}

func newIntegrationsUpdateCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  validateIntegrationsUpdateArgs,
		RunE:  runIntegrationsUpdate,
	}
	cmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")
	cmd.Flags().BoolVar(&integrationUpdateAll, "all", false, "update all managed integrations")
	return cmd
}

var integrationsUpdateCmd = newIntegrationsUpdateCommand(
	"update [name]",
	"Plan or apply an update for a managed integration",
)

func runIntegrationsSync(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	result, err := integrationsRunner.Controller.Sync(ctx, integrationctl.SyncParams{
		DryRun: integrationDryRun,
	})
	if err != nil {
		return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
	}
	printIntegrationReport(cmd, result.Report)
	return nil
}

var integrationsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reconcile workspace desired integrations from .plugin-kit-ai.lock",
	RunE:  runIntegrationsSync,
}

func runIntegrationsRemove(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		result, err := integrationsRunner.Controller.Remove(ctx, integrationctl.RemoveParams{
			Name:   args[0],
			DryRun: integrationDryRun,
		})
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, result.Report)
		return nil
}

func newIntegrationsRemoveCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE:  runIntegrationsRemove,
	}
	cmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")
	return cmd
}

var integrationsRemoveCmd = newIntegrationsRemoveCommand(
	"remove <name>",
	"Plan or remove managed integration targets",
)

func runIntegrationsRepair(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		result, err := integrationsRunner.Controller.Repair(ctx, integrationctl.RepairParams{
			Name:   args[0],
			Target: firstNormalizedTarget(integrationTargets),
			DryRun: integrationDryRun,
		})
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, result.Report)
		return nil
}

func newIntegrationsRepairCommand(use, short string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE:  runIntegrationsRepair,
	}
	cmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")
	cmd.Flags().StringSliceVar(&integrationTargets, "target", nil, "limit repair to one target")
	return cmd
}

var integrationsRepairCmd = newIntegrationsRepairCommand(
	"repair <name>",
	"Plan or repair managed integration drift",
)

func runIntegrationsEnable(cmd *cobra.Command, args []string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	result, err := integrationsRunner.Controller.Enable(ctx, integrationctl.ToggleParams{
		Name:   args[0],
		Target: firstNormalizedTarget(integrationTargets),
		DryRun: integrationDryRun,
	})
	if err != nil {
		return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
	}
	printIntegrationReport(cmd, result.Report)
	return nil
}

var integrationsEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a managed integration target where the native agent supports toggling",
	Args:  cobra.ExactArgs(1),
	RunE:  runIntegrationsEnable,
}

func runIntegrationsDisable(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
		defer stop()
		result, err := integrationsRunner.Controller.Disable(ctx, integrationctl.ToggleParams{
			Name:   args[0],
			Target: firstNormalizedTarget(integrationTargets),
			DryRun: integrationDryRun,
		})
		if err != nil {
			return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
		}
		printIntegrationReport(cmd, result.Report)
		return nil
}

var integrationsDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a managed integration target where the native agent supports toggling",
	Args:  cobra.ExactArgs(1),
	RunE:  runIntegrationsDisable,
}

func init() {
	integrationsEnableCmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")
	integrationsEnableCmd.Flags().StringSliceVar(&integrationTargets, "target", nil, "limit enable to one target")
	integrationsDisableCmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")
	integrationsDisableCmd.Flags().StringSliceVar(&integrationTargets, "target", nil, "limit disable to one target")
	integrationsSyncCmd.Flags().BoolVar(&integrationDryRun, "dry-run", true, "plan only without mutating native targets")

	integrationsCmd.AddCommand(integrationsListCmd)
	integrationsCmd.AddCommand(integrationsDoctorCmd)
	integrationsCmd.AddCommand(integrationsAddCmd)
	integrationsCmd.AddCommand(integrationsUpdateCmd)
	integrationsCmd.AddCommand(integrationsRemoveCmd)
	integrationsCmd.AddCommand(integrationsRepairCmd)
	integrationsCmd.AddCommand(integrationsEnableCmd)
	integrationsCmd.AddCommand(integrationsDisableCmd)
	integrationsCmd.AddCommand(integrationsSyncCmd)

	rootCmd.AddCommand(newIntegrationsAddCommand(
		"add <source>",
		"Short alias for integrations add",
	))
	rootCmd.AddCommand(newIntegrationsUpdateCommand(
		"update [name]",
		"Short alias for integrations update",
	))
	rootCmd.AddCommand(newIntegrationsRemoveCommand(
		"remove <name>",
		"Short alias for integrations remove",
	))
	rootCmd.AddCommand(newIntegrationsRepairCommand(
		"repair <name>",
		"Short alias for integrations repair",
	))
}

func printIntegrationReport(cmd *cobra.Command, report integrationctl.Report) {
	if strings.TrimSpace(report.OperationID) != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Operation: %s\n", report.OperationID)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), report.Summary)
	for _, warning := range report.Warnings {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning)
	}
	for _, target := range report.Targets {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "- %s: action=%s delivery=%s state=%s", target.TargetID, target.ActionClass, target.DeliveryKind, target.State)
		if target.ActivationState != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), " activation=%s", target.ActivationState)
		}
		if target.EvidenceKey != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), " evidence=%s", target.EvidenceKey)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		for _, step := range target.ManualSteps {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  next - %s\n", step)
		}
		for _, restriction := range target.EnvironmentRestrictions {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  restriction - %s\n", restriction)
		}
	}
}

func boolPtr(v bool) *bool { return &v }

func firstNormalizedTarget(values []string) string {
	normalized := integrationctl.NormalizeTargets(values)
	if len(normalized) == 0 {
		return ""
	}
	return normalized[0]
}
