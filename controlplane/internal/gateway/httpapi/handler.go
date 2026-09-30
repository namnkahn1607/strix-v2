package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strix/internal/gateway/pb"
	"time"

	"github.com/buger/jsonparser"
	"google.golang.org/grpc/status"
)

func (gw *Gateway) Handle(w http.ResponseWriter, r *http.Request) {
	const (
		maxReaderSize = 1024 * 1024 // 1 MiB
		maxPromptLen  = 1536
		stubTimeout   = 50 * time.Millisecond
	)

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxReaderSize)
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
		}

		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Assume the path to the client's request prompt was only `/prompt/`.
	promptBytes, dataType, _, err := jsonparser.Get(reqBody, "prompt")
	if err != nil || dataType != jsonparser.String || len(promptBytes) == 0 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	upstreamFallback := func() {
		// Rate limiting for upstream fallback logging?
		gw.log.WarnContext(r.Context(), "cache check miss, falling back to upstream",
			slog.String("grpc_code", status.Code(err).String()), slog.Any("err", err))

		payload, err := gw.client.Forward(r.Context(), reqBody, r.Header)
		if err != nil {
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}

		gw.respondDownstream(w, r, payload)
	}

	if len(promptBytes) > maxPromptLen {
		upstreamFallback()
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), stubTimeout)
	defer cancel()
	resp, err := gw.stub.CheckCache(ctx, &pb.CheckCacheRequest{Prompt: promptBytes})
	if err != nil {
		upstreamFallback()
		return
	}

	switch resp.GetCheckState() {
	case pb.CacheState_HIT:
		gw.respondDownstream(w, r, resp.GetCachedPayload())

	case pb.CacheState_PENDING: // Herd control
	case pb.CacheState_MISS: // Herd control

	default:
		upstreamFallback()
	}
}

func (gw *Gateway) respondDownstream(w http.ResponseWriter, r *http.Request, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(payload); err != nil { // #nosec G705 -- JSON API response (Content-Type + nosniff)
		gw.log.DebugContext(r.Context(), "downstream write failed",
			slog.Any("err", err), slog.Int("bytes", len(payload)))
	}
}
