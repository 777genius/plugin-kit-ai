package main

import (
	"fmt"
	"strings"

	"github.com/hookplex/hookplex/cli/internal/app"
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
}

var skillsInitCmd = &cobra.Command{
	Use:   "init [skill-name]",
	Short: "Create a canonical SKILL.md skill package",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := skillsService.Init(app.SkillsInitOptions{
			Name:        strings.TrimSpace(args[0]),
			Description: strings.TrimSpace(skillsInitDescription),
			Template:    strings.TrimSpace(skillsInitTemplate),
			OutputDir:   strings.TrimSpace(skillsInitOutput),
			Command:     strings.TrimSpace(skillsInitCommand),
			Force:       skillsInitForce,
		})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Created skill %q at %s\n", args[0], out)
		return nil
	},
}

var skillsValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate canonical SKILL.md skills in a project",
	Args:  cobra.MaximumNArgs(1),
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
			return fmt.Errorf("skill validation failed: %s", report.Failures[0].Message)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Validated %d skill(s) in %s\n", len(report.Skills), root)
		return nil
	},
}

var skillsRenderCmd = &cobra.Command{
	Use:   "render [path]",
	Short: "Render Claude/Codex artifacts from canonical SKILL.md files",
	Args:  cobra.MaximumNArgs(1),
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
