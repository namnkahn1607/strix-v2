package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statusJSON bool

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the data plane; the daemon stays alive",
	Args:  cobra.NoArgs,
	RunE: run(func(cmd *cobra.Command, _ []string) error {
		if err := newClient().Stop(cmd.Context()); err != nil {
			return err
		}
		fmt.Println("data plane stopped")
		return nil
	}),
}

var quitCmd = &cobra.Command{
	Use:   "quit",
	Short: "Stop the data plane (if running) and shut the daemon down",
	Args:  cobra.NoArgs,
	RunE: run(func(cmd *cobra.Command, _ []string) error {
		if err := newClient().Quit(cmd.Context()); err != nil {
			return err
		}
		fmt.Println("daemon is shutting down")
		return nil
	}),
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon and data plane status",
	Args:  cobra.NoArgs,
	RunE: run(func(cmd *cobra.Command, _ []string) error {
		s, err := newClient().Status(cmd.Context())
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
		fmt.Printf("build_id:     %s\n", s.BuildID)
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
	}),
}

func init() {
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "print the raw status snapshot as JSON")
}
