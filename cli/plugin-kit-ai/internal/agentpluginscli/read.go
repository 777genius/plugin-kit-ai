package agentpluginscli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/dirswap"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
	clientplanner "github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/planner"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/ports"
	"github.com/spf13/cobra"
)

type publicClient struct {
	ClientID        string                      `json:"client_id"`
	Scope           string                      `json:"scope"`
	Materialization domain.MaterializationState `json:"materialization"`
	Activation      domain.ActivationState      `json:"activation"`
	Authentication  domain.AuthenticationState  `json:"authentication"`
	Policy          domain.PolicyState          `json:"policy"`
	Verification    domain.VerificationState    `json:"verification"`
}

type publicDetectedClient struct {
	ClientID    domain.ClientID        `json:"client_id"`
	DisplayName string                 `json:"display_name"`
	Status      domain.DetectionStatus `json:"status"`
	Surfaces    []domain.ClientSurface `json:"surfaces,omitempty"`
}

type publicInstallation struct {
	InstallationID string         `json:"installation_id"`
	Name           string         `json:"name"`
	Version        string         `json:"version,omitempty"`
	Source         string         `json:"source"`
	NeedsRebind    bool           `json:"needs_rebind,omitempty"`
	Clients        []publicClient `json:"clients"`
}

type doctorFinding struct {
	Status           string `json:"status"`
	Code             string `json:"code"`
	Subject          string `json:"subject,omitempty"`
	InstallationName string `json:"installation_name,omitempty"`
	InstallationID   string `json:"installation_id,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	Message          string `json:"message"`
	RecoveryAction   string `json:"recovery_action,omitempty"`
}

type supportedClient struct {
	ClientID    domain.ClientID    `json:"client_id"`
	PackageMode domain.PackageMode `json:"package_mode"`
}

func newListCommand(app App, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tracked Agent Plugins installations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			state, err := app.StateStore.Load()
			if err != nil {
				return err
			}
			installations := publicInstallations(state, false)
			if opts.format == "json" {
				return writeJSONOutput(cmd.OutOrStdout(), "list", map[string]any{"installations": installations})
			}
			return renderInstallationList(cmd.OutOrStdout(), installations)
		},
	}
}

func newInfoCommand(app App, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name-or-installation-id>",
		Short: "Show lifecycle state without exposing local paths",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			state, err := app.StateStore.Load()
			if err != nil {
				return err
			}
			installation, err := selectInstallation(state, args[0])
			if err != nil {
				return err
			}
			public := publicInstallationView(installation, true)
			if opts.format == "json" {
				return writeJSONOutput(cmd.OutOrStdout(), "info", public)
			}
			return renderInstallation(cmd.OutOrStdout(), public)
		},
	}
}

func newDoctorCommand(app App, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor [name-or-installation-id]",
		Short: "Read-only health check for clients, state, and open operations",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			return runDoctor(cmd.Context(), cmd, app, opts, args)
		},
	}
}

func runDoctor(ctx context.Context, cmd *cobra.Command, app App, opts *options, args []string) error {
	clients, err := app.Detector.Detect(ctx)
	if err != nil {
		return err
	}
	state, err := app.StateStore.Load()
	if err != nil {
		return err
	}
	open, err := app.Directory.ListOpen()
	if err != nil {
		return err
	}
	publicClients := make([]publicDetectedClient, 0, len(clients))
	for _, client := range clients {
		publicClients = append(publicClients, publicDetectedClient{
			ClientID: client.ClientID, DisplayName: client.DisplayName, Status: client.Status,
			Surfaces: append([]domain.ClientSurface(nil), client.Surfaces...),
		})
	}
	report := struct {
		ReadOnly           bool                   `json:"read_only"`
		ToolVersion        string                 `json:"tool_version"`
		SupportedClients   []supportedClient      `json:"supported_clients"`
		Clients            []publicDetectedClient `json:"clients"`
		InstallationCount  int                    `json:"installation_count"`
		OpenOperationCount int                    `json:"open_operation_count"`
		Installation       *publicInstallation    `json:"installation,omitempty"`
		Findings           []doctorFinding        `json:"findings"`
	}{ReadOnly: true, ToolVersion: normalizedToolVersion(app.Version), SupportedClients: doctorSupportedClients(), Clients: publicClients,
		InstallationCount: len(publicInstallations(state, false)), OpenOperationCount: len(open)}
	selected := (*domain.Installation)(nil)
	if len(args) == 1 {
		installation, err := selectInstallation(state, args[0])
		if err != nil {
			return err
		}
		value := publicInstallationView(installation, true)
		report.Installation = &value
		selected = &installation
	}
	report.Findings = doctorFindings(ctx, app, clients, state, open, selected)
	if opts.format == "json" {
		return writeJSONOutput(cmd.OutOrStdout(), "doctor", report)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "agentplugins doctor (read-only)")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Tool version: %s\n", report.ToolVersion)
	for _, client := range clients {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", client.DisplayName, client.Status)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Tracked installations: %d\n", report.InstallationCount)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Open recovery operations: %d\n", report.OpenOperationCount)
	for _, finding := range report.Findings {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s: %s\n", finding.Status, finding.Code, finding.Message)
		if finding.InstallationID != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Installation: %s (%s)\n", finding.InstallationName, finding.InstallationID)
		}
		if finding.ClientID != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Client: %s\n", finding.ClientID)
		}
		if finding.RecoveryAction != "" {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    Recovery: %s\n", finding.RecoveryAction)
		}
	}
	if report.Installation != nil {
		return renderInstallation(cmd.OutOrStdout(), *report.Installation)
	}
	return nil
}

func normalizedToolVersion(version string) string {
	if version = strings.TrimSpace(version); version != "" {
		return version
	}
	return "development"
}

func doctorSupportedClients() []supportedClient {
	ids := []domain.ClientID{domain.ClientCodex, domain.ClientCursor, domain.ClientCopilot, domain.ClientVSCode, domain.ClientKiro}
	result := make([]supportedClient, 0, len(ids))
	for _, id := range ids {
		capabilities, _ := clientplanner.Capabilities(id)
		result = append(result, supportedClient{ClientID: id, PackageMode: capabilities.PackageMode})
	}
	return result
}

func doctorFindings(ctx context.Context, app App, detected []domain.DetectedClient, state domain.StateFileV2, open []dirswap.Receipt, selected *domain.Installation) []doctorFinding {
	var findings []doctorFinding
	detectedByID := make(map[string]domain.DetectedClient, len(detected))
	for _, client := range detected {
		detectedByID[string(client.ClientID)] = client
	}
	if len(open) > 0 {
		findings = append(findings, degradedFinding("open_recovery_operations", "operations", "an interrupted managed-directory operation requires recovery", "rerun the intended add, update, or remove command to perform transactional recovery"))
	}
	installations := state.Installations
	if selected != nil {
		installations = []domain.Installation{*selected}
	}
	for _, installation := range installations {
		if installation.NeedsRebind {
			findings = append(findings, scopedFinding("degraded", "source_rebind_required", installation, "", "the source identity needs an explicit rebind", "automatic mutation is intentionally blocked; recover the original source identity and review an explicit rebind against the recorded installation"))
		}
		for _, binding := range installation.Clients {
			if binding.Materialization == domain.MaterializationAbsent {
				continue
			}
			if _, supported := clientplanner.Capabilities(domain.ClientID(binding.ClientID)); !supported {
				findings = append(findings, scopedFinding("degraded", "unsupported_client_binding", installation, binding.ClientID, "the tracked binding names an unsupported client", blockedStateRecovery))
				continue
			}
			client, visible := detectedByID[binding.ClientID]
			if !visible || client.Status != domain.DetectionDetected {
				findings = append(findings, scopedFinding("degraded", "client_not_visible", installation, binding.ClientID, "the package is tracked but no current client visibility evidence was detected", "install or launch the client so its CLI, desktop application, or configuration directory is visible, then rerun doctor"))
			}
			if binding.ClientID == string(domain.ClientCopilot) && strings.TrimSpace(client.ExecutablePath) == "" {
				findings = append(findings, scopedFinding("degraded", "copilot_cli_missing", installation, binding.ClientID, "GitHub Copilot CLI is unavailable for automatic Copilot activation", "install GitHub Copilot CLI, ensure copilot is on PATH, and rerun doctor"))
			}
			switch binding.Activation {
			case domain.ActivationManual, domain.ActivationPrepared:
				findings = append(findings, scopedFinding("degraded", "activation_pending", installation, binding.ClientID, "client activation is not complete", fmt.Sprintf("complete the displayed external activation step, then rerun the same `agentplugins add ... --target %s%s` command", binding.ClientID, doctorScopeFlag(binding))))
			case domain.ActivationFailed:
				findings = append(findings, scopedFinding("degraded", "activation_failed", installation, binding.ClientID, "client activation failed", fmt.Sprintf("resolve the client-reported activation error, then rerun the same `agentplugins add ... --target %s%s` command", binding.ClientID, doctorScopeFlag(binding))))
			}
			switch binding.Authentication {
			case domain.AuthenticationPending:
				findings = append(findings, scopedFinding("degraded", "authentication_pending", installation, binding.ClientID, "plugin authentication is pending", fmt.Sprintf("complete the displayed external authentication step, then rerun the same `agentplugins add ... --target %s%s` command", binding.ClientID, doctorScopeFlag(binding))))
			case domain.AuthenticationFailed:
				findings = append(findings, scopedFinding("degraded", "authentication_failed", installation, binding.ClientID, "plugin authentication failed", fmt.Sprintf("reauthorize the plugin in the selected client, then rerun the same `agentplugins add ... --target %s%s` command", binding.ClientID, doctorScopeFlag(binding))))
			case domain.AuthenticationNotChecked:
				findings = append(findings, scopedFinding("unknown", "authentication_not_checked", installation, binding.ClientID, "authentication requirements have not been verified", "check the package's authentication instructions and verify access in the selected client"))
			}
			if binding.Materialization == domain.MaterializationDegraded || binding.Verification == domain.VerificationFailed {
				findings = append(findings, scopedFinding("degraded", "installation_verification_failed", installation, binding.ClientID, "the managed package is marked degraded or failed verification", repairAction(installation, binding)))
			}
			if visible && client.Status == domain.DetectionDetected {
				findings = append(findings, checkManagedIntegrity(ctx, app, client, installation, binding)...)
			}
		}
	}
	if len(findings) == 0 {
		findings = append(findings, doctorFinding{Status: "healthy", Code: "no_degradation_detected", Message: "no tracked degradation was detected"})
	}
	return findings
}

const blockedStateRecovery = "automatic mutation is intentionally blocked; restore state-v2.json from a trusted backup that matches the managed source, or recover and review the original source and binding metadata"

func checkManagedIntegrity(ctx context.Context, app App, client domain.DetectedClient, installation domain.Installation, binding domain.ClientBinding) []doctorFinding {
	if binding.Materialization != domain.MaterializationMaterialized || strings.TrimSpace(binding.TargetLocator) == "" {
		return nil
	}
	physicalID := strings.TrimSpace(binding.PhysicalArtifact)
	if physicalID == "" {
		return []doctorFinding{scopedFinding("degraded", "managed_target_unverifiable", installation, binding.ClientID, "the managed target has no physical artifact identity", blockedStateRecovery)}
	}
	target, err := (clientplanner.Planner{ManagedRoot: app.ManagedRoot}).ResolveTarget(ctx, client, domain.InstallScope(binding.Scope), physicalID)
	if err != nil || filepath.Clean(target.ActivePath) != filepath.Clean(binding.TargetLocator) {
		return []doctorFinding{scopedFinding("degraded", "managed_target_mismatch", installation, binding.ClientID, "the recorded managed target does not match the current safe client target", blockedStateRecovery)}
	}
	expected := ""
	sequence := 0
	for _, receipt := range binding.Receipts {
		if receipt.Sequence >= sequence && strings.TrimSpace(receipt.AfterDigest) != "" {
			sequence = receipt.Sequence
			expected = receipt.AfterDigest
		}
	}
	if expected == "" || app.Stager == nil {
		return []doctorFinding{scopedFinding("unknown", "managed_integrity_not_checked", installation, binding.ClientID, "managed-directory integrity evidence is unavailable", repairAction(installation, binding))}
	}
	if err := app.Stager.Verify(ctx, target.ActivePath, expected); err != nil {
		var verification *ports.VerificationError
		if errors.As(err, &verification) && verification.Kind == ports.VerificationExcludedMarker {
			return []doctorFinding{scopedFinding("unknown", "excluded_ownership_marker", installation, binding.ClientID, "the managed package contains an ownership marker excluded from digest verification", "manually review and remove the excluded ownership marker, then rerun doctor; automatic repair is intentionally blocked")}
		}
		if errors.As(err, &verification) && (verification.Kind == ports.VerificationAbsent || verification.Kind == ports.VerificationDigestMismatch) {
			return []doctorFinding{scopedFinding("degraded", "managed_directory_changed", installation, binding.ClientID, "the managed package directory is missing or differs from its recorded digest", repairAction(installation, binding))}
		}
		return []doctorFinding{scopedFinding("unknown", "managed_integrity_check_failed", installation, binding.ClientID, "managed-directory integrity could not be checked because verification infrastructure failed", "retry doctor after resolving the filesystem or temporary verification error")}
	}
	return nil
}

func scopedFinding(status, code string, installation domain.Installation, clientID, message, action string) doctorFinding {
	return doctorFinding{
		Status: status, Code: code, InstallationName: installation.DeclaredName,
		InstallationID: installation.InstallationID, ClientID: clientID,
		Message: message, RecoveryAction: action,
	}
}

func repairAction(installation domain.Installation, binding domain.ClientBinding) string {
	return fmt.Sprintf("run `agentplugins repair %s --target %s%s`", installation.InstallationID, binding.ClientID, doctorScopeFlag(binding))
}

func doctorScopeFlag(binding domain.ClientBinding) string {
	if binding.Scope == string(domain.ScopeProject) {
		return " --scope project"
	}
	return ""
}

func degradedFinding(code, subject, message, action string) doctorFinding {
	return doctorFinding{Status: "degraded", Code: code, Subject: subject, Message: message, RecoveryAction: action}
}

func newVersionCommand(app App, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the agentplugins binary version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateCommonOptions(opts); err != nil {
				return err
			}
			version := strings.TrimSpace(app.Version)
			if version == "" {
				version = "development"
			}
			if opts.format == "json" {
				return writeJSONOutput(cmd.OutOrStdout(), "version", map[string]string{"version": version})
			}
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "agentplugins %s\n", version)
			return err
		},
	}
}

func publicInstallations(state domain.StateFileV2, includeAbsent bool) []publicInstallation {
	values := make([]publicInstallation, 0, len(state.Installations))
	for _, installation := range state.Installations {
		value := publicInstallationView(installation, includeAbsent)
		if len(value.Clients) == 0 && !includeAbsent {
			continue
		}
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool {
		if values[i].Name == values[j].Name {
			return values[i].InstallationID < values[j].InstallationID
		}
		return values[i].Name < values[j].Name
	})
	return values
}

func publicInstallationView(installation domain.Installation, includeAbsent bool) publicInstallation {
	value := publicInstallation{
		InstallationID: installation.InstallationID,
		Name:           installation.DeclaredName, Version: installation.Package.Version,
		Source: publicSource(installation.Source), NeedsRebind: installation.NeedsRebind,
	}
	for _, client := range installation.Clients {
		if client.Materialization == domain.MaterializationAbsent && !includeAbsent {
			continue
		}
		value.Clients = append(value.Clients, publicClient{
			ClientID: client.ClientID, Scope: client.Scope, Materialization: client.Materialization,
			Activation: client.Activation, Authentication: client.Authentication,
			Policy: client.Policy, Verification: client.Verification,
		})
	}
	sort.Slice(value.Clients, func(i, j int) bool { return value.Clients[i].ClientID < value.Clients[j].ClientID })
	return value
}

func publicSource(source domain.SourceBinding) string {
	if source.Repository != "" {
		value := source.Repository
		if source.PackageSubpath != "" {
			value += "//" + source.PackageSubpath
		}
		return value
	}
	canonical := strings.TrimSpace(source.CanonicalSource)
	if canonical == "" || filepath.IsAbs(canonical) {
		return "local"
	}
	parsed, err := url.Parse(canonical)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "local"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func selectInstallation(state domain.StateFileV2, selector string) (domain.Installation, error) {
	selector = strings.TrimSpace(selector)
	var matches []domain.Installation
	for _, installation := range state.Installations {
		if installation.InstallationID == selector {
			return installation, nil
		}
		if installation.DeclaredName == selector {
			matches = append(matches, installation)
		}
	}
	if len(matches) == 0 {
		return domain.Installation{}, fmt.Errorf("installation %q was not found", selector)
	}
	if len(matches) > 1 {
		return domain.Installation{}, fmt.Errorf("installation name %q is ambiguous; use installation_id", selector)
	}
	return matches[0], nil
}

func renderInstallationList(writer io.Writer, installations []publicInstallation) error {
	if len(installations) == 0 {
		_, _ = fmt.Fprintln(writer, "No Agent Plugins installations are tracked.")
		return nil
	}
	for _, installation := range installations {
		if err := renderInstallation(writer, installation); err != nil {
			return err
		}
	}
	return nil
}

func renderInstallation(writer io.Writer, installation publicInstallation) error {
	_, _ = fmt.Fprintf(writer, "%s %s (%s)\n", installation.Name, installation.Version, installation.InstallationID)
	_, _ = fmt.Fprintf(writer, "  Source: %s\n", installation.Source)
	for _, client := range installation.Clients {
		_, _ = fmt.Fprintf(writer, "  %s: materialization=%s activation=%s auth=%s verification=%s\n",
			client.ClientID, client.Materialization, client.Activation, client.Authentication, client.Verification)
	}
	return nil
}
