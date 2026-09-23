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
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	opts.applyDefaults()

	if err := opts.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &opts, nil
}

func (o *Options) applyDefaults() {
	if o.Supervisor.DrainTimeout == 0 {
		o.Supervisor.DrainTimeout = 6 * time.Second
	}
	if o.Gateway.ShutdownTimeout == 0 {
		o.Gateway.ShutdownTimeout = 5 * time.Second
	}
	if o.Engine.ShutdownTimeout == 0 {
		o.Engine.ShutdownTimeout = 3 * time.Second
	}

	if o.Gateway.RpcClient.CheckCacheTimeout == 0 {
		o.Gateway.RpcClient.CheckCacheTimeout = 50 * time.Millisecond
	}
	if o.Gateway.RpcClient.SetCacheTimeout == 0 {
		o.Gateway.RpcClient.SetCacheTimeout = 50 * time.Millisecond
	}
	if o.Gateway.Http.ProxyEndpoint == "" {
		o.Gateway.Http.ProxyEndpoint = "/v1/cache/strix"
	}
	if o.Gateway.Http.Port == 0 {
		o.Gateway.Http.Port = 8080
	}
}

func (o *Options) validate() error {
	if o.Engine.ShutdownTimeout >= o.Gateway.ShutdownTimeout {
		return fmt.Errorf(
			"invalid config: engine_timeout (%s) must be smaller than gateway_timeout (%s)",
			o.Engine.ShutdownTimeout, o.Gateway.ShutdownTimeout,
		)
	}
	if o.Gateway.ShutdownTimeout > o.Supervisor.DrainTimeout {
		return fmt.Errorf(
			"invalid config: gateway timeout (%s) cannot be greater than drain_timeout (%s)",
			o.Gateway.ShutdownTimeout, o.Supervisor.DrainTimeout,
		)
	}

	if o.Gateway.Writeback.PoolSize <= 0 {
		return errors.New("invalid config: gateway.writeback.pool_size is not set")
	}
	if o.Gateway.Writeback.JobQueueCap <= 0 {
		return errors.New("invalid config: gateway.writeback.job_queue_cap is not set")
	}
	if o.Gateway.Http.MaxReaderSize <= 0 {
		return errors.New("invalid config: gateway.http.max_reader_size is not set")
	}

	const maxUpstreams = 8
	if len(o.Upstreams) == 0 {
		return errors.New("invalid config: at least 1 upstream are required")
	}
	if len(o.Upstreams) > maxUpstreams {
		return fmt.Errorf("invalid config: at most %d upstreams are supported", maxUpstreams)
	}

	return nil
}
