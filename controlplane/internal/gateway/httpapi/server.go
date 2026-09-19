package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strix/internal/gateway/pb"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
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

	if err := pollEngineReady(conn); err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("engine readiness check failed: %w", err)
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

// Wait for Engine to be ready using health check Watch RPC.
func pollEngineReady(conn *grpc.ClientConn) error {
	const (
		serviceName  = "strix.v1.CacheService"
		readyTimeout = 20 * time.Second
	)

	healthClient := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), readyTimeout)
	defer cancel()

	stream, err := healthClient.Watch(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: serviceName,
	})
	if err != nil {
		return fmt.Errorf("failed to start health watch: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return fmt.Errorf("health watch stream closed unexpectedly")
		}
		if err != nil {
			return fmt.Errorf("health watch error: %w", err)
		}

		if resp.Status == grpc_health_v1.HealthCheckResponse_SERVING {
			return nil
		}
	}
}
