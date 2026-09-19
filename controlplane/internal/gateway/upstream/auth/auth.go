package auth

import "net/http"

type AuthStrategy interface {
	Apply(req *http.Request) error
}
