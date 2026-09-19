package upstream

import "time"

type Config struct {
	Name                  string        `yaml:"name"`
	Type                  string        `yaml:"type"` // "anthropic" | "openai" | "google"
	Endpoint              string        `yaml:"endpoint"`
	AuthKind              string        `yaml:"auth_kind"`
	AuthKeyEnvVar         string        `yaml:"auth_key_env_var"`
	MaxIdleConnsPerHost   int           `yaml:"max_idle_conns_per_host"`
	IdleConnTimeout       time.Duration `yaml:"idle_conn_timeout"`
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
}
