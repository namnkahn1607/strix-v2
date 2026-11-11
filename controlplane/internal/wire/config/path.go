package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func DefaultDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home directory: %w", err)
		}
		configHome = filepath.Join(homeDir, ".config")
	}

	return filepath.Join(configHome, "strix"), nil
}

func SaveConfigPath(path string) error {
	configDir, err := DefaultDir()
	if err != nil {
		return err
	}

	// Full permissions (rwx) to the owner, but not others.
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Temp file in the same directory + rename: rename(2) is atomic only within one
	// filesystem, so a crash mid-write never leaves a torn pointer.
	tmp, err := os.CreateTemp(configDir, "config_path.tmp-*")
	if err != nil {
		return fmt.Errorf("create temp config path file: %w", err)
	}

	tmpName := tmp.Name()
	cleanUp := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.WriteString(path); err != nil {
		cleanUp()
		return fmt.Errorf("write config path file %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanUp()
		return fmt.Errorf("sync config path file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close config path file: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(configDir, "config_path")); err != nil {
		return fmt.Errorf("commit config path file: %w", err)
	}

	return nil
}

func LoadConfigPath() (string, error) {
	configDir, err := DefaultDir()
	if err != nil {
		return "", err
	}

	configPathFile := filepath.Join(configDir, "config_path")
	//nolint:gosec // G304: directory not resolving from external value
	data, err := os.ReadFile(configPathFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("no config path set. Use 'strix config <path>' first")
		}
		return "", fmt.Errorf("read config path file: %w", err)
	}

	return string(data), nil
}
