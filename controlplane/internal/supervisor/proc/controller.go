package proc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

type Controller struct{}

func (c *Controller) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Trap SIGINT / SIGTERM from OS.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
			// Controller.Run() returned normally. Goroutine exits.
		}
	}()

	paths, err := loadPath()
	if err != nil {
		return err
	}

	deathChan := make(chan *exec.Cmd, 2)

	// Fork Engine process.
	readC, writeC, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("cannot create Engine death pipe: %w", err)
	}

	engineEnv := append(os.Environ(),
		"TOKENIZER_PATH"+paths.tokenizer,
		"TRANSFORMER_PATH"+paths.transformer)

	engineProc, err := forkProc(paths.engine, readC, engineEnv)
	if err != nil {
		closePipeEnds(readC, writeC)
		return err
	}

	waitProc(deathChan, engineProc)
	closePipeEnds(readC)

	// Fork Gateway process.
	readB, writeB, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("cannot create Gateway death pipe, %w", err)
	}

	gatewayEnv := []string{}

	gatewayProc, err := forkProc(paths.gateway, readB, gatewayEnv)
	if err != nil {
		closePipeEnds(readB, writeB, writeC)
		return err
	}

	waitProc(deathChan, gatewayProc)
	closePipeEnds(readB)

	// React to the first event: either OS signal or child death.
	select {
	case <-ctx.Done(): // OS signals
		closePipeEnds(writeB, writeC)
		// Drain child processes.

	case deadProc := <-deathChan:
		// Shutdown signal to all remaining children.
		closePipeEnds(writeB, writeC)

		switch deadProc {
		case engineProc:
			// Wait then kill Gateway in case timeout.

		case gatewayProc:
			// Wait then kill Engine in case timeout.
		}
	}

	return nil
}

func closePipeEnds(ends ...*os.File) {
	for _, end := range ends {
		end.Close()
	}
}

func forkProc(binPath string, readEnd *os.File, env []string) (*exec.Cmd, error) {
	cmd := exec.Command(binPath)
	cmd.ExtraFiles = []*os.File{readEnd}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start process at %q: %w", binPath, err)
	}
	return cmd, nil
}

func waitProc(deathChan chan<- *exec.Cmd, proc *exec.Cmd) {
	go func() {
		proc.Wait() // Should handle error
		deathChan <- proc
	}()
}
