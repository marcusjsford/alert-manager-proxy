package telemetry_test

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/passthrough"
	"alertmanager-proxy/internal/server"
	"alertmanager-proxy/internal/telemetry"

	"go.opentelemetry.io/otel/attribute"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type capture struct {
	mu      sync.Mutex
	spans   []*tracepb.ResourceSpans
	metrics []*metricspb.ResourceMetrics
	logs    []*logspb.ResourceLogs
	failed  atomic.Bool
}

type tracesServer struct {
	collectortrace.UnimplementedTraceServiceServer
	capture *capture
}

func (s tracesServer) Export(_ context.Context, req *collectortrace.ExportTraceServiceRequest) (*collectortrace.ExportTraceServiceResponse, error) {
	if s.capture.failed.Load() {
		return nil, status.Error(codes.Unavailable, "collector unavailable")
	}
	s.capture.mu.Lock()
	defer s.capture.mu.Unlock()
	s.capture.spans = append(s.capture.spans, proto.Clone(req).(*collectortrace.ExportTraceServiceRequest).ResourceSpans...)
	return &collectortrace.ExportTraceServiceResponse{}, nil
}

type metricsServer struct {
	collectormetrics.UnimplementedMetricsServiceServer
	capture *capture
}

func (s metricsServer) Export(_ context.Context, req *collectormetrics.ExportMetricsServiceRequest) (*collectormetrics.ExportMetricsServiceResponse, error) {
	if s.capture.failed.Load() {
		return nil, status.Error(codes.Unavailable, "collector unavailable")
	}
	s.capture.mu.Lock()
	defer s.capture.mu.Unlock()
	s.capture.metrics = append(s.capture.metrics, proto.Clone(req).(*collectormetrics.ExportMetricsServiceRequest).ResourceMetrics...)
	return &collectormetrics.ExportMetricsServiceResponse{}, nil
}

type logsServer struct {
	collectorlogs.UnimplementedLogsServiceServer
	capture *capture
}

func (s logsServer) Export(_ context.Context, req *collectorlogs.ExportLogsServiceRequest) (*collectorlogs.ExportLogsServiceResponse, error) {
	if s.capture.failed.Load() {
		return nil, status.Error(codes.Unavailable, "collector unavailable")
	}
	s.capture.mu.Lock()
	defer s.capture.mu.Unlock()
	s.capture.logs = append(s.capture.logs, proto.Clone(req).(*collectorlogs.ExportLogsServiceRequest).ResourceLogs...)
	return &collectorlogs.ExportLogsServiceResponse{}, nil
}

func collector(t testing.TB) (string, *capture) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	c := &capture{}
	srv := grpc.NewServer()
	collectortrace.RegisterTraceServiceServer(srv, tracesServer{capture: c})
	collectormetrics.RegisterMetricsServiceServer(srv, metricsServer{capture: c})
	collectorlogs.RegisterLogsServiceServer(srv, logsServer{capture: c})
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)
	return "http://" + ln.Addr().String(), c
}

