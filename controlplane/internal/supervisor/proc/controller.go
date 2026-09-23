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
	"syscall"
	"time"
)

func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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
	deathChan := make(chan int, numChildProcs)

	// Fork Engine process.
	readC, writeC, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("engine death pipe: %w", err)
	}
	defer writeC.Close()

	engineEnc := json.NewEncoder(writeC)
	if err := engineEnc.Encode(config.EngineEnvelop{
		Ipc:    opts.Ipc,
		Engine: opts.Engine,
	}); err != nil {
		return fmt.Errorf("encode engine config: %w", err)
	}

	engineEnv := append(os.Environ(),
		"TOKENIZER_PATH="+tokModel, "TRANSFORMER_PATH="+bertModel, "PARENT_PIPE_FD=3")
	engineCmd, err := fork(engineBin, readC, engineEnv)
	_ = readC.Close()
	if err != nil {
		return err
	}
	active[engineCmd.Process.Pid] = engineCmd
	supervise(engineCmd, deathChan)

	// Fork Gateway process.
	readB, writeB, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("gateway death pipe: %w", err)
	}
	defer writeB.Close()

	gatewayEnc := json.NewEncoder(writeB)
	if err := gatewayEnc.Encode(config.GatewayEnvelop{
		Ipc:       opts.Ipc,
		Gateway:   opts.Gateway,
		Upstreams: opts.Upstreams,
	}); err != nil {
		return fmt.Errorf("encode gateway config: %w", err)
	}

	gatewayEnv := append(os.Environ(), "PARENT_PIPE_FD=3")
	gatewayCmd, err := fork(gatewayBin, readB, gatewayEnv)
	_ = readB.Close()
	if err != nil {
		return err
	}
	active[gatewayCmd.Process.Pid] = gatewayCmd
	supervise(gatewayCmd, deathChan)

	// Event loop: react to the first event occur.
	select {
	case <-ctx.Done(): // OS signals

	case pid := <-deathChan:
		delete(active, pid)
	}

	// Shutdown signals to ALL child processes.
	_ = writeC.Close()
	_ = writeB.Close()

	drain(opts.Supervisor.DrainTimeout, active, deathChan)
	return nil
}

func fork(binPath string, pipe *os.File, env []string) (*exec.Cmd, error) {
	cmd := exec.Command(binPath)
	cmd.ExtraFiles = []*os.File{pipe}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn process %q: %w", binPath, err)
	}
	return cmd, nil
}

func supervise(cmd *exec.Cmd, deathChan chan<- int) {
	go func() {
		_ = cmd.Wait()
		deathChan <- cmd.Process.Pid
	}()
}

func drain(timeout time.Duration, active map[int]*exec.Cmd, deathChan <-chan int) {
	if len(active) == 0 {
		return
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for len(active) > 0 {
		select {
		case pid := <-deathChan:
			delete(active, pid)

		case <-timer.C:
			for pid, cmd := range active {
				_ = cmd.Process.Kill()
				delete(active, pid)
			}
			return
		}
	}
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
