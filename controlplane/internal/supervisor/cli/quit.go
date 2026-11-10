package cli

import (
	"fmt"
	"strix/internal/supervisor/client"

	"github.com/spf13/cobra"
)

var quitCmd = &cobra.Command{
	Use:   "quit",
	Short: "Stop service (is running) and shutdown daemon",
	Args:  cobra.NoArgs,
	RunE:  runQuit,
}

func runQuit(cmd *cobra.Command, _ []string) error {
	c := client.New(ControlSocketPath())
	if err := c.Stop(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("Shutting down Strix daemon")
	return nil
}
