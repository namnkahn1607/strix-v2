package control

type ErrorCode string

const (
	CodeAlreadyServing ErrorCode = "already_serving"
	CodeNotRunning     ErrorCode = "not_running"
	CodeStopping       ErrorCode = "stopping"
	CodeNotIdle        ErrorCode = "not_idle"
	CodeInvalidConfig  ErrorCode = "invalid_config"
	CodeBadRequest     ErrorCode = "bad_request"
	CodeServeFailed    ErrorCode = "serve_failed"
	CodeCancelled      ErrorCode = "cancelled"
	CodeInternal       ErrorCode = "internal"
)

// IsConflict reports state-conflict errors, which are HTTP 409 and CLI exit 4.
func (c ErrorCode) IsConflict() bool {
	switch c {
	case CodeAlreadyServing, CodeNotRunning, CodeNotIdle:
		return true
	}
	return false
}
