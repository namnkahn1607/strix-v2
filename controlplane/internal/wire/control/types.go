// Package control holds the control-API types shared by the daemon and the thin client.
package control

import (
	"fmt"
	"runtime/debug"
	"time"
)

// BuildID is stamped by the linker (-X strix/internal/wire/control.BuildID=...).
var BuildID = ""

const BuildHeader = "X-Strix-Build"

func CurrentBuildID() string {
	if BuildID != "" {
		return BuildID
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				return s.Value
			}
		}
	}
	return "dev"
}

type State string

const (
	StateIdle     State = "Idle"
	StateStarting State = "Starting"
	StateRunning  State = "Running"
	StateStopping State = "Stopping"
)

type Pids struct {
	Gateway int `json:"gateway,omitempty"`
	Engine  int `json:"engine,omitempty"`
}

type Progress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// Snapshot is immutable once published.
type Snapshot struct {
	State       State     `json:"state"`
	StartedAt   time.Time `json:"started_at"`
	Uptime      string    `json:"uptime,omitempty"`
	BuildID     string    `json:"build_id"`
	ConfigPath  string    `json:"config_path,omitempty"`
	ConfigError string    `json:"config_error,omitempty"`
	Pids        Pids      `json:"pids"`
	Precache    *Progress `json:"precache,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

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

type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

type ErrorBody struct {
	Error APIError `json:"error"`
}

// IsConflict reports state-conflict errors, which are HTTP 409 and CLI exit 4.
func (c ErrorCode) IsConflict() bool {
	switch c {
	case CodeAlreadyServing, CodeNotRunning, CodeStopping, CodeNotIdle:
		return true
	}
	return false
}

type ConfigRequest struct {
	Path string `json:"path"`
}

type ServeRequest struct {
	Precache bool     `json:"precache"`
	Files    []string `json:"files,omitempty"`
}

// Event is one NDJSON line of the serve progress stream.
type Event struct {
	Phase    string    `json:"phase"` // config, spawn, precache, running, error
	Pids     *Pids     `json:"pids,omitempty"`
	Progress *Progress `json:"progress,omitempty"`
	Error    *APIError `json:"error,omitempty"`
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
