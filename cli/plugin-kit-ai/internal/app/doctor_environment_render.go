package app

import "fmt"

func buildDoctorEnvironmentLines(root string, specs []doctorToolSpec) []string {
	if len(specs) == 0 {
		return nil
	}

	lines := []string{"Environment:"}
	missing := false
	for _, spec := range specs {
		line, found := renderDoctorEnvironmentLine(root, spec)
		if !found {
			lines = append(lines, "  "+doctorMissingLine(spec))
			missing = true
			continue
		}
		lines = append(lines, line)
	}
	if missing {
		lines = append(lines, "  Hint: "+doctorPATHHint())
	}
	return lines
}

func renderDoctorEnvironmentLine(root string, spec doctorToolSpec) (string, bool) {
	path, _, err := doctorFindBinary(spec.Commands)
	if err != nil {
		return "", false
	}
	line := fmt.Sprintf("  %s: ok (%s", spec.Label, path)
	if version := doctorVersion(root, path, spec.VersionArgs); version != "" {
		line += "; " + version
	}
	line += ")"
	return line, true
}
