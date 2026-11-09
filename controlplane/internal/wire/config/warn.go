package config

import "fmt"

const EngineSocketName = "strix.sock"

// Warnings reports non-fatal config problems. Hard errors live in validate.
func (o *Options) Warnings() []string {
	var w []string
	if sum := o.Engine.ShutdownTimeout + o.Gateway.ShutdownTimeout; o.Supervisor.DrainTimeout <= sum {
		w = append(w, fmt.Sprintf(
			"drain_timeout (%s) <= gateway (%s) + engine (%s) shutdown timeouts: "+
				"the daemon ceiling may SIGKILL the engine mid-shutdown",
			o.Supervisor.DrainTimeout, o.Gateway.ShutdownTimeout, o.Engine.ShutdownTimeout))
	}
	if o.Ipc.SocketAddress != "" {
		w = append(w, fmt.Sprintf(
			"ipc.socket_address (%q) is ignored: the daemon places %s inside its per-serve run dir",
			o.Ipc.SocketAddress, EngineSocketName))
	}
	return w
}
