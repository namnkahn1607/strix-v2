package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strix/internal/gateway/httpapi"
	"strix/internal/wire/config"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := readConfigFromPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gateway config: %v\n", err)
		os.Exit(1)
	}

	gw, err := httpapi.NewGateway(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gateway init: %v\n", err)
		os.Exit(1)
	}

	// Monitor death pipe closure
	go func() {
		pipeFD := os.Getenv("PARENT_PIPE_FD")
		if pipeFD != "" {
			fd, _ := strconv.Atoi(pipeFD)
			pipe := os.NewFile(uintptr(fd), "parent_pipe")
			if pipe != nil {
				// Block until pipe closes (parent shutdown signal).
				_, _ = io.Copy(io.Discard, pipe)
				_ = pipe.Close()
				stop()
			}
		}
	}()

	go func() {
		if err := gw.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gateway: %v\n", err)
			stop()
		}
	}()

	<-ctx.Done() // Wait for OS signal or pipe closure

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Gateway.ShutdownTimeout)
	defer cancel()
	if err := gw.Terminate(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "gateway shutdown: %v\n", err)
		os.Exit(1)
	}
}

func readConfigFromPipe() (*config.GatewayEnvelop, error) {
	pipeFD := os.Getenv("PARENT_PIPE_FD")
	if pipeFD == "" {
		return nil, errors.New("PARENT_PIPE_FD not set")
	}

	fd, err := strconv.Atoi(pipeFD)
	if err != nil {
		return nil, fmt.Errorf("invalid PARENT_PIPE_FD: %w", err)
	}

	pipe := os.NewFile(uintptr(fd), "parent_pipe")
	if pipe == nil {
		return nil, fmt.Errorf("failed to open pipe FD %d", fd)
	}

	configData, err := io.ReadAll(pipe)
	if err != nil {
		return nil, fmt.Errorf("read from pipe: %w", err)
	}

	var cfg config.GatewayEnvelop
	if err := json.Unmarshal(configData, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	return &cfg, nil
}
