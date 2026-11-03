package proc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strix/internal/wire/config"
	"syscall"
	"time"
)

type childExit struct {
	name string
	pid  int
	err  error
}

func Execute() error {
	sigCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	gatewayBin, engineBin, err := loadBinaryPath()
	if err != nil {
		return err
	}

	configPath, err := config.LoadConfigPath()
	if err != nil {
		return fmt.Errorf("load config path: %w", err)
	}
	opts, err := config.Parse(configPath)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	modelDir, err := resolveModelDir(configPath, opts.Engine.ModelDirectory)
	if err != nil {
		return err
	}
	opts.Engine.ModelDirectory = modelDir

	const numChildProcs = 2
	deathChan := make(chan childExit, numChildProcs) // Buffered: supervise never blocks
	running := 0
	var deathWriters []*os.File

	// Single teardown path for EVERY return below (early errors included).
	// Bounded: once cancelRun fires, WaitDelay SIGKILLs stragglers.
	defer func() {
		for _, wr := range deathWriters {
			_ = wr.Close()
		}
		cancelRun()
		for ; running > 0; running-- {
			<-deathChan
		}
	}()

	spawn := func(name, bin string, payload any) error {
		rd, wr, err := os.Pipe() // O_CLOEXEC: siblings never inherit each other's write end
		if err != nil {
			return fmt.Errorf("%s death pipe: %w", name, err)
		}
		deathWriters = append(deathWriters, wr) // Register first, teardown owns it from here

		// Written before fork: payload must fit the kernel pipe buffer (64 KiB on
		// Linux), otherwise this blocks forever.
		if err := json.NewEncoder(wr).Encode(payload); err != nil {
			_ = rd.Close()
			return fmt.Errorf("encode %s config: %w", name, err)
		}

		cmd, err := fork(runCtx, bin, rd, opts.Supervisor.DrainTimeout)
		_ = rd.Close()
		if err != nil {
			return err
		}
		running++
		supervise(name, cmd, deathChan)
		return nil
	}

	if err := spawn("engine", engineBin, config.EngineEnvelop{
		Ipc:    opts.Ipc,
		Engine: opts.Engine,
	}); err != nil {
		return err
	}
	if err := spawn("gateway", gatewayBin, config.GatewayEnvelop{
		Ipc:       opts.Ipc,
		Gateway:   opts.Gateway,
		Upstreams: opts.Upstreams,
	}); err != nil {
		return err
	}

	// Event loop: react to the first event that occurs.
	var runErr error
	select {
	case <-sigCtx.Done():
		// Restore default disposition: a second Ctrl-C hard-kills all, and the
		// death pipes still take the children down with the supervisor.
		stopSignals()

	case exit := <-deathChan:
		running--
		if exit.err != nil {
			runErr = fmt.Errorf("%s (pid %d) exited unexpectedly: %w",
				exit.name, exit.pid, exit.err)
		} else {
			runErr = fmt.Errorf("%s (pid %d) exited unexpectedly with status 0",
				exit.name, exit.pid)
		}
	}

	return runErr
}

func fork(ctx context.Context, binPath string, pipe *os.File,
	drainTimeout time.Duration) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binPath)
	cmd.WaitDelay = drainTimeout
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.ExtraFiles = []*os.File{pipe} // -> fd 3 in child
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	const parentPipeFD = 3
	cmd.Env = append(os.Environ(), fmt.Sprintf("PARENT_PIPE_FD=%d", parentPipeFD))

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn process %q: %w", binPath, err)
	}
	return cmd, nil
}

func supervise(name string, cmd *exec.Cmd, deathChan chan<- childExit) {
	go func() {
		err := cmd.Wait()
		deathChan <- childExit{name: name, pid: cmd.Process.Pid, err: err}
	}()
}
