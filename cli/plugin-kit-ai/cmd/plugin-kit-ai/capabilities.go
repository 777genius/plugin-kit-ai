package main

import (
	"fmt"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/capabilities"
	"github.com/spf13/cobra"
)

var (
	capabilitiesPlatform string
	capabilitiesFormat   string
)

var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "Show generated runtime support and contract class",
	Long: `Shows generated runtime-event support metadata for production and beta hook paths.

This command is runtime-focused: it reports Claude and Codex event support plus their contract class.
Packaging-only targets such as Gemini are documented in SUPPORT.md and intentionally do not appear in this output.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries := capabilities.ByPlatform(capabilitiesPlatform)
		switch strings.ToLower(strings.TrimSpace(capabilitiesFormat)) {
		case "", "table":
			_, _ = cmd.OutOrStdout().Write(capabilities.Table(entries))
			return nil
		case "json":
			out, err := capabilities.JSON(entries)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		default:
			return fmt.Errorf("unsupported format %q (use table or json)", capabilitiesFormat)
		}
	},
}

func init() {
	capabilitiesCmd.Flags().StringVar(&capabilitiesPlatform, "platform", "", "limit output to a single platform")
	capabilitiesCmd.Flags().StringVar(&capabilitiesFormat, "format", "table", "output format: table or json")
}
