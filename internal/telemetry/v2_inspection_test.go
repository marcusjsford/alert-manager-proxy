package telemetry_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/inspect"
	"alertmanager-proxy/internal/server"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

type upstreamCapture struct {
	mu   sync.Mutex
	body string
	path string
}

func inspectionUpstream(t *testing.T) (*httptest.Server, *upstreamCapture) {
	t.Helper()
	capture := &upstreamCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		capture.mu.Lock()
		capture.body, capture.path = string(body), r.URL.Path
		capture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/alerts":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/alerts":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/silences":
			_, _ = w.Write([]byte(`{"silenceID":"sid-v2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/silences":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/silence/sid-v2":
			_, _ = w.Write([]byte(`{"id":"sid-v2","matchers":[{"name":"a","value":"b","isRegex":false,"isEqual":true}],"startsAt":"2026-10-29T12:00:00Z","endsAt":"2026-10-29T13:00:00Z","updatedAt":"2026-10-29T12:00:00Z","createdBy":"alice","status":{"state":"active"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/silence/sid-v2":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, capture
}

func requestWithBody(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestV2InspectionSpanKeyParityWithV1(t *testing.T) {
	upstream, _ := inspectionUpstream(t)
	endpoint, exported := collector(t)
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-inspect"
	var logs lockedBuffer
	obs := observer(t, cfg, &logs)
	handler := server.Handler(cfg, obs.Logger, obs)

	start := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	end := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano)
	alertBody := fmt.Sprintf(`[{"labels":{"alertname":"ParityAlert"},"startsAt":%q,"endsAt":%q}]`, start, end)
	silenceBody := fmt.Sprintf(`{"matchers":[{"name":"a","value":"b","isRegex":false}],"startsAt":%q,"endsAt":%q,"createdBy":"alice","comment":"why"}`, start, end)
	alertQuery := url.Values{
		"filter": {`{a="b"}`}, "receiver": {`r.*`}, "active": {"true"},
		"silenced": {"false"}, "inhibited": {"true"}, "unprocessed": {"false"},
	}.Encode()
	v2AlertQuery := url.Values{
		"filter": {`a="b"`}, "receiver": {`r.*`}, "active": {"true"},
		"silenced": {"false"}, "inhibited": {"true"}, "unprocessed": {"false"},
	}.Encode()
	silenceQuery := url.Values{"filter": {`{a="b"}`}}.Encode()
	v2SilenceQuery := url.Values{"filter": {`a="b"`}}.Encode()

	for _, call := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/alerts", alertBody},
		{http.MethodPost, "/api/v2/alerts", alertBody},
		{http.MethodGet, "/api/v1/alerts?" + alertQuery, ""},
		{http.MethodGet, "/api/v2/alerts?" + v2AlertQuery, ""},
		{http.MethodPost, "/api/v1/silences", silenceBody},
		{http.MethodPost, "/api/v2/silences", silenceBody},
		{http.MethodGet, "/api/v1/silences?" + silenceQuery, ""},
		{http.MethodGet, "/api/v2/silences?" + v2SilenceQuery, ""},
		{http.MethodGet, "/api/v1/silence/sid-v2", ""},
		{http.MethodGet, "/api/v2/silence/sid-v2", ""},
		{http.MethodDelete, "/api/v1/silence/sid-v2", ""},
		{http.MethodDelete, "/api/v2/silence/sid-v2", ""},
	} {
		rec := requestWithBody(handler, call.method, call.path, call.body)
		if rec.Code < 200 || rec.Code >= 300 {
			t.Fatalf("%s %s = %d %s", call.method, call.path, rec.Code, rec.Body.String())
		}
	}

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := obs.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	pairs := [][2]string{
		{"POST /api/v1/alerts", "POST /api/v2/alerts"},
		{"GET /api/v1/alerts", "GET /api/v2/alerts"},
		{"POST /api/v1/silences", "POST /api/v2/silences"},
		{"GET /api/v1/silences", "GET /api/v2/silences"},
		{"GET /api/v1/silence/{id}", "GET /api/v2/silence/{id}"},
		{"DELETE /api/v1/silence/{id}", "DELETE /api/v2/silence/{id}"},
	}
	for _, pair := range pairs {
		v1Span := findServerSpan(t, exported, pair[0], "v1")
		v2Span := findServerSpan(t, exported, pair[1], "v2")
		v1Keys, v2Keys := spanAttrKeys(v1Span), spanAttrKeys(v2Span)
		if strings.Join(v1Keys, ",") != strings.Join(v2Keys, ",") {
			t.Errorf("%s vs %s keys differ:\n v1 %v\n v2 %v", pair[0], pair[1], v1Keys, v2Keys)
		}
	}
}

func findServerSpan(t *testing.T, exported *capture, name, version string) *tracepb.Span {
	t.Helper()
	exported.mu.Lock()
	defer exported.mu.Unlock()
	for _, resource := range exported.spans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Kind != tracepb.Span_SPAN_KIND_SERVER || span.Name != name {
					continue
				}
				for _, attr := range span.Attributes {
					if attr.Key == "am.api.version" && attr.Value.GetStringValue() == version {
						return span
					}
				}
			}
		}
	}
	t.Fatalf("server span %q for %s not found", name, version)
	return nil
}

func spanAttrKeys(span *tracepb.Span) []string {
	var keys []string
	for _, attr := range span.Attributes {
		// Automatic otelhttp attributes such as response body size legitimately
		// differ because v1 and v2 have different wire bodies. The acceptance
		// target is the application-level am.* key set.
		if strings.HasPrefix(attr.Key, "am.") && attr.Key != "am.api.version" {
			keys = append(keys, attr.Key)
		}
	}
	sort.Strings(keys)
	return keys
}

