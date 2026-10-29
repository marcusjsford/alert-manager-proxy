// Package upstream sends translated v1 operations to the Alertmanager v2 API
// and maps transport failures into the v1 error contract.
package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"alertmanager-proxy/internal/v1/envelope"
)

// Options configures the v2 client.
type Options struct {
	Base                  *url.URL
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	WrapTransport         func(http.RoundTripper) http.RoundTripper
}

// Client makes context-aware requests to one Alertmanager's v2 API.
type Client struct {
	base *url.URL
	http *http.Client
}

// New builds a client with the same dial and response-header timeouts used for
// pass-through traffic.
func New(opts Options) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: opts.DialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = opts.ResponseHeaderTimeout
	var roundTripper http.RoundTripper = transport
	if opts.WrapTransport != nil {
		roundTripper = opts.WrapTransport(transport)
	}
	base := opts.Base
	if base == nil {
		base = &url.URL{}
	}
	return &Client{
		base: base,
		http: &http.Client{
			Transport: roundTripper,
			// Translation must not turn a rejected write into a redirected GET.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Request builds a v2 request. path is the API path below any configured
// upstream prefix; q may carry repeated parameters.
func (c *Client) Request(ctx context.Context, method, path string, body io.Reader, q url.Values) (*http.Request, error) {
	target := *c.base
	target.Path = joinPath(c.base.Path, path)
	target.RawPath = ""
	target.RawQuery = q.Encode()
	target.Fragment = ""
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	return req, nil
}

// ForwardHeaders copies safe end-to-end headers from the client request,
// preserving authentication and body metadata while dropping hop-by-hop
// headers and Accept-Encoding (the transport negotiates and decodes that).
func ForwardHeaders(dst, src http.Header) {
	drop := map[string]bool{
		"Connection": true, "Proxy-Connection": true, "Keep-Alive": true,
		"Proxy-Authenticate": true, "Te": true, "Trailer": true,
		"Transfer-Encoding": true, "Upgrade": true,
		"Accept-Encoding": true, "Content-Length": true, "Host": true,
	}
	for _, value := range src.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			drop[textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))] = true
		}
	}
	for name, values := range src {
		key := textproto.CanonicalMIMEHeaderKey(name)
		if drop[key] {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}

// ResponseError reports a non-2xx v2 response. Status is the status the
// upstream returned; Body is bounded so an endpoint can inspect a known
// upstream error without unbounded buffering.
type ResponseError struct {
	Status int
	Body   []byte
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("upstream returned status %d", e.Status)
}

// DoJSON sends req, requires a 2xx response, and decodes JSON into out when
// out is non-nil. Every non-2xx response is an error, so a failed write can
// never be mistaken for success.
func (c *Client) DoJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return &ResponseError{Status: resp.StatusCode, Body: body}
	}
	if out == nil {
		_, err := io.Copy(io.Discard, resp.Body)
		return err
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode upstream response: %w", err)
	}
	return nil
}

// WriteFailure reports err in the v1 error contract. Endpoint stories map
// their own expected upstream statuses before calling this fallback.
func WriteFailure(w http.ResponseWriter, r *http.Request, err error, logger *slog.Logger) {
	status, errorType, message := classify(err)
	if logger != nil {
		// Deliberately omit the raw error, headers and query: transport errors
		// can contain credential-bearing URLs and client payloads.
		logger.WarnContext(r.Context(), "upstream request failed",
			"method", r.Method, "path", r.URL.Path, "status", status, "error", message)
	}
	envelope.Error(w, status, errorType, message)
}

func classify(err error) (status int, errorType, message string) {
	var responseErr *ResponseError
	switch {
	case errors.As(err, &responseErr):
		return http.StatusBadGateway, "bad_gateway",
			fmt.Sprintf("upstream Alertmanager returned %d", responseErr.Status)
	case errors.Is(err, context.Canceled):
		return 499, "canceled", "client canceled request"
	case isDialError(err):
		return http.StatusBadGateway, "bad_gateway", "upstream Alertmanager unreachable"
	case isTimeout(err):
		return http.StatusGatewayTimeout, "gateway_timeout", "upstream Alertmanager timed out"
	default:
		return http.StatusBadGateway, "bad_gateway", "upstream request failed"
	}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

func joinPath(basePath, path string) string {
	if basePath == "" || basePath == "/" {
		if path == "" {
			return "/"
		}
		if !strings.HasPrefix(path, "/") {
			return "/" + path
		}
		return path
	}
	return strings.TrimSuffix(basePath, "/") + "/" + strings.TrimPrefix(path, "/")
}
