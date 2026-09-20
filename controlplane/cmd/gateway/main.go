package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strix/internal/gateway/httpapi"
	"syscall"
	"time"
)

func main() {
	gw, err := httpapi.NewGateway()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gateway init: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := gw.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
			stop()
		}
	}()

	<-ctx.Done() // Wait for OS signal

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := gw.Terminate(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "gateway shutdown: %v\n", err)
		os.Exit(1)
	}
}
