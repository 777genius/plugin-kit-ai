package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func writePublicationDoctorJSON(cmd *cobra.Command, report publicationDoctorJSONReport) error {
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", body)
	return nil
}
