package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strix/internal/supervisor/client"
	"strix/internal/wire/control"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "strix",
	Short: "Strix - Semantic Cache Proxy CLI",
}

// Execute returns the exit code. Errors that did not come from a command's RunE are
// cobra's own argument/flag/unknown-command failures, which are all usage errors.
func Execute() (int, error) {
	err := rootCmd.Execute()
	if err == nil {
		return control.ExitOK, nil
	}
	if ee, ok := errors.AsType[*client.Error](err); ok {
		return ee.Code, err
	}
	return control.ExitUsage, err
}

func init() {
	rootCmd.AddCommand(configCmd, stopCmd, quitCmd)
}

func ControlSocketPath() string {
	const file = "control.sock"

	var dir string
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
		dir = filepath.Join(x, "strix")
	} else {
		dir = filepath.Join("/tmp", fmt.Sprintf("strix-%d", os.Geteuid()))
	}

	return filepath.Join(dir, file)
}
