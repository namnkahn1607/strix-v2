package httpapi

import (
	"context"
	"io"
	"net/http"
	"strix/internal/gateway/pb"
	"time"

	"github.com/buger/jsonparser"
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
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Assume the path to the client's request prompt was only `/prompt/`.
	promptStr, _, _, err := jsonparser.Get(reqBody, "prompt")
	if err != nil || len(promptStr) == 0 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	upstreamFallback := func() {
		payload, err := gw.client.Forward(r.Context(), reqBody, r.Header)
		if err != nil {
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
			return
		}

		respondDownstream(w, payload)
	}

	if len(promptStr) > maxPromptLen {
		upstreamFallback()
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), stubTimeout)
	defer cancel()
	resp, err := gw.stub.CheckCache(ctx, &pb.CheckCacheRequest{Prompt: promptStr})
	if err != nil {
		upstreamFallback()
		return
	}

	switch resp.GetCheckState() {
	case pb.CacheState_HIT:
		respondDownstream(w, resp.GetCachedPayload())

	case pb.CacheState_PENDING: // Herd control
	case pb.CacheState_MISS: // Herd control

	default:
		upstreamFallback()
	}
}

func respondDownstream(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(payload) // Should handle error
}
