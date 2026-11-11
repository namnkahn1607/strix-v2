package control

import "fmt"

// APIError is used by daemon to encode error in returning response to CLI client.
type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// ErrorBody is used by CLI client to decode error in returned response by daemon.
type ErrorBody struct {
	Error APIError `json:"error"`
}
