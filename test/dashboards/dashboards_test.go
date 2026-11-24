//go:build dashboards

// Package dashboards verifies the committed Grafana dashboard queries against
// the dedicated three-proxy/ClickHouse stack.
package dashboards

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	proxies     = []string{"http://127.0.0.1:19691", "http://127.0.0.1:19692", "http://127.0.0.1:19693"}
	clickhouse  = "http://127.0.0.1:19623"
	environment = "fanout-test"
)

func request(t *testing.T, method, address, userAgent string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, address, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v (start with make fanout)", method, address, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func timestamp(offset time.Duration, nanos bool) string {
	t := time.Now().UTC().Add(offset)
	if nanos {
		return t.Format("2006-01-02T15:04:05") + ".123456789Z"
	}
	return t.Format(time.RFC3339)
}

func alertBody(run string, resolved bool, nanos bool) []byte {
	start, end := timestamp(time.Hour, nanos), timestamp(2*time.Hour, nanos)
	if resolved {
		start, end = timestamp(-2*time.Hour, nanos), timestamp(-time.Hour, nanos)
	}
	return []byte(fmt.Sprintf(`[{"labels":{"alertname":"DashProbe","fanout_run":%q},"annotations":{"summary":"dashboard probe"},"startsAt":%q,"endsAt":%q}]`, run, start, end))
}

func seed(t *testing.T) string {
	t.Helper()
	run := fmt.Sprintf("dash-%d", time.Now().UnixNano())
	post := func(proxy, ua string, body []byte, path string) {
		if path == "" {
			path = "/api/v1/alerts"
		}
		status, raw := request(t, http.MethodPost, proxy+path, ua, body)
		if status != http.StatusOK {
			t.Fatalf("seed %s %s: %d %s", path, ua, status, raw)
		}
	}
	postError := func(proxy, ua, path string, body []byte) {
		status, raw := request(t, http.MethodPost, proxy+path, ua, body)
		if status < 400 {
			t.Fatalf("seed expected error for %s %s: %d %s", path, ua, status, raw)
		}
	}
	for _, proxy := range proxies {
		post(proxy, "FanoutFull/1", alertBody(run, false, true), "")
	}
	post(proxies[0], "FanoutTwo/1", alertBody(run, false, true), "")
	post(proxies[1], "FanoutTwo/1", alertBody(run, false, true), "/api/v2/alerts")
	post(proxies[2], "FanoutOne/1", alertBody(run, false, true), "")
	postError(proxies[2], "FanoutOne/1", "/api/v1/alerts", []byte(`notjson`))
	post(proxies[0], "FanoutUnknown/1", alertBody(run, false, true), "")
	post(proxies[0], "FanoutFull/1", alertBody(run, true, true), "")
	postError(proxies[1], "FanoutTwo/1", "/api/v2/alerts", []byte(`notjson`))
	big := append([]byte("["), bytes.Repeat([]byte("x"), 1<<20+1)...)
	big = append(big, ']')
	postError(proxies[1], "FanoutTwo/1", "/api/v2/alerts", big)

	silence := []byte(fmt.Sprintf(`{"matchers":[{"name":"fanout_run","value":%q,"isRegex":false,"isEqual":true}],"startsAt":%q,"endsAt":%q,"createdBy":"dash-user","comment":"dashboard seed"}`,
		run, timestamp(-time.Minute, false), timestamp(time.Hour, false)))
	status, raw := request(t, http.MethodPost, proxies[0]+"/api/v1/silences", "FanoutOne/1", silence)
	if status != http.StatusOK {
		t.Fatalf("seed v1 silence: %d %s", status, raw)
	}
	var v1 struct {
		Data struct {
			SilenceID string `json:"silenceId"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &v1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if status, raw := request(t, http.MethodDelete, proxies[0]+"/api/v1/silence/"+v1.Data.SilenceID, "FanoutOne/1", nil); status != http.StatusOK {
		t.Fatalf("seed v1 silence delete: %d %s", status, raw)
	}

	status, raw = request(t, http.MethodPost, proxies[1]+"/api/v2/silences", "FanoutTwo/1", silence)
	if status != http.StatusOK {
		t.Fatalf("seed v2 silence: %d %s", status, raw)
	}
	var v2 struct {
		SilenceID string `json:"silenceID"`
	}
	if err := json.Unmarshal(raw, &v2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	if status, raw := request(t, http.MethodDelete, proxies[1]+"/api/v2/silence/"+v2.SilenceID, "FanoutTwo/1", nil); status != http.StatusOK {
		t.Fatalf("seed v2 silence delete: %d %s", status, raw)
	}
	return run
}

func dashboardQueries(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../deploy/dashboards/alert-path.json")
	if err != nil {
		t.Fatal(err)
	}
	var dashboard struct {
		Panels []struct {
			Title   string `json:"title"`
			Targets []struct {
				RawSQL string `json:"rawSql"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(raw, &dashboard); err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{}
	for _, panel := range dashboard.Panels {
		if len(panel.Targets) == 0 || panel.Targets[0].RawSQL == "" {
			t.Fatalf("panel %q has no rawSql", panel.Title)
		}
		queries[panel.Title] = panel.Targets[0].RawSQL
	}
	return queries
}

func runQuery(t *testing.T, title, sql string) int {
	t.Helper()
	sql = strings.ReplaceAll(sql, "'$environment'", "'"+environment+"'")
	sql = strings.ReplaceAll(sql, "$__timeFilter(Timestamp)", "Timestamp >= now() - INTERVAL 2 HOUR")
	sql = strings.ReplaceAll(sql, "$__timeFilter(TimeUnix)", "TimeUnix >= now() - INTERVAL 2 HOUR")
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, clickhouse+"/?database=otel&default_format=JSON", bytes.NewReader([]byte(sql)))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("amp", "amp-lab-only")
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", title, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: clickhouse %d %s", title, resp.StatusCode, raw)
	}
	var result struct {
		Rows int `json:"rows"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("%s: decode: %v (%s)", title, err, raw)
	}
	return result.Rows
}

func TestDashboardPanelQueries(t *testing.T) {
	run := seed(t)
	t.Logf("seeded dashboard probe run=%s", run)
	time.Sleep(3 * time.Second)
	for title, sql := range dashboardQueries(t) {
		t.Run(title, func(t *testing.T) {
			if rows := runQuery(t, title, sql); rows == 0 {
				t.Fatalf("panel query returned no rows")
			} else {
				t.Logf("rows=%d", rows)
			}
		})
	}
}
