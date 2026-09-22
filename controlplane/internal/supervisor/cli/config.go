package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strix/internal/supervisor/config"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure Strix runtime behavior",
}

var configSetCmd = &cobra.Command{
	Use:   "set <path>",
	Short: "Set the path to Strix configuration file",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Parse and display current configuration",
	RunE:  runConfigGet,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
}

func runConfigSet(_ *cobra.Command, args []string) error {
	configPath := args[0]

	if _, err := os.Stat(configPath); err != nil {
		return fmt.Errorf("config file not found: %w", err)
	}

	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}

	if _, err := config.Parse(absPath); err != nil {
		return err
	}

	if err := saveConfigPath(absPath); err != nil {
		return fmt.Errorf("save config path: %w", err)
	}

	fmt.Printf("Configuration path set to: %s\n", absPath)
	return nil
}

func runConfigGet(_ *cobra.Command, _ []string) error {
	configPath, err := loadConfigPath()
	if err != nil {
		return fmt.Errorf("load config path: %w", err)
	}

	opts, err := config.Parse(configPath)
	if err != nil {
		return err
	}

	fmt.Printf("Configuration file: %s\n\n", configPath)
	fmt.Println("=== Supervisor ===")
	fmt.Printf("  Drain timeout:   %s\n", opts.Supervisor.Process.DrainTimeout)
	fmt.Printf("  Gateway timeout: %s\n", opts.Supervisor.Process.GatewayTimeout)
	fmt.Printf("  Engine timeout:  %s\n", opts.Supervisor.Process.EngineTimeout)
	fmt.Println()

	fmt.Println("=== IPC ===")
	fmt.Printf("  Socket address: %s\n", opts.Ipc.SocketAddress)
	fmt.Println()

	fmt.Println("=== Gateway ===")
	fmt.Printf("  Port:                     %d\n", opts.Gateway.Http.Port)
	fmt.Printf("  Proxy endpoint:           %s\n", opts.Gateway.Http.ProxyEndpoint)
	fmt.Printf("  Max reader size:          %d bytes\n", opts.Gateway.Http.MaxReaderSize)
	fmt.Printf("  Writeback pool:           %d workers\n", opts.Gateway.Writeback.PoolSize)
	fmt.Printf("  Writeback queue capacity: %d jobs\n", opts.Gateway.Writeback.JobQueueCap)
	fmt.Println()

	fmt.Println("=== RPC Client ===")
	fmt.Printf("  CheckCache timeout: %s\n", opts.Gateway.RpcClient.CheckCacheTimeout)
	fmt.Printf("  SetCache timeout:   %s\n", opts.Gateway.RpcClient.SetCacheTimeout)
	fmt.Println()

	fmt.Println("=== Upstreams ===")
	for _, upstr := range opts.Upstreams {
		fmt.Printf("  Name:                     %s\n", upstr.Name)
		fmt.Printf("  Type:                     %s\n", upstr.Type)
		fmt.Printf("  Endpoint:                 %s\n", upstr.Endpoint)
		fmt.Printf("  Auth kind:                %s\n", upstr.AuthKind)
		fmt.Printf("  Auth key env var:         %s\n", upstr.AuthKeyEnvVar)
		fmt.Printf("  Max idle conns per host:  %d\n", upstr.MaxIdleConnsPerHost)
		fmt.Printf("  Idle conn timeout:        %s\n", upstr.IdleConnTimeout)
		fmt.Printf("  Response header timeout:  %s\n", upstr.ResponseHeaderTimeout)
		fmt.Println()
	}

	return nil
}

// Returns the directory storing Strix configuration.
// Follows XDG Base Directory specification.
func getConfigDir() (string, error) {
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

// Persists the configuration file path to disk.
func saveConfigPath(path string) error {
	configDir, err := getConfigDir()
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

// Retrieves the persisted configuration file path.
func loadConfigPath() (string, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return "", err
	}

	configPathFile := filepath.Join(configDir, "config_path")
	data, err := os.ReadFile(configPathFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no config path set. Use 'strix config set <path>' first")
		}
		return "", fmt.Errorf("read config path file: %w", err)
	}

	return string(data), nil
}
