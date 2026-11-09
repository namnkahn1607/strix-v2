// Package core is the Supervisor's state machine. One goroutine (the actor) owns all mutable
// state; everything else talks to it through channels or reads the published immutable Snapshot.
// The package performs no process or socket I/O of its own.
package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strix/internal/wire/config"
	"strix/internal/wire/control"
	"sync/atomic"
	"time"
)

type Config interface {
	Pointer() (string, error)
	Parse(path string) (*config.Options, error)
	Save(path string) error
}

// Plane spawns and tears down the data plane (Engine + Gateway).
type Plane interface {
	// Start spawns the children in dependency order. On error nothing is left running.
	Start(ctx context.Context, opts *config.Options, report func(control.Event)) (Running, error)
}

type Running interface {
	Pids() control.Pids
	// Exited is closed when a child dies without Stop having been called.
	Exited() <-chan struct{}
	Err() error
	// Stop performs the ordered shutdown. It blocks and is idempotent.
	Stop() error
}

type Precacher interface {
	Run(ctx context.Context, opts *config.Options, files []string, progress func(done, total int)) error
}

type Kind int

const (
	KindServe Kind = iota
	KindStop
	KindQuit
	KindConfig
)

type Command struct {
	Kind     Kind
	Path     string
	Files    []string
	Precache bool
	reply    chan Result
}

type Result struct {
	Err     *control.APIError
	Session *Session
}

// Session streams serve progress. Events is closed when serve finishes; Err is then valid.
type Session struct {
	Events chan control.Event
	err    *control.APIError
}

func (s *Session) Err() *control.APIError { return s.err }

type Deps struct {
	Config    Config
	Plane     Plane
	Precacher Precacher
	Now       func() time.Time
	BuildID   string
}

var errCancelled = errors.New("serve cancelled by stop")

type (
	pipelineEvent struct{ ev control.Event }
	started       struct {
		run  Running
		opts *config.Options
	}
	startFailed struct{ err error }
	childDied   struct{}
	stopped     struct{ err error }
)

type Actor struct {
	deps Deps
	cmds chan Command
	evs  chan any
	done chan struct{}
	snap atomic.Pointer[control.Snapshot]

	// Below: owned by the actor goroutine.
	cur         control.Snapshot
	cancelStart context.CancelCauseFunc
	running     Running
	session     *Session
	stopWaiters []chan Result
	quitWaiters []chan Result
	quitting    bool
}

func New(deps Deps) *Actor {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	a := &Actor{
		deps: deps,
		cmds: make(chan Command),
		evs:  make(chan any, 16),
		done: make(chan struct{}),
	}
	a.cur = control.Snapshot{State: control.StateIdle, StartedAt: deps.Now(), BuildID: deps.BuildID}
	a.loadInitialConfig()
	a.publish()
	return a
}

func (a *Actor) loadInitialConfig() {
	path, err := a.deps.Config.Pointer()
	if err != nil {
		a.cur.ConfigError = err.Error()
		return
	}
	a.cur.ConfigPath = path
	if _, err := a.deps.Config.Parse(path); err != nil {
		a.cur.ConfigError = err.Error()
	}
}

func (a *Actor) Status() *control.Snapshot { return a.snap.Load() }

func (a *Actor) Done() <-chan struct{} { return a.done }

// Do submits a command and waits for its reply. stop/quit replies arrive only after the
// requested transition completed.
func (a *Actor) Do(ctx context.Context, c Command) Result {
	c.reply = make(chan Result, 1)
	select {
	case a.cmds <- c:
	case <-a.done:
		return Result{Err: &control.APIError{Code: control.CodeStopping, Message: "daemon is shutting down"}}
	case <-ctx.Done():
		return Result{Err: &control.APIError{Code: control.CodeInternal, Message: ctx.Err().Error()}}
	}
	select {
	case r := <-c.reply:
		return r
	case <-ctx.Done():
		return Result{Err: &control.APIError{Code: control.CodeInternal, Message: ctx.Err().Error()}}
	}
}

