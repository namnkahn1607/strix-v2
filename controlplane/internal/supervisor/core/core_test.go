package core

import (
	"context"
	"errors"
	"strix/internal/wire/config"
	"strix/internal/wire/control"
	"sync"
	"testing"
	"time"
)

type fakeConfig struct {
	ptr      string
	parseErr error
	saved    string
}

func (f *fakeConfig) Pointer() (string, error) {
	if f.ptr == "" {
		return "", errors.New("no config path set")
	}
	return f.ptr, nil
}
func (f *fakeConfig) Parse(string) (*config.Options, error) {
	if f.parseErr != nil {
		return nil, f.parseErr
	}
	return &config.Options{}, nil
}
func (f *fakeConfig) Save(p string) error { f.saved = p; return nil }

type fakeRun struct {
	exited  chan struct{}
	once    sync.Once
	stops   int
	stopped chan struct{}
}

func newFakeRun() *fakeRun {
	return &fakeRun{exited: make(chan struct{}), stopped: make(chan struct{}, 4)}
}
func (r *fakeRun) Pids() control.Pids      { return control.Pids{Gateway: 2, Engine: 1} }
func (r *fakeRun) Exited() <-chan struct{} { return r.exited }
func (r *fakeRun) Err() error              { return errors.New("boom") }
func (r *fakeRun) Stop() error             { r.stops++; r.stopped <- struct{}{}; return nil }
func (r *fakeRun) die()                    { r.once.Do(func() { close(r.exited) }) }

type fakePlane struct {
	run     *fakeRun
	mk      func() Running
	startFn func(ctx context.Context) error
}

func (p *fakePlane) Start(ctx context.Context, _ *config.Options, _ func(control.Event)) (Running, error) {
	if p.startFn != nil {
		if err := p.startFn(ctx); err != nil {
			return nil, err
		}
	}
	if p.mk != nil {
		return p.mk(), nil
	}
	return p.run, nil
}

type fakePre struct {
	fn func(ctx context.Context) error
}

func (p fakePre) Run(ctx context.Context, _ *config.Options, _ []string, _ func(int, int)) error {
	return p.fn(ctx)
}

func start(t *testing.T, cfg *fakeConfig, pl *fakePlane, pre Precacher) *Actor {
	t.Helper()
	a := New(Deps{Config: cfg, Plane: pl, Precacher: pre})
	go a.Run()
	return a
}

func wait(t *testing.T, a *Actor, want control.State) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a.Status().State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("state = %s, want %s", a.Status().State, want)
}

func do(a *Actor, k Kind) Result { return a.Do(context.Background(), Command{Kind: k}) }

func drain(t *testing.T, s *Session) *control.APIError {
	t.Helper()
	for range s.Events {
	}
	return s.Err()
}

func TestIdleCommands(t *testing.T) {
	cfg := &fakeConfig{ptr: "/c.yaml"}
	a := start(t, cfg, &fakePlane{run: newFakeRun()}, nil)

	if r := do(a, KindStop); r.Err != nil {
		t.Fatalf("stop in Idle must be a no-op OK, got %v", r.Err)
	}
	r := a.Do(context.Background(), Command{Kind: KindConfig, Path: "/new.yaml"})
	if r.Err != nil || cfg.saved != "/new.yaml" || a.Status().ConfigPath != "/new.yaml" {
		t.Fatalf("config in Idle: err=%v saved=%q", r.Err, cfg.saved)
	}
	cfg.parseErr = errors.New("bad yaml")
	r = a.Do(context.Background(), Command{Kind: KindConfig, Path: "/bad.yaml"})
	if r.Err == nil || r.Err.Code != control.CodeInvalidConfig || cfg.saved != "/new.yaml" {
		t.Fatalf("invalid config must be rejected without touching the pointer: %v saved=%q", r.Err, cfg.saved)
	}
	if r := do(a, KindQuit); r.Err != nil {
		t.Fatal(r.Err)
	}
	<-a.Done()
}

func TestServeLifecycleAndConflicts(t *testing.T) {
	run := newFakeRun()
	a := start(t, &fakeConfig{ptr: "/c.yaml"}, &fakePlane{run: run}, nil)

	r := do(a, KindServe)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if e := drain(t, r.Session); e != nil {
		t.Fatalf("serve failed: %v", e)
	}
	wait(t, a, control.StateRunning)

	if r := do(a, KindServe); r.Err == nil || r.Err.Code != control.CodeAlreadyServing {
		t.Fatalf("serve in Running: %v", r.Err)
	}
	if r := a.Do(context.Background(), Command{Kind: KindConfig, Path: "/x"}); r.Err == nil || r.Err.Code != control.CodeNotIdle {
		t.Fatalf("config in Running: %v", r.Err)
	}
	if r := do(a, KindStop); r.Err != nil {
		t.Fatal(r.Err)
	}
	wait(t, a, control.StateIdle)
	if run.stops != 1 {
		t.Fatalf("stops = %d", run.stops)
	}
	if r := do(a, KindStop); r.Err != nil { // idempotent
		t.Fatal(r.Err)
	}
}

