// Package daemon wires the Supervisor core to its runtime resources and runs the daemon process.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strix/internal/supervisor/core"
	"strix/internal/supervisor/proc"
	"strix/internal/supervisor/runtime"
	"strix/internal/supervisor/server"
	"strix/internal/wire/config"
	"strix/internal/wire/control"
	"syscall"
	"time"
)

const requestDrainWait = 2 * time.Second

// diskConfig reads config from the pointer file and disk on every call, because the file
// can change between `strix config` and `strix serve`.
type diskConfig struct{}

func (diskConfig) Pointer() (string, error) { return config.LoadConfigPath() }
func (diskConfig) Save(path string) error   { return config.SaveConfigPath(path) }

func (diskConfig) Parse(path string) (*config.Options, error) {
	opts, err := config.Parse(path)
	if err != nil {
		return nil, err
	}
	modelDir, err := proc.ResolveModelDir(path, opts.Engine.ModelDirectory)
	if err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	opts.Engine.ModelDirectory = modelDir
	for _, w := range opts.Warnings() {
		log.Printf("warning: %s", w)
	}
	return opts, nil
}

// precacher is the placeholder for `serve -p`.
type precacher struct{}

func (precacher) Run(_ context.Context, _ *config.Options, files []string, progress func(done, total int)) error {
	// TODO: implement precache. Expected shape:
	//   - stream every line of each .jsonl file (files are absolute and verified to exist),
	//   - push the entries into the Engine (e.g. SetCache over opts.Ipc.SocketAddress),
	//   - call progress(done, total) as entries complete,
	//   - return promptly with ctx.Err() once ctx is cancelled (that is how `strix stop` aborts).
	_ = files
	_ = progress
	return errors.New("precache is not implemented yet")
}

// Run blocks until the daemon has shut down. A *runtime.HeldError means another daemon owns
// the instance lock.
func Run() error {
	dir := runtime.Dir()
	if err := runtime.Prepare(dir); err != nil {
		return err
	}
	lock, err := runtime.Acquire(dir)
	if err != nil {
		return err
	}
	sock := runtime.ControlSocketPath(dir)
	if err := lock.WriteMeta(runtime.Meta{Pid: os.Getpid(), StartedAt: time.Now(), ControlSocket: sock}); err != nil {
		return err
	}
	// Safe only because the lock excludes every other instance.
	if err := runtime.Cleanup(dir); err != nil {
		log.Printf("warning: cleanup of stale runtime files: %v", err)
	}

	plane, err := proc.NewPlane(dir, lock.File())
	if err != nil {
		return err
	}
	actor := core.New(core.Deps{
		Config:    diskConfig{},
		Plane:     plane,
		Precacher: precacher{},
		BuildID:   control.CurrentBuildID(),
	})
	if s := actor.Status(); s.ConfigError != "" {
		log.Printf("warning: config unusable, serve will be refused until fixed: %s", s.ConfigError)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	ln, err := server.Listen(sock)
	if err != nil {
		return err
	}
	srv := server.New(actor, control.CurrentBuildID())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	go actor.Run()

	go func() {
		select {
		case <-sigCh:
			// Restore default disposition: a second signal hard-kills the daemon, and the
			// death pipes still bring the children down.
			signal.Stop(sigCh)
			log.Printf("signal received, shutting down")
			actor.Do(context.Background(), core.Command{Kind: core.KindQuit})
		case <-actor.Done():
		}
	}()

	log.Printf("ready (pid %d, socket %s)", os.Getpid(), sock)

	select {
	case <-actor.Done():
	case err := <-serveErr:
		if err != nil {
			actor.Do(context.Background(), core.Command{Kind: core.KindQuit})
			return fmt.Errorf("control server: %w", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestDrainWait)
	defer cancel()
	_ = srv.Shutdown(ctx)
	if err := runtime.Cleanup(dir); err != nil {
		log.Printf("warning: cleanup: %v", err)
	}
	return nil
}
