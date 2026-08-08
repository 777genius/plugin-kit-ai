package agentpluginscli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/transaction"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/usecase"
	"github.com/spf13/cobra"
)

func newAddCommand(app App, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name-or-source>",
		Short: "Plan and install one Agent Plugins 1.0 package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			return runAdd(cmd.Context(), cmd, app, opts, args[0])
		},
	}
}

func runAdd(ctx context.Context, cmd *cobra.Command, app App, opts *options, source string) error {
	writeProgress(app, opts.format, "Resolving and validating Agent Plugin...")
	loaded, err := app.loadPackage(ctx, source)
	if err != nil {
		return err
	}
	if loaded.cleanup != nil {
		defer loaded.cleanup()
	}
	clients, err := app.Detector.Detect(ctx)
	if err != nil {
		return fmt.Errorf("detect AI clients: %w", err)
	}
	selected, detectedMap, err := selectClient(cmd, app, opts, clients)
	if err != nil {
		return err
	}
	if !opts.dryRun && !app.Terminal && (strings.TrimSpace(opts.target) == "" || !opts.yes) {
		return fmt.Errorf("non-interactive installation requires both --target and --yes")
	}
	planner := clientplanner.Planner{ManagedRoot: app.ManagedRoot, Detected: detectedMap}
	service := usecase.Service{
		StateStore: app.StateStore,
		Planner:    planner,
		Targets:    planner,
		Stager:     app.Stager,
		Activator:  app.Activator,
		Lock:       app.MutationLock,
		Kernel:     transaction.Kernel{StateStore: app.StateStore, Directory: app.Directory},
	}
	input := usecase.AddInput{
		Envelope: loaded.envelope, Client: selected, Scope: domain.InstallScope(opts.scope),
		DryRun: opts.dryRun, Confirmed: false, Interactive: app.Terminal,
		Hints: loaded.hints, BackendExecutable: backendExecutable(selected, detectedMap),
	}
	planned, err := service.Add(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun {
		return renderAddResult(cmd.OutOrStdout(), opts.format, loaded.envelope, planned, true)
	}
	if opts.format == "human" {
		if err := renderHumanPlan(cmd.OutOrStdout(), loaded.envelope, planned); err != nil {
			return err
		}
	}
	confirmed := opts.yes
	if !confirmed && opts.format == "human" && app.Terminal {
		confirmed, err = promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Apply this plan? [y/N]")
		if err != nil {
			return err
		}
	}
	if !confirmed {
		if opts.format == "json" {
			return renderAddResult(cmd.OutOrStdout(), opts.format, loaded.envelope, planned, false)
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No changes made.")
		return nil
	}
	writeProgress(app, opts.format, "Applying transactional client package...")
	input.Confirmed = true
	input.InstallationID = planned.InstallationID
	result, err := service.Add(ctx, input)
	if renderErr := renderAddResult(cmd.OutOrStdout(), opts.format, loaded.envelope, result, false); renderErr != nil && err == nil {
		err = renderErr
	}
	return err
}

func selectClient(
	cmd *cobra.Command,
	app App,
	opts *options,
	clients []domain.DetectedClient,
) (domain.DetectedClient, map[domain.ClientID]domain.DetectedClient, error) {
	detectedMap := make(map[domain.ClientID]domain.DetectedClient, len(clients))
	var detected []domain.DetectedClient
	for _, client := range clients {
		detectedMap[client.ClientID] = client
		if client.Status == domain.DetectionDetected {
			detected = append(detected, client)
		}
	}
	target := normalizeTarget(opts.target)
	if target != "" {
		client, ok := detectedMap[target]
		if !ok || client.Status != domain.DetectionDetected {
			return domain.DetectedClient{}, detectedMap, fmt.Errorf("target %q was not detected", opts.target)
		}
		return client, detectedMap, nil
	}
	if len(detected) == 0 {
		return domain.DetectedClient{}, detectedMap, fmt.Errorf("no supported AI client was detected; use --target after installing a client")
	}
	if len(detected) == 1 {
		return detected[0], detectedMap, nil
	}
	if !app.Terminal || opts.yes || opts.format == "json" {
		return domain.DetectedClient{}, detectedMap, fmt.Errorf("multiple clients detected; choose exactly one with --target")
	}
	sort.Slice(detected, func(i, j int) bool { return detected[i].DisplayName < detected[j].DisplayName })
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Detected clients:")
	for index, client := range detected {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", index+1, client.DisplayName)
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), "Choose one target: ")
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && err != io.EOF {
		return domain.DetectedClient{}, detectedMap, err
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > len(detected) {
		return domain.DetectedClient{}, detectedMap, fmt.Errorf("invalid client selection")
	}
	return detected[choice-1], detectedMap, nil
}

func normalizeTarget(value string) domain.ClientID {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "chatgpt", "openai", "codex":
		return domain.ClientCodex
	case "cursor":
		return domain.ClientCursor
	case "copilot", "github-copilot":
		return domain.ClientCopilot
	case "vscode", "vs-code":
		return domain.ClientVSCode
	case "kiro":
		return domain.ClientKiro
	default:
		return domain.ClientID(strings.ToLower(strings.TrimSpace(value)))
	}
}

func backendExecutable(selected domain.DetectedClient, clients map[domain.ClientID]domain.DetectedClient) string {
	if selected.ClientID == domain.ClientVSCode {
		return clients[domain.ClientCopilot].ExecutablePath
	}
	return selected.ExecutablePath
}

func promptYesNo(reader io.Reader, writer io.Writer, prompt string) (bool, error) {
	_, _ = fmt.Fprint(writer, prompt+" ")
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func renderHumanPlan(writer io.Writer, envelope domain.PackageEnvelope, result usecase.AddResult) error {
	_, _ = fmt.Fprintf(writer, "Plugin: %s %s\n", envelope.Manifest.Name, envelope.Manifest.Version)
	_, _ = fmt.Fprintf(writer, "Target: %s\n", result.Plan.ClientID)
	_, _ = fmt.Fprintf(writer, "Package: %s\n", result.Plan.PackageMode)
	_, _ = fmt.Fprintf(writer, "Result: %s\n", result.Plan.Status)
	for _, component := range result.Plan.Components {
		_, _ = fmt.Fprintf(writer, "  - %s %s: %s\n", component.Kind, component.Name, component.Support)
	}
	for _, action := range result.Plan.UserActions {
		_, _ = fmt.Fprintf(writer, "  Next: %s\n", action)
	}
	return nil
}

func renderAddResult(writer io.Writer, format string, envelope domain.PackageEnvelope, result usecase.AddResult, dryRun bool) error {
	data := struct {
		Plugin  string            `json:"plugin"`
		Version string            `json:"version,omitempty"`
		DryRun  bool              `json:"dry_run"`
		Result  usecase.AddResult `json:"result"`
	}{Plugin: envelope.Manifest.Name, Version: envelope.Manifest.Version, DryRun: dryRun, Result: result}
	if format == "json" {
		return writeJSONOutput(writer, "add", data)
	}
	if dryRun {
		return renderHumanPlan(writer, envelope, result)
	}
	if result.Mutated && fullyInstalled(result.Activation) {
		_, _ = fmt.Fprintln(writer, "Installed and verified for the selected client.")
		return nil
	}
	if result.Mutated {
		if result.Activation.Authentication == domain.AuthenticationPending {
			if result.Activation.Activation == domain.ActivationActive {
				_, _ = fmt.Fprintln(writer, "Package materialized and client activation completed. Authentication is pending.")
			} else {
				_, _ = fmt.Fprintln(writer, "Package prepared. Authentication and client activation are pending.")
			}
		} else {
			_, _ = fmt.Fprintln(writer, "Package prepared. Activation is not complete yet.")
		}
		for _, action := range result.Activation.UserActions {
			_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
		}
	}
	return nil
}

func fullyInstalled(outcome domain.ActivationOutcome) bool {
	authComplete := outcome.Authentication == domain.AuthenticationNotRequired || outcome.Authentication == domain.AuthenticationComplete
	return outcome.Activation == domain.ActivationActive && outcome.Verification == domain.VerificationInstalled && authComplete
}
