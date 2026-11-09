package cli

import (
	"errors"
	"strix/internal/supervisor/daemon"
	"strix/internal/supervisor/runtime"
	"strix/internal/wire/control"

	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the Strix daemon in the foreground",
	Args:  cobra.NoArgs,
	RunE: run(func(*cobra.Command, []string) error {
		err := daemon.Run()
		if _, ok := errors.AsType[*runtime.HeldError](err); ok {
			return &ExitError{Code: control.ExitConflict, Err: err}
		}
		return err
	}),
}
