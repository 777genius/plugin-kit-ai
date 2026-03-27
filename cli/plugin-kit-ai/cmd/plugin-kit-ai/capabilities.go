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
	Short: "Show generated hook and capability support",
	Args:  cobra.NoArgs,
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