func (a *Actor) Run() {
	for {
		select {
		case c := <-a.cmds:
			a.handle(c)
		case e := <-a.evs:
			a.onEvent(e)
		}
		if a.quitting && a.cur.State == control.StateIdle {
			reply(a.quitWaiters, Result{})
			close(a.done)
			return
		}
	}
}

func (a *Actor) publish() {
	s := a.cur
	if s.Precache != nil {
		p := *s.Precache
		s.Precache = &p
	}
	a.snap.Store(&s)
}

func reply(ws []chan Result, r Result) {
	for _, w := range ws {
		w <- r // buffered(1), one send per channel
	}
}

func apiErr(code control.ErrorCode, format string, args ...any) *control.APIError {
	return &control.APIError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (a *Actor) handle(c Command) {
	switch c.Kind {
	case KindConfig:
		a.handleConfig(c)
	case KindServe:
		a.handleServe(c)
	case KindStop:
		a.handleStop(c, false)
	case KindQuit:
		a.quitting = true
		a.handleStop(c, true)
	}
}

func (a *Actor) handleConfig(c Command) {
	if a.cur.State != control.StateIdle || a.quitting {
		c.reply <- Result{Err: apiErr(control.CodeNotIdle, "config is accepted only while Idle (state: %s)", a.cur.State)}
		return
	}
	// Bounded local disk I/O inside the actor: kept here so the Idle check and the
	// pointer write cannot interleave with a serve.
	if _, err := a.deps.Config.Parse(c.Path); err != nil {
		c.reply <- Result{Err: apiErr(control.CodeInvalidConfig, "%v", err)}
		return
	}
	if err := a.deps.Config.Save(c.Path); err != nil {
		c.reply <- Result{Err: apiErr(control.CodeInternal, "%v", err)}
		return
	}
	a.cur.ConfigPath, a.cur.ConfigError = c.Path, ""
	a.publish()
	c.reply <- Result{}
}

func (a *Actor) handleServe(c Command) {
	switch {
	case a.quitting || a.cur.State == control.StateStopping:
		c.reply <- Result{Err: apiErr(control.CodeStopping, "data plane is stopping")}
		return
	case a.cur.State != control.StateIdle:
		c.reply <- Result{Err: apiErr(control.CodeAlreadyServing, "already serving (state: %s)", a.cur.State)}
		return
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	a.cancelStart = cancel
	a.session = &Session{Events: make(chan control.Event, 64)}
	a.cur.State, a.cur.LastError, a.cur.Pids = control.StateStarting, "", control.Pids{}
	a.cur.Precache = nil
	a.publish()
	go a.pipeline(ctx, cancel, c)
	c.reply <- Result{Session: a.session}
}

// handleStop covers stop and quit; both wait for Idle. quit additionally waits for the
// actor to finish.
func (a *Actor) handleStop(c Command, quit bool) {
	if a.cur.State == control.StateIdle {
		c.reply <- Result{}
		return
	}
	if quit {
		a.quitWaiters = append(a.quitWaiters, c.reply)
	} else {
		a.stopWaiters = append(a.stopWaiters, c.reply)
	}
	switch a.cur.State {
	case control.StateStarting:
		a.cur.State = control.StateStopping
		a.publish()
		a.cancelStart(errCancelled)
	case control.StateRunning:
		a.beginStop()
	}
}

func (a *Actor) beginStop() {
	a.cur.State = control.StateStopping
	a.publish()
	r := a.running
	go func() { a.post(stopped{err: r.Stop()}) }()
}

func (a *Actor) post(ev any) {
	select {
	case a.evs <- ev:
	case <-a.done:
	}
}

func (a *Actor) emit(ev control.Event) {
	if a.session == nil {
		return
	}
	select {
	case a.session.Events <- ev:
	default: // Slow or disconnected client must never block the actor.
	}
}

func (a *Actor) finishSession(err *control.APIError) {
	if a.session == nil {
		return
	}
	a.session.err = err
	close(a.session.Events)
	a.session = nil
}

func (a *Actor) toIdle() {
	a.cur.State, a.cur.Pids, a.cur.Precache = control.StateIdle, control.Pids{}, nil
	a.running, a.cancelStart = nil, nil
	a.publish()
	reply(a.stopWaiters, Result{})
	a.stopWaiters = nil
}

func (a *Actor) onEvent(e any) {
	switch ev := e.(type) {
	case pipelineEvent:
		a.emit(ev.ev)
		if ev.ev.Pids != nil {
			a.cur.Pids = *ev.ev.Pids
		}
		if ev.ev.Phase == control.PhasePrecache && ev.ev.Progress != nil {
			p := *ev.ev.Progress
			a.cur.Precache = &p
		}
		a.publish()

	case started:
		a.running = ev.run
		if a.cur.State == control.StateStopping { // stop raced with the end of the pipeline
			a.finishSession(apiErr(control.CodeCancelled, "%v", errCancelled))
			a.beginStop()
			return
		}
		a.cur.State, a.cur.Pids = control.StateRunning, ev.run.Pids()
		a.publish()
		a.emit(control.Event{Phase: control.PhaseRunning, Pids: &a.cur.Pids})
		a.finishSession(nil)
		go func() {
			<-ev.run.Exited()
			a.post(childDied{})
		}()

	case startFailed:
		if errors.Is(ev.err, errCancelled) {
			a.finishSession(apiErr(control.CodeCancelled, "%v", ev.err))
		} else {
			final := apiErr(control.CodeServeFailed, "%v", ev.err)
			var ae *control.APIError
			if errors.As(ev.err, &ae) {
				final = ae
			}
			a.cur.LastError = final.Message
			a.finishSession(final)
		}
		a.toIdle()

	case childDied:
		if a.cur.State != control.StateRunning {
			return
		}
		a.cur.LastError = fmt.Sprintf("child exited unexpectedly: %v", a.running.Err())
		a.beginStop()

	case stopped:
		if ev.err != nil {
			a.cur.LastError = joinMsg(a.cur.LastError, ev.err.Error())
		}
		a.toIdle()
	}
}

func joinMsg(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// pipeline runs off the actor goroutine. It owns any spawned children until it posts
// started (ownership moves to the actor) or startFailed (children already reaped).
func (a *Actor) pipeline(ctx context.Context, cancel context.CancelCauseFunc, c Command) {
	report := func(ev control.Event) { a.post(pipelineEvent{ev}) }
	fail := func(err error) { a.post(startFailed{err}) }

	report(control.Event{Phase: control.PhaseConfig})
	path, err := a.deps.Config.Pointer()
	if err != nil {
		fail(apiErr(control.CodeInvalidConfig, "%v", err))
		return
	}
	opts, err := a.deps.Config.Parse(path) // Always from disk: the file may have changed since `config`.
	if err != nil {
		fail(apiErr(control.CodeInvalidConfig, "%v", err))
		return
	}
	for _, f := range c.Files {
		fi, err := os.Stat(f)
		if err != nil || !fi.Mode().IsRegular() {
			fail(apiErr(control.CodeBadRequest, "precache file %q is not a readable regular file", f))
			return
		}
	}
	if err := context.Cause(ctx); err != nil {
		fail(err)
		return
	}

	run, err := a.deps.Plane.Start(ctx, opts, report)
	if err != nil {
		fail(err)
		return
	}

	pdone := make(chan struct{})
	go func() {
		select {
		case <-run.Exited():
			cancel(fmt.Errorf("child exited during startup: %w", run.Err()))
		case <-pdone:
		}
	}()

	var perr error
	if c.Precache {
		perr = a.deps.Precacher.Run(ctx, opts, c.Files, func(done, total int) {
			report(control.Event{Phase: control.PhasePrecache, Progress: &control.Progress{Done: done, Total: total}})
		})
	}
	close(pdone)

	if cause := context.Cause(ctx); cause != nil {
		_ = run.Stop()
		fail(cause)
		return
	}
	if perr != nil {
		_ = run.Stop()
		fail(fmt.Errorf("precache: %w", perr))
		return
	}
	a.post(started{run: run, opts: opts})
}
