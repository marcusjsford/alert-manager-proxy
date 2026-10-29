package envelope_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"alertmanager-proxy/internal/v1/envelope"
)

func assertContractHeaders(t *testing.T, h http.Header) {
	t.Helper()
	want := map[string]string{
		"Access-Control-Allow-Headers":  envelope.AllowHeaders,
		"Access-Control-Allow-Methods":  envelope.AllowMethods,
		"Access-Control-Allow-Origin":   "*",
		"Access-Control-Expose-Headers": envelope.ExposeHeaders,
		"Cache-Control":                 envelope.CacheControl,
	}
	for name, value := range want {
		if got := h.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestSuccess(t *testing.T) {
	rec := httptest.NewRecorder()
	envelope.Success(rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	assertContractHeaders(t, rec.Header())
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Body.String(); got != `{"status":"success"}` {
		t.Errorf("body = %q", got)
	}
}

func TestSuccessDataKeepsEmptyAndNullData(t *testing.T) {
	for name, data := range map[string]any{"empty slice": []string{}, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			envelope.SuccessData(rec, data)
			if !strings.Contains(rec.Body.String(), `"data":`) {
				t.Errorf("data field missing: %q", rec.Body.String())
			}
			assertContractHeaders(t, rec.Header())
		})
	}
	rec := httptest.NewRecorder()
	envelope.SuccessData(rec, []string{"a"})
	if got := rec.Body.String(); got != `{"status":"success","data":["a"]}` {
		t.Errorf("body = %q", got)
	}
}

func TestError(t *testing.T) {
	rec := httptest.NewRecorder()
	envelope.Error(rec, http.StatusBadRequest, "bad_data", "bad matcher format: bad")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	assertContractHeaders(t, rec.Header())
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Body.String(); got != `{"status":"error","errorType":"bad_data","error":"bad matcher format: bad"}` {
		t.Errorf("body = %q", got)
	}
}

func TestPreflightHasNoBodyOrContentType(t *testing.T) {
	rec := httptest.NewRecorder()
	envelope.Preflight(rec)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	assertContractHeaders(t, rec.Header())
	if got := rec.Header().Get("Content-Type"); got != "" {
		t.Errorf("Content-Type = %q, want empty", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
}

func TestText(t *testing.T) {
	rec := httptest.NewRecorder()
	envelope.Text(rec, http.StatusNotFound, "Error getting silence: <nil>\n")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	assertContractHeaders(t, rec.Header())
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Body.String(); got != "Error getting silence: <nil>\n" {
		t.Errorf("body = %q", got)
	}
}
