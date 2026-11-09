package proc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

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

func ResolveModelDir(configPath, modelDir string) (string, error) {
	if modelDir == "" {
		return "", errors.New("model directory is not configured")
	}

	dir := modelDir
	if !filepath.IsAbs(dir) {
		absConf, err := filepath.Abs(configPath)
		if err != nil {
			return "", fmt.Errorf("resolve config path %q: %w", configPath, err)
		}
		dir = filepath.Join(filepath.Dir(absConf), dir)
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve model directory %q: %w", dir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat model directory %q: %w", resolved, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("model path %q is not a directory", resolved)
	}

	const (
		// #nosec G101 // reason: not a hardcoded credential
		tokenizerFile   = "tokenizer.onnx"
		transformerFile = "transformer.onnx"
	)

	for _, name := range [...]string{tokenizerFile, transformerFile} {
		p := filepath.Join(resolved, name)
		fi, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("model artifact %q: %w", p, err)
		}
		if !fi.Mode().IsRegular() {
			return "", fmt.Errorf("model artifact %q is not a regular file", p)
		}
	}

	return resolved, nil
}
