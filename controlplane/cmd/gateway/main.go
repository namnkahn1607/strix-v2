package main

import (
	"fmt"
	"os"
	"strix/internal/gateway/httpapi"
)

func main() {
	gw, closer, err := httpapi.NewGateway()
	if err != nil {
		fmt.Fprint(os.Stderr, "Error instantiating Gateway: %w\n", err)
		os.Exit(1)
	}
	defer closer()
	if err = gw.Run(); err != nil {
		fmt.Fprint(os.Stderr, "Error operating Gateway: %w\n", err)
		os.Exit(1)
	}
}
