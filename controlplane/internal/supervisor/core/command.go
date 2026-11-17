package core

import "strix/internal/wire/control"

type Kind int

const (
	KindServe Kind = iota
	KindStop
	KindQuit
	KindConfig
)

type Result struct {
	Err     *control.APIError
	Session *Session
}

type Command struct {
	Kind     Kind
	Path     string
	Files    []string
	Precache bool
	reply    chan Result
}
