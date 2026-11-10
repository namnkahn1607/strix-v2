// Package client is the thin control-API client used by non-daemon command.
package client

import (
	"bytes"
	"context"
	"encoding/json"
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
	ctx    context.Context
	method string
	uri    string
	body   any
	wait   time.Duration
}

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
	return c.fire(&Request{
		ctx:    ctx,
		method: http.MethodPut,
		uri:    "/v1/config",
		body:   control.ConfigRequest{Path: absPath},
		wait:   5 * time.Second,
	})
}

func (c *Client) Serve(ctx context.Context, precacheFiles []string) error {
	panic("unimplemented")
}

func (c *Client) Stop(ctx context.Context) error {
	return c.fire(&Request{
		ctx:    ctx,
		method: http.MethodPost,
		uri:    "/v1/stop",
		body:   nil,
		wait:   10 * time.Second,
	})
}

func (c *Client) Quit(ctx context.Context) error {
	return c.fire(&Request{
		ctx:    ctx,
		method: http.MethodPost,
		uri:    "/v1/quit",
		body:   nil,
		wait:   10 * time.Second,
	})
}

func (c *Client) fire(req *Request) error {
	var ctx context.Context
	if req.wait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(req.ctx, req.wait)
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
	r, err := http.NewRequestWithContext(ctx, req.method, url, rd)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(r)
	if err != nil {
		return nil
	}
	// What about http.Response.StatusCode != http.StatusOK?

	_ = resp.Body.Close()
	return nil
}
