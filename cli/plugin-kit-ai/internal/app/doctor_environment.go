package app

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
)

func doctorEnvironmentLines(root string, project runtimecheck.Project) []string {
	specs := doctorToolSpecs(root, project)
	if len(specs) == 0 {
		return nil
	}

	lines := []string{"Environment:"}
	missing := false
	for _, spec := range specs {
		path, _, err := doctorFindBinary(spec.Commands)
		if err != nil {
			lines = append(lines, "  "+doctorMissingLine(spec))
			missing = true
			continue
		}
		line := fmt.Sprintf("  %s: ok (%s", spec.Label, path)
		if version := doctorVersion(root, path, spec.VersionArgs); version != "" {
			line += "; " + version
		}
		line += ")"
		lines = append(lines, line)
	}
	if missing {
		lines = append(lines, "  Hint: "+doctorPATHHint())
	}
	return lines
}

type doctorToolSpec struct {
	Label       string
	Commands    []string
	VersionArgs []string
}

func doctorToolSpecs(root string, project runtimecheck.Project) []doctorToolSpec {
	var specs []doctorToolSpec
	if fileExists(joinDoctorRoot(root, "go.mod")) || strings.TrimSpace(project.Runtime) == "go" {
		specs = appendDoctorToolSpec(specs,
			doctorToolSpec{Label: "go", Commands: []string{"go"}, VersionArgs: []string{"version"}},
			doctorToolSpec{Label: "gofmt", Commands: []string{"gofmt"}},
		)
	}

	switch strings.TrimSpace(project.Runtime) {
	case "python":
		specs = appendDoctorToolSpec(specs, doctorToolSpec{
			Label:       "python runtime",
			Commands:    pythonPathNames(),
			VersionArgs: []string{"--version"},
		})
		switch project.Python.Manager {
		case runtimecheck.PythonManagerUV:
			specs = appendDoctorToolSpec(specs, doctorToolSpec{Label: "uv", Commands: []string{"uv"}, VersionArgs: []string{"--version"}})
		case runtimecheck.PythonManagerPoetry:
			specs = appendDoctorToolSpec(specs, doctorToolSpec{Label: "poetry", Commands: []string{"poetry"}, VersionArgs: []string{"--version"}})
		case runtimecheck.PythonManagerPipenv:
			specs = appendDoctorToolSpec(specs, doctorToolSpec{Label: "pipenv", Commands: []string{"pipenv"}, VersionArgs: []string{"--version"}})
		}
	case "node":
		specs = appendDoctorToolSpec(specs, doctorToolSpec{
			Label:       "node",
			Commands:    []string{"node"},
			VersionArgs: []string{"--version"},
		})
		manager := strings.TrimSpace(project.Node.ManagerBinary)
		if manager != "" && manager != "node" {
			specs = appendDoctorToolSpec(specs, doctorToolSpec{
				Label:       manager,
				Commands:    []string{manager},
				VersionArgs: []string{"--version"},
			})
		}
	case "shell":
		if runtime.GOOS == "windows" {
			specs = appendDoctorToolSpec(specs, doctorToolSpec{Label: "bash", Commands: []string{"bash"}, VersionArgs: []string{"--version"}})
		}
	}

	return specs
}

func appendDoctorToolSpec(dst []doctorToolSpec, specs ...doctorToolSpec) []doctorToolSpec {
	for _, spec := range specs {
		if strings.TrimSpace(spec.Label) == "" || len(spec.Commands) == 0 {
			continue
		}
		duplicate := false
		for _, existing := range dst {
			if existing.Label == spec.Label {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, spec)
		}
	}
	return dst
}
