package cli

import (
	"fmt"
	"path/filepath"
	"strix/internal/supervisor/client"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config <path>",
	Short: "Validate a runtime-configuration file and make daemon use it",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	abs, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}

	c := client.New(ControlSocketPath())
	if err := c.SetConfig(cmd.Context(), abs); err != nil {
		return err
	}

	fmt.Printf("Configuration path set to: %s\n", abs)
	return nil
}
