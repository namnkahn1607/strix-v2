package core

import (
	"context"
	"strix/internal/wire/config"
	"strix/internal/wire/control"
	"time"
)

type Config interface {
	Pointer() (string, error)
	Parse(path string) (*config.Options, error)
	Save(path string) error
}

type Running interface {
	Pids() control.Pids
	// Exited is closed when a child dies without Stop having been called.
	Exited() <-chan struct{}
	Err() error
	// Stop performs the ordered shutdown. Block and idempotent.
	Stop() error
}

// Plane spawns and tears down the child procs.
type Plane interface {
	// Start spawns the children in dependency order. On error nothing is left running.
	Start(ctx context.Context, opts *config.Options, report func(control.Event)) (Running, error)
}

type Precacher interface {
	Run(ctx context.Context, opts *config.Options, files []string, progress func(done, total int)) error
}

type Deps struct {
	Config    Config
	Plane     Plane
	Precacher Precacher
	Now       func() time.Time
}
