package main

import (
	"fmt"
	"os"
	"strix/internal/gateway/httpapi"
)

func main() {
	gw, closer, err := httpapi.NewGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
		os.Exit(1)
	}
	defer closer()
	if err = gw.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
		os.Exit(1)
	}
}
