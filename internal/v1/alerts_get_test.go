package v1

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

type capturedGetRequest struct {
	method, path, auth string
	query              url.Values
}

func getAlertsUpstream(t *testing.T, status int, body string) (*httptest.Server, *capturedGetRequest) {
	t.Helper()
	var mu sync.Mutex
	got := &capturedGetRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*got = capturedGetRequest{
			method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"),
			query: r.URL.Query(),
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func getAlerts(t *testing.T, router http.Handler, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, alertsPath+"?"+rawQuery, nil)
	req.Header.Set("Authorization", "Bearer client-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

const twoAlertV2Response = `[
  {"labels":{"alertname":"B","team":"db"},"annotations":{},"startsAt":"2026-10-29T12:00:00.123Z","endsAt":"2026-10-29T13:00:00.456Z","fingerprint":"bbbb","receivers":[{"name":"recv-b"}],"status":{"state":"active","silencedBy":[],"inhibitedBy":[],"mutedBy":["m1"]},"updatedAt":"2026-10-29T12:00:00.500Z"},
  {"labels":{"alertname":"A","team":"db"},"annotations":{"summary":"s"},"startsAt":"2026-10-29T12:00:00Z","endsAt":"2026-10-29T13:00:00Z","generatorURL":"http://prometheus/g","fingerprint":"aaaa","receivers":[{"name":"r1"},{"name":"r2"}],"status":{"state":"suppressed","silencedBy":["sil"],"inhibitedBy":[],"mutedBy":[]}}
]`

func TestGetAlertsTranslatesQueryAndResponse(t *testing.T) {
	upstream, got := getAlertsUpstream(t, http.StatusOK, twoAlertV2Response)
	router := alertsRouter(t, upstream.URL, nil)

	query := url.Values{
		"filter":      {`{team="db",severity!="critical"}`},
		"receiver":    {`db-.*`},
		"silenced":    {"false"},
		"inhibited":   {"false"},
		"active":      {"true"},
		"unprocessed": {"false"},
	}.Encode()
	rec := getAlerts(t, router, query)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}

	if got.method != http.MethodGet || got.path != "/api/v2/alerts" || got.auth != "Bearer client-token" {
		t.Errorf("upstream request = %s %s auth=%q", got.method, got.path, got.auth)
	}
	if filters := got.query["filter"]; strings.Join(filters, "|") != `team="db"|severity!="critical"` {
		t.Errorf("filters = %q", filters)
	}
	for key, want := range map[string]string{
		"receiver": "db-.*", "silenced": "false", "inhibited": "false",
		"active": "true", "unprocessed": "false",
	} {
		if value := got.query.Get(key); value != want {
			t.Errorf("%s = %q, want %q", key, value, want)
		}
	}

	var body struct {
		Status string `json:"status"`
		Data   []struct {
			Labels       map[string]string `json:"labels"`
			Annotations  map[string]string `json:"annotations"`
			StartsAt     string            `json:"startsAt"`
			EndsAt       string            `json:"endsAt"`
			GeneratorURL string            `json:"generatorURL"`
			Status       struct {
				State       string   `json:"state"`
				SilencedBy  []string `json:"silencedBy"`
				InhibitedBy []string `json:"inhibitedBy"`
			} `json:"status"`
			Receivers   []string `json:"receivers"`
			Fingerprint string   `json:"fingerprint"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	if body.Status != "success" || len(body.Data) != 2 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	a, b := body.Data[0], body.Data[1]
	if a.Fingerprint != "aaaa" || b.Fingerprint != "bbbb" {
		t.Errorf("not sorted by fingerprint: %q then %q", a.Fingerprint, b.Fingerprint)
	}
	if a.Annotations["summary"] != "s" || a.GeneratorURL != "http://prometheus/g" ||
		strings.Join(a.Receivers, ",") != "r1,r2" || a.Status.State != "suppressed" ||
		strings.Join(a.Status.SilencedBy, ",") != "sil" || a.Status.InhibitedBy != nil {
		t.Errorf("first alert = %+v", a)
	}
	if b.Annotations != nil || b.GeneratorURL != "" || strings.Join(b.Receivers, ",") != "recv-b" ||
		b.Status.SilencedBy != nil || b.Status.InhibitedBy != nil {
		t.Errorf("second alert = %+v", b)
	}

	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	first := raw["data"].([]any)[0].(map[string]any)
	second := raw["data"].([]any)[1].(map[string]any)
	for _, dropped := range []string{"updatedAt", "mutedBy"} {
		if _, present := first[dropped]; present {
			t.Errorf("v2-only field %s leaked into v1 output", dropped)
		}
		if _, present := first["status"].(map[string]any)[dropped]; present {
			t.Errorf("v2-only status field %s leaked into v1 output", dropped)
		}
	}
	if _, present := second["status"].(map[string]any)["mutedBy"]; present {
		t.Error("mutedBy leaked into v1 output")
	}
}

func TestGetAlertsDefaultsAndEmptyResult(t *testing.T) {
	upstream, got := getAlertsUpstream(t, http.StatusOK, `[]`)
	router := alertsRouter(t, upstream.URL, nil)

	rec := getAlerts(t, router, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if len(got.query) != 0 {
		t.Errorf("default query forwarded %v", got.query)
	}
	if got := rec.Body.String(); got != `{"status":"success","data":[]}` {
		t.Errorf("body = %q", got)
	}
}

func TestGetAlertsUsesFirstFilterValueAndDropsUnprocessed(t *testing.T) {
	body := `[
	  {"labels":{"alertname":"Processed"},"fingerprint":"a","status":{"state":"active","silencedBy":[],"inhibitedBy":[]}},
	  {"labels":{"alertname":"Pending"},"fingerprint":"b","status":{"state":"unprocessed","silencedBy":[],"inhibitedBy":[]}}
	]`
	upstream, got := getAlertsUpstream(t, http.StatusOK, body)
	router := alertsRouter(t, upstream.URL, nil)

	rec := getAlerts(t, router, `filter=team%3D%22db%22&filter=alertname%3D%22ignored%22&unprocessed=false`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if filters := got.query["filter"]; len(filters) != 1 || filters[0] != `team="db"` {
		t.Errorf("filters = %q", filters)
	}
	var bodyOut struct {
		Data []struct{ Labels map[string]string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &bodyOut); err != nil {
		t.Fatal(err)
	}
	if len(bodyOut.Data) != 1 || bodyOut.Data[0].Labels["alertname"] != "Processed" {
		t.Errorf("data = %s", rec.Body.String())
	}
}

func TestGetAlertsValidationErrors(t *testing.T) {
	cases := []struct{ name, query, message string }{
		{"filter", `filter=%7Bbad`, "bad matcher format: bad"},
		{"active", `active=maybe`, `parameter "active" can either be 'true' or 'false', not "maybe"`},
		{"silenced", `silenced=maybe`, `parameter "silenced" can either be 'true' or 'false', not "maybe"`},
		{"inhibited", `inhibited=maybe`, `parameter "inhibited" can either be 'true' or 'false', not "maybe"`},
		{"unprocessed", `unprocessed=maybe`, `parameter "unprocessed" can either be 'true' or 'false', not "maybe"`},
		{"receiver", `receiver=%5B`, `failed to parse receiver param: [`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hit bool
			upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
			defer upstream.Close()
			rec := getAlerts(t, alertsRouter(t, upstream.URL, nil), tc.query)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rec.Code)
			}
			want := `{"status":"error","errorType":"bad_data","error":` + mustJSON(tc.message) + `}`
			if got := rec.Body.String(); got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if hit {
				t.Error("invalid query reached upstream")
			}
		})
	}
}

func TestGetAlertsMapsUpstreamErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body         string
		status, wantStatus int
		wantMessage        string
	}{
		{"bad request", `"bad filter"`, http.StatusBadRequest, http.StatusBadRequest, "bad filter"},
		{"server error", `{"message":"boom"}`, http.StatusInternalServerError, http.StatusBadGateway, "upstream Alertmanager returned 500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream, _ := getAlertsUpstream(t, tc.status, tc.body)
			rec := getAlerts(t, alertsRouter(t, upstream.URL, nil), "")
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			var body struct{ Status, ErrorType, Error string }
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body = %q", rec.Body.String())
			}
			if body.Status != "error" || body.ErrorType == "" || body.Error != tc.wantMessage {
				t.Errorf("body = %+v, want message %q", body, tc.wantMessage)
			}
		})
	}
}
