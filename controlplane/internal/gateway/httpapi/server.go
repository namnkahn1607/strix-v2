package httpapi

import (
	"fmt"
	"strix/internal/gateway/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Gateway struct {
	stub pb.CacheServiceClient
}

func NewGateway() (*Gateway, func() error, error) {
	const socketAddr = "unix:///tmp/strix.sock"
	conn, err := grpc.NewClient(socketAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("gRPC connection error: %w", err)
	}

	return &Gateway{stub: pb.NewCacheServiceClient(conn)}, conn.Close, nil
}
