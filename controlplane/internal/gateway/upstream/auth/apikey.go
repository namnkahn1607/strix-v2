package auth

import "net/http"

//nolint:unused // reason: idea prototype
type apiKeyAuth struct {
	key string
}

//nolint:unused // reason: idea prototype
func (a apiKeyAuth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.key)
	return nil
}