func httpCollector(t testing.TB) (string, *capture) {
	t.Helper()
	c := &capture{}
	mux := http.NewServeMux()
	for _, signal := range []string{"traces", "metrics", "logs"} {
		mux.HandleFunc("POST /v1/"+signal, func(w http.ResponseWriter, r *http.Request) {
			var reader io.Reader = r.Body
			if r.Header.Get("Content-Encoding") == "gzip" {
				compressed, err := gzip.NewReader(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				defer compressed.Close()
				reader = compressed
			}
			raw, _ := io.ReadAll(reader)
			var response proto.Message
			switch signal {
			case "traces":
				var request collectortrace.ExportTraceServiceRequest
				if err := proto.Unmarshal(raw, &request); err != nil {
					t.Error(err)
				}
				response, _ = (tracesServer{capture: c}).Export(r.Context(), &request)
			case "metrics":
				var request collectormetrics.ExportMetricsServiceRequest
				if err := proto.Unmarshal(raw, &request); err != nil {
					t.Error(err)
				}
				response, _ = (metricsServer{capture: c}).Export(r.Context(), &request)
			case "logs":
				var request collectorlogs.ExportLogsServiceRequest
				if err := proto.Unmarshal(raw, &request); err != nil {
					t.Error(err)
				}
				response, _ = (logsServer{capture: c}).Export(r.Context(), &request)
			}
			encoded, _ := proto.Marshal(response)
			w.Header().Set("Content-Type", "application/x-protobuf")
			_, _ = w.Write(encoded)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL, c
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(raw []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(raw)
}

func (b *lockedBuffer) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.String()
}

func observer(t testing.TB, cfg config.Config, output io.Writer) *telemetry.Runtime {
	t.Helper()
	obs, err := telemetry.New(context.Background(), cfg, "test-version", output)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = obs.Shutdown(ctx)
	})
	return obs
}

func values(attrs []*commonpb.KeyValue) map[string]string {
	out := map[string]string{}
	for _, attr := range attrs {
		out[attr.Key] = attr.Value.GetStringValue()
	}
	return out
}

func TestIdentitySignalsPropagationAndRedaction(t *testing.T) {
	for _, protocol := range []string{"grpc", "http/protobuf"} {
		t.Run(protocol, func(t *testing.T) { identitySignals(t, protocol) })
	}
}

func identitySignals(t *testing.T, protocol string) {
	makeCollector := collector
	if protocol == "http/protobuf" {
		makeCollector = httpCollector
	}
	endpoint, exported := makeCollector(t)
	var jsonLogs lockedBuffer
	var upstreamTraceparents []string
	var upstreamMu sync.Mutex
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMu.Lock()
		upstreamTraceparents = append(upstreamTraceparents, r.Header.Get("Traceparent"))
		upstreamMu.Unlock()
		if r.URL.Query().Get("access_token") != "QUERY_SECRET" || r.Header.Get("Authorization") == "" {
			t.Error("instrumentation changed the query or Authorization sent upstream")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"original":"bytes"}` {
			t.Errorf("request body changed: %s", body)
		}
		w.Header().Set("X-Upstream", "preserved")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"upstream":"unchanged"}`))
	}))
	defer upstream.Close()
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-test-1"
	cfg.Telemetry.Protocol = protocol
	cfg.Clients.Rules = []config.ClientRule{
		{Name: "cidr-client", CIDRs: []string{"10.10.0.0/16"}},
		{Name: "ua-client", UserAgent: `^LegacyScript/`},
		{Name: "auth-client", BasicAuthUser: "alice"},
		{Name: "header-client", Header: "X-Client", HeaderValue: "dashboard"},
	}
	obs := observer(t, cfg, &jsonLogs)
	pass := passthrough.New(cfg.UpstreamURL(), passthrough.Options{
		Logger: obs.Logger, DialTimeout: time.Second, ResponseHeaderTimeout: time.Second, WrapTransport: obs.Transport,
	})
	handler := obs.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obs.Parameters(r.Context(), attribute.String("am.v1.filter", `app="demo"`),
			attribute.String("Authorization", "HELPER_SECRET"), attribute.String("am.silence.comment", "COMMENT_SECRET"),
			attribute.String("am.silence.createdBy", "AUTHOR_SECRET"))
		obs.Alerts(r.Context(), 3)
		obs.Lossy(r.Context(), "timestamp", "millisecond precision")
		obs.Logger.WarnContext(r.Context(), "request diagnostic", "Authorization", "LOG_SECRET")
		pass.ServeHTTP(w, r)
	}))
	cases := []struct{ name, address, ua, user, header string }{
		{"cidr-client", "10.10.1.7:123", "CIDR/1", "", ""},
		{"ua-client", "192.0.2.1:123", "LegacyScript/1", "", ""},
		{"auth-client", "192.0.2.2:123", "Basic/1", "alice", ""},
		{"header-client", "192.0.2.3:123", "Dashboard/1", "", "dashboard"},
		{"unknown", "192.0.2.4:123", "Unmatched/1", "", ""},
	}
	secrets := []string{"HEADER_SECRET", "BASIC_SECRET", "QUERY_SECRET", "HELPER_SECRET", "COMMENT_SECRET", "AUTHOR_SECRET", "LOG_SECRET", "Authorization"}
	for _, tc := range cases {
		req := httptest.NewRequest("POST", "/api/v2/alerts?access_token=QUERY_SECRET&filter=app%3D%22demo%22", strings.NewReader(`{"original":"bytes"}`))
		req.RemoteAddr = tc.address
		req.Header.Set("User-Agent", tc.ua)
		req.Header.Set("Authorization", "Bearer HEADER_SECRET")
		if tc.user != "" {
			req.SetBasicAuth(tc.user, "BASIC_SECRET")
		}
		secrets = append(secrets, req.Header.Get("Authorization"))
		req.Header.Set("X-Client", tc.header)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusCreated || recorder.Body.String() != `{"upstream":"unchanged"}` || recorder.Header().Get("X-Upstream") != "preserved" {
			t.Fatal("instrumentation changed the upstream response")
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := obs.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	exported.mu.Lock()
	defer exported.mu.Unlock()
	servers, clients := map[string]*tracepb.Span{}, map[string]*tracepb.Span{}
	for _, resource := range exported.spans {
		res := values(resource.Resource.Attributes)
		if res["service.instance.id"] != "am-test-1" || res["service.version"] != "test-version" {
			t.Fatalf("wrong trace resource: %v", res)
		}
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				name := values(span.Attributes)["client.name"]
				switch span.Kind {
				case tracepb.Span_SPAN_KIND_SERVER:
					servers[name] = span
				case tracepb.Span_SPAN_KIND_CLIENT:
					clients[name] = span
				}
			}
		}
	}
	metricClients, logClients := map[string]bool{}, map[string]bool{}
	counters := map[string]map[string]int64{}
	for _, resource := range exported.metrics {
		for _, scope := range resource.ScopeMetrics {
			for _, m := range scope.Metrics {
				if !strings.HasPrefix(m.Name, "am_proxy_") {
					t.Fatalf("unexpected automatic metric %s", m.Name)
				}
				var attrs [][]*commonpb.KeyValue
				for _, point := range m.GetSum().GetDataPoints() {
					attrs = append(attrs, point.Attributes)
					if counters[m.Name] == nil {
						counters[m.Name] = map[string]int64{}
					}
					counters[m.Name][values(point.Attributes)["client"]] = point.GetAsInt()
				}
				for _, point := range m.GetHistogram().GetDataPoints() {
					attrs = append(attrs, point.Attributes)
				}
				for _, labels := range attrs {
					for key := range values(labels) {
						switch key {
						case "client", "api_version", "endpoint", "method", "status_code", "instance":
						default:
							t.Fatalf("unapproved metric label %q", key)
						}
					}
					if m.Name == "am_proxy_requests_total" {
						metricClients[values(labels)["client"]] = true
					}
				}
			}
		}
	}
	for _, resource := range exported.logs {
		for _, scope := range resource.ScopeLogs {
			for _, record := range scope.LogRecords {
				attrs := values(record.Attributes)
				if attrs["trace_id"] != hex.EncodeToString(record.TraceId) || attrs["span_id"] != hex.EncodeToString(record.SpanId) {
					t.Fatalf("OTLP log correlation mismatch: %v", attrs)
				}
				if record.Body.GetStringValue() == "request completed" {
					name := attrs["client.name"]
					logClients[name] = true
					if span := servers[name]; span == nil || !bytes.Equal(span.SpanId, record.SpanId) {
						t.Fatalf("access log does not correlate with %s server span", name)
					}
				}
			}
		}
	}
	for i, tc := range cases {
		server, client := servers[tc.name], clients[tc.name]
		if server == nil || client == nil || !bytes.Equal(server.TraceId, client.TraceId) || !bytes.Equal(client.ParentSpanId, server.SpanId) {
			t.Fatalf("missing/mismatched trace pair for %s", tc.name)
		}
		upstreamMu.Lock()
		traceparent := upstreamTraceparents[i]
		upstreamMu.Unlock()
		if !strings.Contains(traceparent, hex.EncodeToString(client.TraceId)+"-"+hex.EncodeToString(client.SpanId)) {
			t.Fatalf("upstream traceparent does not describe %s client span", tc.name)
		}
		if !metricClients[tc.name] || !logClients[tc.name] {
			t.Fatalf("identity missing from metrics/logs for %s", tc.name)
		}
		if counters["am_proxy_requests_total"][tc.name] != 1 || counters["am_proxy_alerts_received_total"][tc.name] != 3 || counters["am_proxy_translation_lossy_total"][tc.name] != 1 {
			t.Fatalf("wrong request/alert/lossy counts for %s: %v", tc.name, counters)
		}
		found := false
		for _, event := range server.Events {
			found = found || event.Name == "am.translation.lossy"
		}
		if !found {
			t.Fatalf("missing lossy event for %s", tc.name)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(jsonLogs.text()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil || record["trace_id"] == nil || record["span_id"] == nil {
			t.Fatalf("uncorrelated JSON request log: %s", line)
		}
	}
	raw, _ := json.Marshal([]any{exported.spans, exported.metrics, exported.logs, jsonLogs.text()})
	for _, secret := range secrets {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("exported sensitive data %q", secret)
		}
	}
}

