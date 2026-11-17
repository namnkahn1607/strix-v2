package control

type State string

const (
	StateIdle     State = "Idle"
	StateStarting State = "Starting"
	StateRunning  State = "Running"
	StateStopping State = "Stopping"
)

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

const (
	PhaseConfig   = "config"
	PhaseSpawn    = "spawn"
	PhasePrecache = "precache"
	PhaseRunning  = "running"
	PhaseError    = "error"
)

const (
	ExitOK       = 0
	ExitGeneral  = 1
	ExitUsage    = 2
	ExitNoDaemon = 3
	ExitConflict = 4
	ExitTimeout  = 5
)

func ExitCodeFor(c ErrorCode) int {
	switch {
	case c.IsConflict():
		return ExitConflict
	case c == CodeBadRequest:
		return ExitUsage
	}
	return ExitGeneral
}
