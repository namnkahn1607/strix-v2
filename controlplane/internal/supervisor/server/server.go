// Package server is the HTTP/JSON-over-UDS front-end of the Supervisor core. It only translates
// requests into core.Commands; it holds no state of its own.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strix/internal/supervisor/core"
	"strix/internal/wire/control"
	"syscall"
	"time"
)

type Server struct {
	actor *core.Actor
	build string
	http  *http.Server
}

func New(actor *core.Actor, build string) *Server {
	s := &Server{actor: actor, build: build}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", s.status)
	mux.HandleFunc("PUT /v1/config", s.config)
	mux.HandleFunc("POST /v1/serve", s.serve)
	mux.HandleFunc("POST /v1/stop", s.stop)
	mux.HandleFunc("POST /v1/quit", s.quit)
	s.http = &http.Server{
		Handler:           s.withBuild(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Listen binds the control socket with mode 0600. Unlinking a stale path first is safe only
// because the caller holds the instance lock.
func Listen(path string) (net.Listener, error) {
	_ = os.Remove(path)
	// umask is process-global; the daemon has no other goroutines creating files yet.
	old := syscall.Umask(0o177)
	ln, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("chmod control socket: %w", err)
	}
	return &peerCredListener{UnixListener: ln.(*net.UnixListener), uid: uint32(os.Geteuid())}, nil
}

func (s *Server) Serve(ln net.Listener) error {
	if err := s.http.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown waits briefly for in-flight requests (e.g. the quit reply being written).
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

// peerCredListener is defense in depth behind the 0700 dir / 0600 socket: reject peers
// whose uid differs from ours.
type peerCredListener struct {
	*net.UnixListener
	uid uint32
}

func (l *peerCredListener) Accept() (net.Conn, error) {
	for {
		c, err := l.UnixListener.AcceptUnix()
		if err != nil {
			return nil, err
		}
		if peerUID(c) == l.uid {
			return c, nil
		}
		_ = c.Close()
	}
}

func peerUID(c *net.UnixConn) uint32 {
	raw, err := c.SyscallConn()
	if err != nil {
		return ^uint32(0)
	}
	uid := ^uint32(0)
	_ = raw.Control(func(fd uintptr) {
		if cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED); err == nil {
			uid = cred.Uid
		}
	})
	return uid
}

func (s *Server) withBuild(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(control.BuildHeader, s.build)
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, e *control.APIError) {
	code := http.StatusInternalServerError
	switch {
	case e.Code.IsConflict():
		code = http.StatusConflict
	case e.Code == control.CodeBadRequest || e.Code == control.CodeInvalidConfig:
		code = http.StatusBadRequest
	}
	writeJSON(w, code, control.ErrorBody{Error: *e})
}

func badRequest(w http.ResponseWriter, format string, args ...any) {
	writeErr(w, &control.APIError{Code: control.CodeBadRequest, Message: fmt.Sprintf(format, args...)})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	snap := *s.actor.Status()
	snap.Uptime = time.Since(snap.StartedAt).Truncate(time.Second).String()
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) config(w http.ResponseWriter, r *http.Request) {
	var req control.ConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "decode body: %v", err)
		return
	}
	// The daemon's cwd differs from the client's, so only absolute paths are meaningful.
	if !filepath.IsAbs(req.Path) {
		badRequest(w, "config path must be absolute")
		return
	}
	s.simple(w, r, core.Command{Kind: core.KindConfig, Path: req.Path})
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	s.simple(w, r, core.Command{Kind: core.KindStop})
}

func (s *Server) quit(w http.ResponseWriter, r *http.Request) {
	s.simple(w, r, core.Command{Kind: core.KindQuit})
}

func (s *Server) simple(w http.ResponseWriter, r *http.Request, c core.Command) {
	if res := s.actor.Do(r.Context(), c); res.Err != nil {
		writeErr(w, res.Err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	var req control.ServeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "decode body: %v", err)
		return
	}
	if req.Precache != (len(req.Files) > 0) {
		badRequest(w, "precache requires files, and files require precache")
		return
	}
	for _, f := range req.Files {
		if !filepath.IsAbs(f) {
			badRequest(w, "precache file %q must be an absolute path", f)
			return
		}
	}

	// Deliberately not r.Context(): a client disconnect must not cancel serve.
	res := s.actor.Do(context.Background(),
		core.Command{Kind: core.KindServe, Files: req.Files, Precache: req.Precache})
	if res.Err != nil {
		writeErr(w, res.Err)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	flush := func() {
		if fl != nil {
			fl.Flush()
		}
	}
	flush()

	for ev := range res.Session.Events {
		if enc.Encode(ev) != nil {
			// Client gone: keep draining so the actor's non-blocking sends stay cheap.
			continue
		}
		flush()
	}
	if e := res.Session.Err(); e != nil {
		_ = enc.Encode(control.Event{Phase: control.PhaseError, Error: e})
		flush()
	}
}
