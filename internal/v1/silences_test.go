package v1

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type capturedSilenceRequest struct {
	method, path, auth, body string
}

func silenceUpstream(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *capturedSilenceRequest) {
	t.Helper()
	got := &capturedSilenceRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*got = capturedSilenceRequest{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"), body: string(body)}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return server, got
}

func postSilence(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, silencesPath, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer client-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func futureSilenceBody(id string, matchers string) string {
	start := time.Now().UTC().Add(time.Hour)
	end := start.Add(time.Hour)
	prefix := ""
	if id != "" {
		prefix = `"id":"` + id + `",`
	}
	return `{` + prefix + `"matchers":` + matchers + `,"startsAt":"` + start.Format(time.RFC3339Nano) + `","endsAt":"` + end.Format(time.RFC3339Nano) + `","createdBy":"alice","comment":"maintenance"}`
}

func TestPostSilenceCreateAndUpdate(t *testing.T) {
	server, got := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"silenceID":"created-id"}`)
	})
	router := alertsRouter(t, server.URL, nil)

	rec := postSilence(t, router, futureSilenceBody("", `[{"name":"alertname","value":"HighLatency","isRegex":false}]`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != `{"status":"success","data":{"silenceId":"created-id"}}` {
		t.Errorf("body = %q", got)
	}
	if got.method != http.MethodPost || got.path != "/api/v2/silences" || got.auth != "Bearer client-token" {
		t.Errorf("upstream request = %s %s auth=%q", got.method, got.path, got.auth)
	}
	var posted struct {
		ID       string `json:"id"`
		Matchers []struct {
			Name    string `json:"name"`
			Value   string `json:"value"`
			IsRegex bool   `json:"isRegex"`
			IsEqual bool   `json:"isEqual"`
		} `json:"matchers"`
		CreatedBy string `json:"createdBy"`
		Comment   string `json:"comment"`
	}
	if err := json.Unmarshal([]byte(got.body), &posted); err != nil {
		t.Fatalf("upstream body %q: %v", got.body, err)
	}
	if posted.ID != "" || len(posted.Matchers) != 1 || posted.Matchers[0].Name != "alertname" ||
		posted.Matchers[0].Value != "HighLatency" || posted.Matchers[0].IsRegex ||
		!posted.Matchers[0].IsEqual || posted.CreatedBy != "alice" || posted.Comment != "maintenance" {
		t.Errorf("posted silence = %+v", posted)
	}

	server2, got2 := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"silenceID":"new-id"}`)
	})
	router2 := alertsRouter(t, server2.URL, nil)
	rec = postSilence(t, router2, futureSilenceBody("old-id", `[{"name":"team","value":"web","isRegex":false,"isEqual":true}]`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"silenceId":"new-id"`) {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(got2.body, `"id":"old-id"`) {
		t.Errorf("update body lost id: %s", got2.body)
	}
}

func TestPostSilenceValidationErrors(t *testing.T) {
	start := time.Now().UTC().Add(time.Hour)
	end := start.Add(time.Hour)
	cases := []struct{ name, body, message string }{
		{"not json", `notjson`, "invalid character 'o' in literal null (expecting 'u')"},
		{"no matchers", `{"matchers":[],"startsAt":"` + start.Format(time.RFC3339) + `","endsAt":"` + end.Format(time.RFC3339) + `"}`, "silence invalid: at least one matcher required"},
		{"ends before starts", `{"matchers":[{"name":"a","value":"b","isRegex":false}],"startsAt":"` + end.Format(time.RFC3339) + `","endsAt":"` + start.Format(time.RFC3339) + `"}`, "silence invalid: end time must not be before start time"},
		{"ends in past", `{"matchers":[{"name":"a","value":"b","isRegex":false}],"startsAt":"2020-01-01T00:00:00Z","endsAt":"2020-01-01T01:00:00Z"}`, "end time can't be in the past"},
		{"bad regex", `{"matchers":[{"name":"a","value":"(","isRegex":true}],"startsAt":"` + start.Format(time.RFC3339) + `","endsAt":"` + end.Format(time.RFC3339) + `"}`, "error parsing regexp: missing closing ): `^(?:()$`"},
		{"all match empty", `{"matchers":[{"name":"a","value":"","isRegex":false}],"startsAt":"` + start.Format(time.RFC3339) + `","endsAt":"` + end.Format(time.RFC3339) + `"}`, "silence invalid: at least one matcher must not match the empty string"},
		{"start equals end", `{"matchers":[{"name":"a","value":"b","isRegex":false}],"startsAt":"` + start.Format(time.RFC3339) + `","endsAt":"` + start.Format(time.RFC3339) + `"}`, "start time must not be equal to end time"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hit bool
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
			defer server.Close()
			rec := postSilence(t, alertsRouter(t, server.URL, nil), tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rec.Code)
			}
			want := `{"status":"error","errorType":"bad_data","error":` + mustJSON(tc.message) + `}`
			if got := rec.Body.String(); got != want {
				t.Errorf("body = %q, want %q", got, want)
			}
			if hit {
				t.Error("invalid silence reached the upstream")
			}
		})
	}
}

func TestPostSilenceMapsUnknownUpdate(t *testing.T) {
	server, _ := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `"silence not found"`)
	})
	rec := postSilence(t, alertsRouter(t, server.URL, nil), futureSilenceBody("missing-id", `[{"name":"a","value":"b","isRegex":false}]`))
	if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"status":"error","errorType":"bad_data","error":"silence not found"}` {
		t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
	}
}

func TestGetSilencesFilterAndOrder(t *testing.T) {
	now := time.Now().UTC()
	body := `[
	  {"id":"active-late","matchers":[{"name":"env","value":"prod","isRegex":false,"isEqual":true}],"startsAt":"` + now.Add(-time.Hour).Format(time.RFC3339Nano) + `","endsAt":"` + now.Add(3*time.Hour).Format(time.RFC3339Nano) + `","updatedAt":"` + now.Format(time.RFC3339Nano) + `","createdBy":"a","status":{"state":"active"}},
	  {"id":"expired","matchers":[{"name":"env","value":"prod","isRegex":false,"isEqual":true}],"startsAt":"` + now.Add(-3*time.Hour).Format(time.RFC3339Nano) + `","endsAt":"` + now.Add(-2*time.Hour).Format(time.RFC3339Nano) + `","updatedAt":"` + now.Format(time.RFC3339Nano) + `","createdBy":"a","status":{"state":"expired"}},
	  {"id":"active-early","matchers":[{"name":"env","value":"prod","isRegex":false,"isEqual":true}],"startsAt":"` + now.Add(-time.Hour).Format(time.RFC3339Nano) + `","endsAt":"` + now.Add(2*time.Hour).Format(time.RFC3339Nano) + `","updatedAt":"` + now.Format(time.RFC3339Nano) + `","createdBy":"a","status":{"state":"active"}},
	  {"id":"other","matchers":[{"name":"env","value":"dev","isRegex":false,"isEqual":true}],"startsAt":"` + now.Add(-time.Hour).Format(time.RFC3339Nano) + `","endsAt":"` + now.Add(2*time.Hour).Format(time.RFC3339Nano) + `","updatedAt":"` + now.Format(time.RFC3339Nano) + `","createdBy":"a","status":{"state":"active"}}
	]`
	server, got := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	})
	router := alertsRouter(t, server.URL, nil)
	query := url.Values{"filter": {`{env=~"prod|stage"}`}}.Encode()
	req := httptest.NewRequest(http.MethodGet, silencesPath+"?"+query, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	if got.path != "/api/v2/silences" || got.method != http.MethodGet {
		t.Errorf("upstream request = %s %s", got.method, got.path)
	}
	var response struct {
		Status string `json:"status"`
		Data   []struct {
			ID     string                 `json:"id"`
			Status struct{ State string } `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "success" || len(response.Data) != 3 {
		t.Fatalf("data = %s", rec.Body.String())
	}
	if response.Data[0].ID != "active-early" || response.Data[1].ID != "active-late" || response.Data[2].ID != "expired" {
		t.Errorf("order/filter = %+v", response.Data)
	}
}

func TestGetSilencesInvalidFilterAndEmpty(t *testing.T) {
	server, _ := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	router := alertsRouter(t, server.URL, nil)
	req := httptest.NewRequest(http.MethodGet, silencesPath+"?filter=%7Bbad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"status":"error","errorType":"bad_data","error":"bad matcher format: bad"}` {
		t.Fatalf("invalid filter = %d %q", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, silencesPath, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"success","data":[]}` {
		t.Fatalf("empty list = %d %q", rec.Code, rec.Body.String())
	}
}

func TestGetSilenceByID(t *testing.T) {
	body := `{"id":"sid","matchers":[{"name":"a","value":"b","isRegex":false,"isEqual":true}],"startsAt":"2026-10-29T12:00:00Z","endsAt":"2026-10-29T13:00:00Z","updatedAt":"2026-10-29T12:00:00Z","createdBy":"alice","status":{"state":"active"}}`
	t.Run("found", func(t *testing.T) {
		server, got := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		})
		req := httptest.NewRequest(http.MethodGet, silencePrefix+"sid", nil)
		rec := httptest.NewRecorder()
		alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"sid"`) {
			t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
		}
		if got.path != "/api/v2/silence/sid" {
			t.Errorf("path = %q", got.path)
		}
	})
	for _, status := range []int{http.StatusNotFound, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server, _ := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})
			req := httptest.NewRequest(http.MethodGet, silencePrefix+"missing", nil)
			rec := httptest.NewRecorder()
			alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound || rec.Body.String() != "Error getting silence: <nil>\n" {
				t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
			}
			if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
				t.Error("missing CORS header")
			}
		})
	}
}

func TestDeleteSilence(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server, got := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) {})
		req := httptest.NewRequest(http.MethodDelete, silencePrefix+"sid", nil)
		rec := httptest.NewRecorder()
		alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.String() != `{"status":"success"}` {
			t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
		}
		if got.method != http.MethodDelete || got.path != "/api/v2/silence/sid" {
			t.Errorf("upstream = %s %s", got.method, got.path)
		}
	})
	for _, status := range []int{http.StatusNotFound, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server, _ := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
			req := httptest.NewRequest(http.MethodDelete, silencePrefix+"missing", nil)
			rec := httptest.NewRecorder()
			alertsRouter(t, server.URL, nil).ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || rec.Body.String() != `{"status":"error","errorType":"bad_data","error":"silence not found"}` {
				t.Fatalf("response = %d %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestSilenceUpstreamServerErrorIsNotSuccess(t *testing.T) {
	server, _ := silenceUpstream(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	rec := postSilence(t, alertsRouter(t, server.URL, nil), futureSilenceBody("", `[{"name":"a","value":"b","isRegex":false}]`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
