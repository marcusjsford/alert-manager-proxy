package telemetry_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/server"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func v1AlertSpan(t *testing.T, exported *capture) *tracepb.Span {
	t.Helper()
	exported.mu.Lock()
	defer exported.mu.Unlock()
	for _, resource := range exported.spans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Kind == tracepb.Span_SPAN_KIND_SERVER && spanAttribute(span, "am.api.version") == "v1" {
					return span
				}
			}
		}
	}
	t.Fatal("v1 server span not exported")
	return nil
}

func spanAttribute(span *tracepb.Span, key string) string {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return attr.Value.GetStringValue()
		}
	}
	return ""
}

func spanAttributeInt(span *tracepb.Span, key string) int64 {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return attr.Value.GetIntValue()
		}
	}
	return 0
}

func spanStringSlice(span *tracepb.Span, key string) []string {
	for _, attr := range span.Attributes {
		if attr.Key != key {
			continue
		}
		var out []string
		for _, value := range attr.Value.GetArrayValue().GetValues() {
			out = append(out, value.GetStringValue())
		}
		return out
	}
	return nil
}

func TestV1PostAlertsTelemetry(t *testing.T) {
	endpoint, exported := collector(t)
	var upstreamBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		upstreamBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	cfg := config.Defaults()
	cfg.Upstream.URL = upstream.URL
	cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-v1"
	cfg.Clients.Rules = []config.ClientRule{{Name: "legacy-prometheus", UserAgent: `^Prometheus/`}}
	var logs lockedBuffer
	obs := observer(t, cfg, &logs)
	handler := server.Handler(cfg, obs.Logger, obs)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(
		`[{"labels":{"alertname":"DiskFull"},"startsAt":"2026-10-29T12:00:00.123456789Z"},{"labels":{"alertname":"HighLatency"}}]`))
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
	attrs := map[string]string{}
	for _, attr := range span.Attributes {
		attrs[attr.Key] = attr.Value.GetStringValue()
	}
	if attrs["http.route"] != "/api/v1/alerts" || attrs["client.name"] != "legacy-prometheus" {
		t.Errorf("span identity/route = %v", attrs)
	}
	if got := spanAttributeInt(span, "am.alerts.count"); got != 2 {
		t.Errorf("am.alerts.count = %d", got)
	}
	if got := spanStringSlice(span, "am.alerts.names"); strings.Join(got, ",") != "DiskFull,HighLatency" {
		t.Errorf("am.alerts.names = %v", got)
	}
	lossy := false
	for _, event := range span.Events {
		if event.Name == "am.translation.lossy" {
			lossy = true
			eventAttrs := values(event.Attributes)
			if eventAttrs["field"] != "alert.timestamps" || eventAttrs["reason"] != "millisecond precision" {
				t.Errorf("lossy event = %v", eventAttrs)
			}
		}
	}
	if !lossy {
		t.Error("sub-millisecond timestamp did not record a lossy translation")
	}

	exported.mu.Lock()
	alertCount := int64(-1)
	var metricLabels map[string]string
	for _, resource := range exported.metrics {
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name != "am_proxy_alerts_received_total" {
					continue
				}
				for _, point := range metric.GetSum().GetDataPoints() {
					alertCount = point.GetAsInt()
					metricLabels = values(point.Attributes)
				}
			}
		}
	}
	exported.mu.Unlock()
	if alertCount != 2 {
		t.Fatalf("alerts received = %d, want 2", alertCount)
	}
	for key, want := range map[string]string{
		"client": "legacy-prometheus", "instance": "am-v1", "api_version": "v1",
		"endpoint": "/api/v1/alerts", "method": "POST",
	} {
		if metricLabels[key] != want {
			t.Errorf("metric %s = %q, want %q", key, metricLabels[key], want)
		}
	}

	if !strings.Contains(upstreamBody, `"alertname":"DiskFull"`) || !strings.Contains(upstreamBody, `"alertname":"HighLatency"`) {
		t.Errorf("upstream body = %s", upstreamBody)
	}
	if strings.Contains(logs.text(), "am.alerts.names") {
		t.Errorf("alertnames leaked into the access log: %s", logs.text())
	}
	if !strings.Contains(logs.text(), `"am.alerts.count":2`) {
		t.Errorf("alert count missing from the access log: %s", logs.text())
	}
}
