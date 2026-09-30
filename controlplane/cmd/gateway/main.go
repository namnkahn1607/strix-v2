package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strix/internal/gateway/httpapi"
	"strix/internal/gateway/middleware"
	"strix/internal/wire/config"
	"syscall"
)

type ctxHandler struct{ slog.Handler }

func (h ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if id, ok := middleware.RequestIDFrom(ctx); ok {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ctxHandler) WithAttrs(a []slog.Attr) slog.Handler {
	return ctxHandler{h.Handler.WithAttrs(a)}
}

func (h ctxHandler) WithGroup(n string) slog.Handler {
	return ctxHandler{h.Handler.WithGroup(n)}
}

func main() {
	lvl := new(slog.LevelVar)
	logger := slog.New(ctxHandler{slog.NewJSONHandler(os.Stderr,
		&slog.HandlerOptions{Level: lvl})}).
		With(slog.String("component", "gateway"), slog.Int("pid", os.Getpid()))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		fmt.Fprintf(os.Stderr, "gateway exit: %v", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
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

	gw, err := httpapi.NewGateway(&cfg, logger)
	if err != nil {
		return err
	}

	go func() {
		_, err := io.Copy(io.Discard, parent)
		if err != nil && !errors.Is(err, os.ErrClosed) {
			// Failed reading parent pipe. Log.
			logger.Warn("parent pipe read failed", slog.Any("err", err))
		}
		logger.Info("parent pipe close, shutting down")
		sigStop()
	}()

	serveErr := make(chan error, 1)
	go func() { serveErr <- gw.Start() }()

	var runErr error
	select {
	case <-sigCtx.Done():
		logger.Info("shutdown requested")
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
