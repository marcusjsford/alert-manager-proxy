package v1

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"alertmanager-proxy/internal/v1/envelope"
)

func testRouter(t *testing.T) *Router {
	t.Helper()
	return NewRouter(Options{
		UpstreamURL: &url.URL{Scheme: "http", Host: "127.0.0.1:1"},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func serve(t *testing.T, router http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func assertEnvelopeHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != envelope.CacheControl {
		t.Errorf("Cache-Control = %q", got)
	}
}

func TestPreflightMatchesRecordedContract(t *testing.T) {
	router := testRouter(t)
	for _, path := range []string{
		"/api/v1/alerts", "/api/v1/silences", "/api/v1/silence/abc",
		"/api/v1/status", "/api/v1/receivers", "/api/v1/nope", "/api/v1/alerts/groups",
	} {
		t.Run(path, func(t *testing.T) {
			rec := serve(t, router, http.MethodOptions, path)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			assertEnvelopeHeaders(t, rec)
			if rec.Header().Get("Access-Control-Allow-Headers") != envelope.AllowHeaders ||
				rec.Header().Get("Access-Control-Allow-Methods") != envelope.AllowMethods ||
				rec.Header().Get("Access-Control-Expose-Headers") != envelope.ExposeHeaders {
				t.Errorf("preflight headers = %v", rec.Header())
			}
			if rec.Header().Get("Content-Type") != "" || rec.Body.Len() != 0 {
				t.Errorf("preflight wrote content: %q %v", rec.Body.String(), rec.Header())
			}
		})
	}
}

func TestPreflightBarePrefixRedirectsLikeV026(t *testing.T) {
	rec := serve(t, testRouter(t), http.MethodOptions, "/api/v1")
	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/api/v1/" {
		t.Errorf("Location = %q", got)
	}
}

func TestKnownRoutesRemainExplicit501(t *testing.T) {
	router := testRouter(t)
	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/alerts"},
		{http.MethodPost, "/api/v1/silences"},
		{http.MethodGet, "/api/v1/silences"},
		{http.MethodGet, "/api/v1/silence/abc"},
		{http.MethodDelete, "/api/v1/silence/abc"},
		{http.MethodGet, "/api/v1/status"},
		{http.MethodGet, "/api/v1/receivers"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := serve(t, router, tc.method, tc.path)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d", rec.Code)
			}
			assertEnvelopeHeaders(t, rec)
			if got := rec.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `"status":"error"`) ||
				!strings.Contains(body, `"errorType":"not_implemented"`) {
				t.Errorf("body = %q", body)
			}
		})
	}
}

func TestUnknownRoutesArePlain404(t *testing.T) {
	router := testRouter(t)
	for _, path := range []string{"/api/v1", "/api/v1/", "/api/v1/nope", "/api/v1/alerts/groups"} {
		t.Run(path, func(t *testing.T) {
			rec := serve(t, router, http.MethodGet, path)
			assertPlainError(t, rec, http.StatusNotFound, "404 page not found\n")
		})
	}
}

func TestWrongMethodIsPlain405WithAllow(t *testing.T) {
	router := testRouter(t)
	cases := []struct{ path, allow string }{
		{"/api/v1/alerts", "GET, OPTIONS, POST"},
		{"/api/v1/silences", "GET, OPTIONS, POST"},
		{"/api/v1/silence/abc", "DELETE, GET, OPTIONS"},
		{"/api/v1/status", "GET, OPTIONS"},
		{"/api/v1/receivers", "GET, OPTIONS"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := serve(t, router, http.MethodPut, tc.path)
			assertPlainError(t, rec, http.StatusMethodNotAllowed, "Method Not Allowed\n")
			if got := rec.Header().Get("Allow"); got != tc.allow {
				t.Errorf("Allow = %q, want %q", got, tc.allow)
			}
		})
	}
}

func TestHeadIsNotImplicitlyAllowed(t *testing.T) {
	rec := serve(t, testRouter(t), http.MethodHead, "/api/v1/alerts")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestKnownRouteTrailingSlashRedirects(t *testing.T) {
	router := testRouter(t)
	cases := []struct{ path, location string }{
		{"/api/v1/alerts/", "/api/v1/alerts"},
		{"/api/v1/status/", "/api/v1/status"},
		{"/api/v1/silence/abc/", "/api/v1/silence/abc"},
		{"/api/v1/alerts/?filter=x", "/api/v1/alerts?filter=x"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			rec := serve(t, router, http.MethodGet, tc.path)
			if rec.Code != http.StatusMovedPermanently {
				t.Fatalf("status = %d, want 301", rec.Code)
			}
			if got := rec.Header().Get("Location"); got != tc.location {
				t.Errorf("Location = %q, want %q", got, tc.location)
			}
		})
	}
}

func TestUnknownTrailingSlashStays404(t *testing.T) {
	rec := serve(t, testRouter(t), http.MethodGet, "/api/v1/nope/")
	assertPlainError(t, rec, http.StatusNotFound, "404 page not found\n")
}

func TestMethodHandlersAreIndependent(t *testing.T) {
	router := testRouter(t)
	for i := range router.routes {
		if router.routes[i].match(alertsPath) {
			router.routes[i].handlers[http.MethodPost] = func(w http.ResponseWriter, _ *http.Request) {
				envelope.Success(w)
			}
		}
	}

	post := serve(t, router, http.MethodPost, alertsPath)
	if post.Code != http.StatusOK || post.Body.String() != `{"status":"success"}` {
		t.Fatalf("POST = %d %q", post.Code, post.Body.String())
	}
	get := serve(t, router, http.MethodGet, alertsPath)
	if get.Code != http.StatusNotImplemented {
		t.Fatalf("GET = %d, want the untouched 501", get.Code)
	}
	put := serve(t, router, http.MethodPut, alertsPath)
	if put.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT = %d, want 405", put.Code)
	}
}

func assertPlainError(t *testing.T, rec *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d", rec.Code, status)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unexpected CORS header on %d: %q", status, got)
	}
	if got := rec.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}
