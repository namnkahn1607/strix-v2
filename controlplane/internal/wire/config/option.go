package config

import "time"

type Options struct {
	Supervisor SupervisorOptions `yaml:"supervisor" json:"supervisor"`
	Ipc        IpcOptions        `yaml:"ipc" json:"ipc"`
	Gateway    GatewayOptions    `yaml:"gateway" json:"gateway"`
	Engine     EngineOptions     `yaml:"engine" json:"engine"`
	Upstreams  []UpstreamOptions `yaml:"upstreams" json:"upstreams"`
}

type SupervisorOptions struct {
	DrainTimeout time.Duration `yaml:"drain_timeout" json:"drain_timeout"`
}

type IpcOptions struct {
	SocketAddress string `yaml:"socket_address" json:"socket_address"`
}

type GatewayOptions struct {
	ShutdownTimeout time.Duration      `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	Http            HttpOptions        `yaml:"http" json:"http"`
	RpcClient       RpcClientOptions   `yaml:"rpc_client" json:"rpc_client"`
	Writeback       WritebackOptions   `yaml:"writeback" json:"writeback"`
	CachePolicy     CachePolicyOptions `yaml:"cache_policy" json:"cache_policy"`
}

type EngineOptions struct {
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	RpcServer       any           `yaml:"rpc_server" json:"rpc_server"`
	Ivf             any           `yaml:"ivf" json:"ivf"`
	Calibration     any           `yaml:"calibration" json:"calibration"`
	Search          any           `yaml:"search" json:"search"`
	Storage         any           `yaml:"storage" json:"storage"`
}

type UpstreamOptions struct {
	Name                  string        `yaml:"name" json:"name"`
	Type                  string        `yaml:"type" json:"type"`
	Endpoint              string        `yaml:"endpoint" json:"endpoint"`
	AuthKind              string        `yaml:"auth_kind" json:"auth_kind"`
	AuthKeyEnvVar         string        `yaml:"auth_key_env_var" json:"auth_key_env_var"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host" json:"max_idle_conns_per_host"`
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout" json:"idle_conn_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout" json:"response_header_timeout"`
}

type HttpOptions struct {
	Port          int    `yaml:"port" json:"port"`
	ProxyEndpoint string `yaml:"proxy_endpoint" json:"proxy_endpoint"`
	MaxReaderSize int    `yaml:"max_reader_size" json:"max_reader_size"`
}

type RpcClientOptions struct {
	CheckCacheTimeout time.Duration `yaml:"check_cache_timeout" json:"check_cache_timeout"`
	SetCacheTimeout   time.Duration `yaml:"set_cache_timeout" json:"set_cache_timeout"`
}

type WritebackOptions struct {
	PoolSize    int `yaml:"pool_size" json:"pool_size"`
	JobQueueCap int `yaml:"job_queue_capacity" json:"job_queue_capacity"`
}

type CachePolicyOptions struct {
	MaxPromptLen int `yaml:"max_prompt_len" json:"max_prompt_len"`
}
