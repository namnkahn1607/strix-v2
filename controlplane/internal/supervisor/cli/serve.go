package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strix/internal/supervisor/client"

	"github.com/spf13/cobra"
)

var precacheFlag bool

var serveCmd = &cobra.Command{
	Use:   "serve [-p] [file.jsonl...]",
	Short: "Start HTTP gateway and cache engine",
	Args: func(_ *cobra.Command, args []string) error {
		switch {
		case precacheFlag && len(args) == 0:
			return errors.New("--precache requires at lease one .jsonl file")
		case !precacheFlag && len(args) > 0:
			return errors.New("unknown arguments")
		}
		return nil
	},
	RunE: runServe,
}

func init() {
	serveCmd.Flags().BoolVarP(&precacheFlag, "precache", "p", false, "precache data before serving")
}

func runServe(cmd *cobra.Command, args []string) error {
	files := make([]string, 0, len(args))
	for _, f := range args {
		abs, err := filepath.Abs(f) // The daemon's CWD might differ
		if err != nil {
			return fmt.Errorf("resolve %q: %w", f, err)
		}
		files = append(files, abs)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	err := client.New(ControlSocketPath()).Serve(ctx, files)
	if err != nil && ctx.Err() != nil {
		fmt.Fprintln(os.Stderr,
			"\nstopped waiting; serve might continue in the background (use `strix stop` to cancel)")
		return context.Canceled
	}
	return err
}
