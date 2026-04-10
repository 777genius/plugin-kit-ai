package main

import "github.com/spf13/cobra"

func newPublicationDoctorCmd(runner inspectRunner) *cobra.Command {
	flags := publicationDoctorFlags{}
	cmd := &cobra.Command{
		Use:           "doctor [path]",
		Short:         "Inspect publication readiness without mutating files",
		Long:          "Read-only publication readiness check for package-capable targets and authored publish/... channels.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublicationDoctor(cmd, runner, publicationDoctorInput(flags, args))
		},
	}
	bindPublicationDoctorFlags(cmd, &flags)
	return cmd
}
