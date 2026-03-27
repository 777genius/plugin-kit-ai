package main

import (
	"os"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/exitx"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitx.Code(err))
	}
}