func TestBoundedMetricValuesAndOptionalRedaction(t *testing.T) {
	endpoint, exported := collector(t)
	cfg := config.Defaults()
	cfg.Upstream.URL = "http://unused:9093"
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-cardinality"
	cfg.Redaction.SilenceComment, cfg.Redaction.SilenceCreatedBy = false, false
	cfg.Redaction.Headers = []string{"X-API-Key", "User-Agent"}
	var output lockedBuffer
	obs := observer(t, cfg, &output)
	handler := obs.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obs.Parameters(r.Context(), attribute.String("am.silence.comment", "visible-comment"),
			attribute.String("am.silence.createdBy", "visible-author"), attribute.String("http.request.header.x_api_key", "APIKEY_SECRET"),
			attribute.String("url.full", "https://url-user:URL_PASSWORD@example.test/a?token=URL_QUERY_SECRET"))
		w.WriteHeader(http.StatusNoContent)
	}))
	for i := range 25 {
		req := httptest.NewRequest(fmt.Sprintf("CUSTOM_%d", i), fmt.Sprintf("/api/v2/silence/id-%d?filter=matcher-%d", i, i), nil)
		req.RemoteAddr = fmt.Sprintf("203.0.113.%d:1234", i+1)
		req.Header.Set("User-Agent", fmt.Sprintf("UA_SECRET_%d", i))
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := obs.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	exported.mu.Lock()
	defer exported.mu.Unlock()
	found := false
	for _, resource := range exported.metrics {
		for _, scope := range resource.ScopeMetrics {
			for _, m := range scope.Metrics {
				if m.Name != "am_proxy_requests_total" {
					continue
				}
				points := m.GetSum().GetDataPoints()
				if len(points) != 1 {
					t.Fatalf("dynamic inputs created %d metric series", len(points))
				}
				attrs := values(points[0].Attributes)
				if attrs["method"] != "_OTHER" || attrs["endpoint"] != "/api/v2/silence/{id}" || attrs["client"] != "unknown" {
					t.Fatalf("unbounded metric attributes: %v", attrs)
				}
				found = points[0].GetAsInt() == 25
			}
		}
	}
	if !found {
		t.Fatal("bounded request counter not exported")
	}
	raw, _ := json.Marshal([]any{exported.spans, exported.logs, output.text()})
	if !bytes.Contains(raw, []byte("visible-comment")) || !bytes.Contains(raw, []byte("visible-author")) {
		t.Fatal("optional silence redaction cannot be disabled")
	}
	for _, secret := range []string{"APIKEY_SECRET", "UA_SECRET_", "URL_PASSWORD", "URL_QUERY_SECRET", "url-user"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("redaction leaked %s", secret)
		}
	}
	metrics, _ := json.Marshal(exported.metrics)
	for _, unbounded := range []string{"203.0.113.", "CUSTOM_", "id-", "matcher-", "UA_SECRET_"} {
		if bytes.Contains(metrics, []byte(unbounded)) {
			t.Fatalf("metric contains unbounded input %s", unbounded)
		}
	}
}

