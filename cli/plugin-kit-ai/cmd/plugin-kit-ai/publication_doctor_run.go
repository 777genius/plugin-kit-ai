package main

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/app"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/spf13/cobra"
)

type publicationDoctorFlags struct {
	target      string
	format      string
	dest        string
	packageRoot string
}

type publicationDoctorInputData struct {
	root        string
	target      string
	format      string
	dest        string
	packageRoot string
}

func publicationDoctorInput(flags publicationDoctorFlags, args []string) publicationDoctorInputData {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	return publicationDoctorInputData{
		root:        root,
		target:      flags.target,
		format:      flags.format,
		dest:        flags.dest,
		packageRoot: flags.packageRoot,
	}
}

func runPublicationDoctor(cmd *cobra.Command, runner inspectRunner, in publicationDoctorInputData) error {
	report, warnings, diagnosis, localRoot, err := inspectPublicationDoctor(runner, in)
	if err != nil {
		return err
	}
	return renderPublicationDoctor(cmd, publicationDoctorRenderInput{
		format:    in.format,
		target:    in.target,
		report:    report,
		warnings:  warnings,
		diagnosis: diagnosis,
		localRoot: localRoot,
	})
}

func inspectPublicationDoctor(runner inspectRunner, in publicationDoctorInputData) (pluginmanifest.Inspection, []pluginmanifest.Warning, publicationDiagnosis, *app.PluginPublicationVerifyRootResult, error) {
	report, warnings, err := runner.Inspect(app.PluginInspectOptions{
		Root:   in.root,
		Target: in.target,
	})
	if err != nil {
		return pluginmanifest.Inspection{}, nil, publicationDiagnosis{}, nil, err
	}
	diagnosis := diagnosePublication(in.root, in.target, report)
	localRoot, err := maybeVerifyPublicationLocalRoot(runner, in.root, in.target, in.dest, in.packageRoot, diagnosis.Status)
	if err != nil {
		return pluginmanifest.Inspection{}, nil, publicationDiagnosis{}, nil, err
	}
	diagnosis = mergePublicationDiagnosisWithLocalRoot(diagnosis, in.target, localRoot)
	return report, warnings, diagnosis, localRoot, nil
}
