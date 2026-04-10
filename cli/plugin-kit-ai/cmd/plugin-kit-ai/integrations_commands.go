package main

import (
	"context"

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
