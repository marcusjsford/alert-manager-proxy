// Package server wires the client-facing and admin handlers.
package server

import (
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/inspect"
	"alertmanager-proxy/internal/passthrough"
	"alertmanager-proxy/internal/telemetry"
	v1 "alertmanager-proxy/internal/v1"
)

// Handler returns the client-facing handler: /api/v1/* goes to the v1
// translation layer, everything else passes through to the upstream.
//
// Dispatch is a plain prefix check rather than http.ServeMux: ServeMux cleans
// paths and redirects (e.g. "//" or "/./"), which would alter pass-through
// requests before the upstream ever saw them.
func Handler(cfg config.Config, logger *slog.Logger, obs *telemetry.Runtime) http.Handler {
	v1Router := v1.NewRouter(v1.Options{
		UpstreamURL:           cfg.UpstreamURL(),
		DialTimeout:           cfg.Upstream.DialTimeout,
		ResponseHeaderTimeout: cfg.Upstream.ResponseHeaderTimeout,
		Logger:                logger,
		Telemetry:             obs,
	})
	opts := passthrough.Options{
		DialTimeout:           cfg.Upstream.DialTimeout,
		ResponseHeaderTimeout: cfg.Upstream.ResponseHeaderTimeout,
		Logger:                logger,
	}
	if obs != nil {
		opts.WrapTransport = obs.Transport
		if obs.Enabled {
			opts.ModifyResponse = func(resp *http.Response) error { inspect.Response(resp, obs); return nil }
		}
	}
	pass := passthrough.New(cfg.UpstreamURL(), opts)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v1.Owns(r.URL.Path) {
			v1Router.ServeHTTP(w, r)
			return
		}
		if obs != nil && obs.Enabled {
			r = r.WithContext(inspect.WithServerSpan(r.Context(), trace.SpanFromContext(r.Context())))
			inspect.Request(r, obs)
		}
		pass.ServeHTTP(w, r)
	})
	if obs != nil {
		return obs.Handler(handler)
	}
	return handler
}

// AdminHandler serves the proxy's own health, independent of the upstream's.
func AdminHandler() http.Handler {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("OK\n"))
	}
	mux.HandleFunc("GET /-/healthy", ok)
	mux.HandleFunc("GET /-/ready", ok)
	return mux
}
