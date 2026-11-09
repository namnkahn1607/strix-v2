package runtime

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPrepare(t *testing.T) {
	base := t.TempDir()

	ok := filepath.Join(base, "ok")
	if err := Prepare(ok); err != nil {
		t.Fatalf("fresh dir: %v", err)
	}

	loose := filepath.Join(base, "loose")
	if err := os.Mkdir(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(loose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(loose); err == nil {
		t.Fatal("mode 0755 must be rejected")
	}

	link := filepath.Join(base, "link")
	if err := os.Symlink(ok, link); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(link); err == nil {
		t.Fatal("symlink must be rejected")
	}

	if err := Prepare("/mnt/strix"); err == nil {
		t.Fatal("/mnt must be rejected")
	}
}

func TestSecondAcquireFailsWithMetadata(t *testing.T) {
	dir := t.TempDir()
	l1, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := l1.WriteMeta(Meta{Pid: 4242, StartedAt: time.Now(), ControlSocket: "/x/control.sock"}); err != nil {
		t.Fatal(err)
	}

	_, err = Acquire(dir)
	var held *HeldError
	if !errors.As(err, &held) || held.Meta.Pid != 4242 {
		t.Fatalf("err = %v, want HeldError with pid 4242", err)
	}
}

func TestLockOutlivesDaemonWhileChildHoldsFD(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}

	child := exec.Command("sleep", "100")
	child.ExtraFiles = []*os.File{l.File()}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()

	_ = l.File().Close() // Simulates the daemon dying: only the child still holds the description.
	var held *HeldError
	if _, err := Acquire(dir); !errors.As(err, &held) {
		t.Fatalf("lock must stay held while a child lives, got %v", err)
	}

	_ = child.Process.Signal(syscall.SIGKILL)
	_ = child.Wait()
	l2, err := Acquire(dir)
	if err != nil {
		t.Fatalf("lock must be free after last holder died: %v", err)
	}
	_ = l2.File().Close()
}

func TestCleanupRemovesStaleOnlyRunArtifacts(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "run-123", "sub"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "control.sock"), nil, 0o600)
	_ = os.WriteFile(filepath.Join(dir, "strix.lock"), nil, 0o600)

	if err := Cleanup(dir); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"run-123", "control.sock"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s should be gone", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "strix.lock")); err != nil {
		t.Fatal("strix.lock must never be unlinked")
	}
}
