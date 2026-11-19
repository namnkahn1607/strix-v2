// Package control holds the control-API types shared by daemon and the thin client.
package control

import (
	"fmt"
	"time"
)

// APIError is used by daemon to encode error in returning response to CLI client.
type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

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
	ConfigPath  string    `json:"config_path,omitempty"`
	ConfigError string    `json:"config_error,omitempty"`
	Pids        Pids      `json:"pids"`
	Precache    *Progress `json:"precache,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// ErrorBody is used by CLI client to decode error in returned response by daemon.
type ErrorBody struct {
	Error APIError `json:"error"`
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
	Pid      *Pids     `json:"pid,omitempty"`
	Progress *Progress `json:"progress,omitempty"`
	Error    *APIError `json:"error,omitempty"`
}
