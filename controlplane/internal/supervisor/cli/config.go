package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config <path>",
	Short: "Validate a configuration file and make the daemon point at it",
	Args:  cobra.ExactArgs(1),
	RunE: run(func(cmd *cobra.Command, args []string) error {
		abs, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("resolve absolute path: %w", err)
		}

		if err := newClient().SetConfig(cmd.Context(), abs); err != nil {
			return err
		}

		fmt.Printf("Configuration path set to: %s\n", abs)
		return nil
	}),
}
