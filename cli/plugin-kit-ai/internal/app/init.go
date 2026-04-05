package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"runtime/debug"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
)

// InitOptions is parsed CLI state for plugin-kit-ai init.
type InitOptions struct {
	ProjectName           string
	Platform              string
	Runtime               string
	TypeScript            bool
	RuntimePackage        bool
	RuntimePackageVersion string
	OutputDir             string // empty → ./<project-name> under cwd
	Force                 bool
	Extras                bool
	ClaudeExtendedHooks   bool
}

// InitRunner runs plugin-kit-ai init.
type InitRunner struct{}

var stableRuntimePackageVersionRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// Run validates options, writes scaffold files, and returns the absolute output directory.
func (InitRunner) Run(opts InitOptions) (outDir string, err error) {
	name := strings.TrimSpace(opts.ProjectName)
	if err := scaffold.ValidateProjectName(name); err != nil {
		return "", err
	}

	p := strings.ToLower(strings.TrimSpace(opts.Platform))
	if _, ok := scaffold.LookupPlatform(p); !ok {
		return "", errUnknownPlatform(opts.Platform)
	}
	if opts.ClaudeExtendedHooks && p != "claude" {
		return "", fmt.Errorf("--claude-extended-hooks is only supported with --platform claude")
	}
	if p == "gemini" {
		if opts.TypeScript {
			return "", fmt.Errorf("--typescript is not supported with --platform %s", p)
		}
		if err := pluginmanifest.ValidateGeminiExtensionName(name); err != nil {
			return "", err
		}
		runtimeFlag := strings.ToLower(strings.TrimSpace(opts.Runtime))
		if runtimeFlag != "" && runtimeFlag != scaffold.RuntimeGo {
			return "", fmt.Errorf("--runtime is not supported with --platform %s", p)
		}
	}
	if p == "opencode" || p == "cursor" {
		if opts.TypeScript {
			return "", fmt.Errorf("--typescript is not supported with --platform %s", p)
		}
		if strings.TrimSpace(opts.Runtime) != "" {
			return "", fmt.Errorf("--runtime is not supported with --platform %s", p)
		}
	}
	if p == "codex-package" && strings.TrimSpace(opts.Runtime) != "" {
		return "", fmt.Errorf("--runtime is not supported with --platform %s", p)
	}
	if p == "codex-package" && opts.TypeScript {
		return "", fmt.Errorf("--typescript is not supported with --platform %s", p)
	}
	if opts.RuntimePackage && (p == "gemini" || p == "codex-package" || p == "opencode" || p == "cursor") {
		return "", fmt.Errorf("--runtime-package is not supported with --platform %s", p)
	}
	r := strings.ToLower(strings.TrimSpace(opts.Runtime))
	if p != "codex-package" && p != "opencode" && p != "cursor" {
		if _, ok := scaffold.LookupRuntime(r); !ok {
			return "", errUnknownRuntime(opts.Runtime)
		}
	}
	if opts.TypeScript && r != scaffold.RuntimeNode {
		return "", fmt.Errorf("--typescript requires --runtime node")
	}
	if opts.RuntimePackage && r != scaffold.RuntimePython && r != scaffold.RuntimeNode {
		return "", fmt.Errorf("--runtime-package requires --runtime python or --runtime node")
	}
	if !opts.RuntimePackage && strings.TrimSpace(opts.RuntimePackageVersion) != "" {
		return "", fmt.Errorf("--runtime-package-version requires --runtime-package")
	}
	runtimePackageVersion := strings.TrimSpace(opts.RuntimePackageVersion)
	if opts.RuntimePackage && runtimePackageVersion == "" {
		runtimePackageVersion = defaultRuntimePackageVersion()
		if runtimePackageVersion == "" {
			return "", fmt.Errorf("--runtime-package requires --runtime-package-version when the CLI build does not have a stable tagged version")
		}
	}

	out := strings.TrimSpace(opts.OutputDir)
	if out == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		out = filepath.Join(wd, name)
	} else {
		abs, err := filepath.Abs(out)
		if err != nil {
			return "", fmt.Errorf("resolve output path: %w", err)
		}
		out = abs
	}

	d := scaffold.Data{
		ProjectName:           name,
		ModulePath:            scaffold.DefaultModulePath(name),
		Description:           "plugin-kit-ai plugin",
		Version:               "0.1.0",
		GoSDKReplacePath:      defaultGoSDKReplacePath(),
		Platform:              p,
		Runtime:               r,
		TypeScript:            opts.TypeScript,
		SharedRuntimePackage:  opts.RuntimePackage,
		RuntimePackageVersion: runtimePackageVersion,
		HasSkills:             opts.Extras,
		HasCommands:           opts.Extras,
		WithExtras:            opts.Extras,
		ClaudeExtendedHooks:   opts.ClaudeExtendedHooks,
	}
	if p == "codex-runtime" {
		d.CodexModel = scaffold.DefaultCodexModel
	}
	if err := scaffold.Write(out, d, opts.Force); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(out, pluginmanifest.FileName)); err != nil {
		if _, srcErr := os.Stat(filepath.Join(out, pluginmodel.SourceDirName, pluginmanifest.FileName)); srcErr != nil {
			if os.IsNotExist(err) && os.IsNotExist(srcErr) {
				return out, nil
			}
			if !os.IsNotExist(err) {
				return "", err
			}
			return "", srcErr
		}
	}
	generated, err := pluginmanifest.Generate(out, "all")
	if err != nil {
		return "", err
	}
	if err := pluginmanifest.WriteArtifacts(out, generated.Artifacts); err != nil {
		return "", err
	}
	if err := pluginmanifest.RemoveArtifacts(out, generated.StalePaths); err != nil {
		return "", err
	}
	return out, nil
}

func errUnknownPlatform(platform string) error {
	return &unknownPlatformError{platform: platform}
}

func errUnknownRuntime(runtime string) error {
	return &unknownRuntimeError{runtime: runtime}
}

type unknownPlatformError struct {
	platform string
}

func (e *unknownPlatformError) Error() string {
	return "unknown platform " + `"` + e.platform + `"`
}

type unknownRuntimeError struct {
	runtime string
}

func (e *unknownRuntimeError) Error() string {
	return "unknown runtime " + `"` + e.runtime + `"`
}

func defaultRuntimePackageVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if version := normalizeStableRuntimePackageVersion(bi.Main.Version); version != "" {
			return version
		}
	}
	return ""
}

func normalizeStableRuntimePackageVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || version == "(devel)" || version == "devel" {
		return ""
	}
	if !stableRuntimePackageVersionRe.MatchString(version) {
		return ""
	}
	return strings.TrimPrefix(version, "v")
}

func defaultGoSDKReplacePath() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if version := normalizeStableRuntimePackageVersion(bi.Main.Version); version != "" {
			return ""
		}
	}
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return ""
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	sdkDir := filepath.Join(root, "sdk")
	if _, err := os.Stat(filepath.Join(sdkDir, "go.mod")); err != nil {
		return ""
	}
	return filepath.ToSlash(sdkDir)
}
