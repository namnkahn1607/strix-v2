package proc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"strix/internal/wire/config"
	"strix/internal/wire/control"
	"syscall"
	"testing"
	"time"
)

const scriptTmpl = `#!/bin/bash
read -r cfg <&3
if [ "%[2]s" = engine ]; then
  addr=${cfg#*unix://}; addr=${addr%%%%\"*}; touch "$addr"
fi
trap '%[1]s' TERM
echo "$$ %[2]s-start" >> "$LOG"
while true; do sleep 0.05; done
`

func writeScript(t *testing.T, dir, name, onTerm string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	body := fmt.Sprintf(scriptTmpl, onTerm, name)
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func newPlane(t *testing.T, gwTerm, engTerm string) (*Plane, string, *os.File) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "order.log")
	t.Setenv("LOG", log)
	lock, err := os.Create(filepath.Join(dir, "strix.lock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	return &Plane{
		RuntimeDir: dir,
		Lock:       lock,
		GatewayBin: writeScript(t, dir, "gateway", gwTerm),
		EngineBin:  writeScript(t, dir, "engine", engTerm),
	}, log, lock
}

func opts(gw, eng, ceiling time.Duration) *config.Options {
	var o config.Options
	o.Gateway.ShutdownTimeout, o.Engine.ShutdownTimeout, o.Supervisor.DrainTimeout = gw, eng, ceiling
	return &o
}

func startPlane(t *testing.T, p *Plane, o *config.Options) *running {
	t.Helper()
	r, err := p.Start(context.Background(), o, func(control.Event) {})
	if err != nil {
		t.Fatal(err)
	}
	return r.(*running)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(3 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("condition not met in time")
}

func TestStopOrderGatewayBeforeEngine(t *testing.T) {
	p, log, _ := newPlane(t,
		`echo gw-term >> "$LOG"; sleep 0.3; echo gw-exit >> "$LOG"; exit 0`,
		`echo eng-term >> "$LOG"; exit 0`)
	r := startPlane(t, p, opts(2*time.Second, time.Second, 5*time.Second))
	waitFor(t, func() bool { b, _ := os.ReadFile(log); return strings.Count(string(b), "-start") == 2 })

	if err := r.Stop(); err != nil {
		t.Fatalf("clean stop must not escalate: %v", err)
	}
	b, _ := os.ReadFile(log)
	var got []string
	for _, l := range strings.Fields(string(b)) {
		if strings.HasSuffix(l, "term") || strings.HasSuffix(l, "exit") {
			got = append(got, l)
		}
	}
	want := "gw-term gw-exit eng-term"
	if strings.Join(got, " ") != want {
		t.Fatalf("shutdown order = %v, want %s", got, want)
	}
	if _, err := os.Stat(r.runDir); !os.IsNotExist(err) {
		t.Fatalf("run dir not removed: %v", err)
	}
}

func TestTimeoutEscalatesToSigkill(t *testing.T) {
	p, log, _ := newPlane(t, `exit 0`, ``) // Engine: empty trap action → TERM is swallowed.
	r := startPlane(t, p, opts(time.Second, 300*time.Millisecond, 5*time.Second))
	waitFor(t, func() bool { b, _ := os.ReadFile(log); return strings.Count(string(b), "-start") == 2 })

	begin := time.Now()
	err := r.Stop()
	took := time.Since(begin)
	if err == nil || !strings.Contains(err.Error(), "engine") {
		t.Fatalf("expected engine SIGKILL error, got %v", err)
	}
	if took < 300*time.Millisecond || took > 2*time.Second {
		t.Fatalf("escalation took %s, want ~300ms", took)
	}
}

func TestCeilingBoundsTotalShutdown(t *testing.T) {
	p, log, _ := newPlane(t, ``, ``) // Both swallow TERM.
	r := startPlane(t, p, opts(10*time.Second, time.Second, 400*time.Millisecond))
	waitFor(t, func() bool { b, _ := os.ReadFile(log); return strings.Count(string(b), "-start") == 2 })

	begin := time.Now()
	_ = r.Stop()
	if took := time.Since(begin); took > 2*time.Second {
		t.Fatalf("ceiling not enforced: %s", took)
	}
}

func TestUnexpectedChildDeathSignalsExited(t *testing.T) {
	p, log, _ := newPlane(t, `exit 0`, `exit 0`)
	r := startPlane(t, p, opts(time.Second, 500*time.Millisecond, 5*time.Second))
	waitFor(t, func() bool { b, _ := os.ReadFile(log); return strings.Count(string(b), "-start") == 2 })

	_ = syscall.Kill(r.gateway.pid, syscall.SIGKILL)
	select {
	case <-r.Exited():
	case <-time.After(3 * time.Second):
		t.Fatal("unexpected death not reported")
	}
	if r.Err() == nil {
		t.Fatal("Err must describe the death")
	}
	_ = r.Stop()
}

func TestChildrenInheritLockFDAndOwnProcessGroup(t *testing.T) {
	p, log, lock := newPlane(t, `exit 0`, `exit 0`)
	r := startPlane(t, p, opts(time.Second, 500*time.Millisecond, 5*time.Second))
	defer func() { _ = r.Stop() }()
	waitFor(t, func() bool { b, _ := os.ReadFile(log); return strings.Count(string(b), "-start") == 2 })

	want, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", lock.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []*child{r.gateway, r.engine} {
		got, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%d", c.pid, instanceLockFD))
		if err != nil || got != want {
			t.Fatalf("%s fd %d = %q (err %v), want %q", c.name, instanceLockFD, got, err, want)
		}
		pgid, err := syscall.Getpgid(c.pid)
		if err != nil || pgid == syscall.Getpgrp() {
			t.Fatalf("%s shares the daemon's process group (pgid %d, err %v)", c.name, pgid, err)
		}
	}
}
