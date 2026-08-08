package agentpluginscli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

func newUpdateCommand(app App, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "update <name-or-installation-id>",
		Short: "Safely update one tracked Agent Plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			return runUpdate(cmd.Context(), cmd, app, opts, args[0])
		},
	}
}

func runUpdate(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector string) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	if installation.NeedsRebind {
		return fmt.Errorf("installation %s requires explicit rebind before update", installation.InstallationID)
	}
	if installation.Package.LoaderKind != domain.LoaderKindAgentPlugins {
		return fmt.Errorf("legacy plugin.yaml installations cannot switch format during update; use plugin-kit-ai integrations update or agentplugins migrate-format")
	}
	writeProgress(app, opts.format, "Resolving and validating the updated Agent Plugin...")
	loaded, err := app.loadPackage(ctx, updateSource(installation))
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	if domain.ComputeSourceBindingID(loaded.envelope.Source) != installation.Source.SourceBindingID {
		return fmt.Errorf("resolved source identity changed; use agentplugins rebind after reviewing provenance")
	}
	clients, err := app.Detector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("detect AI clients: %w", err)
	}
	selected, detectedMap, err := selectBoundClient(cmd, app, opts, installation, clients, true)
	if err != nil {
		return err
	}
	if err := requireNonInteractiveMutation(app, opts, "update"); err != nil && !opts.dryRun {
		return err
	}
	service := lifecycleService(app, detectedMap)
	input := usecase.AddInput{
		Envelope: loaded.envelope, Client: selected, Scope: domain.InstallScope(opts.scope),
		DryRun: opts.dryRun, Interactive: app.Terminal, Hints: loaded.hints,
		BackendExecutable: backendExecutable(selected, detectedMap),
	}
	planned, err := service.Update(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun || planned.NoChange {
		return renderUpdateResult(cmd.OutOrStdout(), opts.format, loaded.envelope, planned, opts.dryRun)
	}
	if opts.format == "human" {
		if err := renderHumanPlan(cmd.OutOrStdout(), loaded.envelope, planned); err != nil {
			return err
		}
	}
	confirmed := opts.yes
	if !confirmed && opts.format == "human" && app.Terminal {
		confirmed, err = promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Apply this update? [y/N]")
		if err != nil {
			return err
		}
	}
	if !confirmed {
		if opts.format == "json" {
			return renderUpdateResult(cmd.OutOrStdout(), opts.format, loaded.envelope, planned, false)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
		return nil
	}
	writeProgress(app, opts.format, "Applying transactional package update...")
	input.Confirmed = true
	result, updateErr := service.Update(ctx, input)
	if renderErr := renderUpdateResult(cmd.OutOrStdout(), opts.format, loaded.envelope, result, false); renderErr != nil && updateErr == nil {
		updateErr = renderErr
	}
	return updateErr
}

func newRemoveCommand(app App, opts *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "remove <name-or-installation-id>",
		Short: "Safely remove one tracked Agent Plugin target",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			return runRemove(cmd.Context(), cmd, app, opts, args[0])
		},
	}
	command.Flags().BoolVar(&opts.externalUninstalled, "external-uninstalled", false, "confirm the selected client plugin was uninstalled manually or was never activated/imported")
	return command
}

