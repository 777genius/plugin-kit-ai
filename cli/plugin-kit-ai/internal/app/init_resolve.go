package app

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
)

func resolveInitConfig(name string, opts InitOptions) (resolvedInitConfig, error) {
	templateName := scaffold.NormalizeTemplate(opts.Template)
	if !scaffold.IsKnownTemplate(templateName) {
		return resolvedInitConfig{}, fmt.Errorf("unknown template %q", opts.Template)
	}
	config := resolvedInitConfig{
		TemplateName:          templateName,
		Platform:              strings.ToLower(strings.TrimSpace(opts.Platform)),
		Runtime:               strings.ToLower(strings.TrimSpace(opts.Runtime)),
		RuntimePackageVersion: strings.TrimSpace(opts.RuntimePackageVersion),
	}
	if scaffold.IsPackageOnlyJobTemplate(templateName) {
		return resolvePackageOnlyInitConfig(name, opts, config)
	}
	return resolveLauncherInitConfig(name, opts, config)
}

func resolvePackageOnlyInitConfig(name string, opts InitOptions, config resolvedInitConfig) (resolvedInitConfig, error) {
	if opts.ClaudeExtendedHooks {
		return resolvedInitConfig{}, fmt.Errorf("--claude-extended-hooks is only supported with --template custom-logic and --platform claude")
	}
	if opts.TypeScript {
		return resolvedInitConfig{}, fmt.Errorf("--typescript is not supported with --template %s; use --template custom-logic when you need runtime code", config.TemplateName)
	}
	if opts.RuntimePackage {
		return resolvedInitConfig{}, fmt.Errorf("--runtime-package is not supported with --template %s; use --template custom-logic when you need a shared runtime package", config.TemplateName)
	}
	if config.RuntimePackageVersion != "" {
		return resolvedInitConfig{}, fmt.Errorf("--runtime-package-version requires --template custom-logic with --runtime-package")
	}
	if opts.RuntimeExplicit && config.Runtime != "" {
		return resolvedInitConfig{}, fmt.Errorf("--runtime is not supported with --template %s; use --template custom-logic when you need launcher-backed code", config.TemplateName)
	}
	targets, err := resolvePackageOnlyTargets(config.TemplateName, opts, config.Platform)
	if err != nil {
		return resolvedInitConfig{}, err
	}
	for _, target := range targets {
		if target == "gemini" {
			if err := pluginmanifest.ValidateGeminiExtensionName(name); err != nil {
				return resolvedInitConfig{}, fmt.Errorf("project name %q must be lowercase kebab-case when --template %s includes gemini output: %w", name, config.TemplateName, err)
			}
		}
	}
	config.Platform = ""
	config.Runtime = ""
	config.Targets = targets
	return config, nil
}

func resolvePackageOnlyTargets(templateName string, opts InitOptions, platform string) ([]string, error) {
	if !opts.PlatformExplicit {
		return scaffold.DefaultJobTemplateTargets(templateName), nil
	}
	meta, ok := scaffold.LookupPlatform(platform)
	if !ok {
		return nil, errUnknownPlatform(opts.Platform)
	}
	switch meta.Name {
	case "claude", "codex-package", "gemini", "opencode", "cursor", "cursor-workspace":
		return []string{meta.Name}, nil
	default:
		return nil, fmt.Errorf("--template %s only supports package and workspace outputs; use --template custom-logic for %s", templateName, meta.Name)
	}
}

func resolveLauncherInitConfig(name string, opts InitOptions, config resolvedInitConfig) (resolvedInitConfig, error) {
	if config.TemplateName == scaffold.InitTemplateCustomLogic {
		normalizeCustomLogicDefaults(opts, &config)
		switch config.Platform {
		case "codex-runtime", "claude", "gemini":
		default:
			return resolvedInitConfig{}, fmt.Errorf("--template custom-logic supports launcher-backed targets only; choose codex-runtime, claude, or gemini")
		}
	}
	if _, ok := scaffold.LookupPlatform(config.Platform); !ok {
		return resolvedInitConfig{}, errUnknownPlatform(opts.Platform)
	}
	if opts.ClaudeExtendedHooks && config.Platform != "claude" {
		return resolvedInitConfig{}, fmt.Errorf("--claude-extended-hooks is only supported with --platform claude")
	}
	if err := validateInitPlatformOptions(name, opts, config); err != nil {
		return resolvedInitConfig{}, err
	}
	if err := validateInitRuntimeOptions(opts, &config); err != nil {
		return resolvedInitConfig{}, err
	}
	return config, nil
}

func normalizeCustomLogicDefaults(opts InitOptions, config *resolvedInitConfig) {
	if !opts.PlatformExplicit || config.Platform == "" {
		config.Platform = "codex-runtime"
	}
	if !opts.RuntimeExplicit {
		config.Runtime = scaffold.RuntimeGo
	}
}

func validateInitPlatformOptions(name string, opts InitOptions, config resolvedInitConfig) error {
	switch config.Platform {
	case "gemini":
		if opts.TypeScript {
			return fmt.Errorf("--typescript is not supported with --platform %s", config.Platform)
		}
		if err := pluginmanifest.ValidateGeminiExtensionName(name); err != nil {
			return err
		}
		if runtimeFlag := strings.ToLower(strings.TrimSpace(config.Runtime)); runtimeFlag != "" && runtimeFlag != scaffold.RuntimeGo {
			return fmt.Errorf("--runtime is not supported with --platform %s", config.Platform)
		}
	case "opencode", "cursor", "cursor-workspace":
		if opts.TypeScript {
			return fmt.Errorf("--typescript is not supported with --platform %s", config.Platform)
		}
		if strings.TrimSpace(config.Runtime) != "" {
			return fmt.Errorf("--runtime is not supported with --platform %s", config.Platform)
		}
	case "codex-package":
		if strings.TrimSpace(config.Runtime) != "" {
			return fmt.Errorf("--runtime is not supported with --platform %s", config.Platform)
		}
		if opts.TypeScript {
			return fmt.Errorf("--typescript is not supported with --platform %s", config.Platform)
		}
	}
	if opts.RuntimePackage && (config.Platform == "gemini" || config.Platform == "codex-package" || config.Platform == "opencode" || config.Platform == "cursor" || config.Platform == "cursor-workspace") {
		return fmt.Errorf("--runtime-package is not supported with --platform %s", config.Platform)
	}
	return nil
}

func validateInitRuntimeOptions(opts InitOptions, config *resolvedInitConfig) error {
	if config.Platform != "codex-package" && config.Platform != "opencode" && config.Platform != "cursor" && config.Platform != "cursor-workspace" {
		if _, ok := scaffold.LookupRuntime(config.Runtime); !ok {
			return errUnknownRuntime(opts.Runtime)
		}
	}
	if opts.TypeScript && config.Runtime != scaffold.RuntimeNode {
		return fmt.Errorf("--typescript requires --runtime node")
	}
	if opts.RuntimePackage && config.Runtime != scaffold.RuntimePython && config.Runtime != scaffold.RuntimeNode {
		return fmt.Errorf("--runtime-package requires --runtime python or --runtime node")
	}
	if !opts.RuntimePackage && config.RuntimePackageVersion != "" {
		return fmt.Errorf("--runtime-package-version requires --runtime-package")
	}
	if opts.RuntimePackage && config.RuntimePackageVersion == "" {
		config.RuntimePackageVersion = defaultRuntimePackageVersion()
		if config.RuntimePackageVersion == "" {
			return fmt.Errorf("--runtime-package requires --runtime-package-version when the CLI build does not have a stable tagged version")
		}
	}
	return nil
}
