package inspect

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type fakeObserver struct {
	alerts     []int64
	parameters []attribute.KeyValue
	spanParams []attribute.KeyValue
	skipped    []string
}

func (f *fakeObserver) Alerts(_ context.Context, count int64) { f.alerts = append(f.alerts, count) }
func (f *fakeObserver) Parameters(_ context.Context, attrs ...attribute.KeyValue) {
	f.parameters = append(f.parameters, attrs...)
}
func (f *fakeObserver) SpanParameters(_ context.Context, attrs ...attribute.KeyValue) {
	f.spanParams = append(f.spanParams, attrs...)
}
func (f *fakeObserver) ParametersOn(_ context.Context, _ trace.Span, attrs ...attribute.KeyValue) {
	f.parameters = append(f.parameters, attrs...)
}
func (f *fakeObserver) InspectionSkipped(_ context.Context, reason string) {
	f.skipped = append(f.skipped, reason)
}

func stringAttr(attrs []attribute.KeyValue, key string) (string, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsString(), true
		}
	}
	return "", false
}

func intAttr(attrs []attribute.KeyValue, key string) (int64, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsInt64(), true
		}
	}
	return 0, false
}

func sliceAttr(attrs []attribute.KeyValue, key string) ([]string, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsStringSlice(), true
		}
	}
	return nil, false
}

func boolAttr(attrs []attribute.KeyValue, key string) (bool, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value.AsBool(), true
		}
	}
	return false, false
}

func readRequest(t *testing.T, req *http.Request) string {
	t.Helper()
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func future() string {
	return time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
}

func TestInspectAlertsRequest(t *testing.T) {
	body := fmt.Sprintf(`[{"labels":{"alertname":"A"},"startsAt":%q,"endsAt":%q},{"labels":{"alertname":"B"},"endsAt":%q}]`,
		future(), time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339Nano), time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/alerts", strings.NewReader(body))
	obs := &fakeObserver{}
	Request(req, obs)
	if got := readRequest(t, req); got != body {
		t.Errorf("body changed: %q", got)
	}
	if len(obs.alerts) != 1 || obs.alerts[0] != 2 {
		t.Errorf("alerts = %v", obs.alerts)
	}
	if firing, _ := intAttr(obs.parameters, "am.alerts.firing"); firing != 1 {
		t.Errorf("firing = %d", firing)
	}
	if resolved, _ := intAttr(obs.parameters, "am.alerts.resolved"); resolved != 1 {
		t.Errorf("resolved = %d", resolved)
	}
	if names, _ := sliceAttr(obs.spanParams, "am.alerts.names"); strings.Join(names, ",") != "A,B" {
		t.Errorf("names = %v", names)
	}
}

func TestInspectSilenceRequest(t *testing.T) {
	body := fmt.Sprintf(`{"id":"sid","matchers":[{"name":"a","value":"b","isRegex":false}],"startsAt":%q,"endsAt":%q,"createdBy":"alice","comment":"why"}`,
		future(), time.Now().UTC().Add(2*time.Hour).Format(time.RFC3339Nano))
	req := httptest.NewRequest(http.MethodPost, "/api/v2/silences", strings.NewReader(body))
	obs := &fakeObserver{}
	Request(req, obs)
	if got := readRequest(t, req); got != body {
		t.Errorf("body changed: %q", got)
	}
	for key, want := range map[string]string{
		"am.silence.operation": "update", "am.silence.id": "sid",
		"am.silence.matchers": `{a="b"}`, "am.silence.createdBy": "alice", "am.silence.comment": "why",
	} {
		if got, ok := stringAttr(obs.parameters, key); !ok || got != want {
			t.Errorf("%s = %q (present %v), want %q", key, got, ok, want)
		}
	}
}

func TestInspectQueryAndSilenceRoutes(t *testing.T) {
	obs := &fakeObserver{}
	Request(httptest.NewRequest(http.MethodGet,
		"/api/v2/alerts?filter=a%3D%22b%22&receiver=r.%2A&active=false&silenced=false&inhibited=false&unprocessed=true", nil), obs)
	for key, want := range map[string]string{
		"am.v1.filter": `a="b"`, "am.v1.receiver": `r.*`,
	} {
		if got, _ := stringAttr(obs.parameters, key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]bool{
		"am.v1.active": false, "am.v1.silenced": false, "am.v1.inhibited": false, "am.v1.unprocessed": true,
	} {
		if got, ok := boolAttr(obs.parameters, key); !ok || got != want {
			t.Errorf("%s = %v (present %v), want %v", key, got, ok, want)
		}
	}

	obs = &fakeObserver{}
	Request(httptest.NewRequest(http.MethodGet, "/api/v2/silences?filter=a%3D%22b%22", nil), obs)
	if got, _ := stringAttr(obs.parameters, "am.silence.operation"); got != "list" {
		t.Errorf("list operation = %q", got)
	}
	if got, _ := stringAttr(obs.parameters, "am.v1.filter"); got != `a="b"` {
		t.Errorf("list filter = %q", got)
	}

	obs = &fakeObserver{}
	Request(httptest.NewRequest(http.MethodGet, "/api/v2/silence/sid", nil), obs)
	if got, _ := stringAttr(obs.parameters, "am.silence.operation"); got != "get" {
		t.Errorf("get operation = %q", got)
	}
	if got, _ := stringAttr(obs.parameters, "am.silence.id"); got != "sid" {
		t.Errorf("get id = %q", got)
	}

	obs = &fakeObserver{}
	Request(httptest.NewRequest(http.MethodDelete, "/api/v2/silence/sid", nil), obs)
	if got, _ := stringAttr(obs.parameters, "am.silence.operation"); got != "delete" {
		t.Errorf("delete operation = %q", got)
	}
	if got, _ := stringAttr(obs.parameters, "am.silence.id"); got != "sid" {
		t.Errorf("delete id = %q", got)
	}
}

func TestInspectSkipsMalformedAndOversizedUnchanged(t *testing.T) {
	for name, body := range map[string]string{
		"malformed": "notjson",
		"oversized": strings.Repeat("x", MaxBody+1),
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v2/alerts", strings.NewReader(body))
			obs := &fakeObserver{}
			Request(req, obs)
			if got := readRequest(t, req); got != body {
				t.Errorf("body changed or truncated: got %d bytes, want %d", len(got), len(body))
			}
			if len(obs.skipped) != 1 {
				t.Errorf("skipped = %v", obs.skipped)
			}
		})
	}
}

