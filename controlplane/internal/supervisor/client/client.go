// Package client is the thin control-API client used by every non-daemon strix command.
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strix/internal/wire/control"
	"time"
)

const (
	dialTimeout = 2 * time.Second
	// Daemon ceiling is 10s by default, but is configurable; this margin is added to
	// whatever the status endpoint cannot tell us, so stop/quit wait generously.
	shutdownWait = 10*time.Second + 5*time.Second
)

// Error carries the exit code the CLI must return.
type Error struct {
	Code int
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type Client struct {
	http   *http.Client
	build  string
	warned bool
	Warn   io.Writer
}

func New(sock string) *Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: dialTimeout}
			return d.DialContext(ctx, "unix", sock)
		},
		DisableKeepAlives: true,
	}
	return &Client{http: &http.Client{Transport: tr}, build: control.CurrentBuildID(), Warn: os.Stderr}
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://strix"+path, rd)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, classify(err)
	}
	if got := resp.Header.Get(control.BuildHeader); got != "" && got != c.build && !c.warned {
		c.warned = true
		fmt.Fprintf(c.Warn, "warning: daemon build %s differs from client build %s (restart the daemon?)\n", got, c.build)
	}

	return resp, nil
}

func decodeErr(resp *http.Response) error {
	defer resp.Body.Close()
	var eb control.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&eb); err != nil || eb.Error.Code == "" {
		return &Error{Code: control.ExitGeneral, Err: fmt.Errorf("daemon returned HTTP %d", resp.StatusCode)}
	}
	return &Error{Code: control.ExitCodeFor(eb.Error.Code), Err: &eb.Error}
}

func classify(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: control.ExitTimeout, Err: fmt.Errorf("timed out waiting for the daemon")}
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return &Error{Code: control.ExitNoDaemon, Err: noDaemon(err)}
	}

	var oe *net.OpError
	if errors.As(err, &oe) && oe.Op == "dial" {
		return &Error{Code: control.ExitNoDaemon, Err: noDaemon(err)}
	}

	return &Error{Code: control.ExitGeneral, Err: err}
}

func noDaemon(err error) error {
	return fmt.Errorf("strix daemon is not running (run `make strix`): %w", err)
}

func (c *Client) SetConfig(ctx context.Context, absPath string) error {
	return c.simple(ctx, http.MethodPut, "/v1/config", control.ConfigRequest{Path: absPath}, 5*time.Second)
}

func (c *Client) simple(ctx context.Context, method, path string, body any, wait time.Duration) error {
	if wait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, wait)
		defer cancel()
	}
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return decodeErr(resp)
	}
	_ = resp.Body.Close()
	return nil
}

func (c *Client) Status(ctx context.Context) (*control.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout+3*time.Second)
	defer cancel()
	resp, err := c.do(ctx, http.MethodGet, "/v1/status", nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeErr(resp)
	}
	defer resp.Body.Close()
	var s control.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, &Error{Code: control.ExitGeneral, Err: err}
	}
	return &s, nil
}

func (c *Client) Stop(ctx context.Context) error {
	return c.simple(ctx, http.MethodPost, "/v1/stop", nil, shutdownWait)
}

func (c *Client) Quit(ctx context.Context) error {
	return c.simple(ctx, http.MethodPost, "/v1/quit", nil, shutdownWait)
}

// Serve streams progress events to onEvent until the stream ends. Cancelling ctx only stops
// waiting; the daemon keeps serving in the background.
func (c *Client) Serve(ctx context.Context, req control.ServeRequest, onEvent func(control.Event)) error {
	resp, err := c.do(ctx, http.MethodPost, "/v1/serve", req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return decodeErr(resp)
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var ev control.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			return &Error{Code: control.ExitGeneral, Err: fmt.Errorf("bad event: %w", err)}
		}
		onEvent(ev)
		switch {
		case ev.Phase == control.PhaseError && ev.Error != nil:
			return &Error{Code: control.ExitCodeFor(ev.Error.Code), Err: ev.Error}
		case ev.Phase == control.PhaseRunning:
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return classify(err)
	}
	return &Error{Code: control.ExitGeneral, Err: errors.New("daemon closed the stream before serve finished")}
}
