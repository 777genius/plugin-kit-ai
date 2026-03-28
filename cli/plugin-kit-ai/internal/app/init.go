package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/scaffold"
)

// InitOptions is parsed CLI state for plugin-kit-ai init.
type InitOptions struct {
	ProjectName         string
	Platform            string
	Runtime             string
	OutputDir           string // empty → ./<project-name> under cwd
	Force               bool
	Extras              bool
	ClaudeExtendedHooks bool
}

// InitRunner runs plugin-kit-ai init.
type InitRunner struct{}

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
		if err := pluginmanifest.ValidateGeminiExtensionName(name); err != nil {
			return "", err
		}
	}
	r := strings.ToLower(strings.TrimSpace(opts.Runtime))
	if _, ok := scaffold.LookupRuntime(r); !ok {
		return "", errUnknownRuntime(opts.Runtime)
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
		ProjectName:         name,
		ModulePath:          scaffold.DefaultModulePath(name),
		Description:         "plugin-kit-ai plugin",
		Version:             "0.1.0",
		Platform:            p,
		Runtime:             r,
		HasSkills:           opts.Extras,
		HasCommands:         opts.Extras,
		WithExtras:          opts.Extras,
		ClaudeExtendedHooks: opts.ClaudeExtendedHooks,
	}
	if p == "codex" {
		d.CodexModel = scaffold.DefaultCodexModel
	}
	if err := scaffold.Write(out, d, opts.Force); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(out, pluginmanifest.FileName)); err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return "", err
	}
	rendered, err := pluginmanifest.Render(out, "all")
	if err != nil {
		return "", err
	}
	if err := pluginmanifest.WriteArtifacts(out, rendered.Artifacts); err != nil {
		return "", err
	}
	if err := pluginmanifest.RemoveArtifacts(out, rendered.StalePaths); err != nil {
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
