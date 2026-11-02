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

func TestV1PostAlertsTimestampPrecisionPolicy(t *testing.T) {
	direct, proxied := setup(t)
	run := fmt.Sprintf("v1-ts-%d", time.Now().UnixNano())
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second).Add(123456789 * time.Nanosecond)
	end := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second).Add(987654321 * time.Nanosecond)
	body := fmt.Sprintf(`[{"labels":{"alertname":"V1TimestampProbe","run":%q},"startsAt":%q,"endsAt":%q}]`,
		run, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if got := do(t, proxied, http.MethodPost, "/api/v1/alerts", body); got.status != http.StatusOK {
		t.Fatalf("v1 post = %d %s", got.status, got.body)
	}

	query := url.Values{"filter": {fmt.Sprintf(`run=%q`, run)}}.Encode()
	var v2 []struct {
		StartsAt string `json:"startsAt"`
		EndsAt   string `json:"endsAt"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := do(t, direct, http.MethodGet, "/api/v2/alerts?"+query, "")
		if got.status == http.StatusOK && json.Unmarshal(got.body, &v2) == nil && len(v2) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("posted alert not visible via v2: %d %s", got.status, got.body)
		}
		time.Sleep(250 * time.Millisecond)
	}
	for name, pair := range map[string][2]string{"startsAt": {v2[0].StartsAt, start.Format(time.RFC3339Nano)}, "endsAt": {v2[0].EndsAt, end.Format(time.RFC3339Nano)}} {
		got, err := time.Parse(time.RFC3339Nano, pair[0])
		if err != nil {
			t.Fatalf("%s = %q: %v", name, pair[0], err)
		}
		want, _ := time.Parse(time.RFC3339Nano, pair[1])
		if !got.Equal(want.Truncate(time.Millisecond)) {
			t.Errorf("%s = %s, want millisecond-truncated %s", name, pair[0], want.Truncate(time.Millisecond))
		}
	}

	v1 := do(t, proxied, http.MethodGet, "/api/v1/alerts?"+query, "")
	if v1.status != http.StatusOK {
		t.Fatalf("v1 read = %d %s", v1.status, v1.body)
	}
	var v1Body struct {
		Data []struct {
			StartsAt string `json:"startsAt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(v1.body, &v1Body); err != nil || len(v1Body.Data) != 1 {
		t.Fatalf("v1 body = %s", v1.body)
	}
	v1Time, err := time.Parse(time.RFC3339Nano, v1Body.Data[0].StartsAt)
	if err != nil || !v1Time.Equal(start.Truncate(time.Millisecond)) {
		t.Errorf("v1 startsAt = %q (%v), want millisecond-truncated value", v1Body.Data[0].StartsAt, err)
	}
}
