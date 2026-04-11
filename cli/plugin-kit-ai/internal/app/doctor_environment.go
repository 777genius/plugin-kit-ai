package app

import "github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"

func doctorEnvironmentLines(root string, project runtimecheck.Project) []string {
	specs := doctorToolSpecs(root, project)
	return buildDoctorEnvironmentLines(root, specs)
}
