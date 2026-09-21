package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/goccy/go-yaml"
)

func Parse(path string) (*Options, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var opts Options
	if err := yaml.Unmarshal(buf, &opts); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	opts.applyDefaults()

	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &opts, nil
}

func (o *Options) applyDefaults() {
	if o.Supervisor.Process.DrainTimeout == 0 {
		o.Supervisor.Process.DrainTimeout = 6 * time.Second
	}
	if o.Supervisor.Process.GatewayTimeout == 0 {
		o.Supervisor.Process.GatewayTimeout = 5 * time.Second
	}
	if o.Supervisor.Process.EngineTimeout == 0 {
		o.Supervisor.Process.EngineTimeout = 3 * time.Second
	}

	if o.Gateway.RpcClient.CheckCacheTimeout == 0 {
		o.Gateway.RpcClient.CheckCacheTimeout = 50 * time.Millisecond
	}
	if o.Gateway.RpcClient.SetCacheTimeout == 0 {
		o.Gateway.RpcClient.SetCacheTimeout = 50 * time.Millisecond
	}
	if o.Gateway.Network.ProxyEndpoint == "" {
		o.Gateway.Network.ProxyEndpoint = "/v1/cache/strix"
	}
	if o.Gateway.Network.Port == 0 {
		o.Gateway.Network.Port = 8080
	}
}

func (o *Options) validate() error {
	if o.Supervisor.Process.EngineTimeout >= o.Supervisor.Process.GatewayTimeout {
		return fmt.Errorf(
			"invalid config: engine_timeout (%s) must be smaller than gateway_timeout (%s)",
			o.Supervisor.Process.EngineTimeout, o.Supervisor.Process.GatewayTimeout,
		)
	}
	if o.Supervisor.Process.GatewayTimeout > o.Supervisor.Process.DrainTimeout {
		return fmt.Errorf(
			"invalid config: gateway timeout (%s) cannot be greater than drain_timeout (%s)",
			o.Supervisor.Process.GatewayTimeout, o.Supervisor.Process.GatewayTimeout,
		)
	}

	if o.Gateway.Writeback.PoolSize <= 0 {
		return errors.New("invalid config: gateway.writeback.pool_size not found")
	}
	if o.Gateway.Network.MaxReaderSize <= 0 {
		return errors.New("invalid config: gateway.network.max_reader_size not found")
	}

	return nil
}
