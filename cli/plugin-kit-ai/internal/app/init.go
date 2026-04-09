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
	Template              string
	Platform              string
	PlatformExplicit      bool
	Runtime               string
	RuntimeExplicit       bool
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

	templateName := scaffold.NormalizeTemplate(opts.Template)
	if !scaffold.IsKnownTemplate(templateName) {
		return "", fmt.Errorf("unknown template %q", opts.Template)
	}

	p := strings.ToLower(strings.TrimSpace(opts.Platform))
	r := strings.ToLower(strings.TrimSpace(opts.Runtime))
	targets := []string(nil)
	runtimePackageVersion := strings.TrimSpace(opts.RuntimePackageVersion)

	if scaffold.IsPackageOnlyJobTemplate(templateName) {
		if opts.ClaudeExtendedHooks {
			return "", fmt.Errorf("--claude-extended-hooks is only supported with --template custom-logic and --platform claude")
		}
		if opts.TypeScript {
			return "", fmt.Errorf("--typescript is not supported with --template %s; use --template custom-logic when you need runtime code", templateName)
		}
		if opts.RuntimePackage {
			return "", fmt.Errorf("--runtime-package is not supported with --template %s; use --template custom-logic when you need a shared runtime package", templateName)
		}
		if runtimePackageVersion != "" {
			return "", fmt.Errorf("--runtime-package-version requires --template custom-logic with --runtime-package")
		}
		if opts.RuntimeExplicit && r != "" {
			return "", fmt.Errorf("--runtime is not supported with --template %s; use --template custom-logic when you need launcher-backed code", templateName)
		}
		if opts.PlatformExplicit {
			platform, ok := scaffold.LookupPlatform(p)
			if !ok {
				return "", errUnknownPlatform(opts.Platform)
			}
			switch platform.Name {
			case "claude", "codex-package", "gemini", "opencode", "cursor":
				targets = []string{platform.Name}
			default:
				return "", fmt.Errorf("--template %s only supports package and workspace outputs; use --template custom-logic for %s", templateName, platform.Name)
			}
		} else {
			targets = scaffold.DefaultJobTemplateTargets(templateName)
		}
		for _, target := range targets {
			if target == "gemini" {
				if err := pluginmanifest.ValidateGeminiExtensionName(name); err != nil {
					return "", fmt.Errorf("project name %q must be lowercase kebab-case when --template %s includes gemini output: %w", name, templateName, err)
				}
			}
		}
		p = ""
		r = ""
	} else {
		if templateName == scaffold.InitTemplateCustomLogic {
			if !opts.PlatformExplicit {
				p = "codex-runtime"
			}
			switch p {
			case "codex-runtime", "claude", "gemini":
			case "":
				p = "codex-runtime"
			default:
				return "", fmt.Errorf("--template custom-logic supports launcher-backed targets only; choose codex-runtime, claude, or gemini")
			}
			if !opts.RuntimeExplicit {
				r = scaffold.RuntimeGo
			}
			if p == "gemini" && !opts.RuntimeExplicit {
				r = scaffold.RuntimeGo
			}
		}
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
			runtimeFlag := strings.ToLower(strings.TrimSpace(r))
			if runtimeFlag != "" && runtimeFlag != scaffold.RuntimeGo {
				return "", fmt.Errorf("--runtime is not supported with --platform %s", p)
			}
		}
		if p == "opencode" || p == "cursor" {
			if opts.TypeScript {
				return "", fmt.Errorf("--typescript is not supported with --platform %s", p)
			}
			if strings.TrimSpace(r) != "" {
				return "", fmt.Errorf("--runtime is not supported with --platform %s", p)
			}
		}
		if p == "codex-package" && strings.TrimSpace(r) != "" {
			return "", fmt.Errorf("--runtime is not supported with --platform %s", p)
		}
		if p == "codex-package" && opts.TypeScript {
			return "", fmt.Errorf("--typescript is not supported with --platform %s", p)
		}
		if opts.RuntimePackage && (p == "gemini" || p == "codex-package" || p == "opencode" || p == "cursor") {
			return "", fmt.Errorf("--runtime-package is not supported with --platform %s", p)
		}
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
		if !opts.RuntimePackage && runtimePackageVersion != "" {
			return "", fmt.Errorf("--runtime-package-version requires --runtime-package")
		}
		if opts.RuntimePackage && runtimePackageVersion == "" {
			runtimePackageVersion = defaultRuntimePackageVersion()
			if runtimePackageVersion == "" {
				return "", fmt.Errorf("--runtime-package requires --runtime-package-version when the CLI build does not have a stable tagged version")
			}
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
		Description:           initDescription(templateName),
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
		JobTemplate:           templateName,
		Targets:               targets,
	}
	if templateName == scaffold.InitTemplateCustomLogic {
		d.Description = "Build custom plugin logic with one plugin repo"
	}
	if templateName == scaffold.InitTemplateOnlineService {
		d.Description = "Connect an online service with one plugin repo"
	}
	if templateName == scaffold.InitTemplateLocalTool {
		d.Description = "Connect a local tool with one plugin repo"
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

func initDescription(templateName string) string {
	switch templateName {
	case scaffold.InitTemplateOnlineService:
		return "Connect an online service with one plugin repo"
	case scaffold.InitTemplateLocalTool:
		return "Connect a local tool with one plugin repo"
	case scaffold.InitTemplateCustomLogic:
		return "Build custom plugin logic with one plugin repo"
	default:
		return "plugin-kit-ai plugin"
	}
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
