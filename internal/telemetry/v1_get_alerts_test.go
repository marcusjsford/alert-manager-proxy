package telemetry_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/server"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func spanAttributeBool(span *tracepb.Span, key string) (bool, bool) {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return attr.Value.GetBoolValue(), true
		}
	}
	return false, false
}

func TestV1GetAlertsTelemetry(t *testing.T) {
	endpoint, exported := collector(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"labels":{"alertname":"DiskFull"},"startsAt":"2026-10-29T12:00:00.123Z","endsAt":"2026-10-29T13:00:00Z","fingerprint":"abc","receivers":[{"name":"team"}],"status":{"state":"active","silencedBy":[],"inhibitedBy":[]}}]`))
	}))
	defer upstream.Close()

	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-get"
	cfg.Clients.Rules = []config.ClientRule{{Name: "legacy-prometheus", UserAgent: `^Prometheus/`}}
	obs := observer(t, cfg, &lockedBuffer{})
	handler := server.Handler(cfg, obs.Logger, obs)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/alerts?filter=team%3D%22db%22&receiver=db-.*&silenced=false&active=true", nil)
	req.Header.Set("User-Agent", "Prometheus/2.55.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := obs.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	span := v1AlertSpan(t, exported)
	if got := spanAttribute(span, "http.route"); got != "/api/v1/alerts" {
		t.Errorf("http.route = %q", got)
	}
	if got := spanAttribute(span, "client.name"); got != "legacy-prometheus" {
		t.Errorf("client.name = %q", got)
	}
	if got := spanAttribute(span, "am.v1.filter"); got != `team="db"` {
		t.Errorf("am.v1.filter = %q", got)
	}
	if got := spanAttribute(span, "am.v1.receiver"); got != "db-.*" {
		t.Errorf("am.v1.receiver = %q", got)
	}
	for key, want := range map[string]bool{
		"am.v1.active": true, "am.v1.silenced": false,
		"am.v1.inhibited": true, "am.v1.unprocessed": true,
	} {
		got, present := spanAttributeBool(span, key)
		if !present || got != want {
			t.Errorf("%s = %v (present %v), want %v", key, got, present, want)
		}
	}

	lossy := false
	for _, event := range span.Events {
		if event.Name == "am.translation.lossy" {
			lossy = true
			attrs := values(event.Attributes)
			if attrs["field"] != "alert.timestamps" || attrs["reason"] != "millisecond precision" {
				t.Errorf("lossy event = %v", attrs)
			}
		}
	}
	if !lossy {
		t.Error("GET timestamps did not record the lossy translation")
	}

	var lossyCount int64
	exported.mu.Lock()
	for _, resource := range exported.metrics {
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name != "am_proxy_translation_lossy_total" {
					continue
				}
				for _, point := range metric.GetSum().GetDataPoints() {
					lossyCount += point.GetAsInt()
				}
			}
		}
	}
	exported.mu.Unlock()
	if lossyCount != 1 {
		t.Errorf("lossy counter = %d, want 1", lossyCount)
	}
}
