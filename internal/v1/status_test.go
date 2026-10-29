package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

//nolint:gosec // synthetic test credentials for masking assertions
const statusTestConfig = `global:
  resolve_timeout: 1m
  smtp_tls_config:
    insecure_skip_verify: false
  slack_app_url: https://slack.com/api/chat.postMessage
  rocketchat_api_url: https://open.rocket.chat/
route:
  receiver: default
receivers:
- name: default
  labels:
    name: default
  webhook_configs:
  - url: https://user:webhook-secret@example.invalid/hook
    http_config:
      basic_auth:
        username: alice
        password: webhook-secret
    timeout: 0s
  slack_configs:
  - api_url: https://hooks.slack.com/services/slack-secret
templates: []
`

func statusUpstream(t *testing.T, status, receivers any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body any
		switch r.URL.Path {
		case "/api/v2/status":
			body = status
		case "/api/v2/receivers":
			body = receivers
		default:
			http.NotFound(w, r)
			return
		}
		raw, _ := json.Marshal(body)
		_, _ = w.Write(raw)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestGetStatusRendersLegacyConfigAndMasksSecrets(t *testing.T) {
	status := map[string]any{
		"config": map[string]any{"original": statusTestConfig},
		"cluster": map[string]any{
			"name": "cluster-a", "status": "ready",
			"peers": []map[string]string{{"name": "peer-a", "address": "10.0.0.1:9094"}},
		},
		"uptime": time.Now().UTC().Format(time.RFC3339Nano),
		"versionInfo": map[string]string{
			"version": "0.34.1", "revision": "abc", "branch": "HEAD",
		},
	}
	server := statusUpstream(t, status, nil)
	req := httptest.NewRequest(http.MethodGet, statusPath, nil)
	rec := httptest.NewRecorder()
	alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			ConfigYAML    string         `json:"configYAML"`
			ConfigJSON    map[string]any `json:"configJSON"`
			VersionInfo   map[string]string
			ClusterStatus struct {
				Name, Status string
				Peers        []map[string]string
			} `json:"clusterStatus"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	yaml := response.Data.ConfigYAML
	for _, unwanted := range []string{"smtp_tls_config", "slack_app_url", "rocketchat_api_url", "labels:", "timeout: 0s"} {
		if strings.Contains(yaml, unwanted) {
			t.Errorf("configYAML still contains target-only field %q:\n%s", unwanted, yaml)
		}
	}
	if !strings.Contains(yaml, "url: <secret>") || !strings.Contains(yaml, "password: <secret>") {
		t.Errorf("configYAML did not mask secrets:\n%s", yaml)
	}
	if strings.Contains(yaml, "webhook-secret") || strings.Contains(yaml, "slack-secret") {
		t.Error("configYAML leaked a secret")
	}

	configJSON := response.Data.ConfigJSON
	if _, ok := configJSON["event_recorder"]; ok {
		t.Error("event_recorder survived JSON rendering")
	}
	if _, ok := configJSON["tracing"]; ok {
		t.Error("tracing survived JSON rendering")
	}
	global := configJSON["global"].(map[string]any)
	for _, key := range legacyGlobalFields {
		if _, ok := global[key]; ok {
			t.Errorf("global.%s survived JSON rendering", key)
		}
	}
	if configJSON["templates"] != nil {
		t.Errorf("templates = %v, want null", configJSON["templates"])
	}
	receivers := configJSON["receivers"].([]any)
	receiver := receivers[0].(map[string]any)
	if _, ok := receiver["labels"]; ok {
		t.Error("receiver labels survived JSON rendering")
	}
	webhook := receiver["webhook_configs"].([]any)[0].(map[string]any)
	if _, ok := webhook["timeout"]; ok {
		t.Error("webhook timeout survived JSON rendering")
	}
	if body := rec.Body.String(); strings.Contains(body, "webhook-secret") || strings.Contains(body, "slack-secret") || strings.Contains(body, "user:webhook") {
		t.Error("configJSON leaked a secret")
	}
	if response.Data.ClusterStatus.Name != "cluster-a" || response.Data.ClusterStatus.Status != "ready" {
		t.Errorf("clusterStatus = %+v", response.Data.ClusterStatus)
	}
}

func TestStatusReportsTargetVersionInfo(t *testing.T) {
	status := map[string]any{
		"config":      map[string]any{"original": statusTestConfig},
		"cluster":     map[string]any{"name": "c", "status": "ready", "peers": []any{}},
		"uptime":      time.Now().UTC().Format(time.RFC3339Nano),
		"versionInfo": map[string]string{"version": "0.34.1", "revision": "rev", "branch": "HEAD"},
	}
	server := statusUpstream(t, status, nil)
	req := httptest.NewRequest(http.MethodGet, statusPath, nil)
	rec := httptest.NewRecorder()
	alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
	var response struct {
		Data struct{ VersionInfo map[string]string } `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if got := response.Data.VersionInfo["version"]; got != "0.34.1" {
		t.Fatalf("versionInfo.version = %q, want the real upstream version", got)
	}
}

func TestGetReceiversTranslatesNames(t *testing.T) {
	server := statusUpstream(t, nil, []map[string]any{
		{"name": "default", "labels": map[string]string{"name": "default"}},
		{"name": "db-team", "labels": map[string]string{"name": "db-team"}},
	})
	req := httptest.NewRequest(http.MethodGet, receiversPath, nil)
	rec := httptest.NewRecorder()
	alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"success","data":["default","db-team"]}` {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}

	server = statusUpstream(t, nil, []any{})
	req = httptest.NewRequest(http.MethodGet, receiversPath, nil)
	rec = httptest.NewRecorder()
	alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"success","data":[]}` {
		t.Fatalf("empty response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestStatusUpstreamErrorIsTruthful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	req := httptest.NewRequest(http.MethodGet, statusPath, nil)
	rec := httptest.NewRecorder()
	alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
