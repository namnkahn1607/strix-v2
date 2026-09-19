package main

import (
	"fmt"
	"os"
	"strix/internal/supervisor/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprint(os.Stderr, "Error executing root command: %w\n", err)
		os.Exit(1)
	}
}
