// Package api is the typed client of the Pipetower REST API (v1).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pipetower/pipetower-cli/internal/version"
)

// Client talks to one Pipetower instance.
type Client struct {
	instance string
	token    string
	http     *http.Client
}

// New creates a client for the instance URL (without /api/v1) and token.
func New(instance, token string) *Client {
	return &Client{
		instance: strings.TrimRight(instance, "/"),
		token:    token,
		http:     newHTTPClient(),
	}
}

// newHTTPClient creates a client which waits at most 30 s for an answer (not for a whole download)
// and does not follow redirects: they would turn a POST into a GET.
func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second

	return &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// Instance returns the URL of the Pipetower instance.
func (c *Client) Instance() string {
	return c.instance
}

// Endpoint describes one API request.
type Endpoint struct {
	Method string
	Path   string
	Query  url.Values
	Body   any
}

// JSON performs the request and returns the raw JSON response body.
func (c *Client) JSON(ctx context.Context, e Endpoint) (json.RawMessage, error) {
	status, body, err := c.send(ctx, e, "application/json")
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, newError(c.instance, e, status, body)
	}
	return body, nil
}

// Text performs the request and returns the plain text response body.
func (c *Client) Text(ctx context.Context, e Endpoint) (string, error) {
	status, body, err := c.send(ctx, e, "text/plain, application/json")
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", newError(c.instance, e, status, body)
	}
	return string(body), nil
}

// Raw performs the request without interpreting the status code.
func (c *Client) Raw(ctx context.Context, e Endpoint) (int, []byte, error) {
	return c.send(ctx, e, "application/json")
}

// Get performs the request and decodes the JSON response into T.
func Get[T any](ctx context.Context, c *Client, e Endpoint) (T, error) {
	var value T
	body, err := c.JSON(ctx, e)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return value, fmt.Errorf("unexpected response from %s: %w", e.Path, err)
	}
	return value, nil
}

// send executes the HTTP request and reads the whole body.
func (c *Client) send(ctx context.Context, e Endpoint, accept string) (int, []byte, error) {
	req, err := c.request(ctx, e, accept)
	if err != nil {
		return 0, nil, err
	}

	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, &ConnectionError{URL: c.instance, Err: err}
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 && res.StatusCode < 400 {
		return 0, nil, &RedirectError{URL: c.instance, Location: res.Header.Get("Location")}
	}
	body, err := io.ReadAll(res.Body)
	return res.StatusCode, body, err
}

// request builds the authenticated HTTP request.
func (c *Client) request(ctx context.Context, e Endpoint, accept string) (*http.Request, error) {
	body, err := encodeBody(e.Body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, e.Method, c.url(e), body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", version.UserAgent())
	if e.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// url builds the absolute URL of the endpoint.
func (c *Client) url(e Endpoint) string {
	u := c.instance + "/api/v1/" + strings.TrimLeft(e.Path, "/")
	if len(e.Query) > 0 {
		u += "?" + e.Query.Encode()
	}
	return u
}

// encodeBody encodes the request body as JSON, raw bytes are sent as they are.
func encodeBody(body any) (io.Reader, error) {
	switch value := body.(type) {
	case nil:
		return nil, nil
	case []byte:
		return bytes.NewReader(value), nil
	default:
		encoded, err := json.Marshal(value)
		return bytes.NewReader(encoded), err
	}
}
