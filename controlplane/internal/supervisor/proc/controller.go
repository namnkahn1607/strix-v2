package proc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"strix/internal/supervisor/core"
	"strix/internal/wire/config"
	"strix/internal/wire/control"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Inherited fd numbers: ExtraFiles[i] becomes fd 3+i in the child. The death pipe number
// is a shared contract with the C++ engine and the Go gateway (PARENT_PIPE_FD).
const (
	parentPipeFD = 3
	// Instance lock (fd 4). Children must never close it: it keeps a new daemon out until
	// every child of a SIGKILLed daemon is gone.
	instanceLockFD = 4
)

const engineReadyTimeout = 60 * time.Second

var _ = instanceLockFD // Documented contract; no child reads it by number.

type Plane struct {
	RuntimeDir string
	Lock       *os.File
	GatewayBin string
	EngineBin  string
}

func NewPlane(runtimeDir string, lock *os.File) (*Plane, error) {
	gw, eng, err := loadBinaryPath()
	if err != nil {
		return nil, err
	}
	return &Plane{RuntimeDir: runtimeDir, Lock: lock, GatewayBin: gw, EngineBin: eng}, nil
}

type child struct {
	name    string
	cmd     *exec.Cmd
	pid     int
	deathWr *os.File
	exited  chan struct{}
	waitErr error // valid after exited is closed
	timeout time.Duration
}

type running struct {
	runDir  string
	gateway *child
	engine  *child
	ceiling time.Duration

	stopping atomic.Bool
	exited   chan struct{}
	once     sync.Once
	errMu    sync.Mutex
	err      error

	stopOnce sync.Once
	stopErr  error
}

func (r *running) Pids() control.Pids {
	var p control.Pids
	if r.gateway != nil {
		p.Gateway = r.gateway.pid
	}
	if r.engine != nil {
		p.Engine = r.engine.pid
	}
	return p
}

func (r *running) Exited() <-chan struct{} { return r.exited }

func (r *running) Err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}

func (r *running) onChildExit(c *child) {
	if r.stopping.Load() {
		return
	}
	r.once.Do(func() {
		reason := "exited with status 0"
		if c.waitErr != nil {
			reason = c.waitErr.Error()
		}
		r.errMu.Lock()
		r.err = fmt.Errorf("%s (pid %d): %s", c.name, c.pid, reason)
		r.errMu.Unlock()
		close(r.exited)
	})
}

func (p *Plane) Start(ctx context.Context, opts *config.Options,
	report func(control.Event)) (core.Running, error) {
	runDir, err := os.MkdirTemp(p.RuntimeDir, "run-*") // Mode 0700.
	if err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}

	// The daemon, not the user, owns the engine socket location.
	opts.Ipc.SocketAddress = "unix://" + runDir + "/" + config.EngineSocketName

	r := &running{runDir: runDir, ceiling: opts.Supervisor.DrainTimeout, exited: make(chan struct{})}
	abort := func(err error) (core.Running, error) {
		r.stopping.Store(true)
		_ = r.stop()
		return nil, err
	}

	report(control.Event{Phase: control.PhaseSpawn})

	if r.engine, err = p.spawn(r, "engine", p.EngineBin, opts.Engine.ShutdownTimeout,
		config.EngineEnvelop{Ipc: opts.Ipc, Engine: opts.Engine}); err != nil {
		return abort(err)
	}
	// The engine creates its socket only after the model is loaded and gRPC is listening,
	// so the socket's existence is the readiness signal; the gateway fails its own startup
	// if the engine is not reachable yet.
	if err := waitEngineReady(ctx, r.engine, strings.TrimPrefix(opts.Ipc.SocketAddress, "unix://")); err != nil {
		return abort(err)
	}
	if r.gateway, err = p.spawn(r, "gateway", p.GatewayBin, opts.Gateway.ShutdownTimeout,
		config.GatewayEnvelop{Ipc: opts.Ipc, Gateway: opts.Gateway, Upstreams: opts.Upstreams}); err != nil {
		return abort(err)
	}

	pids := r.Pids()
	report(control.Event{Phase: control.PhaseSpawn, Pids: &pids})
	return r, nil
}

func (p *Plane) spawn(r *running, name, bin string, timeout time.Duration, payload any) (*child, error) {
	rd, wr, err := os.Pipe() // O_CLOEXEC: siblings never inherit each other's write end.
	if err != nil {
		return nil, fmt.Errorf("%s death pipe: %w", name, err)
	}

	// Written before fork: payload must fit the kernel pipe buffer (64 KiB on Linux),
	// otherwise this blocks forever.
	if err := json.NewEncoder(wr).Encode(payload); err != nil {
		_ = rd.Close()
		_ = wr.Close()
		return nil, fmt.Errorf("encode %s config: %w", name, err)
	}

	// No CommandContext: shutdown is explicit and ordered, never a side effect of a context.
	cmd := exec.Command(bin)
	cmd.ExtraFiles = []*os.File{rd, p.Lock}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), fmt.Sprintf("PARENT_PIPE_FD=%d", parentPipeFD))
	// Own process group: a terminal Ctrl-C reaches only the daemon, which then shuts the
	// children down in order instead of the kernel signalling all of them at once.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	err = cmd.Start()
	_ = rd.Close()
	if err != nil {
		_ = wr.Close()
		return nil, fmt.Errorf("spawn %s %q: %w", name, bin, err)
	}

	c := &child{name: name, cmd: cmd, pid: cmd.Process.Pid, deathWr: wr,
		exited: make(chan struct{}), timeout: timeout}
	go func() {
		c.waitErr = cmd.Wait()
		close(c.exited)
		r.onChildExit(c)
	}()
	return c, nil
}

// Stop shuts down the Gateway first (drain in-flight requests), then the Engine, each with
// its own SIGTERM→SIGKILL timeout, all bounded by the daemon ceiling.
func (r *running) Stop() error {
	r.stopping.Store(true)
	return r.stop()
}

func (r *running) stop() error {
	r.stopOnce.Do(func() {
		deadline := time.Now().Add(r.ceiling)
		var forced []string
		for _, c := range []*child{r.gateway, r.engine} {
			if c != nil && !c.terminate(deadline) {
				forced = append(forced, c.name)
			}
		}
		// Only now: closing earlier would signal the engine through its death pipe before
		// the gateway finished draining.
		for _, c := range []*child{r.gateway, r.engine} {
			if c != nil {
				_ = c.deathWr.Close()
			}
		}
		_ = os.RemoveAll(r.runDir)
		if len(forced) > 0 {
			r.stopErr = fmt.Errorf("SIGKILL after timeout: %v", forced)
		}
	})
	return r.stopErr
}

// terminate returns false when the child had to be SIGKILLed.
func (c *child) terminate(ceiling time.Time) bool {
	select {
	case <-c.exited:
		return true
	default:
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)

	t := time.NewTimer(max(min(c.timeout, time.Until(ceiling)), 0))
	defer t.Stop()
	select {
	case <-c.exited:
		return true
	case <-t.C:
		_ = c.cmd.Process.Kill()
		<-c.exited
		return false
	}
}

func waitEngineReady(ctx context.Context, eng *child, sock string) error {
	deadline := time.NewTimer(engineReadyTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(sock); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-eng.exited:
			return fmt.Errorf("engine (pid %d) exited before becoming ready: %v", eng.pid, eng.waitErr)
		case <-deadline.C:
			return fmt.Errorf("engine not ready after %s", engineReadyTimeout)
		case <-tick.C:
		}
	}
}
