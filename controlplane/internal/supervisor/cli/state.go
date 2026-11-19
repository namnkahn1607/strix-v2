package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strix/internal/supervisor/client"

	"github.com/spf13/cobra"
)

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon and data plane status",
	Args:  cobra.NoArgs,
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, _ []string) error {
	c := client.New(ControlSocketPath())
	s, err := c.Status(cmd.Context())
	if err != nil {
		return err
	}

	if statusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	}
	fmt.Printf("state:        %s\n", s.State)
	fmt.Printf("uptime:       %s\n", s.Uptime)
	fmt.Printf("config_path:  %s\n", s.ConfigPath)
	if s.ConfigError != "" {
		fmt.Printf("config_error: %s\n", s.ConfigError)
	}
	if s.Pids.Gateway != 0 || s.Pids.Engine != 0 {
		fmt.Printf("pids:         gateway=%d engine=%d\n", s.Pids.Gateway, s.Pids.Engine)
	}
	if s.Precache != nil {
		fmt.Printf("precache:     %d/%d\n", s.Precache.Done, s.Precache.Total)
	}
	if s.LastError != "" {
		fmt.Printf("last_error:   %s\n", s.LastError)
	}
	return nil
}
