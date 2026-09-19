package httpapi

import "strix/internal/gateway/pb"

type Gateway struct {
	stub pb.CacheServiceClient
}

