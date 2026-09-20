package main

import (
	"fmt"
	"os"
	"strix/internal/supervisor/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "root command: %v\n", err)
		os.Exit(1)
	}
}
