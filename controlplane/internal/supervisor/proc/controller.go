package proc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

type Controller struct{}

func NewController() *Controller {
	return &Controller{}
}

func Execute() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	paths, err := loadPath()
	if err != nil {
		return err
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

	engineEnv := append(os.Environ(),
		"TOKENIZER_PATH="+paths.tokenizer, "TRANSFORMER_PATH="+paths.transformer)
	engineCmd, err := fork(paths.engine, readC, engineEnv)
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

	gatewayCmd, err := fork(paths.gateway, readB, nil)
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

	drain(active, deathChan)
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

func drain(active map[int]*exec.Cmd, deathChan <-chan int) {
	if len(active) == 0 {
		return
	}

	const drainTimeout = 6 * time.Second
	timer := time.NewTimer(drainTimeout)
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