func TestCollectorOutageDoesNotBlockRequests(t *testing.T) {
	endpoint, exported := collector(t)
	exported.failed.Store(true)
	cfg := config.Defaults()
	var delivered atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		delivered.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer upstream.Close()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint = true, endpoint
	cfg.Telemetry.InstanceID = "am-outage"
	cfg.Telemetry.ExportTimeout, cfg.Telemetry.BatchInterval, cfg.Telemetry.MetricInterval = 100*time.Millisecond, 10*time.Millisecond, 20*time.Millisecond
	var output lockedBuffer
	obs := observer(t, cfg, &output)
	handler := server.Handler(cfg, obs.Logger, obs)
	started := time.Now()
	for i := range 500 {
		req := httptest.NewRequest("POST", fmt.Sprintf("/api/v2/silence/arbitrary-%d", i), nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusAccepted {
			t.Fatal("collector error changed request outcome")
		}
	}
	if delivered.Load() != 500 || time.Since(started) > 10*time.Second {
		t.Fatal("export backpressured client requests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
	defer cancel()
	started = time.Now()
	_ = obs.Flush(ctx)
	_ = obs.Shutdown(ctx)
	if time.Since(started) > time.Second {
		t.Fatal("shutdown was not bounded")
	}
	if !strings.Contains(output.text(), "telemetry export failed") {
		t.Fatal("export failure was not observable")
	}
}

func TestInstrumentationPreservesProtocolUpgrade(t *testing.T) {
	endpoint, exported := collector(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: lab-echo\r\n\r\n")
		_ = rw.Flush()
		line, err := rw.ReadString('\n')
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = rw.WriteString("echo:" + line)
		_ = rw.Flush()
	}))
	defer upstream.Close()
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-upgrade"
	obs := observer(t, cfg, io.Discard)
	proxy := httptest.NewServer(server.Handler(cfg, obs.Logger, obs))
	defer proxy.Close()
	conn, err := net.DialTimeout("tcp", strings.TrimPrefix(proxy.URL, "http://"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: lab\r\nConnection: Upgrade\r\nUpgrade: lab-echo\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade returned %d", response.StatusCode)
	}
	_, _ = io.WriteString(conn, "hello\n")
	if line, err := reader.ReadString('\n'); err != nil || line != "echo:hello\n" {
		t.Fatalf("bidirectional forwarding failed: %q %v", line, err)
	}
	conn.Close()
	// The upstream has closed after echoing; allow the server span to finish.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		_ = obs.Flush(ctx)
		cancel()
		exported.mu.Lock()
		for _, resource := range exported.spans {
			for _, scope := range resource.ScopeSpans {
				for _, span := range scope.Spans {
					for _, attr := range span.Attributes {
						if span.Kind == tracepb.Span_SPAN_KIND_SERVER && attr.Key == "http.response.status_code" && attr.Value.GetIntValue() == 101 {
							exported.mu.Unlock()
							return
						}
					}
				}
			}
		}
		exported.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("upgraded server response was not observed as 101")
}

func TestCollectorAbsentAtStartup(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	defer upstream.Close()
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, "http://127.0.0.1:1", "am-offline"
	cfg.Telemetry.ExportTimeout = 100 * time.Millisecond
	started := time.Now()
	obs := observer(t, cfg, io.Discard)
	if time.Since(started) > time.Second {
		t.Fatal("collector connection became a startup dependency")
	}
	recorder := httptest.NewRecorder()
	server.Handler(cfg, obs.Logger, obs).ServeHTTP(recorder, httptest.NewRequest("POST", "/api/v2/alerts", nil))
	if recorder.Code != http.StatusAccepted {
		t.Fatal("an absent collector prevented upstream delivery")
	}
}

func TestAbortedRequestStillLogsAndPropagatesAbort(t *testing.T) {
	endpoint, _ := collector(t)
	cfg := config.Defaults()
	cfg.Upstream.URL = "http://unused:9093"
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-abort"
	var output lockedBuffer
	obs := observer(t, cfg, &output)
	handler := obs.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }))
	func() {
		defer func() {
			if recovered := recover(); recovered != http.ErrAbortHandler { //nolint:errorlint // net/http requires this exact panic sentinel, not a wrapped error.
				t.Fatalf("abort semantics changed: %v", recovered)
			}
		}()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v2/alerts", nil))
	}()
	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output.text())), &record); err != nil || record["am.request.aborted"] != true || record["trace_id"] == nil {
		t.Fatalf("missing correlated aborted-request log: %s, %v", output.text(), err)
	}
}

