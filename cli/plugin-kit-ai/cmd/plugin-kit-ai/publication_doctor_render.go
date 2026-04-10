package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/exitx"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/spf13/cobra"
)

type publicationDoctorRenderInput struct {
	format    string
	target    string
	report    pluginmanifest.Inspection
	warnings  []pluginmanifest.Warning
	diagnosis publicationDiagnosis
	localRoot *app.PluginPublicationVerifyRootResult
}

func bindPublicationDoctorFlags(cmd *cobra.Command, flags *publicationDoctorFlags) {
	cmd.Flags().StringVar(&flags.target, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&flags.format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&flags.dest, "dest", "", "optional materialized marketplace root to verify for local codex-package or claude publication flows")
	cmd.Flags().StringVar(&flags.packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
}

func renderPublicationDoctor(cmd *cobra.Command, in publicationDoctorRenderInput) error {
	switch normalizedPublicationDoctorFormat(in.format) {
	case "text":
		return renderPublicationDoctorText(cmd, in.warnings, in.diagnosis, in.localRoot)
	case "json":
		return renderPublicationDoctorJSON(cmd, in.report, in.warnings, in.target, in.diagnosis, in.localRoot)
	default:
		return fmt.Errorf("unsupported format %q (use text or json)", in.format)
	}
}

func normalizedPublicationDoctorFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		return "text"
	case "json":
		return "json"
	default:
		return "invalid"
	}
}

func renderPublicationDoctorText(cmd *cobra.Command, warnings []pluginmanifest.Warning, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	writePublicationDoctorWarnings(cmd, warnings)
	writePublicationDoctorLines(cmd, diagnosis.Lines)
	if localRoot != nil {
		writePublicationDoctorLines(cmd, localRoot.Lines)
	}
	if diagnosis.Ready {
		return nil
	}
	return exitx.Wrap(errors.New("publication doctor found issues"), 1)
}

func writePublicationDoctorWarnings(cmd *cobra.Command, warnings []pluginmanifest.Warning) {
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
	}
}

func writePublicationDoctorLines(cmd *cobra.Command, lines []string) {
	for _, line := range lines {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
	}
}
