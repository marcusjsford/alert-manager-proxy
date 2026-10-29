package v1

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type capturedAlertRequest struct {
	method, path, auth, contentType, body string
}

func alertsRouter(t *testing.T, upstreamURL string, mutate func(*Options)) *Router {
	t.Helper()
	u, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		UpstreamURL: u,
		DialTimeout: time.Second, ResponseHeaderTimeout: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if mutate != nil {
		mutate(&opts)
	}
	return NewRouter(opts)
}

func alertsUpstream(t *testing.T, status int, response string, captured *capturedAlertRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if captured != nil {
			*captured = capturedAlertRequest{
				method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"),
				contentType: r.Header.Get("Content-Type"), body: string(body),
			}
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
}

func postAlerts(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, alertsPath, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestPostAlertsTranslatesFieldsAndHeaders(t *testing.T) {
	var got capturedAlertRequest
	upstream := alertsUpstream(t, http.StatusOK, "", &got)
	defer upstream.Close()
	router := alertsRouter(t, upstream.URL, nil)

	body := `[{"labels":{"alertname":"DiskFull","team":"db"},"annotations":{"summary":"disk almost full"},"startsAt":"2026-10-29T12:00:00.123456Z","endsAt":"2026-10-29T13:00:00Z","generatorURL":"http://prometheus/graph?g0"}]`
	rec := postAlerts(t, router, body)

	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"success"}` {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if got.method != http.MethodPost || got.path != "/api/v2/alerts" {
		t.Errorf("upstream saw %s %s", got.method, got.path)
	}
	if got.auth != "Bearer client-token" || got.contentType != "application/json" {
		t.Errorf("headers lost: auth=%q content-type=%q", got.auth, got.contentType)
	}
	var alerts []struct {
		Labels       map[string]string `json:"labels"`
		Annotations  map[string]string `json:"annotations"`
		StartsAt     string            `json:"startsAt"`
		EndsAt       string            `json:"endsAt"`
		GeneratorURL string            `json:"generatorURL"`
	}
	if err := json.Unmarshal([]byte(got.body), &alerts); err != nil {
		t.Fatalf("upstream body %q: %v", got.body, err)
	}
	if len(alerts) != 1 {
		t.Fatalf("upstream body = %s", got.body)
	}
	alert := alerts[0]
	if alert.Labels["alertname"] != "DiskFull" || alert.Labels["team"] != "db" {
		t.Errorf("labels = %v", alert.Labels)
	}
	if alert.Annotations["summary"] != "disk almost full" {
		t.Errorf("annotations = %v", alert.Annotations)
	}
	if alert.GeneratorURL != "http://prometheus/graph?g0" {
		t.Errorf("generatorURL = %q", alert.GeneratorURL)
	}
	start, err := time.Parse(time.RFC3339Nano, alert.StartsAt)
	if err != nil || !start.Equal(time.Date(2026, 10, 29, 12, 0, 0, 123456000, time.UTC)) {
		t.Errorf("startsAt = %q (%v)", alert.StartsAt, err)
	}
	end, err := time.Parse(time.RFC3339Nano, alert.EndsAt)
	if err != nil || !end.Equal(time.Date(2026, 10, 29, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("endsAt = %q (%v)", alert.EndsAt, err)
	}
}

func TestPostAlertsMissingTimesAreOmitted(t *testing.T) {
	var got capturedAlertRequest
	upstream := alertsUpstream(t, http.StatusOK, "", &got)
	defer upstream.Close()
	router := alertsRouter(t, upstream.URL, nil)

	rec := postAlerts(t, router, `[{"labels":{"alertname":"NoTimes"}}]`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	var raw []map[string]any
	if err := json.Unmarshal([]byte(got.body), &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"startsAt", "endsAt", "annotations", "generatorURL"} {
		if _, present := raw[0][field]; present {
			t.Errorf("empty %s should be omitted: %s", field, got.body)
		}
	}
}

func TestPostAlertsEmptyArrayIsAccepted(t *testing.T) {
	var got capturedAlertRequest
	upstream := alertsUpstream(t, http.StatusOK, "", &got)
	defer upstream.Close()
	router := alertsRouter(t, upstream.URL, nil)

	rec := postAlerts(t, router, `[]`)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"success"}` {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
	if got.body != `[]` {
		t.Errorf("upstream body = %q", got.body)
	}
}

func TestPostAlertsDecodeAndValidationErrors(t *testing.T) {
	cases := []struct{ name, body, message string }{
		{"empty labels", `[{"labels":{}}]`, "at least one label pair required"},
		{"missing labels", `[{"annotations":{"summary":"no labels"}}]`, "at least one label pair required"},
		{"null labels", `[{"labels":null}]`, "at least one label pair required"},
		{"null alert", `[null]`, "alert must not be null"},
		{"object not array", `{"labels":{"alertname":"X"}}`, "json: cannot unmarshal object into Go value of type []*types.Alert"},
		{"not json", `notjson`, "invalid character 'o' in literal null (expecting 'u')"},
		{"non-string label", `[{"labels":{"alertname":5}}]`, "json: cannot unmarshal number into Go struct field Alert.labels of type model.LabelValue"},
		{"bad startsAt", `[{"labels":{"alertname":"X"},"startsAt":"bad"}]`, `parsing time "bad" as "2006-01-02T15:04:05Z07:00": cannot parse "bad" as "2006"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hit bool
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
			defer upstream.Close()
			rec := postAlerts(t, alertsRouter(t, upstream.URL, nil), tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rec.Code)
			}
			want := `{"status":"error","errorType":"bad_data","error":` + mustJSON(tc.message) + `}`
			if got := rec.Body.String(); got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if hit {
				t.Error("invalid alerts reached the upstream")
			}
		})
	}
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestPostAlertsMapsV2Errors(t *testing.T) {
	cases := []struct {
		name, upstreamBody     string
		upstreamStatus, status int
		message                string
	}{
		{"bare string", `"at least one label pair required"`, http.StatusBadRequest, http.StatusBadRequest, "at least one label pair required"},
		{"object", `{"code":602,"message":"0.labels in body is required"}`, http.StatusUnprocessableEntity, http.StatusBadRequest, "0.labels in body is required"},
		{"empty body", ``, http.StatusBadRequest, http.StatusBadRequest, "upstream Alertmanager returned 400"},
		{"server error", `{"message":"boom"}`, http.StatusInternalServerError, http.StatusBadGateway, "upstream Alertmanager returned 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := alertsUpstream(t, tc.upstreamStatus, tc.upstreamBody, nil)
			defer upstream.Close()
			rec := postAlerts(t, alertsRouter(t, upstream.URL, nil), `[{"labels":{"alertname":"X"}}]`)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			var body struct{ Status, ErrorType, Error string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body = %q", rec.Body.String())
			}
			if body.Status != "error" || body.ErrorType == "" || body.Error != tc.message {
				t.Errorf("body = %+v, want message %q", body, tc.message)
			}
			if strings.Contains(rec.Body.String(), `"success"`) {
				t.Error("failed write acknowledged as success")
			}
		})
	}
}

func TestPostAlertsTimeoutAndRecovery(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		release := make(chan struct{})
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
		defer upstream.Close()
		defer close(release)
		router := alertsRouter(t, upstream.URL, func(o *Options) { o.ResponseHeaderTimeout = 50 * time.Millisecond })
		rec := postAlerts(t, router, `[{"labels":{"alertname":"Slow"}}]`)
		if rec.Code != http.StatusGatewayTimeout {
			t.Fatalf("status = %d, want 504", rec.Code)
		}
	})

	t.Run("retry then recover", func(t *testing.T) {
		var mu sync.Mutex
		calls := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			mu.Lock()
			calls++
			call := calls
			mu.Unlock()
			if call == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer upstream.Close()
		router := alertsRouter(t, upstream.URL, nil)
		if rec := postAlerts(t, router, `[{"labels":{"alertname":"Retry"}}]`); rec.Code != http.StatusBadGateway {
			t.Fatalf("first status = %d, want 502", rec.Code)
		}
		if rec := postAlerts(t, router, `[{"labels":{"alertname":"Retry"}}]`); rec.Code != http.StatusOK {
			t.Fatalf("retry status = %d, want 200", rec.Code)
		}
	})
}
