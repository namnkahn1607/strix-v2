// Package core is the Supervisor's state machine. One goroutine (the actor) owns all mutable
// states. The package performs no process or socket I/O.
package core

import "strix/internal/wire/control"

// Session streams serve progress. Events is closed upon server finishes; Err is then valid.
type Session struct {
	Events chan control.Event
	err    *control.APIError
}

func (s *Session) Err() *control.APIError { return s.err }
