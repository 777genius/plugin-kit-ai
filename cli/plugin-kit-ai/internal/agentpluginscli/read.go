package agentpluginscli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/agentplugins/domain"
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
		Clients            []publicDetectedClient `json:"clients"`
		InstallationCount  int                    `json:"installation_count"`
		OpenOperationCount int                    `json:"open_operation_count"`
		Installation       *publicInstallation    `json:"installation,omitempty"`
	}{ReadOnly: true, Clients: publicClients, InstallationCount: len(publicInstallations(state, false)), OpenOperationCount: len(open)}
	if len(args) == 1 {
		installation, err := selectInstallation(state, args[0])
		if err != nil {
			return err
		}
		value := publicInstallationView(installation, true)
		report.Installation = &value
	}
	if opts.format == "json" {
		return writeJSONOutput(cmd.OutOrStdout(), "doctor", report)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "agentplugins doctor (read-only)")
	for _, client := range clients {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s\n", client.DisplayName, client.Status)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Tracked installations: %d\n", report.InstallationCount)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Open recovery operations: %d\n", report.OpenOperationCount)
	if report.Installation != nil {
		return renderInstallation(cmd.OutOrStdout(), *report.Installation)
	}
	return nil
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