func TestV2InspectionPreservesBodiesAndCountsSkipped(t *testing.T) {
	upstream, capture := inspectionUpstream(t)
	endpoint, exported := collector(t)
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-bytes"
	var logs lockedBuffer
	obs := observer(t, cfg, &logs)
	handler := server.Handler(cfg, obs.Logger, obs)

	start := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	end := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano)
	body := fmt.Sprintf(`[{"labels":{"alertname":"Bytes"},"startsAt":%q,"endsAt":%q}]`, start, end)
	rec := requestWithBody(handler, http.MethodPost, "/api/v2/alerts", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("post = %d %s", rec.Code, rec.Body.String())
	}
	capture.mu.Lock()
	if capture.body != body {
		t.Errorf("upstream body changed: %q", capture.body)
	}
	capture.mu.Unlock()

	// A malformed body is forwarded untouched and counted as skipped.
	malformed := "notjson"
	rec = requestWithBody(handler, http.MethodPost, "/api/v2/silences", malformed)
	if rec.Code != http.StatusOK {
		t.Fatalf("malformed silence post = %d %s", rec.Code, rec.Body.String())
	}
	capture.mu.Lock()
	if capture.body != malformed {
		t.Errorf("malformed upstream body changed: %q", capture.body)
	}
	capture.mu.Unlock()

	oversized := strings.Repeat("x", inspect.MaxBody+1)
	rec = requestWithBody(handler, http.MethodPost, "/api/v2/alerts", oversized)
	if rec.Code != http.StatusOK {
		t.Fatalf("oversized post = %d %s", rec.Code, rec.Body.String())
	}
	capture.mu.Lock()
	if capture.body != oversized {
		t.Errorf("oversized upstream body changed: got %d bytes, want %d", len(capture.body), len(oversized))
	}
	capture.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := obs.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	exported.mu.Lock()
	defer exported.mu.Unlock()
	var skipped, alerts int64
	for _, resource := range exported.metrics {
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, point := range metric.GetSum().GetDataPoints() {
					switch metric.Name {
					case "am_proxy_inspection_skipped_total":
						skipped += point.GetAsInt()
					case "am_proxy_alerts_received_total":
						alerts += point.GetAsInt()
					}
				}
			}
		}
	}
	if skipped != 2 {
		t.Errorf("inspection skipped = %d, want 2", skipped)
	}
	if alerts != 1 {
		t.Errorf("v2 alerts received = %d, want 1", alerts)
	}
}

func TestV2InspectionPreservesRequestsAndResponses(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/alerts":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/alerts":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/silences":
			_, _ = w.Write([]byte(`{"silenceID":"sid-v2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/silences":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/silence/sid-v2":
			_, _ = w.Write([]byte(`{"id":"sid-v2","matchers":[{"name":"a","value":"b","isRegex":false,"isEqual":true}],"startsAt":"2026-10-29T12:00:00Z","endsAt":"2026-10-29T13:00:00Z","updatedAt":"2026-10-29T12:00:00Z","createdBy":"alice","status":{"state":"active"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v2/silence/sid-v2":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	endpoint, _ := collector(t)
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-preserve"
	var logs lockedBuffer
	obs := observer(t, cfg, &logs)
	enabled := server.Handler(cfg, obs.Logger, obs)
	disabled := server.Handler(cfg, obs.Logger, nil)

	start := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	end := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339Nano)
	alertBody := fmt.Sprintf(`[{"labels":{"alertname":"Preserve"},"startsAt":%q,"endsAt":%q}]`, start, end)
	silenceBody := fmt.Sprintf(`{"matchers":[{"name":"a","value":"b","isRegex":false}],"startsAt":%q,"endsAt":%q,"createdBy":"alice","comment":"why"}`, start, end)
	calls := []struct{ method, path, body string }{
		{http.MethodPost, "/api/v2/alerts", alertBody},
		{http.MethodGet, "/api/v2/alerts", ""},
		{http.MethodPost, "/api/v2/silences", silenceBody},
		{http.MethodGet, "/api/v2/silences", ""},
		{http.MethodGet, "/api/v2/silence/sid-v2", ""},
		{http.MethodDelete, "/api/v2/silence/sid-v2", ""},
	}
	for _, call := range calls {
		mu.Lock()
		bodies = nil
		mu.Unlock()
		first := requestWithBody(enabled, call.method, call.path, call.body)
		mu.Lock()
		enabledBodies := append([]string(nil), bodies...)
		mu.Unlock()
		second := requestWithBody(disabled, call.method, call.path, call.body)
		mu.Lock()
		allBodies := append([]string(nil), bodies...)
		mu.Unlock()
		if len(enabledBodies) != 1 || len(allBodies) != 2 || enabledBodies[0] != allBodies[1] {
			t.Errorf("%s %s upstream bodies differ: enabled=%q disabled=%q", call.method, call.path, enabledBodies, allBodies[1:])
		}
		if first.Code != second.Code || !bytesEqual(first.Body.Bytes(), second.Body.Bytes()) {
			t.Errorf("%s %s response differs: enabled=%d %q disabled=%d %q", call.method, call.path, first.Code, first.Body.String(), second.Code, second.Body.String())
		}
	}
}

func bytesEqual(a, b []byte) bool {
	return string(a) == string(b)
}
