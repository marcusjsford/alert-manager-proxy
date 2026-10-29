//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestV1PostAlertsVisibleViaV2(t *testing.T) {
	direct, proxied := setup(t)
	run := fmt.Sprintf("v1-post-%d", time.Now().UnixNano())
	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	end := time.Now().UTC().Add(10 * time.Minute).Truncate(time.Millisecond)
	body := fmt.Sprintf(`[{
		"labels":{"alertname":"V1PostProbe","run":%q,"team":"integration"},
		"annotations":{"summary":"posted through v1","run":%q},
		"startsAt":%q,"endsAt":%q,"generatorURL":"http://prometheus/graph?g0"
	}]`, run, run, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))

	if got := do(t, proxied, http.MethodPost, "/api/v1/alerts", body); got.status != http.StatusOK || string(got.body) != `{"status":"success"}` {
		t.Fatalf("v1 post = %d %s", got.status, got.body)
	}

	query := url.Values{"filter": {fmt.Sprintf(`run=%q`, run)}}.Encode()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var alerts []struct {
			Labels       map[string]string `json:"labels"`
			Annotations  map[string]string `json:"annotations"`
			StartsAt     time.Time         `json:"startsAt"`
			EndsAt       time.Time         `json:"endsAt"`
			GeneratorURL string            `json:"generatorURL"`
		}
		got := do(t, direct, http.MethodGet, "/api/v2/alerts?"+query, "")
		if got.status == http.StatusOK {
			if err := json.Unmarshal(got.body, &alerts); err != nil {
				t.Fatalf("decode v2 alerts: %v (%s)", err, got.body)
			}
			if len(alerts) == 1 {
				alert := alerts[0]
				if alert.Labels["alertname"] != "V1PostProbe" || alert.Labels["team"] != "integration" || alert.Labels["run"] != run {
					t.Errorf("labels = %v", alert.Labels)
				}
				if alert.Annotations["summary"] != "posted through v1" || alert.Annotations["run"] != run {
					t.Errorf("annotations = %v", alert.Annotations)
				}
				if !alert.StartsAt.Equal(start) || !alert.EndsAt.Equal(end) {
					t.Errorf("times = %s/%s, want %s/%s", alert.StartsAt, alert.EndsAt, start, end)
				}
				if alert.GeneratorURL != "http://prometheus/graph?g0" {
					t.Errorf("generatorURL = %q", alert.GeneratorURL)
				}
				return
			}
			if len(alerts) > 1 {
				t.Fatalf("query matched %d alerts: %s", len(alerts), got.body)
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("v1-posted alert not visible via v2: %d %s", got.status, got.body)
		}
		time.Sleep(250 * time.Millisecond)
	}
}
