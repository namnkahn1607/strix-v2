package upstream

import (
	"context"
	"net/http"
)

type Upstream interface {
	Forward(ctx context.Context, body []byte, header http.Header) (respBody []byte, err error)
}
