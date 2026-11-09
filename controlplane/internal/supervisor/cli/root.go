package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "strix",
	Short: "Strix - Semantic Cache Proxy CLI",
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return err
	}
	return nil
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func ControlSocketPath() string {
	const file = "control.sock"

	var dir string
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
		dir = filepath.Join(x, "strix")
	}
	dir = filepath.Join("/tmp", fmt.Sprintf("strix-%d", os.Geteuid()))

	return filepath.Join(dir, file)
}
