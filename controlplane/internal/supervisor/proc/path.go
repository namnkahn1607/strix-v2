package proc

import (
	"fmt"
	"os"
	"path/filepath"
)

type path struct {
	gateway     string
	engine      string
	tokenizer   string
	transformer string
}

func loadPath() (*path, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve executable path: %w", err)
	}

	const (
		gatewayBin = "strix_gateway"
		engineBin  = "strix_engine"
		tokFile    = "tokenizer.onnx"
		bertFile   = "transformer.onnx"
	)

	execDir := filepath.Dir(execPath)
	return &path{
		gateway:     filepath.Join(execDir, gatewayBin),
		engine:      filepath.Join(execDir, engineBin),
		tokenizer:   filepath.Join(execDir, tokFile),
		transformer: filepath.Join(execDir, bertFile),
	}, nil
}
