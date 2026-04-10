package main

import (
	"context"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl"
	"github.com/spf13/cobra"
)

func runIntegrationsList(cmd *cobra.Command, args []string) error {
	return runIntegrationReportAction(cmd, integrationsRunner.Controller.List)
}

var integrationsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed integrations from local state",
	RunE:  runIntegrationsList,
}

func runIntegrationsDoctor(cmd *cobra.Command, args []string) error {
	return runIntegrationReportAction(cmd, integrationsRunner.Controller.Doctor)
}

var integrationsDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Inspect integration state and open lifecycle journals",
	RunE:  runIntegrationsDoctor,
}

func runIntegrationsAdd(cmd *cobra.Command, args []string) error {
	return executeIntegrationsAdd(cmd, integrationctl.AddParams{
		Source:          args[0],
		Targets:         integrationctl.NormalizeTargets(integrationTargets),
		Scope:           strings.TrimSpace(integrationScope),
		AutoUpdate:      boolPtr(integrationAutoUpdate),
		AdoptNewTargets: strings.TrimSpace(integrationAdoptNewTargets),
		AllowPrerelease: boolPtr(integrationAllowPre),
		DryRun:          integrationDryRun,
	})
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
	return validateUpdateArgs(integrationUpdateAll, args)
}

func runIntegrationsUpdate(cmd *cobra.Command, args []string) error {
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	return executeIntegrationsUpdate(cmd, integrationctl.UpdateParams{
		Name:   name,
		All:    integrationUpdateAll,
		DryRun: integrationDryRun,
	})
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
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Sync(ctx, integrationctl.SyncParams{DryRun: integrationDryRun})
	})
}

var integrationsSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Reconcile workspace desired integrations from .plugin-kit-ai.lock",
	RunE:  runIntegrationsSync,
}

func runIntegrationsRemove(cmd *cobra.Command, args []string) error {
	return executeIntegrationsRemove(cmd, integrationctl.RemoveParams{
		Name:   args[0],
		DryRun: integrationDryRun,
	})
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
	return executeIntegrationsRepair(cmd, integrationctl.RepairParams{
		Name:   args[0],
		Target: firstNormalizedTarget(integrationTargets),
		DryRun: integrationDryRun,
	})
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

func newRootAddCommand() *cobra.Command {
	var (
		targets         []string
		scope           string
		autoUpdate      bool
		adoptNewTargets string
		allowPre        bool
		dryRun          bool
	)

	cmd := &cobra.Command{
		Use:   "add <source>",
		Short: "Install an integration across supported agent targets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeIntegrationsAdd(cmd, integrationctl.AddParams{
				Source:          args[0],
				Targets:         integrationctl.NormalizeTargets(targets),
				Scope:           strings.TrimSpace(scope),
				AutoUpdate:      boolPtr(autoUpdate),
				AdoptNewTargets: strings.TrimSpace(adoptNewTargets),
				AllowPrerelease: boolPtr(allowPre),
				DryRun:          dryRun,
			})
		},
	}
	cmd.Flags().StringSliceVar(&targets, "target", nil, "limit installation to one or more targets")
	cmd.Flags().StringVar(&scope, "scope", "user", "scope intent for the installation")
	cmd.Flags().BoolVar(&autoUpdate, "auto-update", true, "desired auto-update policy")
	cmd.Flags().StringVar(&adoptNewTargets, "adopt-new-targets", "manual", "policy for newly supported targets: manual or auto")
	cmd.Flags().BoolVar(&allowPre, "pre", false, "allow prerelease updates")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan only without mutating native targets")
	return cmd
}

func newRootUpdateCommand() *cobra.Command {
	var (
		dryRun    bool
		updateAll bool
	)

	cmd := &cobra.Command{
		Use:   "update [name]",
		Short: "Update a managed integration",
		Args: func(cmd *cobra.Command, args []string) error {
			return validateUpdateArgs(updateAll, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return executeIntegrationsUpdate(cmd, integrationctl.UpdateParams{
				Name:   name,
				All:    updateAll,
				DryRun: dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan only without mutating native targets")
	cmd.Flags().BoolVar(&updateAll, "all", false, "update all managed integrations")
	return cmd
}

func newRootRemoveCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a managed integration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeIntegrationsRemove(cmd, integrationctl.RemoveParams{
				Name:   args[0],
				DryRun: dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan only without mutating native targets")
	return cmd
}

func newRootRepairCommand() *cobra.Command {
	var (
		targets []string
		dryRun  bool
	)

	cmd := &cobra.Command{
		Use:   "repair <name>",
		Short: "Repair managed integration drift",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return executeIntegrationsRepair(cmd, integrationctl.RepairParams{
				Name:   args[0],
				Target: firstNormalizedTarget(targets),
				DryRun: dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "plan only without mutating native targets")
	cmd.Flags().StringSliceVar(&targets, "target", nil, "limit repair to one target")
	return cmd
}

func runIntegrationsEnable(cmd *cobra.Command, args []string) error {
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Enable(ctx, integrationctl.ToggleParams{
			Name:   args[0],
			Target: firstNormalizedTarget(integrationTargets),
			DryRun: integrationDryRun,
		})
	})
}

var integrationsEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a managed integration target where the native agent supports toggling",
	Args:  cobra.ExactArgs(1),
	RunE:  runIntegrationsEnable,
}

func runIntegrationsDisable(cmd *cobra.Command, args []string) error {
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Disable(ctx, integrationctl.ToggleParams{
			Name:   args[0],
			Target: firstNormalizedTarget(integrationTargets),
			DryRun: integrationDryRun,
		})
	})
}

var integrationsDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a managed integration target where the native agent supports toggling",
	Args:  cobra.ExactArgs(1),
	RunE:  runIntegrationsDisable,
}
