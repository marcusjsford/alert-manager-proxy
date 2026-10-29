//go:build telemetry

// Package telemetry verifies real OTLP receivers and ClickHouse persistence.
package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func query(t *testing.T, sql string) (string, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:19523/", strings.NewReader(sql))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth("amp", "amp-lab-only")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ClickHouse HTTP %d: %s", resp.StatusCode, raw)
	}
	return strings.TrimSpace(string(raw)), nil
}

func waitCount(t *testing.T, sql string, minimum int) int {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		value, err := query(t, sql)
		if err == nil {
			if n, err := strconv.Atoi(value); err == nil && n >= minimum {
				return n
			}
			last = value
		} else {
			last = err.Error()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("pipeline query did not reach %d: %s; last=%s", minimum, sql, last)
	return 0
}

func zero(t *testing.T, sql string) {
	t.Helper()
	value, err := query(t, sql)
	if err != nil || value != "0" {
		t.Fatalf("expected zero: %s; got %q, %v", sql, value, err)
	}
}

func TestCollectorClickHousePipeline(t *testing.T) {
	for _, tc := range []struct{ instance, address string }{
		{"am-grpc", "http://127.0.0.1:19595"},
		{"am-http", "http://127.0.0.1:19596"},
	} {
		t.Run(tc.instance, func(t *testing.T) {
			started := time.Now().UnixNano()
			id := make([]byte, 16)
			if _, err := rand.Read(id); err != nil {
				t.Fatal(err)
			}
			traceID := hex.EncodeToString(id)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tc.address+"/api/v2/status?token=PIPELINE_QUERY_SECRET", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("User-Agent", "AmpSmoke/1")
			req.Header.Set("Authorization", "Bearer PIPELINE_AUTH_SECRET")
			req.Header.Set("Traceparent", "00-"+traceID+"-1122334455667788-01")
			resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK || !strings.Contains(string(raw), `"version":"0.34.1"`) {
				t.Fatalf("pass-through failed: %d %s, %v", resp.StatusCode, raw, err)
			}
			traceSQL := fmt.Sprintf(`SELECT count() FROM otel.otel_traces AS c INNER JOIN otel.otel_traces AS p
ON c.TraceId=p.TraceId AND c.ParentSpanId=p.SpanId
WHERE c.TraceId='%s' AND c.ResourceAttributes['service.instance.id']='%s'
AND c.SpanAttributes['client.name']='lab-smoke' AND p.SpanAttributes['client.name']='lab-smoke'`, traceID, tc.instance)
			pairs := waitCount(t, traceSQL, 1)
			logSQL := fmt.Sprintf(`SELECT count() FROM otel.otel_logs AS l INNER JOIN otel.otel_traces AS s
ON l.TraceId=s.TraceId AND l.SpanId=s.SpanId WHERE l.TraceId='%s' AND l.Body='request completed'
AND l.LogAttributes['trace_id']=l.TraceId AND l.LogAttributes['span_id']=l.SpanId`, traceID)
			logs := waitCount(t, logSQL, 1)
			metricSQL := fmt.Sprintf(`SELECT count() FROM otel.otel_metrics_sum
WHERE ResourceAttributes['service.instance.id']='%s' AND MetricName='am_proxy_requests_total'
AND Attributes['client']='lab-smoke' AND Attributes['endpoint']='/api/v2/status' AND Value >= 1
AND TimeUnix >= fromUnixTimestamp64Nano(%d)`, tc.instance, started)
			metrics := waitCount(t, metricSQL, 1)
			t.Logf("instance=%s trace_id=%s parent_child_pairs=%d correlated_access_logs=%d request_metric_points=%d", tc.instance, traceID, pairs, logs, metrics)
			zero(t, fmt.Sprintf(`SELECT count() FROM otel.otel_traces WHERE TraceId='%s' AND position(toString(SpanAttributes), 'PIPELINE_') > 0`, traceID))
			zero(t, fmt.Sprintf(`SELECT count() FROM otel.otel_logs WHERE TraceId='%s' AND position(toString(LogAttributes), 'PIPELINE_') > 0`, traceID))
		})
	}
	for _, table := range []string{"otel_metrics_sum", "otel_metrics_histogram"} {
		zero(t, fmt.Sprintf(`SELECT count() FROM otel.%s WHERE arrayExists(k -> NOT has(
['client','api_version','endpoint','method','status_code','instance'], k), mapKeys(Attributes))`, table))
		zero(t, fmt.Sprintf(`SELECT count() FROM otel.%s WHERE NOT startsWith(MetricName, 'am_proxy_')`, table))
	}
}
