//go:build integration

// Package integration exercises the proxy against a real Alertmanager.
// Run with `make integration`, or set AMP_IT_UPSTREAM_URL and
// `go test -tags integration ./test/integration/`.
package integration

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"alertmanager-proxy/internal/config"
	"alertmanager-proxy/internal/server"
)

type target struct {
	name string
	base string
}

type result struct {
	status      int
	contentType string
	body        []byte
}

func setup(t *testing.T) (direct, proxied target) {
	t.Helper()
	upstream := os.Getenv("AMP_IT_UPSTREAM_URL")
	if upstream == "" {
		t.Skip("AMP_IT_UPSTREAM_URL not set")
	}
	cfg := config.Defaults()
	cfg.Upstream.URL = upstream
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Handler(cfg, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(srv.Close)
	return target{"direct", upstream}, target{"proxied", srv.URL}
}

func do(t *testing.T, tg target, method, path, body string) result {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = bytes.NewBufferString(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, tg.base+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s %s: %v", tg.name, method, path, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return result{resp.StatusCode, resp.Header.Get("Content-Type"), b}
}

// assertSame GETs path from both targets and requires identical status,
// content type and body bytes. Alertmanager state can settle between the two
// reads (e.g. an alert leaving "unprocessed"), so it retries briefly before
// declaring a difference.
func assertSame(t *testing.T, direct, proxied target, path string) {
	t.Helper()
	var d, p result
	for range 20 {
		d = do(t, direct, http.MethodGet, path, "")
		p = do(t, proxied, http.MethodGet, path, "")
		if d.status == p.status && d.contentType == p.contentType && bytes.Equal(d.body, p.body) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Errorf("GET %s differs:\n direct:  %d %s %q\n proxied: %d %s %q",
		path, d.status, d.contentType, d.body, p.status, p.contentType, p.body)
}

func alertJSON(name string) string {
	return fmt.Sprintf(`[{"labels":{"alertname":%q,"source":"amp-integration"},"annotations":{"summary":"pass-through test"},"generatorURL":"http://example/graph"}]`, name)
}

func TestPassThroughMatchesDirect(t *testing.T) {
	direct, proxied := setup(t)

	t.Run("GET /api/v2/status", func(t *testing.T) { assertSame(t, direct, proxied, "/api/v2/status") })
	t.Run("GET / (UI)", func(t *testing.T) { assertSame(t, direct, proxied, "/") })

	t.Run("POST /api/v2/alerts", func(t *testing.T) {
		suffix := time.Now().Format("150405.000")
		d := do(t, direct, http.MethodPost, "/api/v2/alerts", alertJSON("AmpDirect"+suffix))
		p := do(t, proxied, http.MethodPost, "/api/v2/alerts", alertJSON("AmpProxied"+suffix))
		if d.status != http.StatusOK || p.status != d.status || !bytes.Equal(d.body, p.body) {
			t.Fatalf("post differs: direct %d %q, proxied %d %q", d.status, d.body, p.status, p.body)
		}
		// An unrelated alert, so the filtered comparison below would catch a
		// proxy that dropped or rewrote the query string (found by mutation:
		// without it, filtered and unfiltered results were identical).
		if n := do(t, direct, http.MethodPost, "/api/v2/alerts",
			`[{"labels":{"alertname":"AmpNoise`+suffix+`","source":"other"}}]`); n.status != http.StatusOK {
			t.Fatalf("post noise alert: %d %s", n.status, n.body)
		}
		// The alert posted through the proxy must actually land upstream.
		path := "/api/v2/alerts?filter=" + "alertname%3D%22AmpProxied" + suffix + "%22"
		got := do(t, direct, http.MethodGet, path, "")
		if !bytes.Contains(got.body, []byte("AmpProxied"+suffix)) {
			t.Fatalf("alert posted via proxy not found upstream: %s", got.body)
		}
		assertSame(t, direct, proxied, "/api/v2/alerts?filter=source%3D%22amp-integration%22")
	})

	t.Run("GET /api/v2/silences", func(t *testing.T) {
		now := time.Now().UTC()
		silence := fmt.Sprintf(`{"matchers":[{"name":"source","value":"amp-integration","isRegex":false,"isEqual":true}],"startsAt":%q,"endsAt":%q,"createdBy":"amp-integration","comment":"pass-through test"}`,
			now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
		p := do(t, proxied, http.MethodPost, "/api/v2/silences", silence)
		if p.status != http.StatusOK || !bytes.Contains(p.body, []byte("silenceID")) {
			t.Fatalf("create silence via proxy: %d %s", p.status, p.body)
		}
		assertSame(t, direct, proxied, "/api/v2/silences")
	})

	t.Run("GET /api/v1/status is owned by the proxy", func(t *testing.T) {
		d := do(t, direct, http.MethodGet, "/api/v1/status", "")
		p := do(t, proxied, http.MethodGet, "/api/v1/status", "")
		if d.status != http.StatusGone {
			t.Errorf("upstream answered v1 with %d; expected 410 from a v1-less Alertmanager", d.status)
		}
		if p.status != http.StatusNotImplemented {
			t.Errorf("proxy answered v1 with %d, want 501", p.status)
		}
	})
}