func TestUnknownV1RouteIsObservedWithoutUpstreamCall(t *testing.T) {
	endpoint, exported := collector(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unimplemented v1 request reached upstream")
	}))
	defer upstream.Close()
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-v1"
	cfg.Clients.Rules = []config.ClientRule{{Name: "legacy-prometheus", UserAgent: `^Prometheus/`}}
	var output lockedBuffer
	obs := observer(t, cfg, &output)
	req := httptest.NewRequest("GET", "/api/v1/nope", nil)
	req.Header.Set("User-Agent", "Prometheus/2.55.1")
	recorder := httptest.NewRecorder()
	server.Handler(cfg, obs.Logger, obs).ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown route changed: %d %s", recorder.Code, recorder.Body.String())
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := obs.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	exported.mu.Lock()
	defer exported.mu.Unlock()
	count := 0
	for _, resource := range exported.spans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				count++
				attrs := values(span.Attributes)
				if span.Kind != tracepb.Span_SPAN_KIND_SERVER || attrs["client.name"] != "legacy-prometheus" || attrs["am.api.version"] != "v1" || attrs["http.route"] != "/api/v1/*" {
					t.Fatalf("incorrect v1 request trace: %v", attrs)
				}
			}
		}
	}
	if count != 1 || !strings.Contains(output.text(), `"status_code":404`) || !strings.Contains(output.text(), `"am.api.version":"v1"`) {
		t.Fatal("v1 placeholder request was not fully observed")
	}
}
