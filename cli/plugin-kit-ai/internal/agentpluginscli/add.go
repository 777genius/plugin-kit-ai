package agentpluginscli

import (
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
	var activationComplete, authComplete bool
	command := &cobra.Command{
		Use:   "add <name-or-source>",
		Short: "Plan and install one Agent Plugins 1.0 package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			return runAdd(cmd.Context(), cmd, app, opts, args[0], activationComplete, authComplete)
		},
	}
	command.Flags().BoolVar(&activationComplete, "activation-complete", false, "attest that manual client activation is complete")
	command.Flags().BoolVar(&authComplete, "auth-complete", false, "attest that required authentication is complete or none is required after review")
	return command
}

func runAdd(ctx context.Context, cmd *cobra.Command, app App, opts *options, source string, activationComplete, authComplete bool) error {
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
		ActivationComplete: activationComplete, AuthComplete: authComplete,
		PersistAuthoritativeObservations: true,
	}
	planned, err := service.Add(ctx, input)
	if err != nil {
		return err
	}
	if opts.dryRun || planned.NoChange {
		return renderAddResult(cmd.OutOrStdout(), opts.format, loaded.envelope, planned, opts.dryRun)
	}
	if opts.format == "human" {
		if err := renderHumanPlan(cmd.OutOrStdout(), loaded.envelope, planned); err != nil {
			return err
		}
	}
	freshInstall := planned.Activation.Activation == ""
	if !freshInstall && opts.format == "human" && app.Terminal && !opts.yes && !activationComplete && !authComplete {
		if err := renderAddResult(cmd.OutOrStdout(), opts.format, loaded.envelope, planned, false); err != nil {
			return err
		}
		return resumeInteractiveLifecycle(ctx, cmd, service, input, loaded.envelope, planned)
	}
	confirmed := opts.yes
	if !confirmed && opts.format == "human" && app.Terminal {
		prompt := "Apply this plan? [y/N]"
		if !freshInstall {
			prompt = "Apply these explicit lifecycle attestations? [y/N]"
		}
		confirmed, err = promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), prompt)
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
	if err == nil && freshInstall && opts.format == "human" && app.Terminal && !opts.yes {
		return resumeInteractiveLifecycle(ctx, cmd, service, input, loaded.envelope, result)
	}
	return err
}

func resumeInteractiveLifecycle(
	ctx context.Context,
	cmd *cobra.Command,
	service usecase.Service,
	input usecase.AddInput,
	envelope domain.PackageEnvelope,
	current usecase.AddResult,
) error {
	if current.Activation.Activation != domain.ActivationActive || current.Activation.Verification != domain.VerificationInstalled {
		if cliClientVerifierAvailable(input, current.Plan) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Client verification must return recognized positive evidence. Inspect the plugin manually, update the client if its output format is unsupported, then rerun add.")
			return nil
		}
		complete, err := promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Have you completed activation and verified the plugin is enabled in the client? [y/N]")
		if err != nil {
			return err
		}
		if !complete {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Activation remains unconfirmed. Rerun add when it is complete.")
			return nil
		}
		resume := input
		resume.Confirmed = true
		resume.InstallationID = current.InstallationID
		resume.ActivationComplete = true
		resume.AuthComplete = false
		current, err = service.Add(ctx, resume)
		if err != nil {
			return err
		}
	}

	if current.Activation.Authentication == domain.AuthenticationPending || current.Activation.Authentication == domain.AuthenticationNotChecked {
		complete, err := promptYesNo(cmd.InOrStdin(), cmd.OutOrStdout(), "Have you completed required authentication, or reviewed the package and confirmed none is required? [y/N]")
		if err != nil {
			return err
		}
		if !complete {
			if current.Activation.ActivationAttested {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Activation is user-attested. Authentication remains unconfirmed; rerun add after completing or reviewing it.")
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Authentication remains unconfirmed. Rerun add after completing or reviewing it.")
			}
			return nil
		}
		resume := input
		resume.Confirmed = true
		resume.InstallationID = current.InstallationID
		resume.ActivationComplete = false
		resume.AuthComplete = true
		current, err = service.Add(ctx, resume)
		if err != nil {
			return err
		}
	}

	return renderAddResult(cmd.OutOrStdout(), "human", envelope, current, false)
}

