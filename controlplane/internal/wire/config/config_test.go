package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func valid() *Options {
	o := &Options{}
	o.Engine.ModelDirectory = "m"
	o.Gateway.Http.MaxReaderSize = 1
	o.Gateway.Writeback.PoolSize = 1
	o.Gateway.CachePolicy.MaxPromptLen = 1
	o.Upstreams = []UpstreamOptions{{Name: "p", Type: "t", Endpoint: "e", AuthKind: "none"}}
	o.applyDefaults()
	return o
}

func TestShutdownTimeoutValidation(t *testing.T) {
	cases := []struct {
		name              string
		eng, gw, drain    time.Duration
		wantErr, wantWarn bool
	}{
		{"defaults", 3 * time.Second, 5 * time.Second, 10 * time.Second, false, false},
		{"engine == gateway", 5 * time.Second, 5 * time.Second, 20 * time.Second, true, false},
		{"engine > gateway", 6 * time.Second, 5 * time.Second, 20 * time.Second, true, false},
		{"gateway > drain", 3 * time.Second, 11 * time.Second, 10 * time.Second, true, false},
		{"gateway == drain is allowed but warns", 3 * time.Second, 10 * time.Second, 10 * time.Second, false, true},
		{"sum == drain warns", 3 * time.Second, 5 * time.Second, 8 * time.Second, false, true},
		{"sum > drain warns", 4 * time.Second, 5 * time.Second, 8 * time.Second, false, true},
		{"sum < drain quiet", 3 * time.Second, 5 * time.Second, 9 * time.Second, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := valid()
			o.Engine.ShutdownTimeout, o.Gateway.ShutdownTimeout, o.Supervisor.DrainTimeout = c.eng, c.gw, c.drain
			err := o.validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("validate err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil {
				return
			}
			warned := false
			for _, w := range o.Warnings() {
				warned = warned || strings.Contains(w, "drain_timeout")
			}
			if warned != c.wantWarn {
				t.Fatalf("warned = %v, want %v (%v)", warned, c.wantWarn, o.Warnings())
			}
		})
	}
}

func TestDefaultTimeoutsAreConsistent(t *testing.T) {
	o := valid()
	if err := o.validate(); err != nil {
		t.Fatal(err)
	}
	for _, w := range o.Warnings() {
		t.Errorf("defaults must not warn: %s", w)
	}
}

func TestIpcSocketAddressIgnoredWithWarning(t *testing.T) {
	o := valid()
	o.Ipc.SocketAddress = "unix:///tmp/x.sock"
	found := false
	for _, w := range o.Warnings() {
		found = found || strings.Contains(w, "ipc.socket_address")
	}
	if !found {
		t.Fatal("expected a warning about the ignored socket address")
	}
}

func TestSaveConfigPathAtomicAndNoTempLeft(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	if err := SaveConfigPath("/a.yaml"); err != nil {
		t.Fatal(err)
	}
	if err := SaveConfigPath("/b.yaml"); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfigPath()
	if err != nil || got != "/b.yaml" {
		t.Fatalf("got %q, %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(home, "strix"))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}
