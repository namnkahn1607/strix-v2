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
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	const maxReaderSize = 1024 * 1024 // 1 MiB
	r.Body = http.MaxBytesReader(w, r.Body, maxReaderSize)
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}

	// Assume the path to the client's request prompt was only `/prompt/`.
	promptStr, _, _, err := jsonparser.Get(reqBody, "prompt")
	if err != nil || len(promptStr) == 0 {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	const engineTimeout = 50 * time.Millisecond
	checkCtx, checkCancel := context.WithTimeout(r.Context(), engineTimeout)
	defer checkCancel()

	checkRes, err := gw.stub.CheckCache(checkCtx, &pb.CheckCacheRequest{Prompt: promptStr})
	if err != nil {
		// Upstream fallback
		return
	}

	switch checkRes.GetCheckState() {
	case pb.CacheState_HIT:
		respondDownstream(w, checkRes.GetCachedPayload())

	case pb.CacheState_PENDING: // Herd control
	case pb.CacheState_MISS: // Herd control

	default: // Upstream fallback
	}
}

func respondDownstream(w http.ResponseWriter, payload []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(payload) // Should handle error
}
