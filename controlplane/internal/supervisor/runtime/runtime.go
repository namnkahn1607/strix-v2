// Package runtime owns the daemon's runtime directory and the kernel-enforced instance lock.
package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	controlName = "control.sock"
	lockName    = "strix.lock"
	runPrefix   = "run-"
)

func Dir() string {
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" {
		return filepath.Join(x, "strix")
	}
	return filepath.Join("/tmp", fmt.Sprintf("strix-%d", os.Geteuid()))
}

func ControlSocketPath(dir string) string { return filepath.Join(dir, controlName) }

const ()

// Prepare creates the runtime dir if missing and refuses anything that is not a real
// directory owned by the effective uid with mode exactly 0700.
func Prepare(dir string) error {
	clean := filepath.Clean(dir)
	// flock on WSL2 DrvFs (/mnt/*) is unreliable, and the exclusion guarantee depends on it.
	if clean == "/mnt" || strings.HasPrefix(clean, "/mnt/") {
		return fmt.Errorf("runtime dir %q is under /mnt: flock is unreliable on DrvFs", dir)
	}
	if err := os.Mkdir(clean, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create runtime dir: %w", err)
	}
	fi, err := os.Lstat(clean) // Lstat: a symlink must not be followed.
	if err != nil {
		return fmt.Errorf("stat runtime dir: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("runtime dir %q is not a real directory (symlink or file)", dir)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("runtime dir %q: cannot read ownership", dir)
	}
	if int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("runtime dir %q is owned by uid %d, expected %d", dir, st.Uid, os.Geteuid())
	}
	if fi.Mode().Perm() != 0o700 {
		return fmt.Errorf("runtime dir %q has mode %04o, expected 0700", dir, fi.Mode().Perm())
	}
	return nil
}

// Meta is advisory: the flock, not this file, is authoritative.
type Meta struct {
	Pid           int       `json:"pid"`
	StartedAt     time.Time `json:"started_at"`
	ControlSocket string    `json:"control_socket"`
}

// HeldError means another daemon holds the lock.
type HeldError struct{ Meta Meta }

func (e *HeldError) Error() string {
	if e.Meta.Pid == 0 {
		return "another strix daemon is already running"
	}
	return fmt.Sprintf("another strix daemon is already running (pid %d, since %s, socket %s)",
		e.Meta.Pid, e.Meta.StartedAt.Format(time.RFC3339), e.Meta.ControlSocket)
}

type Lock struct{ f *os.File }

// File is passed to children via ExtraFiles. flock attaches to the open file description,
// so it is released only when the last holder (daemon or any child) exits.
func (l *Lock) File() *os.File { return l.f }

func Acquire(dir string) (*Lock, error) {
	path := filepath.Join(dir, lockName)
	// os.OpenFile adds O_CLOEXEC; ExtraFiles clears it only in the intended children.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		var meta Meta
		if errors.Is(err, syscall.EWOULDBLOCK) {
			if buf, rerr := os.ReadFile(path); rerr == nil {
				_ = json.Unmarshal(buf, &meta)
			}
			_ = f.Close()
			return nil, &HeldError{Meta: meta}
		}
		_ = f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return &Lock{f: f}, nil
}

func (l *Lock) WriteMeta(m Meta) error {
	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := l.f.Truncate(0); err != nil {
		return fmt.Errorf("truncate lock metadata: %w", err)
	}
	if _, err := l.f.WriteAt(buf, 0); err != nil {
		return fmt.Errorf("write lock metadata: %w", err)
	}
	return nil
}

// Cleanup removes leftovers of a previous daemon. Only call while holding the lock; it is
// what makes unlink-then-bind safe. strix.lock itself is never unlinked: removing a held
// lock file would let a new daemon lock a different inode.
func Cleanup(dir string) error {
	var errs []error
	runs, _ := filepath.Glob(filepath.Join(dir, runPrefix+"*"))
	for _, r := range runs {
		if err := os.RemoveAll(r); err != nil {
			errs = append(errs, err)
		}
	}
	if err := os.Remove(ControlSocketPath(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
