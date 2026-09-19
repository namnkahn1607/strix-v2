package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strix/internal/gateway/pb"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Gateway struct {
	stub   pb.CacheServiceClient
	server *http.Server
}

func NewGateway() (*Gateway, func() error, error) {
	const socketAddr = "unix:///tmp/strix.sock"
	conn, err := grpc.NewClient(socketAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("gRPC connection error: %w", err)
	}

	const (
		endpoint = "/v1/cache/strix"
		port     = ":8080"
	)

	gw := &Gateway{}
	mux := http.NewServeMux()
	mux.HandleFunc(endpoint, gw.Handle)
	gw.server = &http.Server{
		Addr:              port,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return gw, conn.Close, nil
}

func (gw *Gateway) Run() error {
	var runErr error
	runErr = gw.server.ListenAndServe()

	const shutdownTimeout = 3 * time.Second
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	if err := gw.server.Shutdown(shutdownCtx); err != nil {
		return errors.Join(runErr, err)
	}

	return runErr
}
