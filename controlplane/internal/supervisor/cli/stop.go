package cli

import (
	"fmt"
	"strix/internal/supervisor/client"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop service, all child procs shutdown, the daemon stays alive",
	Args:  cobra.NoArgs,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, _ []string) error {
	c := client.New(ControlSocketPath())
	if err := c.Stop(cmd.Context()); err != nil {
		return err
	}

	fmt.Println("Service stopped")
	return nil
}
