//go:build lab

// Package lab verifies the persistent, loopback-only Docker development lab.
package lab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

var proxies = []string{"http://127.0.0.1:19391", "http://127.0.0.1:19392", "http://127.0.0.1:19393"}

func request(t *testing.T, method, address string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, address, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v (start with make lab)", method, address, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

func readJSON(t *testing.T, address string, out any) {
	t.Helper()
	status, raw := request(t, http.MethodGet, address, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s: %d %s", address, status, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("decode %s: %v", address, err)
	}
}

func waitFor(t *testing.T, description string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func setProbe(t *testing.T, value string) {
	t.Helper()
	status, raw := request(t, http.MethodPost, "http://127.0.0.1:19280/probe", []byte(value))
	if status != http.StatusNoContent {
		t.Fatalf("set probe %s: %d %s", value, status, raw)
	}
}

func clearNotifications(t *testing.T) {
	t.Helper()
	status, raw := request(t, http.MethodDelete, "http://127.0.0.1:19280/notifications", nil)
	if status != http.StatusNoContent {
		t.Fatalf("clear notifications: %d %s", status, raw)
	}
}

func hasAlert(t *testing.T, proxy, name string) bool {
	t.Helper()
	query := url.Values{"filter": {fmt.Sprintf(`alertname=%q`, name)}}.Encode()
	var alerts []struct{ Labels map[string]string }
	readJSON(t, proxy+"/api/v2/alerts?"+query, &alerts)
	for _, alert := range alerts {
		if alert.Labels["alertname"] == name && alert.Labels["source"] == "amp-lab-legacy-prometheus" {
			return true
		}
	}
	return false
}

func waitLabNotification(t *testing.T, name, status string) {
	t.Helper()
	waitFor(t, status+" notification for "+name, func() bool {
		var notifications []struct {
			Alerts []struct {
				Status string
				Labels map[string]string
			}
		}
		readJSON(t, "http://127.0.0.1:19280/notifications", &notifications)
		for _, notification := range notifications {
			for _, alert := range notification.Alerts {
				if alert.Labels["alertname"] == name && alert.Labels["source"] == "amp-lab-legacy-prometheus" && alert.Status == status {
					return true
				}
			}
		}
		return false
	})
}

func TestThreeMemberCluster(t *testing.T) {
	names := map[string]bool{}
	for i, proxy := range proxies {
		var status struct {
			VersionInfo struct{ Version string }
			Cluster     struct {
				Name, Status string
				Peers        []struct{ Name string }
			}
		}
		waitFor(t, "three-member ready cluster", func() bool {
			readJSON(t, proxy+"/api/v2/status", &status)
			return status.Cluster.Status == "ready" && len(status.Cluster.Peers) == 3
		})
		if status.VersionInfo.Version != "0.34.1" {
			t.Fatalf("%s version = %s", proxy, status.VersionInfo.Version)
		}
		names[status.Cluster.Name] = true
		var direct struct{ Cluster struct{ Name string } }
		readJSON(t, fmt.Sprintf("http://127.0.0.1:%d/api/v2/status", 19291+i), &direct)
		if direct.Cluster.Name != status.Cluster.Name {
			t.Fatalf("%s does not front its corresponding Alertmanager", proxy)
		}
	}
	if len(names) != 3 {
		t.Fatalf("proxies front %d distinct members, want 3", len(names))
	}
}

func TestRepresentativeV1Client(t *testing.T) {
	var build struct{ Data struct{ Version string } }
	readJSON(t, "http://127.0.0.1:19290/api/v1/status/buildinfo", &build)
	if build.Data.Version != "2.55.1" {
		t.Fatalf("representative Prometheus version = %s", build.Data.Version)
	}
	var targets struct {
		Data struct {
			ActiveAlertmanagers []struct{ URL string }
		}
	}
	want := map[string]bool{
		"http://am-v026:9093/api/v1/alerts": false,
		"http://proxy-1:9095/api/v1/alerts": false,
		"http://proxy-2:9095/api/v1/alerts": false,
		"http://proxy-3:9095/api/v1/alerts": false,
	}
	waitFor(t, "Prometheus target discovery", func() bool {
		readJSON(t, "http://127.0.0.1:19290/api/v1/alertmanagers", &targets)
		for target := range want {
			want[target] = false
		}
		for _, target := range targets.Data.ActiveAlertmanagers {
			if _, ok := want[target.URL]; !ok {
				t.Fatalf("unexpected lab destination %s", target.URL)
			}
			want[target.URL] = true
		}
		for _, found := range want {
			if !found {
				return false
			}
		}
		return true
	})
	var reference struct{ VersionInfo struct{ Version string } }
	readJSON(t, "http://127.0.0.1:19226/api/v2/status", &reference)
	if reference.VersionInfo.Version != "0.26.0" {
		t.Fatalf("reference version = %s", reference.VersionInfo.Version)
	}
	waitFor(t, "real Prometheus v1 delivery to 0.26.0", func() bool {
		var alerts struct {
			Status string
			Data   []struct{ Labels map[string]string }
		}
		readJSON(t, "http://127.0.0.1:19226/api/v1/alerts", &alerts)
		for _, alert := range alerts.Data {
			if alerts.Status == "success" && alert.Labels["alertname"] == "LegacyClientProbe" &&
				alert.Labels["source"] == "amp-lab-legacy-prometheus" {
				return true
			}
		}
		return false
	})
	for _, proxy := range proxies {
		waitFor(t, "real Prometheus v1 alert delivery through "+proxy, func() bool {
			return hasAlert(t, proxy, "LegacyClientProbe")
		})
	}
	for _, proxy := range proxies {
		var status struct {
			Status string
			Data   struct {
				ConfigYAML  string            `json:"configYAML"`
				VersionInfo map[string]string `json:"versionInfo"`
			} `json:"data"`
		}
		readJSON(t, proxy+"/api/v1/status", &status)
		if status.Status != "success" || status.Data.ConfigYAML == "" ||
			status.Data.VersionInfo["version"] != "0.34.1" {
			t.Fatalf("%s v1 status did not expose the target version/config: %+v", proxy, status)
		}
		var receivers struct {
			Status string
			Data   []string `json:"data"`
		}
		readJSON(t, proxy+"/api/v1/receivers", &receivers)
		found := false
		for _, receiver := range receivers.Data {
			found = found || receiver == "lab"
		}
		if receivers.Status != "success" || !found {
			t.Fatalf("%s v1 receivers did not contain lab: %+v", proxy, receivers)
		}
	}
}

func TestV1FiringAndResolvedThroughProxy(t *testing.T) {
	setProbe(t, "0")
	for _, proxy := range proxies {
		waitFor(t, "Prometheus observes the resolved probe on "+proxy, func() bool {
			return !hasAlert(t, proxy, "LegacyResolveProbe")
		})
	}
	clearNotifications(t)

	setProbe(t, "1")
	for _, proxy := range proxies {
		waitFor(t, "v1 firing probe delivery through "+proxy, func() bool {
			return hasAlert(t, proxy, "LegacyResolveProbe")
		})
	}
	waitLabNotification(t, "LegacyResolveProbe", "firing")

	setProbe(t, "0")
	waitLabNotification(t, "LegacyResolveProbe", "resolved")
}

func TestV2FiringAndResolvedNotifications(t *testing.T) {
	run := fmt.Sprintf("lab-%d", time.Now().UnixNano())
	start := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	post := func(end time.Time) {
		body := []byte(fmt.Sprintf(`[{"labels":{"alertname":"LabDeliveryProbe","lab_run":%q},"annotations":{"summary":"synthetic lab delivery"},"startsAt":%q,"endsAt":%q}]`,
			run, start, end.UTC().Format(time.RFC3339Nano)))
		for _, proxy := range proxies {
			status, raw := request(t, http.MethodPost, proxy+"/api/v2/alerts", body)
			if status != http.StatusOK {
				t.Fatalf("post to %s: %d %s", proxy, status, raw)
			}
		}
	}
	post(time.Now().Add(3 * time.Minute))
	query := url.Values{"filter": {`lab_run="` + run + `"`}}.Encode()
	for _, proxy := range proxies {
		var alerts []struct{ Labels map[string]string }
		readJSON(t, proxy+"/api/v2/alerts?"+query, &alerts)
		if len(alerts) != 1 || alerts[0].Labels["lab_run"] != run {
			t.Fatalf("%s lost or changed the filtered alert: %+v", proxy, alerts)
		}
	}
	waitNotification := func(state string) {
		waitFor(t, state+" webhook notification", func() bool {
			var notifications []struct {
				Alerts []struct {
					Status string
					Labels map[string]string
				}
			}
			readJSON(t, "http://127.0.0.1:19280/notifications", &notifications)
			for _, notification := range notifications {
				for _, alert := range notification.Alerts {
					if alert.Labels["lab_run"] == run && alert.Status == state {
						return true
					}
				}
			}
			return false
		})
	}
	waitNotification("firing")
	post(time.Now().Add(-time.Second))
	waitNotification("resolved")
}

func TestSilenceGossip(t *testing.T) {
	run := fmt.Sprintf("silence-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	body := []byte(fmt.Sprintf(`{"matchers":[{"name":"lab_run","value":%q,"isRegex":false,"isEqual":true}],"startsAt":%q,"endsAt":%q,"createdBy":"amp-lab","comment":"synthetic gossip probe"}`,
		run, now.Format(time.RFC3339Nano), now.Add(time.Minute).Format(time.RFC3339Nano)))
	status, raw := request(t, http.MethodPost, proxies[0]+"/api/v2/silences", body)
	if status != http.StatusOK {
		t.Fatalf("create silence: %d %s", status, raw)
	}
	var created struct{ SilenceID string }
	if err := json.Unmarshal(raw, &created); err != nil || created.SilenceID == "" {
		t.Fatalf("invalid create response: %s", raw)
	}
	for _, proxy := range proxies {
		waitFor(t, "silence gossip to "+proxy, func() bool {
			status, body := request(t, http.MethodGet, proxy+"/api/v2/silence/"+created.SilenceID, nil)
			return status == http.StatusOK && strings.Contains(string(body), created.SilenceID)
		})
	}
}

func TestV1SilenceGossipAndMuting(t *testing.T) {
	run := fmt.Sprintf("v1-silence-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	body := fmt.Sprintf(`{"matchers":[{"name":"lab_run","value":%q,"isRegex":false,"isEqual":true}],"startsAt":%q,"endsAt":%q,"createdBy":"amp-lab-v1","comment":"v1 silence gossip probe"}`,
		run, now.Format(time.RFC3339Nano), now.Add(5*time.Minute).Format(time.RFC3339Nano))
	status, raw := request(t, http.MethodPost, proxies[0]+"/api/v1/silences", []byte(body))
	if status != http.StatusOK {
		t.Fatalf("v1 create silence: %d %s", status, raw)
	}
	var created struct {
		Status string
		Data   struct {
			SilenceID string `json:"silenceId"`
		}
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.Status != "success" || created.Data.SilenceID == "" {
		t.Fatalf("invalid v1 create response: %s", raw)
	}
	for _, proxy := range proxies {
		waitFor(t, "v1 silence gossip to "+proxy, func() bool {
			status, body := request(t, http.MethodGet, proxy+"/api/v2/silence/"+created.Data.SilenceID, nil)
			return status == http.StatusOK && strings.Contains(string(body), created.Data.SilenceID)
		})
	}

	clearNotifications(t)
	alert := fmt.Sprintf(`[{"labels":{"alertname":"V1SilenceProbe","lab_run":%q},"annotations":{"summary":"must stay muted"},"startsAt":%q,"endsAt":%q}]`,
		run, now.Add(-time.Minute).Format(time.RFC3339Nano), now.Add(3*time.Minute).Format(time.RFC3339Nano))
	for _, proxy := range proxies {
		if status, raw := request(t, http.MethodPost, proxy+"/api/v2/alerts", []byte(alert)); status != http.StatusOK {
			t.Fatalf("post suppressed alert to %s: %d %s", proxy, status, raw)
		}
	}
	query := url.Values{"filter": {fmt.Sprintf(`lab_run=%q`, run)}}.Encode()
	for _, proxy := range proxies {
		waitFor(t, "silenced alert state on "+proxy, func() bool {
			var alerts []struct {
				Labels map[string]string
				Status struct{ State string }
			}
			readJSON(t, proxy+"/api/v2/alerts?"+query, &alerts)
			return len(alerts) == 1 && alerts[0].Labels["lab_run"] == run && alerts[0].Status.State == "suppressed"
		})
	}

	// group_wait is 1s and group_interval 2s; give the notification pipeline
	// time to prove that a silenced new alert is not delivered.
	time.Sleep(6 * time.Second)
	var notifications []struct {
		Alerts []struct{ Labels map[string]string }
	}
	readJSON(t, "http://127.0.0.1:19280/notifications", &notifications)
	for _, notification := range notifications {
		for _, alert := range notification.Alerts {
			if alert.Labels["lab_run"] == run {
				t.Fatalf("silenced alert was delivered to the receiver: %+v", alert.Labels)
			}
		}
	}

	if status, raw := request(t, http.MethodDelete, proxies[0]+"/api/v1/silence/"+created.Data.SilenceID, nil); status != http.StatusOK {
		t.Fatalf("v1 delete silence: %d %s", status, raw)
	}
}