func TestStopCancelsStarting(t *testing.T) {
	run := newFakeRun()
	pre := fakePre{fn: func(ctx context.Context) error { <-ctx.Done(); return context.Cause(ctx) }}
	a := start(t, &fakeConfig{ptr: "/c.yaml"}, &fakePlane{run: run}, pre)

	r := a.Do(context.Background(), Command{Kind: KindServe, Precache: true, Files: nil})
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	wait(t, a, control.StateStarting)

	if r := do(a, KindServe); r.Err == nil || r.Err.Code != control.CodeAlreadyServing {
		t.Fatalf("serve in Starting: %v", r.Err)
	}
	if r := do(a, KindStop); r.Err != nil {
		t.Fatal(r.Err)
	}
	if e := drain(t, r.Session); e == nil || e.Code != control.CodeCancelled {
		t.Fatalf("serve result = %v, want cancelled", e)
	}
	if st := a.Status(); st.State != control.StateIdle || st.LastError != "" {
		t.Fatalf("after cancel: %+v", st)
	}
	if run.stops != 1 {
		t.Fatalf("children must be reaped once, got %d", run.stops)
	}
}

func TestInvalidConfigAtServeStopsBeforeSpawn(t *testing.T) {
	spawned := false
	pl := &fakePlane{run: newFakeRun(), startFn: func(context.Context) error { spawned = true; return nil }}
	cfg := &fakeConfig{ptr: "/c.yaml", parseErr: errors.New("bad")}
	a := start(t, cfg, pl, nil)

	r := do(a, KindServe)
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	if e := drain(t, r.Session); e == nil || e.Code != control.CodeInvalidConfig {
		t.Fatalf("got %v", e)
	}
	if spawned {
		t.Fatal("serve spawned a child despite invalid config")
	}
	wait(t, a, control.StateIdle)
	if a.Status().LastError == "" {
		t.Fatal("last_error not recorded")
	}
}

func TestMissingPointerKeepsDaemonIdleWithConfigError(t *testing.T) {
	a := start(t, &fakeConfig{}, &fakePlane{run: newFakeRun()}, nil)
	if a.Status().ConfigError == "" || a.Status().State != control.StateIdle {
		t.Fatalf("status = %+v", a.Status())
	}
	r := do(a, KindServe)
	if e := drain(t, r.Session); e == nil || e.Code != control.CodeInvalidConfig {
		t.Fatalf("got %v", e)
	}
}

func TestChildDeathIsFailStop(t *testing.T) {
	run := newFakeRun()
	a := start(t, &fakeConfig{ptr: "/c.yaml"}, &fakePlane{run: run}, nil)
	r := do(a, KindServe)
	drain(t, r.Session)
	wait(t, a, control.StateRunning)

	run.die()
	wait(t, a, control.StateIdle)
	if a.Status().LastError == "" || run.stops != 1 {
		t.Fatalf("status=%+v stops=%d", a.Status(), run.stops)
	}
	if r := do(a, KindServe); r.Err != nil { // can serve again, no auto-restart happened
		t.Fatal(r.Err)
	}
}

func TestServeDuringStoppingRejectedAndStopWaits(t *testing.T) {
	run := newFakeRun()
	block := make(chan struct{})
	slow := &slowStopRun{fakeRun: run, block: block}
	a := start(t, &fakeConfig{ptr: "/c.yaml"}, &fakePlane{mk: func() Running { return slow }}, nil)

	r := do(a, KindServe)
	drain(t, r.Session)
	wait(t, a, control.StateRunning)

	stopDone := make(chan Result, 2)
	go func() { stopDone <- do(a, KindStop) }()
	wait(t, a, control.StateStopping)

	if r := do(a, KindServe); r.Err == nil || r.Err.Code != control.CodeStopping {
		t.Fatalf("serve in Stopping: %v", r.Err)
	}
	go func() { stopDone <- do(a, KindStop) }() // second stop waits, does not error
	select {
	case <-stopDone:
		t.Fatal("stop returned before shutdown finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(block)
	for range 2 {
		if r := <-stopDone; r.Err != nil {
			t.Fatal(r.Err)
		}
	}
	wait(t, a, control.StateIdle)
}

type slowStopRun struct {
	*fakeRun
	block chan struct{}
}

func (r *slowStopRun) Stop() error { <-r.block; return nil }

func TestQuitStopsDataPlane(t *testing.T) {
	run := newFakeRun()
	a := start(t, &fakeConfig{ptr: "/c.yaml"}, &fakePlane{run: run}, nil)
	drain(t, do(a, KindServe).Session)
	wait(t, a, control.StateRunning)

	if r := do(a, KindQuit); r.Err != nil {
		t.Fatal(r.Err)
	}
	<-a.Done()
	if run.stops != 1 {
		t.Fatalf("stops = %d", run.stops)
	}
	if r := do(a, KindServe); r.Err == nil || r.Err.Code != control.CodeStopping {
		t.Fatalf("serve after quit: %v", r.Err)
	}
}

func TestConcurrentServeOneWins(t *testing.T) {
	a := start(t, &fakeConfig{ptr: "/c.yaml"}, &fakePlane{run: newFakeRun()}, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, conflict := 0, 0
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := do(a, KindServe)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case r.Err == nil:
				ok++
			case r.Err.Code == control.CodeAlreadyServing:
				conflict++
			}
		}()
	}
	wg.Wait()
	if ok != 1 || conflict != 15 {
		t.Fatalf("ok=%d conflict=%d", ok, conflict)
	}
}
