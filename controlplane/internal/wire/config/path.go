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

	configPathFile := filepath.Join(configDir, "config_path")
	if err := os.WriteFile(configPathFile, []byte(path), 0600); err != nil {
		return fmt.Errorf("write config path file: %w", err)
	}

	return nil
}

func LoadConfigPath() (string, error) {
	configDir, err := DefaultDir()
	if err != nil {
		return "", err
	}

	configPathFile := filepath.Join(configDir, "config_path")
	data, err := os.ReadFile(configPathFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("no config path set. Use 'strix config set <path>' first")
		}
		return "", fmt.Errorf("read config path file: %w", err)
	}

	return string(data), nil
}
