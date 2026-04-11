package app

import (
	"fmt"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/cli/internal/validate"
)

func loadReadyExportProject(root, platform string, launcher *pluginmanifest.Launcher) (runtimecheck.Project, error) {
	if err := validateExportServiceReadiness(root, platform); err != nil {
		return runtimecheck.Project{}, err
	}
	return inspectReadyExportProject(root, platform, launcher)
}

func validateExportServiceReadiness(root, platform string) error {
	report, err := validate.Validate(root, platform)
	if err != nil {
		if re, ok := err.(*validate.ReportError); ok {
			report = re.Report
		} else {
			return err
		}
	}
	if failures := exportBlockingFailures(report.Failures); len(failures) > 0 {
		return fmt.Errorf("export requires validate --strict to pass for %s", platform)
	}
	if len(report.Warnings) > 0 {
		return fmt.Errorf("export requires validate --strict to pass for %s: %d warning(s) present", platform, len(report.Warnings))
	}
	return nil
}

func inspectReadyExportProject(root, platform string, launcher *pluginmanifest.Launcher) (runtimecheck.Project, error) {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  []string{platform},
		Launcher: launcher,
	})
	if err != nil {
		return runtimecheck.Project{}, err
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return runtimecheck.Project{}, fmt.Errorf("export requires runtime readiness: %s", diagnosis.Reason)
	}
	return project, nil
}
