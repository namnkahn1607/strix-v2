package config

import "time"

type Options struct {
	Supervisor SupervisorOptions `yaml:"supervisor"`
	Ipc        IpcOptions        `yaml:"ipc"`
	Gateway    GatewayOptions    `yaml:"gateway"`
	Upstreams  []UpstreamOptions `yaml:"upstreams"`
}

type SupervisorOptions struct {
	Process ProcessOptions `yaml:"process"`
}

type ProcessOptions struct {
	DrainTimeout   time.Duration `yaml:"drain_timeout"`
	GatewayTimeout time.Duration `yaml:"gateway_timeout"`
	EngineTimeout  time.Duration `yaml:"engine_timeout"`
}

type IpcOptions struct {
	SocketAddress string `yaml:"socket_address"`
}

type GatewayOptions struct {
	Http      HttpOptions      `yaml:"http"`
	RpcClient RpcClientOptions `yaml:"rpc_client"`
	Writeback WritebackOptions `yaml:"writeback"`
}

type HttpOptions struct {
	Port          int    `yaml:"port"`
	ProxyEndpoint string `yaml:"proxy_endpoint"`
	MaxReaderSize int    `yaml:"max_reader_size"`
}

type RpcClientOptions struct {
	CheckCacheTimeout time.Duration `yaml:"check_cache_timeout"`
	SetCacheTimeout   time.Duration `yaml:"set_cache_timeout"`
}

type WritebackOptions struct {
	PoolSize    int `yaml:"pool_size"`
	JobQueueCap int `yaml:"job_queue_capacity"`
}

type CachePolicyOptions struct {
	MaxPromptLen int `yaml:"max_prompt_len"`
}

type UpstreamOptions struct {
	Name                  string        `yaml:"name"`
	Type                  string        `yaml:"type"`
	Endpoint              string        `yaml:"endpoint"`
	AuthKind              string        `yaml:"auth_kind"`
	AuthKeyEnvVar         string        `yaml:"auth_key_env_var"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_connections_per_host"`
	IdleConnTimeout       time.Duration `yaml:"idle_connection_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
}
