package config

type GatewayEnvelop struct {
	Ipc       IpcOptions        `json:"ipc"`
	Gateway   GatewayOptions    `json:"gateway"`
	Upstreams []UpstreamOptions `json:"upstreams"`
}

type EngineEnvelop struct {
	Ipc    IpcOptions    `json:"ipc"`
	Engine EngineOptions `json:"engine"`
}
