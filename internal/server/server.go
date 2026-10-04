// Package server wires the client-facing and admin handlers.
package server

import (
	"log/slog"
	"net/http"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/passthrough"
	v1 "alertmanager-proxy/internal/v1"
)

// Handler returns the client-facing handler: /api/v1/* goes to the v1
// translation layer, everything else passes through to the upstream.
//
// Dispatch is a plain prefix check rather than http.ServeMux: ServeMux cleans
// paths and redirects (e.g. "//" or "/./"), which would alter pass-through
// requests before the upstream ever saw them.
func Handler(cfg config.Config, logger *slog.Logger) http.Handler {
	v1Router := v1.NewRouter()
	pass := passthrough.New(cfg.UpstreamURL(), passthrough.Options{
		DialTimeout:           cfg.Upstream.DialTimeout,
		ResponseHeaderTimeout: cfg.Upstream.ResponseHeaderTimeout,
		Logger:                logger,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v1.Owns(r.URL.Path) {
			v1Router.ServeHTTP(w, r)
			return
		}
		pass.ServeHTTP(w, r)
	})
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
