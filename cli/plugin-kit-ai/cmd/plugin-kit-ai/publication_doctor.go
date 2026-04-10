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

func newPublicationDoctorCmd(runner inspectRunner) *cobra.Command {
	var target string
	var format string
	var dest string
	var packageRoot string
	cmd := &cobra.Command{
		Use:           "doctor [path]",
		Short:         "Inspect publication readiness without mutating files",
		Long:          "Read-only publication readiness check for package-capable targets and authored publish/... channels.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			return runPublicationDoctor(cmd, runner, root, target, format, dest, packageRoot)
		},
	}
	cmd.Flags().StringVar(&target, "target", "all", `publication target ("all", "claude", "codex-package", or "gemini")`)
	cmd.Flags().StringVar(&format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&dest, "dest", "", "optional materialized marketplace root to verify for local codex-package or claude publication flows")
	cmd.Flags().StringVar(&packageRoot, "package-root", "", "relative package root inside the destination marketplace root (default: plugins/<name>)")
	return cmd
}

func runPublicationDoctor(cmd *cobra.Command, runner inspectRunner, root, target, format, dest, packageRoot string) error {
	report, warnings, err := runner.Inspect(app.PluginInspectOptions{
		Root:   root,
		Target: target,
	})
	if err != nil {
		return err
	}
	diagnosis := diagnosePublication(root, target, report)
	localRoot, err := maybeVerifyPublicationLocalRoot(runner, root, target, dest, packageRoot, diagnosis.Status)
	if err != nil {
		return err
	}
	diagnosis = mergePublicationDiagnosisWithLocalRoot(diagnosis, target, localRoot)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "text":
		return renderPublicationDoctorText(cmd, warnings, diagnosis, localRoot)
	case "json":
		return renderPublicationDoctorJSON(cmd, report, warnings, target, diagnosis, localRoot)
	default:
		return fmt.Errorf("unsupported format %q (use text or json)", format)
	}
}

func renderPublicationDoctorText(cmd *cobra.Command, warnings []pluginmanifest.Warning, diagnosis publicationDiagnosis, localRoot *app.PluginPublicationVerifyRootResult) error {
	for _, warning := range warnings {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", warning.Message)
	}
	for _, line := range diagnosis.Lines {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	if localRoot != nil {
		for _, line := range localRoot.Lines {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}
	if diagnosis.Ready {
		return nil
	}
	return exitx.Wrap(errors.New("publication doctor found issues"), 1)
}
