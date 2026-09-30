package proc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strix/internal/wire/config"
	"sync"
	"syscall"
	"time"
)

type childExit struct {
	name string
	pid  int
	err  error
}

func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	gatewayBin, engineBin, err := loadBinaryPath()
	if err != nil {
		return err
	}
	tokModel, bertModel, err := loadModelPath()
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

	const numChildProcs = 2
	active := make(map[int]*exec.Cmd, numChildProcs)
	deathChan := make(chan childExit, numChildProcs)

	// Fork Engine process.
	readC, writeC, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("engine death pipe: %w", err)
	}

	engineEnc := json.NewEncoder(writeC)
	if err := engineEnc.Encode(config.EngineEnvelop{
		Ipc:    opts.Ipc,
		Engine: opts.Engine,
	}); err != nil {
		return fmt.Errorf("encode engine config: %w", err)
	}

	engineEnv := append(os.Environ(),
		"TOKENIZER_PATH="+tokModel, "TRANSFORMER_PATH="+bertModel, "PARENT_PIPE_FD=3")
	engineCmd, err := fork(runCtx, engineBin, readC, engineEnv, opts.Supervisor.DrainTimeout)
	_ = readC.Close()
	if err != nil {
		return err
	}
	active[engineCmd.Process.Pid] = engineCmd
	supervise("engine", engineCmd, deathChan)

	var deathWriters []*os.File
	closeDeathWriters := sync.OnceFunc(func() {
		for _, wr := range deathWriters {
			_ = wr.Close()
		}
	})
	defer closeDeathWriters()

	// Fork Gateway process.
	readB, writeB, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("gateway death pipe: %w", err)
	}

	gatewayEnc := json.NewEncoder(writeB)
	if err := gatewayEnc.Encode(config.GatewayEnvelop{
		Ipc:       opts.Ipc,
		Gateway:   opts.Gateway,
		Upstreams: opts.Upstreams,
	}); err != nil {
		return fmt.Errorf("encode gateway config: %w", err)
	}

	gatewayEnv := append(os.Environ(), "PARENT_PIPE_FD=3")
	gatewayCmd, err := fork(runCtx, gatewayBin, readB, gatewayEnv, opts.Supervisor.DrainTimeout)
	_ = readB.Close()
	if err != nil {
		return err
	}
	active[gatewayCmd.Process.Pid] = gatewayCmd
	supervise("gateway", gatewayCmd, deathChan)

	// Event loop: react to the first event occur.
	var runErr error
	select {
	case <-ctx.Done(): // OS signals

	case exit := <-deathChan:
		delete(active, exit.pid)
		if exit.err != nil {
			runErr = fmt.Errorf("%s (pid %d) exited unexpectedly: %w",
				exit.name, exit.pid, exit.err)
		} else {
			runErr = fmt.Errorf("%s (pid %d) exited unexpectedly with status 0",
				exit.name, exit.pid)
		}
		cancelRun()
	}

	closeDeathWriters() // Shutdown signals to ALL child processes
	cancelRun()
	for len(active) > 0 {
		exit := <-deathChan
		delete(active, exit.pid)
	}

	return runErr
}

func fork(ctx context.Context, binPath string, pipe *os.File,
	env []string, drainTimeout time.Duration) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, binPath)
	cmd.WaitDelay = drainTimeout
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.ExtraFiles = []*os.File{pipe}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

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

func loadBinaryPath() (gateway, engine string, err error) {
	const (
		gatewayBin = "strix_gateway"
		engineBin  = "strix_engine"
	)

	execPath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve executable path: %w", err)
	}

	binDir := filepath.Dir(execPath)
	return filepath.Join(binDir, gatewayBin), filepath.Join(binDir, engineBin), nil
}

func loadModelPath() (tokenizer, transformer string, err error) {
	const (
		tokFile  = "tokenizer.onnx"
		bertFile = "transformer.onnx"
	)

	execPath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve executable path: %w", err)
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(execPath), ".."))
	modelDir := filepath.Join(repoRoot, "model")
	return filepath.Join(modelDir, tokFile), filepath.Join(modelDir, bertFile), nil
}
