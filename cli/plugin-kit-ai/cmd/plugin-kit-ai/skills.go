package main

import (
	"fmt"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/app"
	"github.com/spf13/cobra"
)

var skillsService app.SkillsService

var (
	skillsInitOutput      string
	skillsInitDescription string
	skillsInitTemplate    string
	skillsInitCommand     string
	skillsInitForce       bool

	skillsRenderTarget string
)

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Experimental skill authoring tools",
	Long:  "Experimental SKILL.md-native authoring, validation, and rendering tools for Claude and Codex.",
}

var skillsInitCmd = &cobra.Command{
	Use:   "init [skill-name]",
	Short: "Create a canonical SKILL.md skill package",
	Example: strings.Join([]string{
		"  plugin-kit-ai skills init lint-repo --template go-command",
		"  plugin-kit-ai skills init format-changed --template cli-wrapper --command \"ruff format .\"",
		"  plugin-kit-ai skills init review-checklist --template docs-only",
	}, "\n"),
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := strings.TrimSpace(skillsInitOutput)
		if root == "" {
			root = "."
		}
		out, err := skillsService.Init(app.SkillsInitOptions{
			Name:        strings.TrimSpace(args[0]),
			Description: strings.TrimSpace(skillsInitDescription),
			Template:    strings.TrimSpace(skillsInitTemplate),
			OutputDir:   root,
			Command:     strings.TrimSpace(skillsInitCommand),
			Force:       skillsInitForce,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created skill %q at %s\nNext: edit skills/%s/SKILL.md, then run `plugin-kit-ai skills validate %s` and `plugin-kit-ai skills render %s --target all`.\n", args[0], out, args[0], root, root)
		return nil
	},
}

var skillsValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate canonical SKILL.md skills in a project",
	Example: strings.Join([]string{
		"  plugin-kit-ai skills validate .",
		"  plugin-kit-ai skills validate ./examples/skills/go-command-lint",
	}, "\n"),
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		report, err := skillsService.Validate(app.SkillsValidateOptions{Root: root})
		if err != nil {
			return err
		}
		if len(report.Failures) > 0 {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Skill validation found %d problem(s):\n", len(report.Failures))
			for _, failure := range report.Failures {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "- %s: %s\n", failure.Path, failure.Message)
			}
			return fmt.Errorf("skill validation failed")
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Validated %d skill(s) in %s\n", len(report.Skills), root)
		return nil
	},
}

var skillsRenderCmd = &cobra.Command{
	Use:   "render [path]",
	Short: "Render Claude/Codex artifacts from canonical SKILL.md files",
	Example: strings.Join([]string{
		"  plugin-kit-ai skills render . --target all",
		"  plugin-kit-ai skills render ./examples/skills/cli-wrapper-formatter --target codex",
	}, "\n"),
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		artifacts, err := skillsService.Render(app.SkillsRenderOptions{
			Root:   root,
			Target: skillsRenderTarget,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Rendered %d artifact(s) in %s\n", len(artifacts), root)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(skillsCmd)
	skillsCmd.AddCommand(skillsInitCmd)
	skillsCmd.AddCommand(skillsValidateCmd)
	skillsCmd.AddCommand(skillsRenderCmd)

	skillsInitCmd.Flags().StringVarP(&skillsInitOutput, "output", "o", ".", "project root containing skills/")
	skillsInitCmd.Flags().StringVar(&skillsInitDescription, "description", "", "skill description")
	skillsInitCmd.Flags().StringVar(&skillsInitTemplate, "template", "go-command", `template ("go-command", "cli-wrapper", "docs-only")`)
	skillsInitCmd.Flags().StringVar(&skillsInitCommand, "command", "replace-me", "default command for cli-wrapper template")
	skillsInitCmd.Flags().BoolVarP(&skillsInitForce, "force", "f", false, "overwrite existing authored files")

	skillsRenderCmd.Flags().StringVar(&skillsRenderTarget, "target", "all", `render target ("all", "claude", "codex")`)
}