func runRemove(ctx context.Context, cmd *cobra.Command, app App, opts *options, selector string) error {
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	installation, err := selectInstallation(state, selector)
	if err != nil {
		return err
	}
	if installation.Package.LoaderKind == domain.LoaderKindLegacy {
		return runLegacyRemove(ctx, cmd, app, opts, installation)
	}
	clients, err := app.Detector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("detect AI clients: %w", err)
	}
	selected, detectedMap, err := selectBoundClient(cmd, app, opts, installation, clients, false)
	if err != nil {
		return err
	}
	if err := requireNonInteractiveMutation(app, opts, "removal"); err != nil && !opts.dryRun {
		return err
	}
	service := lifecycleService(app, detectedMap)
	input := usecase.RemoveInput{
		Selector: selector, Client: selected, Scope: domain.InstallScope(opts.scope),
		DryRun: opts.dryRun, Interactive: app.Terminal,
		ExternalUninstalled: opts.externalUninstalled,
		BackendExecutable:   backendExecutable(selected, detectedMap),
	}
	planned, err := service.Remove(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun {
		return renderRemoveResult(cmd.OutOrStdout(), opts.format, installation, planned, true)
	}
	if !planned.Deactivation.ArtifactRemovalAllowed {
		if opts.format == "human" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s %s\n", installation.DeclaredName, installation.Package.Version)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Target: %s\n", selected.ClientID)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Result: removal is blocked until the external client uninstall is confirmed")
		}
		return renderRemoveResult(cmd.OutOrStdout(), opts.format, installation, planned, false)
	}
	if opts.format == "human" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plugin: %s %s\n", installation.DeclaredName, installation.Package.Version)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Target: %s\n", selected.ClientID)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Result: managed package and its lifecycle binding will be removed")
	}
	confirmed := opts.yes
	if !confirmed && opts.format == "human" && app.Terminal {
		confirmed, err = promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Remove this target? [y/N]")
		if err != nil {
			return err
		}
	}
	if !confirmed {
		if opts.format == "json" {
			return renderRemoveResult(cmd.OutOrStdout(), opts.format, installation, planned, false)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
		return nil
	}
	writeProgress(app, opts.format, "Removing the selected managed package...")
	input.Confirmed = true
	result, removeErr := service.Remove(ctx, input)
	if renderErr := renderRemoveResult(cmd.OutOrStdout(), opts.format, installation, result, false); renderErr != nil && removeErr == nil {
		removeErr = renderErr
	}
	return removeErr
}

