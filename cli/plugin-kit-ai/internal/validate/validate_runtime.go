package validate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/runtimecheck"
	"github.com/777genius/plugin-kit-ai/sdk/platformmeta"
)

func validatePluginRuntimeFiles(root string, manifest pluginmanifest.Manifest, launcher *pluginmanifest.Launcher, report *Report) {
	requireLauncher := false
	for _, target := range manifest.EnabledTargets() {
		profile, ok := platformmeta.Lookup(target)
		if !ok {
			continue
		}
		if profile.Launcher.Requirement == platformmeta.LauncherRequired {
			requireLauncher = true
			break
		}
	}
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
	case "python":
		validatePluginLauncher(root, launcher, report)
		validateRuntimeFileExists(root, "src/main.py", report)
		if err := validatePythonRuntime(root, manifest.EnabledTargets(), launcher); err != nil {
			msg := err.Error()
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Path:    extractFailurePath(msg),
				Message: msg,
			})
		}
	case "node":
		validatePluginLauncher(root, launcher, report)
		validateRuntimeFileExists(root, "package.json", report)
		validateNodeRuntimeTarget(root, launcher.Entrypoint, report)
		if err := validateNodeRuntime(); err != nil {
			msg := err.Error()
			report.Failures = append(report.Failures, Failure{
				Kind:    FailureRuntimeNotFound,
				Path:    extractFailurePath(msg),
				Message: msg,
			})
		}
	case "shell":
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
}

func validatePluginLauncher(root string, launcher *pluginmanifest.Launcher, report *Report) {
	if launcher == nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    filepath.Join(pluginmodel.SourceDirName, pluginmanifest.LauncherFileName),
			Message: "launcher invalid: missing " + filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, pluginmanifest.LauncherFileName)),
		})
		return
	}
	info, err := statLauncher(root, launcher.Entrypoint)
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    launcher.Entrypoint,
			Message: "launcher invalid: missing " + launcher.Entrypoint,
		})
		return
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    launcher.Entrypoint,
			Message: "launcher invalid: not executable " + launcher.Entrypoint,
		})
	}
}

func statLauncher(root, entrypoint string) (os.FileInfo, error) {
	rel := strings.TrimPrefix(filepath.Clean(entrypoint), "./")
	candidates := []string{filepath.Join(root, rel)}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, filepath.Join(root, rel+".cmd"))
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil {
			return info, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, os.ErrNotExist
}

func validateRuntimeFileExists(root, rel string, report *Report) {
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureRuntimeTargetMissing,
			Path:    rel,
			Message: "runtime target missing: " + rel,
		})
	}
}

func validateRuntimeTargetExecutable(root, rel string, report *Report) {
	info, err := os.Stat(filepath.Join(root, rel))
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureRuntimeTargetMissing,
			Path:    rel,
			Message: "runtime target missing: " + rel,
		})
		return
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureRuntimeTargetMissing,
			Path:    rel,
			Message: "runtime target missing: " + rel + " is not executable",
		})
	}
}

func validatePythonRuntime(root string, targets []string, launcher *pluginmanifest.Launcher) error {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root:     root,
		Targets:  targets,
		Launcher: launcher,
	})
	if err != nil {
		return fmt.Errorf("runtime not found: python runtime inspection failed: %v", err)
	}
	diagnosis := runtimecheck.Diagnose(project)
	if diagnosis.Status != runtimecheck.StatusReady {
		return fmt.Errorf("runtime not found: %s. %s", diagnosis.Reason, pythonRecoveryMessage(project.Python))
	}
	if err := requireMinVersion("python", project.Python.VersionOutput, 3, 10); err != nil {
		return fmt.Errorf("runtime not found: found %s interpreter at %s but %v. %s",
			project.Python.ReadySourceDisplay(),
			filepath.ToSlash(project.Python.ReadyInterpreter),
			err,
			pythonRecoveryMessage(project.Python),
		)
	}
	return nil
}

func pythonRecoveryMessage(shape runtimecheck.PythonShape) string {
	message := "Run plugin-kit-ai doctor ., then plugin-kit-ai bootstrap ."
	if fallback := shape.BootstrapFallbackCommand(); strings.TrimSpace(fallback) != "" {
		message += " If needed, fall back to " + fallback + "."
	}
	return message
}

func validateNodeRuntime() error {
	path, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("runtime not found: node runtime required; checked PATH for node. Install Node.js 20+")
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("runtime not found: found node at %s but it is not runnable (%v); install or repair Node.js 20+", path, err)
	}
	if err := requireMinVersion("node", string(out), 20, 0); err != nil {
		return fmt.Errorf("runtime not found: found node at %s but %v; install or repair Node.js 20+", path, err)
	}
	return nil
}

func validateNodeRuntimeTarget(root, entrypoint string, report *Report) {
	project, err := runtimecheck.Inspect(runtimecheck.Inputs{
		Root: root,
		Launcher: &pluginmanifest.Launcher{
			Runtime:    "node",
			Entrypoint: entrypoint,
		},
	})
	if err != nil {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    entrypoint,
			Message: "node runtime inspection failed: " + err.Error(),
		})
		return
	}
	shape := project.Node
	if shape.StructuralIssue != "" {
		report.Failures = append(report.Failures, Failure{
			Kind:    FailureLauncherInvalid,
			Path:    shape.LauncherTarget,
			Message: "node runtime configuration invalid: " + shape.StructuralIssue,
		})
		return
	}
	if shape.RuntimeTargetOK {
		return
	}
	message := "runtime target missing: " + shape.RuntimeTarget
	if shape.UsesBuiltOutput {
		if shape.IsTypeScript {
			message += " (TypeScript scaffold expects built output; run plugin-kit-ai bootstrap . or " + shape.BuildCommandString() + ")"
		} else {
			message += " (launcher points to built output; run plugin-kit-ai bootstrap . or restore the launcher target)"
		}
	} else {
		message += " (restore the generated scaffold target or update the launcher)"
	}
	report.Failures = append(report.Failures, Failure{
		Kind:    FailureRuntimeTargetMissing,
		Path:    shape.RuntimeTarget,
		Message: message,
	})
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)`)

func requireMinVersion(runtimeName, output string, wantMajor, wantMinor int) error {
	major, minor, err := parseMajorMinor(output)
	if err != nil {
		return fmt.Errorf("reported unsupported version output %q", strings.TrimSpace(output))
	}
	if major > wantMajor || (major == wantMajor && minor >= wantMinor) {
		return nil
	}
	return fmt.Errorf("reported version %d.%d is below the supported minimum %d.%d", major, minor, wantMajor, wantMinor)
}

func parseMajorMinor(output string) (int, int, error) {
	matches := versionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if len(matches) != 3 {
		return 0, 0, fmt.Errorf("no major.minor version found")
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, err
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, err
	}
	return major, minor, nil
}
