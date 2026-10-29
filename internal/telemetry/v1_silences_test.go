package telemetry_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/server"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func spanHasAttribute(span *tracepb.Span, key string) bool {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return true
		}
	}
	return false
}

func TestV1SilenceTelemetryAndRedaction(t *testing.T) {
	for _, redact := range []bool{true, false} {
		name := "redaction off"
		if redact {
			name = "redaction on"
		}
		t.Run(name, func(t *testing.T) {
			endpoint, exported := collector(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"silenceID":"sid-1"}`))
			}))
			defer upstream.Close()

			cfg := config.Defaults()
			cfg.Upstream.URL = upstream.URL
			cfg.Telemetry.Enabled, cfg.Telemetry.Endpoint, cfg.Telemetry.InstanceID = true, endpoint, "am-silence"
			cfg.Redaction.SilenceCreatedBy = redact
			cfg.Redaction.SilenceComment = redact
			var logs lockedBuffer
			obs := observer(t, cfg, &logs)
			handler := server.Handler(cfg, obs.Logger, obs)

			start := time.Now().UTC().Add(time.Hour)
			body := fmt.Sprintf(`{"matchers":[{"name":"alertname","value":"HighLatency","isRegex":false}],"startsAt":%q,"endsAt":%q,"createdBy":"alice-secret","comment":"comment-secret"}`,
				start.Format(time.RFC3339Nano), start.Add(time.Hour).Format(time.RFC3339Nano))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/silences", strings.NewReader(body))
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
			if got := spanAttribute(span, "am.silence.operation"); got != "create" {
				t.Errorf("operation = %q", got)
			}
			if got := spanAttribute(span, "am.silence.id"); got != "sid-1" {
				t.Errorf("silence id = %q", got)
			}
			if got := spanAttribute(span, "am.silence.matchers"); got != `{alertname="HighLatency"}` {
				t.Errorf("matchers = %q", got)
			}
			if redact {
				for _, key := range []string{"am.silence.createdBy", "am.silence.comment"} {
					if spanHasAttribute(span, key) {
						t.Errorf("redacted span attribute %s present", key)
					}
				}
				if strings.Contains(logs.text(), "alice-secret") || strings.Contains(logs.text(), "comment-secret") {
					t.Errorf("redacted value leaked into log: %s", logs.text())
				}
			} else {
				if got := spanAttribute(span, "am.silence.createdBy"); got != "alice-secret" {
					t.Errorf("createdBy = %q", got)
				}
				if got := spanAttribute(span, "am.silence.comment"); got != "comment-secret" {
					t.Errorf("comment = %q", got)
				}
			}
		})
	}
}