func TestInspectSilenceResponsesDoNotRewriteBytes(t *testing.T) {
	t.Run("create id", func(t *testing.T) {
		body := `{"silenceID":"sid"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v2/silences", nil)
		req = req.WithContext(WithServerSpan(req.Context(), trace.SpanFromContext(context.Background())))
		resp := &http.Response{Request: req, Body: io.NopCloser(strings.NewReader(body)), StatusCode: 200}
		obs := &fakeObserver{}
		Response(resp, obs)
		raw, _ := io.ReadAll(resp.Body)
		if string(raw) != body {
			t.Errorf("response body changed: %q", raw)
		}
		if got, _ := stringAttr(obs.parameters, "am.silence.id"); got != "sid" {
			t.Errorf("silence id = %q", got)
		}
	})

	t.Run("get matchers", func(t *testing.T) {
		body := `{"id":"sid","matchers":[{"name":"a","value":"b","isRegex":false,"isEqual":true}]}`
		req := httptest.NewRequest(http.MethodGet, "/api/v2/silence/sid", nil)
		req = req.WithContext(WithServerSpan(req.Context(), trace.SpanFromContext(context.Background())))
		resp := &http.Response{Request: req, Body: io.NopCloser(strings.NewReader(body)), StatusCode: 200}
		obs := &fakeObserver{}
		Response(resp, obs)
		raw, _ := io.ReadAll(resp.Body)
		if string(raw) != body {
			t.Errorf("response body changed: %q", raw)
		}
		if got, _ := stringAttr(obs.parameters, "am.silence.matchers"); got != `{a="b"}` {
			t.Errorf("matchers = %q", got)
		}
	})
}

func TestInspectBodyReaderReplays(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v2/alerts", io.NopCloser(bytes.NewBufferString("notjson")))
	obs := &fakeObserver{}
	Request(req, obs)
	if got := readRequest(t, req); got != "notjson" {
		t.Errorf("replayed body = %q", got)
	}
}
