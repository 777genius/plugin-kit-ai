package validate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

func validatePluginRuntimeFiles(root string, manifest pluginmanifest.Manifest, launcher *pluginmanifest.Launcher, report *Report) {
	requireLauncher := runtimeLauncherRequired(manifest.EnabledTargets())
	if launcher == nil {
		if requireLauncher {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureLauncherInvalid,
				Path:    filepath.Join(pluginmodel.SourceDirName, pluginmanifest.LauncherFileName),
				Message: "launcher invalid: missing " + filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, pluginmanifest.LauncherFileName)),
			})
		}
		return
	}
	switch launcher.Runtime {
	case "go":
		validateGoRuntimeFiles(root, report)
	case "python":
		validatePythonRuntimeFiles(root, manifest.EnabledTargets(), launcher, report)
	case "node":
		validateNodeRuntimeFiles(root, launcher, report)
	case "shell":
		validateShellRuntimeFiles(root, launcher, report)
	}
}

func runtimeLauncherRequired(targets []string) bool {
	for _, target := range targets {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		if profile.Launcher.Requirement == platformmeta.LauncherRequired {
			return true
		}
	}
	return false
}

func validateGoRuntimeFiles(root string, report *Report) {
	validateRuntimeFileExists(root, "go.mod", report)
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureBuildFailed,
			Target:  "./...",
			Message: fmt.Sprintf("%v\n%s", err, out),
		})
	}
}

func validatePythonRuntimeFiles(root string, targets []string, launcher *pluginmanifest.Launcher, report *Report) {
	validatePluginLauncher(root, launcher, report)
	validateRuntimeFileExists(root, "src/main.py", report)
	if err := validatePythonRuntime(root, targets, launcher); err != nil {
		appendRuntimeNotFound(report, err)
	}
}

func validateNodeRuntimeFiles(root string, launcher *pluginmanifest.Launcher, report *Report) {
	validatePluginLauncher(root, launcher, report)
	validateRuntimeFileExists(root, "package.json", report)
	validateNodeRuntimeTarget(root, launcher.Entrypoint, report)
	if err := validateNodeRuntime(); err != nil {
		appendRuntimeNotFound(report, err)
	}
}

func validateShellRuntimeFiles(root string, launcher *pluginmanifest.Launcher, report *Report) {
	validatePluginLauncher(root, launcher, report)
	validateRuntimeTargetExecutable(root, "scripts/main.sh", report)
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("bash"); err != nil {
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Path:    "bash",
				Message: "runtime not found: bash (shell runtime on Windows requires bash in PATH; install Git Bash or another bash-compatible shell)",
			})
		}
	}
}

func appendRuntimeNotFound(report *Report, err error) {
	msg := err.Error()
	report.Failures = append(report.Failures, Failure{
		Kind:    FailureRuntimeNotFound,
		Path:    extractFailurePath(msg),
		Message: msg,
	})
}
