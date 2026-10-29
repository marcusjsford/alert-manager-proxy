package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
)

// echoed is what the fake upstream saw.
type echoed struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   string
}

func newProxy(t *testing.T, upstreamURL string, mutate func(*config.Config)) *httptest.Server {
	t.Helper()
	cfg := config.Defaults()
	cfg.Upstream.URL = upstreamURL
	if mutate != nil {
		mutate(&cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(Handler(cfg, logger, nil))
	t.Cleanup(srv.Close)
	return srv
}

func TestPassThroughPreservesRequestAndResponse(t *testing.T) {
	var got echoed
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = echoed{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Clone(), string(body)}
		w.Header().Set("X-Upstream", "yes")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"from":"upstream"}`))
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL, nil)

	req, _ := http.NewRequest(http.MethodPost,
		proxy.URL+"/api/v2/silences?filter=a%3D%22b%22&filter=c%3D~%22d%22", strings.NewReader(`{"x":1}`))
	req.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
	req.Header.Set("X-Custom", "kept")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if got.Method != http.MethodPost || got.Path != "/api/v2/silences" {
		t.Errorf("upstream saw %s %s", got.Method, got.Path)
	}
	if got.Query != "filter=a%3D%22b%22&filter=c%3D~%22d%22" {
		t.Errorf("query rewritten: %q", got.Query)
	}
	if got.Body != `{"x":1}` {
		t.Errorf("body rewritten: %q", got.Body)
	}
	if got.Header.Get("Authorization") != "Basic Zm9vOmJhcg==" || got.Header.Get("X-Custom") != "kept" {
		t.Errorf("headers lost: %v", got.Header)
	}
	if got.Header.Get("X-Forwarded-For") == "" {
		t.Errorf("X-Forwarded-For not set")
	}
	if resp.StatusCode != http.StatusTeapot || resp.Header.Get("X-Upstream") != "yes" || string(body) != `{"from":"upstream"}` {
		t.Errorf("response altered: %d %v %q", resp.StatusCode, resp.Header, body)
	}
}

func TestPassThroughDoesNotCleanPaths(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
	}))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL, nil)

	// Raw request so the client doesn't normalise the path either.
	conn, err := net.Dial("tcp", strings.TrimPrefix(proxy.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = io.WriteString(conn, "GET /api/v2//alerts/./groups HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")
	status, _ := io.ReadAll(conn)
	if !strings.HasPrefix(string(status), "HTTP/1.1 200") {
		t.Fatalf("unexpected response: %q", status)
	}
	if gotPath != "/api/v2//alerts/./groups" {
		t.Fatalf("path was cleaned to %q", gotPath)
	}
}

func TestV1RoutesAreOwnedByTheProxy(t *testing.T) {
	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { upstreamHit = true }))
	defer upstream.Close()
	proxy := newProxy(t, upstream.URL, nil)

	for _, path := range []string{"/api/v1/status", "/api/v1/receivers"} {
		resp, err := http.Get(proxy.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotImplemented {
			t.Errorf("%s: status %d, want 501", path, resp.StatusCode)
		}
		if body["status"] != "error" || body["errorType"] == "" || body["error"] == "" {
			t.Errorf("%s: not a v1 error envelope: %v", path, body)
		}
	}
	for _, path := range []string{"/api/v1", "/api/v1/nope", "/api/v1/alerts/groups"} {
		resp, err := http.Get(proxy.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, resp.StatusCode)
		}
	}
	if upstreamHit {
		t.Error("v1 request reached the upstream")
	}

	// A path that merely starts with the same characters is not v1.
	resp, err := http.Get(proxy.URL + "/api/v1x")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !upstreamHit {
		t.Error("/api/v1x should pass through")
	}
}

func TestUnreachableUpstreamIs502Fast(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // nothing listens here now: connection refused

	proxy := newProxy(t, "http://"+addr, func(c *config.Config) { c.Upstream.DialTimeout = 500 * time.Millisecond })
	start := time.Now()
	resp, err := http.Get(proxy.URL + "/api/v2/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status %d, want 502", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("took %s, want under the dial timeout", elapsed)
	}
}

func TestBlackholedUpstreamTimesOutAtDialTimeout(t *testing.T) {
	// 10.255.255.1 is non-routable here: SYNs go unanswered, the realistic
	// "host down" case that connection-refused doesn't cover.
	proxy := newProxy(t, "http://10.255.255.1:9093", func(c *config.Config) { c.Upstream.DialTimeout = 300 * time.Millisecond })
	start := time.Now()
	resp, err := http.Get(proxy.URL + "/api/v2/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status %d, want 502", resp.StatusCode)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %s, dial timeout not enforced", elapsed)
	}
}

func TestSlowUpstreamIs504(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer upstream.Close()
	defer close(release)
	proxy := newProxy(t, upstream.URL, func(c *config.Config) { c.Upstream.ResponseHeaderTimeout = 200 * time.Millisecond })

	resp, err := http.Get(proxy.URL + "/api/v2/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("status %d, want 504", resp.StatusCode)
	}
}

func TestAdminHandler(t *testing.T) {
	srv := httptest.NewServer(AdminHandler())
	defer srv.Close()
	for _, path := range []string{"/-/healthy", "/-/ready"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", path, resp.StatusCode)
		}
	}
}
