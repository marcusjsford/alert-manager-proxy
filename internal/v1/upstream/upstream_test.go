package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"alertmanager-proxy/internal/v1/envelope"
)

func clientFor(t *testing.T, base string, mutate func(*Options)) *Client {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Base: u, DialTimeout: time.Second, ResponseHeaderTimeout: time.Second}
	if mutate != nil {
		mutate(&opts)
	}
	return New(opts)
}

func TestRequestPreservesPathQueryHeadersAndAuth(t *testing.T) {
	var got struct {
		path, query, auth, custom, body, method string
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = struct{ path, query, auth, custom, body, method string }{
			path: r.URL.Path, query: r.URL.RawQuery, auth: r.Header.Get("Authorization"),
			custom: r.Header.Get("X-Custom"), body: string(body), method: r.Method,
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := clientFor(t, server.URL+"/prefix", nil)
	inbound, _ := http.NewRequest(http.MethodPost, "/api/v1/alerts", nil)
	inbound.Header.Set("Authorization", "Bearer client-token")
	inbound.Header.Set("X-Custom", "kept")
	inbound.Header.Set("Accept-Encoding", "gzip")
	inbound.Header.Set("Connection", "X-Hop")
	inbound.Header.Set("X-Hop", "dropped")

	q := url.Values{"filter": {`a="b"`, `c="d"`}}
	req, err := client.Request(t.Context(), http.MethodPost, "/api/v2/alerts", strings.NewReader("payload"), q)
	if err != nil {
		t.Fatal(err)
	}
	ForwardHeaders(req.Header, inbound.Header)
	var out struct {
		OK bool `json:"ok"`
	}
	if err := client.DoJSON(req, &out); err != nil {
		t.Fatal(err)
	}

	if !out.OK {
		t.Error("response not decoded")
	}
	if got.path != "/prefix/api/v2/alerts" {
		t.Errorf("path = %q", got.path)
	}
	if got.query != `filter=a%3D%22b%22&filter=c%3D%22d%22` {
		t.Errorf("query = %q", got.query)
	}
	if got.method != http.MethodPost || got.body != "payload" {
		t.Errorf("method/body = %s %q", got.method, got.body)
	}
	if got.auth != "Bearer client-token" || got.custom != "kept" {
		t.Errorf("headers lost: auth=%q custom=%q", got.auth, got.custom)
	}
}

func TestForwardHeadersDropsHopByHopAndEncoding(t *testing.T) {
	src := http.Header{}
	src.Set("Authorization", "Basic abc")
	src.Set("Connection", "X-Hop")
	src.Set("X-Hop", "secret")
	src.Set("Accept-Encoding", "gzip")
	src.Set("Content-Length", "999")
	dst := http.Header{}
	ForwardHeaders(dst, src)

	if dst.Get("Authorization") != "Basic abc" {
		t.Errorf("Authorization = %q", dst.Get("Authorization"))
	}
	for _, name := range []string{"X-Hop", "Accept-Encoding", "Content-Length", "Connection"} {
		if got := dst.Get(name); got != "" {
			t.Errorf("%s = %q, want dropped", name, got)
		}
	}
}

func TestNon2xxIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"status":"error"}`))
	}))
	defer server.Close()

	client := clientFor(t, server.URL, nil)
	req, _ := client.Request(t.Context(), http.MethodPost, "/api/v2/alerts", strings.NewReader("x"), nil)
	err := client.DoJSON(req, nil)
	var responseErr *ResponseError
	if !errors.As(err, &responseErr) {
		t.Fatalf("error = %v, want ResponseError", err)
	}
	if responseErr.Status != http.StatusInternalServerError || !bytes.Contains(responseErr.Body, []byte(`"status":"error"`)) {
		t.Errorf("response error = %+v", responseErr)
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()

	client := clientFor(t, server.URL, nil)
	req, _ := client.Request(t.Context(), http.MethodPost, "/api/v2/alerts", strings.NewReader("x"), nil)
	err := client.DoJSON(req, nil)
	var responseErr *ResponseError
	if !errors.As(err, &responseErr) || responseErr.Status != http.StatusFound {
		t.Fatalf("error = %v, want 302 ResponseError", err)
	}
	if hits.Load() != 1 {
		t.Errorf("upstream hits = %d, want 1", hits.Load())
	}
}

func TestResponseHeaderTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer server.Close()
	defer close(release)

	client := clientFor(t, server.URL, func(o *Options) { o.ResponseHeaderTimeout = 50 * time.Millisecond })
	req, _ := client.Request(t.Context(), http.MethodGet, "/api/v2/status", nil, nil)
	err := client.DoJSON(req, nil)
	status, _, _ := classify(err)
	if status != http.StatusGatewayTimeout {
		t.Fatalf("classified %d for %v, want 504", status, err)
	}
}

func TestUnreachableUpstream(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	client := clientFor(t, "http://"+addr, nil)
	req, _ := client.Request(t.Context(), http.MethodPost, "/api/v2/alerts", strings.NewReader("x"), nil)
	err = client.DoJSON(req, nil)
	status, errorType, _ := classify(err)
	if status != http.StatusBadGateway || errorType != "bad_gateway" {
		t.Fatalf("classified %d/%s for %v, want 502/bad_gateway", status, errorType, err)
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	client := clientFor(t, "http://127.0.0.1:1", nil)
	req, _ := client.Request(ctx, http.MethodPost, "/api/v2/alerts", strings.NewReader("x"), nil)
	err := client.DoJSON(req, nil)
	status, errorType, _ := classify(err)
	if status != 499 || errorType != "canceled" {
		t.Fatalf("classified %d/%s for %v, want 499/canceled", status, errorType, err)
	}
}

func TestTransportWrapperIsUsed(t *testing.T) {
	var wrapped atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := clientFor(t, server.URL, func(o *Options) {
		o.WrapTransport = func(http.RoundTripper) http.RoundTripper {
			wrapped.Add(1)
			return http.DefaultTransport
		}
	})
	req, _ := client.Request(t.Context(), http.MethodGet, "/api/v2/status", nil, nil)
	if err := client.DoJSON(req, nil); err != nil {
		t.Fatal(err)
	}
	if wrapped.Load() != 1 {
		t.Errorf("wrapper calls = %d, want 1", wrapped.Load())
	}
}

func TestFailedWriteNeverBecomesSuccess(t *testing.T) {
	t.Run("upstream 500", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		assertFailure(t, clientFor(t, server.URL, nil))
	})

	t.Run("upstream refused", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := ln.Addr().String()
		_ = ln.Close()
		assertFailure(t, clientFor(t, "http://"+addr, nil))
	})
}

func assertFailure(t *testing.T, client *Client) {
	t.Helper()
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := client.Request(r.Context(), http.MethodPost, "/api/v2/alerts", strings.NewReader("x"), nil)
		if err != nil {
			WriteFailure(w, r, err, logger)
			return
		}
		ForwardHeaders(req.Header, r.Header)
		if err := client.DoJSON(req, nil); err != nil {
			WriteFailure(w, r, err, logger)
			return
		}
		envelope.Success(w)
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/alerts", nil))

	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("failed write acknowledged as success: %d %q", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not a v1 error envelope: %q", rec.Body.String())
	}
	if body["status"] != "error" || body["errorType"] == "" || body["error"] == "" {
		t.Errorf("body = %v", body)
	}
}

func TestWriteFailureDoesNotLogCredentialsOrQuery(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts?token=query-secret", nil)
	req.Header.Set("Authorization", "Bearer header-secret")
	err := errors.New(`Post "http://user:pass@upstream/?token=url-secret": dial tcp: refused`)

	rec := httptest.NewRecorder()
	WriteFailure(rec, req, err, logger)

	if got := rec.Code; got != http.StatusBadGateway {
		t.Errorf("status = %d", got)
	}
	for _, secret := range []string{"header-secret", "query-secret", "url-secret", "user:pass"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("log leaked %q: %s", secret, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "/api/v1/alerts") {
		t.Errorf("log omitted the request path: %s", logs.String())
	}
}
