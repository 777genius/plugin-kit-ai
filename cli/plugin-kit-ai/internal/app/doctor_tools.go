package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
)

func doctorFindBinary(commands []string) (string, string, error) {
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		path, err := runtimecheck.LookPath(command)
		if err == nil {
			return path, command, nil
		}
	}
	return "", "", os.ErrNotExist
}

func doctorMissingLine(spec doctorToolSpec) string {
	if len(spec.Commands) == 1 {
		return spec.Label + ": missing from PATH"
	}
	return spec.Label + ": missing from PATH (checked: " + strings.Join(spec.Commands, ", ") + ")"
}

func doctorVersion(root, path string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	out, err := runtimecheck.RunCommand(root, path, args...)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func doctorPATHHint() string {
	hint := "if the runtime is installed but doctor cannot see it here, check PATH for non-interactive shells"
	if runtime.GOOS == "darwin" {
		hint += " (for example ~/.zshenv on macOS)"
	}
	if missing := doctorMissingCommonPATHDirs(); len(missing) > 0 {
		hint += "; common directories missing from PATH: " + strings.Join(missing, ", ")
	}
	return hint + "."
}

func doctorMissingCommonPATHDirs() []string {
	candidates := []string{"/usr/local/bin"}
	switch runtime.GOOS {
	case "darwin":
		candidates = append([]string{"/opt/homebrew/bin", "/usr/local/go/bin"}, candidates...)
	case "linux":
		candidates = append([]string{"/usr/local/sbin"}, candidates...)
	}

	current := map[string]struct{}{}
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			current[entry] = struct{}{}
		}
	}

	var missing []string
	for _, candidate := range candidates {
		if _, ok := current[candidate]; !ok {
			missing = append(missing, candidate)
		}
	}
	return missing
}

func joinDoctorRoot(root, rel string) string {
	return filepath.Join(root, rel)
}
