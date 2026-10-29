// Package v1 serves the Alertmanager API v1 surface, translated to v2.
//
// The router owns the shared v1 HTTP contract recorded from Alertmanager
// v0.26.0: method dispatch, CORS/preflight, unknown paths/methods, and the
// response envelopes. Endpoint stories replace a route's 501 handler.
package v1

import (
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"alertmanager-proxy/internal/telemetry"
	"alertmanager-proxy/internal/v1/envelope"
	"alertmanager-proxy/internal/v1/upstream"
)

// Prefix is the path prefix owned by this package.
const Prefix = "/api/v1"

const (
	alertsPath    = Prefix + "/alerts"
	silencesPath  = Prefix + "/silences"
	silencePrefix = Prefix + "/silence/"
	statusPath    = Prefix + "/status"
	receiversPath = Prefix + "/receivers"
)

// Options configures the v1 layer.
type Options struct {
	UpstreamURL           *url.URL
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	Logger                *slog.Logger
	Telemetry             *telemetry.Runtime
}

// Owns reports whether path belongs to the v1 API.
func Owns(path string) bool {
	return path == Prefix || strings.HasPrefix(path, Prefix+"/")
}

type route struct {
	match    func(string) bool
	handlers map[string]http.HandlerFunc
}

// Router dispatches v1 requests to translated handlers.
type Router struct {
	obs      *telemetry.Runtime
	upstream *upstream.Client
	logger   *slog.Logger
	routes   []route
}

// NewRouter returns the v1 handler.
func NewRouter(opts Options) *Router {
	logger := opts.Logger
	wrap := (func(http.RoundTripper) http.RoundTripper)(nil)
	if opts.Telemetry != nil {
		logger = opts.Telemetry.Logger
		wrap = opts.Telemetry.Transport
	}
	if logger == nil {
		logger = slog.Default()
	}
	r := &Router{obs: opts.Telemetry, logger: logger}
	// The client is shared by the endpoint stories that replace the nil
	// handlers. Keeping it here centralises auth forwarding, timeouts and the
	// 030 transport instrumentation.
	r.upstream = upstream.New(upstream.Options{
		Base:                  opts.UpstreamURL,
		DialTimeout:           opts.DialTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
		WrapTransport:         wrap,
	})
	r.routes = r.defaultRoutes()
	return r
}

// Upstream returns the shared v2 client used by endpoint handlers.
func (r *Router) Upstream() *upstream.Client {
	return r.upstream
}

// Obs exposes bounded telemetry to endpoint handlers.
func (r *Router) Obs() *telemetry.Runtime {
	return r.obs
}

// defaultRoutes declares methods separately so 040 can translate POST alerts
// while 050 still owns GET, each handler replacing only its own nil entry.
func (r *Router) defaultRoutes() []route {
	return []route{
		{exact(alertsPath), map[string]http.HandlerFunc{
			http.MethodGet: r.getAlerts, http.MethodPost: r.postAlerts,
		}},
		{exact(silencesPath), map[string]http.HandlerFunc{
			http.MethodGet: r.getSilences, http.MethodPost: r.postSilences,
		}},
		{silenceID, map[string]http.HandlerFunc{
			http.MethodGet: r.getSilence, http.MethodDelete: r.deleteSilence,
		}},
		{exact(statusPath), map[string]http.HandlerFunc{
			http.MethodGet: r.getStatus,
		}},
		{exact(receiversPath), map[string]http.HandlerFunc{
			http.MethodGet: r.getReceivers,
		}},
	}
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path
	if req.Method == http.MethodOptions {
		preflight(w, path)
		return
	}
	if rt, ok := r.find(path); ok {
		handler, allowed := rt.handlers[req.Method]
		if !allowed {
			allow(w, rt.methods())
			return
		}
		if handler == nil {
			r.notImplemented(w, req)
			return
		}
		handler(w, req)
		return
	}
	if trimmed, ok := r.trailingSlashTarget(path); ok {
		if req.URL.RawQuery != "" {
			trimmed += "?" + req.URL.RawQuery
		}
		// trimmed matched a known /api/v1 route, so the Location is a same-origin path.
		http.Redirect(w, req, trimmed, http.StatusMovedPermanently) //nolint:gosec // same-origin path built from the matched route
		return
	}
	http.Error(w, "404 page not found", http.StatusNotFound)
}

func (r *Router) find(path string) (route, bool) {
	for _, rt := range r.routes {
		if rt.match(path) {
			return rt, true
		}
	}
	return route{}, false
}

func (r *Router) trailingSlashTarget(path string) (string, bool) {
	if !strings.HasSuffix(path, "/") {
		return "", false
	}
	trimmed := strings.TrimSuffix(path, "/")
	if _, ok := r.find(trimmed); ok {
		return trimmed, true
	}
	return "", false
}

func preflight(w http.ResponseWriter, path string) {
	// v0.26's wildcard OPTIONS route required the trailing slash for the bare
	// prefix and redirected the slashless form.
	if path == Prefix {
		w.Header().Set("Location", Prefix+"/")
		w.WriteHeader(http.StatusTemporaryRedirect)
		return
	}
	envelope.Preflight(w)
}

func (rt route) methods() []string {
	methods := make([]string, 0, len(rt.handlers))
	for method := range rt.handlers {
		methods = append(methods, method)
	}
	return methods
}

func allow(w http.ResponseWriter, methods []string) {
	all := append(append([]string(nil), methods...), http.MethodOptions)
	sort.Strings(all)
	w.Header().Set("Allow", strings.Join(all, ", "))
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

func exact(path string) func(string) bool {
	return func(candidate string) bool { return candidate == path }
}

func silenceID(path string) bool {
	if !strings.HasPrefix(path, silencePrefix) {
		return false
	}
	id := strings.TrimPrefix(path, silencePrefix)
	return id != "" && !strings.Contains(id, "/")
}

func (r *Router) notImplemented(w http.ResponseWriter, req *http.Request) {
	r.logger.WarnContext(req.Context(), "v1 operation not translated",
		"method", req.Method, "path", req.URL.Path)
	envelope.Error(w, http.StatusNotImplemented, "not_implemented",
		"alertmanager-proxy: "+req.Method+" "+req.URL.Path+" is not translated to API v2 yet")
}
