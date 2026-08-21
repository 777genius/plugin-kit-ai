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

type switchOutput struct {
	DryRun         bool                         `json:"dry_run"`
	Source         string                       `json:"source"`
	Revision       string                       `json:"revision,omitempty"`
	TreeDigest     string                       `json:"tree_digest"`
	ManifestDigest string                       `json:"manifest_digest"`
	Directory      *domain.DirectoryOrigin      `json:"directory,omitempty"`
	Group          *usecase.GroupResult         `json:"group,omitempty"`
	Retained       *usecase.BindingChangeResult `json:"retained,omitempty"`
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
			if isShortName(strings.TrimSpace(destination)) {
				return fmt.Errorf("switch --to requires a qualified distribution ID or exact local/full-SHA source; a short name could select a changing default")
			}
			return runSwitch(cmd.Context(), cmd, app, opts, args[0], destination)
		},
	}
	command.Flags().StringVar(&destination, "to", "", "qualified distribution ID or exact immutable/local source")
	_ = command.MarkFlagRequired("to")
	return command
}

func runSwitch(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector, source string) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	targetIDs := installationTargets(installation, string(domain.ScopeUser))
	loaded, err := app.loadPackageFor(ctx, source, packageResolutionRequest{Targets: targetIDs, Operation: domain.DirectoryInstall})
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	if loaded.envelope.Manifest.Name != installation.DeclaredName {
		return fmt.Errorf("switch source manifest name %q does not match installed product %q", loaded.envelope.Manifest.Name, installation.DeclaredName)
	}
	output := switchOutput{DryRun: true, Source: publicPackageSource(loaded.envelope.Source), Revision: loaded.envelope.Source.ResolvedRevision,
		TreeDigest: loaded.envelope.TreeDigest, ManifestDigest: loaded.envelope.ManifestDigest, Directory: cloneDirectoryOrigin(loaded.directory)}
	service := app.Lifecycle
	service.StateStore = app.StateStore
	if installation.DataRetained && len(installation.Clients) == 0 {
		planned, err := service.SwitchRetained(ctx, usecase.BindingChangeInput{Selector: selector, Envelope: loaded.envelope}, loaded.origin, loaded.directory)
		output.Retained = &planned
		if err != nil {
			return err
		}
		if opts.dryRun {
			return renderSwitchResult(cmd.OutOrStdout(), opts.format, output)
		}
		if opts.format == "human" {
			if err := renderSwitchResult(cmd.OutOrStdout(), opts.format, output); err != nil {
				return err
			}
		}
		applied, err := service.SwitchRetained(ctx, usecase.BindingChangeInput{Selector: selector, Envelope: loaded.envelope, Confirmed: true}, loaded.origin, loaded.directory)
		output.DryRun, output.Retained = false, &applied
		if renderErr := renderSwitchResult(cmd.OutOrStdout(), opts.format, output); renderErr != nil && err == nil {
			err = renderErr
		}
		return err
	}
	clients, err := app.Detector.Detect(ctx)
	if err != nil {
		return err
	}
	detected := make(map[domain.ClientID]domain.DetectedClient, len(clients))
	for _, client := range clients {
		detected[client.ClientID] = client
	}
	service = lifecycleService(app, detected)
	inputs := make([]usecase.AddInput, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		client, ok := detected[targetID]
		if !ok || client.Status != domain.DetectionDetected {
			return fmt.Errorf("switch target %s is not detected; no target was changed", targetID)
		}
		clientPackage := cloneLoadedPackage(loaded)
		if err := prepareLoadedPackageForClient(&clientPackage, targetID); err != nil {
			return err
		}
		inputs = append(inputs, usecase.AddInput{Envelope: clientPackage.envelope, Client: client, Scope: domain.ScopeUser, Hints: clientPackage.hints,
			BackendExecutable: backendExecutable(client, detected), OriginMode: loaded.origin, DirectoryResolution: cloneDirectoryOrigin(loaded.directory),
			DistributionSuspended: loaded.distributionSuspended, ReleaseRevoked: loaded.releaseRevoked})
	}
	operationID, err := newOperationGroupID()
	if err != nil {
		return err
	}
	planned, err := service.SwitchGroup(ctx, usecase.GroupInput{Targets: inputs, OperationGroupID: operationID, DryRun: true, Switch: true})
	output.Group = &planned
	if err != nil {
		return fmt.Errorf("switch preflight failed; no target was changed: %w", err)
	}
	if opts.dryRun {
		return renderSwitchResult(cmd.OutOrStdout(), opts.format, output)
	}
	if opts.format == "human" {
		if err := renderSwitchResult(cmd.OutOrStdout(), opts.format, output); err != nil {
			return err
		}
	}
	applied, err := service.SwitchGroup(ctx, usecase.GroupInput{Targets: inputs, OperationGroupID: operationID, Confirmed: true, Switch: true})
	output.DryRun, output.Group = false, &applied
	if renderErr := renderSwitchResult(cmd.OutOrStdout(), opts.format, output); renderErr != nil && err == nil {
		err = renderErr
	}
	return err
}

func renderSwitchResult(writer io.Writer, format string, result switchOutput) error {
	if format == "json" {
		return writeJSONOutput(writer, "switch", result)
	}
	status := "planned"
	if !result.DryRun {
		status = "completed"
	}
	_, _ = fmt.Fprintf(writer, "Switch: %s\n", status)
	if result.Group != nil {
		for _, target := range result.Group.Targets {
			_, _ = fmt.Fprintf(writer, "  %s: %s\n", target.Plan.ClientID, target.Plan.Status)
		}
	}
	if result.Retained != nil {
		_, _ = fmt.Fprintln(writer, "  retained PLUGIN_DATA: preserved")
	}
	return nil
}

func runPurgeData(ctx context.Context, cmd *cobra.Command, app App, opts *options, installation domain.Installation) error {
	targets, err := parseTargetOption(opts.target)
	if err != nil {
		return err
	}
	service := app.Lifecycle
	service.StateStore = app.StateStore
	if installation.DataRetained && len(installation.Clients) == 0 {
		if len(targets) != 0 {
			return fmt.Errorf("a data_retained purge does not accept --target")
		}
		if err := service.PurgeRetainedData(ctx, installation.InstallationID, false); err != nil {
			return err
		}
		if opts.dryRun {
			return renderPurgeDataResult(cmd.OutOrStdout(), opts.format, installation, true)
		}
		if err := service.PurgeRetainedData(ctx, installation.InstallationID, true); err != nil {
			return err
		}
		return renderPurgeDataResult(cmd.OutOrStdout(), opts.format, installation, false)
	}
	if len(targets) == 0 {
		return fmt.Errorf("removing active bindings with --purge-data requires an explicit --target list")
	}
	return runRemoveMany(ctx, cmd, app, opts, installation.InstallationID, targets)
}

func renderPurgeDataResult(writer io.Writer, format string, installation domain.Installation, dryRun bool) error {
	data := struct {
		Plugin string `json:"plugin"`
		DryRun bool   `json:"dry_run"`
		Status string `json:"status"`
	}{Plugin: installation.DeclaredName, DryRun: dryRun, Status: map[bool]string{true: "planned", false: "purged"}[dryRun]}
	if format == "json" {
		return writeJSONOutput(writer, "remove", data)
	}
	_, err := fmt.Fprintf(writer, "Data purge: %s\n", data.Status)
	return err
}