func cliClientVerifierAvailable(input usecase.AddInput, plan domain.DeliveryPlan) bool {
	if strings.TrimSpace(input.BackendExecutable) == "" {
		return false
	}
	switch input.Client.ClientID {
	case domain.ClientCodex, domain.ClientCopilot, domain.ClientVSCode:
		return true
	case domain.ClientKiro:
		if !strings.Contains(strings.ToLower(input.BackendExecutable), "kiro") || len(plan.Components) == 0 {
			return false
		}
		for _, component := range plan.Components {
			if component.Support == domain.SupportUnsupported || component.Kind != domain.ComponentMCPServer {
				return false
			}
		}
		return true
	default:
		return false
	}
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
	line, err := readInputLine(cmd.InOrStdin())
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
	case "openai", "codex":
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
	line, err := readInputLine(reader)
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

func readInputLine(reader io.Reader) (string, error) {
	var line strings.Builder
	var buffer [1]byte
	for {
		read, err := reader.Read(buffer[:])
		if read > 0 {
			if buffer[0] == '\n' {
				return line.String(), nil
			}
			line.WriteByte(buffer[0])
		}
		if err != nil {
			return line.String(), err
		}
	}
}

func renderHumanPlan(writer io.Writer, envelope domain.PackageEnvelope, result usecase.AddResult) error {
	_, _ = fmt.Fprintf(writer, "Plugin: %s %s\n", envelope.Manifest.Name, envelope.Manifest.Version)
	_, _ = fmt.Fprintf(writer, "Target: %s\n", result.Plan.ClientID)
	_, _ = fmt.Fprintf(writer, "Package: %s\n", result.Plan.PackageMode)
	_, _ = fmt.Fprintf(writer, "Result: %s\n", result.Plan.Status)
	_, _ = fmt.Fprintf(writer, "Authentication: %s\n", result.Plan.Authentication)
	_, _ = fmt.Fprintf(writer, "Verification: %s\n", result.Plan.Verification)
	for _, component := range result.Plan.Components {
		_, _ = fmt.Fprintf(writer, "  - %s %s: %s\n", component.Kind, component.Name, component.Support)
	}
	for _, diagnostic := range result.Plan.Diagnostics {
		_, _ = fmt.Fprintf(writer, "  Warning: %s: %s\n", diagnostic.Code, diagnostic.Message)
	}
	for _, warning := range result.Plan.Warnings {
		_, _ = fmt.Fprintf(writer, "  Warning: %s\n", warning)
	}
	for _, action := range result.Plan.UserActions {
		_, _ = fmt.Fprintf(writer, "  Planned action: %s\n", action)
	}
	for _, action := range result.Plan.LocalActions {
		_, _ = fmt.Fprintf(writer, "  Planned action: %s\n", action)
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
	if result.NoChange {
		_, _ = fmt.Fprintln(writer, "Already installed and lifecycle verification is complete. No changes made.")
		return nil
	}
	if result.Mutated && fullyInstalled(result.Activation) {
		if result.Activation.ActivationAttested || result.Activation.AuthenticationAttested {
			_, _ = fmt.Fprintln(writer, "Lifecycle is user-attested for the explicitly confirmed phase; it was not observed from the client.")
		} else {
			_, _ = fmt.Fprintln(writer, "Installed and verified for the selected client.")
		}
		return nil
	}
	if result.Mutated {
		if result.Activation.Authentication == domain.AuthenticationPending {
			if result.Activation.Activation == domain.ActivationActive {
				_, _ = fmt.Fprintln(writer, "Package materialized and client activation completed. Authentication is pending.")
			} else {
				_, _ = fmt.Fprintln(writer, "Package prepared. Authentication and client activation are pending.")
			}
		} else if result.Activation.Authentication == domain.AuthenticationNotChecked && result.Activation.Activation == domain.ActivationActive {
			_, _ = fmt.Fprintln(writer, "Package materialized and client activation verified. Authentication requirements have not been checked.")
		} else {
			_, _ = fmt.Fprintln(writer, "Package prepared. Activation is not complete yet.")
		}
	}
	if action := nextLifecycleAction(result); action != "" && !fullyInstalled(result.Activation) {
		_, _ = fmt.Fprintf(writer, "Next: %s\n", action)
	}
	return nil
}

func nextLifecycleAction(result usecase.AddResult) string {
	action := ""
	if len(result.Activation.LocalActions) > 0 {
		action = result.Activation.LocalActions[0]
	} else if len(result.Activation.UserActions) > 0 {
		action = result.Activation.UserActions[0]
	}
	if result.Activation.Authentication == domain.AuthenticationPending {
		if action != "" {
			return fmt.Sprintf("%s; complete authentication, then rerun add to verify activation and authentication", action)
		}
		return fmt.Sprintf("complete authentication for the prepared package at %s, then rerun add to verify activation and authentication", result.Plan.ActivePath)
	}
	if result.Activation.Authentication == domain.AuthenticationNotChecked {
		if action != "" {
			return action + "; verify this plugin's authentication requirements before using it"
		}
		return "verify this plugin's authentication requirements before using it"
	}
	if action != "" {
		return action
	}
	return "start a new client session and verify the plugin is available"
}

func fullyInstalled(outcome domain.ActivationOutcome) bool {
	authComplete := outcome.Authentication == domain.AuthenticationNotRequired || outcome.Authentication == domain.AuthenticationComplete
	return outcome.Activation == domain.ActivationActive && outcome.Verification == domain.VerificationInstalled && authComplete
}
