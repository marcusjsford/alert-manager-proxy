//go:build fanout

// Package fanout verifies partial-fanout detection against the dedicated
// three-AM/three-proxy ClickHouse stack.
package fanout

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

var proxies = []string{"http://127.0.0.1:19691", "http://127.0.0.1:19692", "http://127.0.0.1:19693"}

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
	client := &http.Client{Timeout: 5 * time.Second}
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

func waitFor(t *testing.T, description string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func alertBody(run, name string, v1 bool) []byte {
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	end := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
	if v1 {
		return []byte(fmt.Sprintf(`[{"labels":{"alertname":%q,"fanout_run":%q},"startsAt":%q,"endsAt":%q}]`, name, run, start, end))
	}
	return []byte(fmt.Sprintf(`[{"labels":{"alertname":%q,"fanout_run":%q},"startsAt":%q,"endsAt":%q}]`, name, run, start, end))
}

type fanoutRow struct {
	Environment      string `json:"partial.environment"`
	Client           string `json:"client"`
	ClientAddress    string `json:"client_address"`
	InstancesReached int    `json:"instances_reached"`
	Expected         int    `json:"expected"`
	Observed         int    `json:"observed_instances"`
}

func query(t *testing.T) []fanoutRow {
	t.Helper()
	sql, err := os.ReadFile("../../deploy/queries/partial_fanout.sql")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		"http://127.0.0.1:19623/?database=otel&default_format=JSON", bytes.NewReader(sql))
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("amp", "amp-lab-only")
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("query clickhouse: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clickhouse query: %d %s", resp.StatusCode, raw)
	}
	var result struct {
		Data []fanoutRow `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode clickhouse response: %v (%s)", err, raw)
	}
	return result.Data
}

func TestPartialFanoutDetection(t *testing.T) {
	for _, proxy := range proxies {
		waitFor(t, "proxy ready "+proxy, func() bool {
			status, _ := request(t, http.MethodGet, proxy+"/api/v2/status", "", nil)
			return status == http.StatusOK
		})
	}
	run := fmt.Sprintf("fanout-%d", time.Now().UnixNano())
	post := func(proxy, ua, name string, v1 bool) {
		path := "/api/v1/alerts"
		if !v1 {
			path = "/api/v2/alerts"
		}
		status, raw := request(t, http.MethodPost, proxy+path, ua, alertBody(run, name, v1))
		if status != http.StatusOK {
			t.Fatalf("post %s %s: %d %s", path, name, status, raw)
		}
	}
	// Full fanout: all three nodes.
	for _, proxy := range proxies {
		post(proxy, "FanoutFull/1", "FanoutProbe", true)
	}
	// Partial two: one v1 and one v2 destination.
	post(proxies[0], "FanoutTwo/1", "FanoutProbe", true)
	post(proxies[1], "FanoutTwo/1", "FanoutProbe", false)
	// Partial one.
	post(proxies[2], "FanoutOne/1", "FanoutProbe", true)
	// Unknown client: no matching identity rule, detected by address from spans.
	post(proxies[0], "FanoutUnknown/1", "FanoutProbe", true)

	var rows []fanoutRow
	waitFor(t, "partial-fanout query rows", func() bool {
		rows = query(t)
		if len(rows) == 0 {
			return false
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if row.Environment != "fanout-test" {
				continue
			}
			seen[row.Client] = true
			if row.Client == "fanout-two" && row.InstancesReached != 2 {
				return false
			}
			if row.Client == "fanout-one" && row.InstancesReached != 1 {
				return false
			}
			if row.Client == "unknown" && (row.InstancesReached != 1 || row.ClientAddress == "") {
				return false
			}
			if row.Client == "fanout-full" {
				return false
			}
		}
		return seen["fanout-two"] && seen["fanout-one"] && seen["unknown"]
	})
	for _, row := range rows {
		if row.Environment != "fanout-test" {
			continue
		}
		if row.Expected != 3 {
			t.Errorf("%s expected = %d, want 3", row.Client, row.Expected)
		}
		if row.Observed != 3 {
			t.Errorf("%s observed instances = %d, want 3", row.Client, row.Observed)
		}
	}
	if !strings.Contains(run, "fanout-") {
		t.Fatal("run label was not generated")
	}
}
