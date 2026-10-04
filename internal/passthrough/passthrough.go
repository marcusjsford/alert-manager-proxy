// Package passthrough forwards requests to the upstream Alertmanager unchanged.
// It is a first-class path, not a fallback (ADR decision 6): it must never
// rewrite request or response bodies.
package passthrough

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

// Options configures the upstream transport.
type Options struct {
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	Logger                *slog.Logger
}

// New returns a handler that proxies every request to upstream.
func New(upstream *url.URL, opts Options) http.Handler {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: opts.DialTimeout, KeepAlive: 30 * time.Second}).DialContext
	transport.ResponseHeaderTimeout = opts.ResponseHeaderTimeout

	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.SetXForwarded()
		},
		Transport: transport,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status := statusFor(err)
			logger.Warn("upstream request failed",
				"method", r.Method, "path", r.URL.Path, "status", status, "error", err)
			w.WriteHeader(status)
		},
	}
}

// statusFor maps a transport error to the status the client sees: 504 when the
// upstream accepted the connection but was too slow, 502 for everything else
// (refused, unresolvable, reset, or no answer within the dial timeout).
//
// Dial errors are checked first: a dial timeout also satisfies
// errors.Is(err, context.DeadlineExceeded), but the upstream never accepted
// the connection, so it is unreachable (502), not slow (504).
func statusFor(err error) int {
	if isDialError(err) {
		return http.StatusBadGateway
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

func isDialError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}
