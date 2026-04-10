package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmanifest"
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/777genius/plugin-kit-ai/cli/internal/scaffold"
)

type resolvedInitConfig struct {
	TemplateName          string
	Platform              string
	Runtime               string
	Targets               []string
	RuntimePackageVersion string
}

// Run validates options, writes scaffold files, and returns the absolute output directory.
func (InitRunner) Run(opts InitOptions) (outDir string, err error) {
	name := strings.TrimSpace(opts.ProjectName)
	if err := scaffold.ValidateProjectName(name); err != nil {
		return "", err
	}
	config, err := resolveInitConfig(name, opts)
	if err != nil {
		return "", err
	}
	out, err := resolveInitOutputDir(name, opts.OutputDir)
	if err != nil {
		return "", err
	}
	if err := writeInitProject(out, buildInitScaffoldData(name, opts, config), opts.Force); err != nil {
		return "", err
	}
	if err := generateInitArtifacts(out); err != nil {
		return "", err
	}
	return out, nil
}

func resolveInitOutputDir(projectName, outputDir string) (string, error) {
	out := strings.TrimSpace(outputDir)
	if out == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("get working directory: %w", err)
		}
		return filepath.Join(wd, projectName), nil
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}
	return abs, nil
}

func buildInitScaffoldData(name string, opts InitOptions, config resolvedInitConfig) scaffold.Data {
	data := scaffold.Data{
		ProjectName:           name,
		ModulePath:            scaffold.DefaultModulePath(name),
		Description:           initDescription(config.TemplateName),
		Version:               "0.1.0",
		GoSDKReplacePath:      defaultGoSDKReplacePath(),
		Platform:              config.Platform,
		Runtime:               config.Runtime,
		TypeScript:            opts.TypeScript,
		SharedRuntimePackage:  opts.RuntimePackage,
		RuntimePackageVersion: config.RuntimePackageVersion,
		HasSkills:             opts.Extras,
		HasCommands:           opts.Extras,
		WithExtras:            opts.Extras,
		ClaudeExtendedHooks:   opts.ClaudeExtendedHooks,
		JobTemplate:           config.TemplateName,
		Targets:               config.Targets,
	}
	if config.Platform == "codex-runtime" {
		data.CodexModel = scaffold.DefaultCodexModel
	}
	return data
}

func writeInitProject(out string, data scaffold.Data, force bool) error {
	return scaffold.Write(out, data, force)
}

func generateInitArtifacts(out string) error {
	if _, err := os.Stat(filepath.Join(out, pluginmanifest.FileName)); err != nil {
		if _, srcErr := os.Stat(filepath.Join(out, pluginmodel.SourceDirName, pluginmanifest.FileName)); srcErr != nil {
			if os.IsNotExist(err) && os.IsNotExist(srcErr) {
				return nil
			}
			if !os.IsNotExist(err) {
				return err
			}
			return srcErr
		}
	}
	generated, err := pluginmanifest.Generate(out, "all")
	if err != nil {
		return err
	}
	if err := pluginmanifest.WriteArtifacts(out, generated.Artifacts); err != nil {
		return err
	}
	if err := pluginmanifest.RemoveArtifacts(out, generated.StalePaths); err != nil {
		return err
	}
	return nil
}
