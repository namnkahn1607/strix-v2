package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strix/internal/gateway/pb"
	"strix/internal/gateway/upstream"
	"strix/internal/wire/config"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

type Gateway struct {
	stub   pb.CacheServiceClient
	client upstream.Upstream
	server *http.Server

	closers  []io.Closer
	stopOnce sync.Once
	stopErr  error

	log *slog.Logger
}

func NewGateway(cfg *config.GatewayEnvelop, logger *slog.Logger) (*Gateway, error) {
	gw := &Gateway{log: logger.With("subsystem", "httpapi")}

	var success bool
	defer func() {
		// Guard: rollback to resource cleanup once init fails.
		if !success {
			_ = gw.closeResources()
		}
	}()

	conn, err := grpc.NewClient(cfg.Ipc.SocketAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("gRPC connection: %w", err)
	}
	gw.addCloser(conn)

	if err := pollEngineReady(conn); err != nil {
		return nil, fmt.Errorf("engine readiness check: %w", err)
	}
	gw.stub = pb.NewCacheServiceClient(conn)

	mux := http.NewServeMux()
	mux.HandleFunc(cfg.Gateway.Http.ProxyEndpoint, gw.Handle)
	gw.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Gateway.Http.Port),
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		// Should this logger inherits the "subsystem: httpapi" loggin metadata?
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	success = true
	return gw, nil
}

func (gw *Gateway) Start() error {
	err := gw.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (gw *Gateway) Terminate(ctx context.Context) error {
	gw.stopOnce.Do(func() {
		var errs []error

		// 1. Stop receiving new requests and drain in-flight ones.
		if gw.server != nil {
			if err := gw.server.Shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("http server shutdown: %w", err))
			}
		}

		// 2. Clean all Egress resources after Ingress is safe.
		if err := gw.closeResources(); err != nil {
			errs = append(errs, err)
		}

		gw.stopErr = errors.Join(errs...)
	})

	return gw.stopErr
}

func (gw *Gateway) addCloser(c io.Closer) {
	gw.closers = append(gw.closers, c)
}

func (gw *Gateway) closeResources() error {
	var errs []error
	for _, closer := range slices.Backward(gw.closers) { // LIFO order
		if err := closer.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func pollEngineReady(conn *grpc.ClientConn) error {
	const (
		serviceName  = "strix.v1.CacheService"
		readyTimeout = 20 * time.Second
	)

	healthClient := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), readyTimeout)
	defer cancel()

	stream, err := healthClient.Watch(ctx,
		&grpc_health_v1.HealthCheckRequest{Service: serviceName}, grpc.WaitForReady(true))
	if err != nil {
		return fmt.Errorf("start health watch: %w", err)
	}

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("health watch stream closed unexpectedly")
		}
		if err != nil {
			return fmt.Errorf("health watch: %w", err)
		}

		if resp.Status == grpc_health_v1.HealthCheckResponse_SERVING {
			return nil
		}
	}
}
