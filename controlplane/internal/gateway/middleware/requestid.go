package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
)

type ctxKey int

const requestIDKey ctxKey = 0

var validIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func WithRequestID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !validIDPattern.MatchString(id) {
			id = newRequestID()
		}

		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next(w, r.WithContext(ctx))
	}
}

func RequestIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey).(string)
	return id, ok
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
