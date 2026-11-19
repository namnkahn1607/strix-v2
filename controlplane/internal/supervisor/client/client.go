// Package client is the thin control-API client used by non-daemon command.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strix/internal/wire/control"
	"time"
)

type Client struct {
	http *http.Client
}

type Request struct {
	method  string
	uri     string
	body    any
	timeout time.Duration
}

// Extractor receives *http.Response and returns T + error.
type Extractor[T any] func(resp *http.Response) (T, error)

func New(sock string) *Client {
	const dialTimeout = 2 * time.Second

	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: dialTimeout}
			return d.DialContext(ctx, "unix", sock)
		},
		DisableKeepAlives: true,
	}
	return &Client{http: &http.Client{Transport: tr}}
}

func (c *Client) SetConfig(ctx context.Context, absPath string) error {
	return c.FireForget(ctx, &Request{
		method:  http.MethodPut,
		uri:     "/v1/config",
		body:    control.ConfigRequest{Path: absPath},
		timeout: 5 * time.Second,
	})
}

func (c *Client) Serve(ctx context.Context, precacheFiles []string) error {
	panic("unimplemented")
}

func (c *Client) Stop(ctx context.Context) error {
	return c.FireForget(ctx, &Request{
		method:  http.MethodPost,
		uri:     "/v1/stop",
		body:    nil,
		timeout: 10 * time.Second,
	})
}

func (c *Client) Quit(ctx context.Context) error {
	return c.FireForget(ctx, &Request{
		method:  http.MethodPost,
		uri:     "/v1/quit",
		body:    nil,
		timeout: 10 * time.Second,
	})
}

func (c *Client) FireForget(ctx context.Context, req *Request) error {
	return c.execute(ctx, req, nil)
}

func (c *Client) Do[T any](ctx context.Context, req *Request, extract Extractor[T]) (T, error) {
	var zero T
	if extract != nil {
		return zero, errors.New("extractor cannot be null; use Client.FireForget instead")
	}

	var result T
	err := c.execute(ctx, req, func(resp *http.Response) error {
		var extractErr error
		result, extractErr = extract(resp)
		return extractErr
	})
	if err != nil {
		return zero, err
	}
	return result, nil
}

func (c *Client) execute(ctx context.Context, req *Request, handleResp func(*http.Response) error) error {
	if req.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.timeout)
		defer cancel()
	}

	var rd io.Reader
	if req.body != nil {
		b, err := json.Marshal(req.body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}

	var url = "http://strix" + req.uri
	httpReq, err := http.NewRequestWithContext(ctx, req.method, url, rd)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return classify(err)
	}
	if resp.StatusCode != http.StatusOK {
		return decodeErr(resp)
	}

	if handleResp != nil {
		return handleResp(resp)
	}

	_ = resp.Body.Close()
	return nil
}

func decodeErr(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	var eb control.ErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&eb); err != nil || eb.Error.Code == "" {
		return &Error{
			Code: control.ExitGeneral,
			Err:  fmt.Errorf("daemon returned HTTP %d", resp.StatusCode),
		}
	}
	return &Error{Code: control.ExitCodeFor(eb.Error.Code), Err: &eb.Error}
}

func classify(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{
			Code: control.ExitTimeout,
			Err:  fmt.Errorf("timed out waiting for the daemon"),
		}
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
