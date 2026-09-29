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
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gateway exit: %v", err)
		os.Exit(1)
	}
}

func run() error {
	sigCtx, sigStop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer sigStop()

	parent, err := parentPipe()
	if err != nil {
		return err
	}
	defer func() { _ = parent.Close() }()

	var cfg config.GatewayEnvelop
	if err := json.NewDecoder(parent).Decode(&cfg); err != nil {
		return err
	}

	gw, err := httpapi.NewGateway(&cfg)
	if err != nil {
		return err
	}

	go func() {
		_, err := io.Copy(io.Discard, parent)
		if err != nil && !errors.Is(err, os.ErrClosed) {
			// Failed reading parent pipe. Log.
		}
		sigStop()
	}()

	serveErr := make(chan error, 1)
	go func() { serveErr <- gw.Start() }()

	var runErr error
	select {
	case <-sigCtx.Done():
	case runErr = <-serveErr:
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Gateway.ShutdownTimeout)
	defer shutdownCancel()
	return errors.Join(runErr, gw.Terminate(shutdownCtx))
}

func parentPipe() (*os.File, error) {
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
		return nil, fmt.Errorf("open pipe FD: %d", fd)
	}

	return pipe, nil
}
