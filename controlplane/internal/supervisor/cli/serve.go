package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strix/internal/wire/control"

	"github.com/spf13/cobra"
)

var precacheFlag bool

var serveCmd = &cobra.Command{
	Use:   "serve [-p] [file.jsonl...]",
	Short: "Start the data plane (Engine and Gateway)",
	Args: func(_ *cobra.Command, args []string) error {
		switch {
		case precacheFlag && len(args) == 0:
			return errors.New("--precache requires at least one .jsonl file")
		case !precacheFlag && len(args) > 0:
			return errors.New("files given without --precache")
		}
		return nil
	},
	RunE: run(runServe),
}

func init() {
	serveCmd.Flags().BoolVarP(&precacheFlag, "precache", "p", false,
		"precache the given .jsonl files before reporting Running")
}

func runServe(cmd *cobra.Command, args []string) error {
	files := make([]string, 0, len(args))
	for _, f := range args {
		abs, err := filepath.Abs(f) // The daemon's cwd differs from ours.
		if err != nil {
			return fmt.Errorf("resolve %q: %w", f, err)
		}
		files = append(files, abs)
	}

	// Ctrl-C only stops waiting. Cancelling serve must be explicit (`strix stop`).
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	err := newClient().Serve(ctx, control.ServeRequest{Precache: precacheFlag, Files: files}, printEvent)
	if err != nil && ctx.Err() != nil {
		fmt.Fprintln(os.Stderr, "\nstopped waiting; serve continues in the background (use `strix stop` to cancel)")
		return &ExitError{Code: control.ExitGeneral, Err: context.Canceled}
	}
	return err
}

func printEvent(ev control.Event) {
	switch ev.Phase {
	case control.PhaseConfig:
		fmt.Println("[config] reading and validating configuration")
	case control.PhaseSpawn:
		if ev.Pids != nil {
			fmt.Printf("[spawn] engine pid %d, gateway pid %d\n", ev.Pids.Engine, ev.Pids.Gateway)
		} else {
			fmt.Println("[spawn] starting engine and gateway")
		}
	case control.PhasePrecache:
		if ev.Progress != nil {
			fmt.Printf("[precache] %d/%d\n", ev.Progress.Done, ev.Progress.Total)
		}
	case control.PhaseRunning:
		fmt.Println("[running] data plane is up")
	}
}
