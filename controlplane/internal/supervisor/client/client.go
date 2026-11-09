// Package client is the thin control-API client used by non-daemon command.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

type Client struct {
	http *http.Client
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

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}

	var url = "http://strix" + path
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