func runLegacyRemove(ctx context.Context, cmd *cobra.Command, app App, opts *options, installation domain.Installation) error {
	if app.LegacyLifecycle == nil {
		return fmt.Errorf("legacy lifecycle bridge is not configured")
	}
	if target := strings.TrimSpace(opts.target); target != "" && target != "legacy-all" {
		return fmt.Errorf("legacy removal is integration-wide; use --target legacy-all after reviewing every target")
	}
	if !opts.dryRun && !app.Terminal && (opts.target != "legacy-all" || !opts.yes) {
		return fmt.Errorf("non-interactive legacy removal requires --target legacy-all and --yes")
	}
	service := usecase.Service{
		StateStore: app.StateStore, Legacy: app.LegacyLifecycle, LegacyLock: app.LegacyStateLock, Lock: app.MutationLock,
		Kernel: transaction.Kernel{StateStore: app.StateStore, Directory: app.Directory},
	}
	input := usecase.LegacyRemoveInput{Selector: installation.InstallationID, DryRun: opts.dryRun}
	planned, err := service.RemoveLegacy(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun {
		return renderLegacyRemove(cmd.OutOrStdout(), opts.format, planned, true)
	}
	if opts.format == "human" {
		renderLegacyRemovePlan(cmd.OutOrStdout(), planned)
	}
	confirmed := opts.yes
	if !confirmed && opts.format == "human" && app.Terminal {
		confirmed, err = promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Remove every legacy target listed above? [y/N]")
		if err != nil {
			return err
		}
	}
	if !confirmed {
		if opts.format == "json" {
			return renderLegacyRemove(cmd.OutOrStdout(), opts.format, planned, false)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
		return nil
	}
	input.Confirmed = true
	result, removeErr := service.RemoveLegacy(ctx, input)
	if renderErr := renderLegacyRemove(cmd.OutOrStdout(), opts.format, result, false); renderErr != nil && removeErr == nil {
		removeErr = renderErr
	}
	return removeErr
}

func renderLegacyRemove(writer io.Writer, format string, result usecase.LegacyRemoveResult, dryRun bool) error {
	data := struct {
		DryRun bool                       `json:"dry_run"`
		Result usecase.LegacyRemoveResult `json:"result"`
	}{DryRun: dryRun, Result: result}
	if format == "json" {
		return writeJSONOutput(writer, "remove", data)
	}
	if dryRun {
		renderLegacyRemovePlan(writer, result)
		return nil
	}
	if result.Mutated {
		_, _ = fmt.Fprintln(writer, "Legacy targets removed and State v2 reconciled.")
	}
	return nil
}

func renderLegacyRemovePlan(writer io.Writer, result usecase.LegacyRemoveResult) {
	_, _ = fmt.Fprintf(writer, "Legacy plugin: %s\n", result.Plugin)
	_, _ = fmt.Fprintln(writer, "Removal is integration-wide through the original plugin-kit-ai lifecycle:")
	for _, target := range result.Targets {
		_, _ = fmt.Fprintf(writer, "  - %s\n", target)
	}
	if result.Reconciled {
		_, _ = fmt.Fprintln(writer, "Legacy lifecycle already reports the installation absent; only State v2 reconciliation remains.")
	}
}

func lifecycleService(app App, detected map[domain.ClientID]domain.DetectedClient) usecase.Service {
	planner := clientplanner.Planner{ManagedRoot: app.ManagedRoot, Detected: detected}
	return usecase.Service{
		StateStore: app.StateStore, Planner: planner, Targets: planner,
		Stager: app.Stager, Activator: app.Activator,
		Lock:   app.MutationLock,
		Kernel: transaction.Kernel{StateStore: app.StateStore, Directory: app.Directory},
	}
}

func updateSource(installation domain.Installation) string {
	if installation.Source.Repository == "" && filepath.IsAbs(installation.Source.CanonicalSource) {
		return installation.Source.CanonicalSource
	}
	if value := strings.TrimSpace(installation.Source.RequestedSource); value != "" {
		return value
	}
	if installation.Source.Repository != "" {
		value := "github:" + installation.Source.Repository
		if installation.Source.PackageSubpath != "" {
			value += "//" + installation.Source.PackageSubpath
		}
		return value
	}
	return installation.Source.CanonicalSource
}

func requireNonInteractiveMutation(app App, opts *options, action string) error {
	if app.Terminal || (strings.TrimSpace(opts.target) != "" && opts.yes) {
		return nil
	}
	return fmt.Errorf("non-interactive %s requires both --target and --yes", action)
}

func selectBoundClient(
	cmd *cobra.Command,
	app App,
	opts *options,
	installation domain.Installation,
	clients []domain.DetectedClient,
	requireDetected bool,
) (domain.DetectedClient, map[domain.ClientID]domain.DetectedClient, error) {
	detectedMap := make(map[domain.ClientID]domain.DetectedClient, len(clients))
	for _, client := range clients {
		detectedMap[client.ClientID] = client
	}
	bound := make(map[domain.ClientID]struct{})
	for _, binding := range installation.Clients {
		if binding.Materialization != domain.MaterializationAbsent && binding.Scope == opts.scope {
			bound[domain.ClientID(binding.ClientID)] = struct{}{}
		}
	}
	if len(bound) == 0 {
		return domain.DetectedClient{}, detectedMap, fmt.Errorf("plugin has no materialized target in %s scope", opts.scope)
	}
	choose := func(clientID domain.ClientID) (domain.DetectedClient, error) {
		if _, ok := bound[clientID]; !ok {
			return domain.DetectedClient{}, fmt.Errorf("plugin is not installed for target %q in %s scope", clientID, opts.scope)
		}
		client, ok := detectedMap[clientID]
		if !ok {
			client = domain.DetectedClient{ClientID: clientID, DisplayName: string(clientID), Status: domain.DetectionNotDetected}
		}
		if requireDetected && client.Status != domain.DetectionDetected {
			return domain.DetectedClient{}, fmt.Errorf("target %q is no longer detected; remove remains available", clientID)
		}
		return client, nil
	}
	if target := normalizeTarget(opts.target); target != "" {
		client, err := choose(target)
		return client, detectedMap, err
	}
	ids := make([]domain.ClientID, 0, len(bound))
	for clientID := range bound {
		ids = append(ids, clientID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 1 {
		client, err := choose(ids[0])
		return client, detectedMap, err
	}
	if !app.Terminal || opts.yes || opts.format == "json" {
		return domain.DetectedClient{}, detectedMap, fmt.Errorf("plugin has multiple installed targets; choose exactly one with --target")
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Installed targets:")
	for index, clientID := range ids {
		client := detectedMap[clientID]
		display := client.DisplayName
		if display == "" {
			display = string(clientID)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", index+1, display)
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "Choose one target: ")
	line, readErr := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if readErr != nil && readErr != io.EOF {
		return domain.DetectedClient{}, detectedMap, readErr
	}
	choice, parseErr := strconv.Atoi(strings.TrimSpace(line))
	if parseErr != nil || choice < 1 || choice > len(ids) {
		return domain.DetectedClient{}, detectedMap, fmt.Errorf("invalid client selection")
	}
	client, err := choose(ids[choice-1])
	return client, detectedMap, err
}

func renderUpdateResult(writer io.Writer, format string, envelope domain.PackageEnvelope, result usecase.AddResult, dryRun bool) error {
	data := struct {
		Plugin  string            `json:"plugin"`
		Version string            `json:"version,omitempty"`
		DryRun  bool              `json:"dry_run"`
		Result  usecase.AddResult `json:"result"`
	}{Plugin: envelope.Manifest.Name, Version: envelope.Manifest.Version, DryRun: dryRun, Result: result}
	if format == "json" {
		return writeJSONOutput(writer, "update", data)
	}
	if dryRun {
		return renderHumanPlan(writer, envelope, result)
	}
	if result.NoChange {
		_, _ = fmt.Fprintln(writer, "Already up to date. No changes made.")
		return nil
	}
	if result.Mutated && fullyInstalled(result.Activation) {
		_, _ = fmt.Fprintln(writer, "Updated and verified for the selected client.")
		return nil
	}
	if result.Mutated {
		if result.Activation.Authentication == domain.AuthenticationPending {
			if result.Activation.Activation == domain.ActivationActive {
				_, _ = fmt.Fprintln(writer, "Package updated and client activation completed. Authentication is pending.")
			} else {
				_, _ = fmt.Fprintln(writer, "Package updated. Authentication and client activation are pending.")
			}
		} else {
			_, _ = fmt.Fprintln(writer, "Package updated. Activation is not complete yet.")
		}
		for _, action := range result.Activation.UserActions {
			_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
		}
		for _, action := range result.Activation.LocalActions {
			_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
		}
	}
	return nil
}

func renderRemoveResult(writer io.Writer, format string, installation domain.Installation, result usecase.RemoveResult, dryRun bool) error {
	data := struct {
		Plugin  string               `json:"plugin"`
		Version string               `json:"version,omitempty"`
		DryRun  bool                 `json:"dry_run"`
		Result  usecase.RemoveResult `json:"result"`
	}{Plugin: installation.DeclaredName, Version: installation.Package.Version, DryRun: dryRun, Result: result}
	if format == "json" {
		return writeJSONOutput(writer, "remove", data)
	}
	if dryRun {
		if result.Deactivation.ArtifactRemovalAllowed {
			_, _ = fmt.Fprintf(writer, "Would remove %s from %s. No changes made.\n", installation.DeclaredName, result.ClientID)
		} else {
			_, _ = fmt.Fprintf(writer, "Would not remove %s from %s until the external client uninstall is confirmed. No changes made.\n", installation.DeclaredName, result.ClientID)
		}
		for _, action := range result.Deactivation.UserActions {
			_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
		}
		for _, action := range result.Deactivation.LocalActions {
			_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
		}
		return nil
	}
	if result.Mutated {
		_, _ = fmt.Fprintln(writer, "Removed the selected managed package.")
	}
	for _, action := range result.Deactivation.UserActions {
		_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
	}
	for _, action := range result.Deactivation.LocalActions {
		_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
	}
	return nil
}
