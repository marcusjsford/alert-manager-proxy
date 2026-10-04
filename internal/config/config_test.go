package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaultsPlusEnv(t *testing.T) {
	cfg, err := Load("", env(map[string]string{
		"AMP_UPSTREAM_URL":          "http://am:9093",
		"AMP_UPSTREAM_DIAL_TIMEOUT": "250ms",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream.URL != "http://am:9093" || cfg.Upstream.DialTimeout != 250*time.Millisecond {
		t.Fatalf("env not applied: %+v", cfg.Upstream)
	}
	if cfg.ListenAddress != Defaults().ListenAddress {
		t.Fatalf("default listen address lost: %q", cfg.ListenAddress)
	}
}

func TestLoadFileThenEnvOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yml")
	yml := "listen_address: \":1111\"\nupstream:\n  url: http://from-file:9093\n  response_header_timeout: 3s\n"
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, env(map[string]string{"AMP_UPSTREAM_URL": "http://from-env:9093"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddress != ":1111" || cfg.Upstream.ResponseHeaderTimeout != 3*time.Second {
		t.Fatalf("file not applied: %+v", cfg)
	}
	if cfg.Upstream.URL != "http://from-env:9093" {
		t.Fatalf("env should override file, got %q", cfg.Upstream.URL)
	}
}

func TestValidate(t *testing.T) {
	cases := map[string]struct {
		env  map[string]string
		want string
	}{
		"missing upstream":  {map[string]string{}, "upstream.url is required"},
		"relative upstream": {map[string]string{"AMP_UPSTREAM_URL": "am:9093"}, "absolute http(s) URL"},
		"same ports": {map[string]string{
			"AMP_UPSTREAM_URL": "http://am:9093", "AMP_LISTEN_ADDRESS": ":1", "AMP_ADMIN_LISTEN_ADDRESS": ":1",
		}, "must differ"},
		"bad duration": {map[string]string{
			"AMP_UPSTREAM_URL": "http://am:9093", "AMP_SHUTDOWN_TIMEOUT": "soon",
		}, "AMP_SHUTDOWN_TIMEOUT"},
		"non-positive timeout": {map[string]string{
			"AMP_UPSTREAM_URL": "http://am:9093", "AMP_UPSTREAM_DIAL_TIMEOUT": "0s",
		}, "dial_timeout must be positive"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load("", env(tc.env))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}
