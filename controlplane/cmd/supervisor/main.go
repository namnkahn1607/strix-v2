package main

import (
	"fmt"
	"os"
	"strix/internal/supervisor/cli"
)

func main() {
	code, err := cli.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "strix: %v\n", err)
	}
	os.Exit(code)
}
