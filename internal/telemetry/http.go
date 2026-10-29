package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"alertmanager-proxy/internal/clientid"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

type stateKey struct{}

type requestState struct {
	identity clientid.Identity
	route    string
	version  string
	method   string
	mu       sync.Mutex
	params   []attribute.KeyValue
	upgraded atomic.Bool
}

// Route maps arbitrary paths into a finite set of templates without cleaning paths.
func Route(path string) (version, route string) {
	for _, api := range []string{"v1", "v2"} {
		prefix := "/api/" + api
		if path != prefix && !strings.HasPrefix(path, prefix+"/") {
			continue
		}
		tail := strings.TrimPrefix(path, prefix)
		switch tail {
		case "/alerts", "/silences", "/status", "/receivers":
			return api, prefix + tail
		case "/alerts/groups":
			if api == "v2" {
				return api, prefix + tail
			}
		}
		if strings.HasPrefix(tail, "/silence/") && len(tail) > len("/silence/") && !strings.Contains(strings.TrimPrefix(tail, "/silence/"), "/") {
			return api, prefix + "/silence/{id}"
		}
		return api, prefix + "/*"
	}
	switch path {
	case "/", "/-/healthy", "/-/ready", "/metrics":
		return "other", path
	default:
		return "other", "/*"
	}
}

func methodName(method string) string {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		return method
	default:
		return "_OTHER"
	}
}

func state(ctx context.Context) *requestState {
	if s, ok := ctx.Value(stateKey{}).(*requestState); ok {
		return s
	}
	return &requestState{identity: clientid.Identity{Name: "unknown"}, version: "other", route: "/*", method: "_OTHER"}
}

func (s *requestState) spanAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("client.name", s.identity.Name), attribute.String("client.address", s.identity.Address),
		attribute.String("user_agent.original", s.identity.UserAgent), attribute.String("am.api.version", s.version),
		attribute.String("http.route", s.route),
	}
}

func (r *Runtime) labels(s *requestState) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("client", s.identity.Name), attribute.String("api_version", s.version),
		attribute.String("endpoint", s.route), attribute.String("method", s.method), attribute.String("instance", r.Instance),
	}
}

func (r *Runtime) httpOptions() []otelhttp.Option {
	return []otelhttp.Option{
		otelhttp.WithTracerProvider(r.traces), otelhttp.WithPropagators(r.prop),
		otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
			s := state(req.Context())
			return s.method + " " + s.route
		}),
	}
}

// Handler adds identity, traces, bounded metrics and one correlated access log.
// httpsnoop preserves Flusher/Hijacker/etc., important to transparent forwarding.
func (r *Runtime) Handler(next http.Handler) http.Handler {
	if !r.Enabled {
		return next
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		s := state(req.Context())
		trace.SpanFromContext(req.Context()).SetAttributes(r.redactor.attributes(s.spanAttributes())...)
		result := httpsnoop.Metrics{Code: http.StatusOK}
		defer func() {
			aborted := recover()
			if aborted != nil {
				r.Parameters(req.Context(), attribute.Bool("am.request.aborted", true))
				trace.SpanFromContext(req.Context()).RecordError(errors.New("request aborted"))
				trace.SpanFromContext(req.Context()).SetStatus(codes.Error, "request aborted")
				if result.Code == http.StatusOK && result.Written == 0 {
					result.Code = 0 // no successful response was observed
				}
			}
			if s.upgraded.Load() {
				result.Code = http.StatusSwitchingProtocols
			}
			r.completed(req, s, result)
			if aborted != nil {
				panic(aborted) // preserve net/http's abort/panic semantics
			}
		}()
		result.CaptureMetrics(w, func(w http.ResponseWriter) { next.ServeHTTP(w, req) })
	})
	traced := otelhttp.NewHandler(inner, "alertmanager-proxy", r.httpOptions()...)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		version, route := Route(req.URL.Path)
		s := &requestState{identity: r.Resolver.Resolve(req), version: version, route: route, method: methodName(req.Method)}
		traced.ServeHTTP(w, req.WithContext(context.WithValue(req.Context(), stateKey{}, s)))
	})
}

