package cli

import (
	"errors"
	"strix/internal/supervisor/client"
	"strix/internal/supervisor/runtime"
	"strix/internal/wire/control"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "strix",
	Short:         "Strix - Semantic Cache Proxy CLI",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// ExitError carries the process exit code chosen by a command.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Execute returns the exit code. Errors that did not come from a command's RunE are
// cobra's own argument/flag/unknown-command failures, which are all usage errors (2).
func Execute() (int, error) {
	err := rootCmd.Execute()
	if err == nil {
		return control.ExitOK, nil
	}
	if ee, ok := errors.AsType[*ExitError](err); ok {
		return ee.Code, err
	}
	return control.ExitUsage, err
}

// run wraps a RunE body so that its errors are tagged with an exit code.
func run(fn func(cmd *cobra.Command, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := fn(cmd, args)
		if err == nil {
			return nil
		}
		if _, ok := errors.AsType[*ExitError](err); ok {
			return err
		}
		if ce, ok := errors.AsType[*client.Error](err); ok {
			return &ExitError{Code: ce.Code, Err: err}
		}
		return &ExitError{Code: control.ExitGeneral, Err: err}
	}
}

func newClient() *client.Client {
	return client.New(runtime.ControlSocketPath(runtime.Dir()))
}

func init() {
	rootCmd.AddCommand(daemonCmd, configCmd, serveCmd, stopCmd, quitCmd, statusCmd)
}
