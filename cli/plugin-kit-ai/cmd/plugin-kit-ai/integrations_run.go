package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/install/integrationctl"
	"github.com/spf13/cobra"
)

func newIntegrationSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), syscall.SIGTERM)
}

func runIntegrationReportAction(cmd *cobra.Command, action func(context.Context) (integrationctl.Report, error)) error {
	ctx, stop := newIntegrationSignalContext()
	defer stop()
	report, err := action(ctx)
	if err != nil {
		return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
	}
	printIntegrationReport(cmd, report)
	return nil
}

func runIntegrationResultAction(cmd *cobra.Command, action func(context.Context) (integrationctl.Result, error)) error {
	ctx, stop := newIntegrationSignalContext()
	defer stop()
	result, err := action(ctx)
	if err != nil {
		return exitx.Wrap(err, integrationctl.ExitCodeFromErr(err))
	}
	printIntegrationReport(cmd, result.Report)
	return nil
}

func executeIntegrationsAdd(cmd *cobra.Command, params integrationctl.AddParams) error {
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Add(ctx, params)
	})
}

func executeIntegrationsUpdate(cmd *cobra.Command, params integrationctl.UpdateParams) error {
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Update(ctx, params)
	})
}

func executeIntegrationsRemove(cmd *cobra.Command, params integrationctl.RemoveParams) error {
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Remove(ctx, params)
	})
}

func executeIntegrationsRepair(cmd *cobra.Command, params integrationctl.RepairParams) error {
	return runIntegrationResultAction(cmd, func(ctx context.Context) (integrationctl.Result, error) {
		return integrationsRunner.Controller.Repair(ctx, params)
	})
}