func (r *Runtime) completed(req *http.Request, s *requestState, result httpsnoop.Metrics) {
	trace.SpanFromContext(req.Context()).SetAttributes(attribute.Int("am.response.status_code", result.Code))
	labels := append(r.labels(s), attribute.String("status_code", strconv.Itoa(result.Code)))
	r.requests.Add(req.Context(), 1, metric.WithAttributes(labels...))
	r.duration.Record(req.Context(), result.Duration.Seconds(), metric.WithAttributes(labels...))
	attrs := []slog.Attr{
		slog.String("client.name", s.identity.Name), slog.String("client.address", s.identity.Address),
		slog.String("user_agent.original", s.identity.UserAgent), slog.String("am.api.version", s.version),
		slog.String("http.route", s.route), slog.String("method", s.method),
		slog.Int("status_code", result.Code), slog.Float64("duration_seconds", result.Duration.Seconds()),
	}
	s.mu.Lock()
	for _, kv := range s.params {
		attrs = append(attrs, slog.Any(string(kv.Key), kv.Value.AsInterface()))
	}
	s.mu.Unlock()
	r.Logger.LogAttrs(req.Context(), slog.LevelInfo, "request completed", attrs...)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// Transport observes an upstream call, including body consumption, without buffering.
func (r *Runtime) Transport(base http.RoundTripper) http.RoundTripper {
	if !r.Enabled {
		return base
	}
	inner := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		trace.SpanFromContext(req.Context()).SetAttributes(r.redactor.attributes(state(req.Context()).spanAttributes())...)
		return base.RoundTrip(req)
	})
	traced := otelhttp.NewTransport(inner, r.httpOptions()...)
	return roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		started := time.Now()
		resp, err := traced.RoundTrip(req)
		code := 0
		if resp != nil {
			code = resp.StatusCode
			if code == http.StatusSwitchingProtocols {
				state(req.Context()).upgraded.Store(true)
			}
		}
		labels := append(r.labels(state(req.Context())), attribute.String("status_code", strconv.Itoa(code)))
		finished := func() {
			r.upstream.Record(req.Context(), time.Since(started).Seconds(), metric.WithAttributes(labels...))
		}
		if err != nil || resp == nil || resp.Body == nil || resp.Body == http.NoBody {
			finished()
		} else {
			observed := &observedBody{ReadCloser: resp.Body, finished: finished}
			if duplex, ok := resp.Body.(io.ReadWriteCloser); ok {
				observed.duplex = true
				resp.Body = &observedDuplex{observedBody: observed, writer: duplex}
			} else {
				resp.Body = observed
			}
		}
		return resp, err
	})
}

type observedBody struct {
	io.ReadCloser
	once     sync.Once
	finished func()
	duplex   bool
}

func (b *observedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil && !b.duplex {
		b.once.Do(b.finished)
	}
	return n, err
}

type observedDuplex struct {
	*observedBody
	writer io.Writer
}

func (b *observedDuplex) Write(p []byte) (int, error) { return b.writer.Write(p) }

func (b *observedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.finished)
	return err
}

// Parameters adds redacted detail to the span and access log, never metric labels.
func (r *Runtime) Parameters(ctx context.Context, attrs ...attribute.KeyValue) {
	if !r.Enabled {
		return
	}
	attrs = r.redactor.attributes(attrs)
	trace.SpanFromContext(ctx).SetAttributes(attrs...)
	s := state(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, kv := range attrs {
		if len(s.params) < 128 {
			s.params = append(s.params, kv)
		}
	}
}

// Lossy reports an explicit translation decision; field/reason are span-only.
func (r *Runtime) Lossy(ctx context.Context, field, reason string) {
	if !r.Enabled {
		return
	}
	trace.SpanFromContext(ctx).AddEvent("am.translation.lossy", trace.WithAttributes(
		attribute.String("field", field), attribute.String("reason", reason)))
	r.lossy.Add(ctx, 1, metric.WithAttributes(r.labels(state(ctx))...))
}

// Alerts records parsed received alert counts for translation/inspection stories.
func (r *Runtime) Alerts(ctx context.Context, count int64) {
	if !r.Enabled || count < 0 {
		return
	}
	r.Parameters(ctx, attribute.Int64("am.alerts.count", count))
	if count > 0 {
		r.alerts.Add(ctx, count, metric.WithAttributes(r.labels(state(ctx))...))
	}
}
