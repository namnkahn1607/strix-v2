package auth

import "net/http"

type apiKeyAuth struct {
	key string
}

func (a apiKeyAuth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.key)
	return nil
}
