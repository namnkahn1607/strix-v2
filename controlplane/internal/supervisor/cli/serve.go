package cli

import (
	"strix/internal/supervisor/proc"

	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start Strix gateway daemon and cache engine",
	RunE:  runServe,
}

func runServe(_ *cobra.Command, _ []string) error {
	return proc.Execute()
}
