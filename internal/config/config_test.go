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

func TestTelemetryAndIdentityValidation(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"protocol", func(c *Config) { c.Telemetry.Protocol = "udp" }},
		{"credentials", func(c *Config) { c.Telemetry.Endpoint = "http://user:secret@collector:4317" }},
		{"timeout", func(c *Config) { c.Telemetry.ExportTimeout = 0 }},
		{"missing instance", func(c *Config) { c.Telemetry.Enabled = true }},
		{"IP instance", func(c *Config) { c.Telemetry.Enabled = true; c.Telemetry.InstanceID = "192.0.2.1" }},
		{"grpc URL path", func(c *Config) { c.Telemetry.Endpoint = "http://collector:4317/prefix" }},
		{"bad trusted CIDR", func(c *Config) { c.Clients.TrustedProxies = []string{"10.0.0.1"} }},
		{"unbounded name", func(c *Config) { c.Clients.Rules = []ClientRule{{Name: "raw name", UserAgent: ".*"}} }},
		{"IP name", func(c *Config) { c.Clients.Rules = []ClientRule{{Name: "192.0.2.1", UserAgent: ".*"}} }},
		{"invalid regex", func(c *Config) { c.Clients.Rules = []ClientRule{{Name: "legacy", UserAgent: "["}} }},
		{"empty rule", func(c *Config) { c.Clients.Rules = []ClientRule{{Name: "legacy"}} }},
		{"incomplete header", func(c *Config) { c.Clients.Rules = []ClientRule{{Name: "legacy", Header: "X-Client"}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Upstream.URL = "http://am:9093"
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	cfg, err := Load("", env(map[string]string{
		"AMP_UPSTREAM_URL": "http://am:9093", "AMP_TELEMETRY_ENABLED": "true",
		"AMP_OTLP_PROTOCOL": "http/protobuf", "AMP_OTLP_ENDPOINT": "http://collector:4318",
		"AMP_SERVICE_INSTANCE_ID": "am-1", "AMP_OTLP_EXPORT_TIMEOUT": "250ms",
	}))
	if err != nil || !cfg.Telemetry.Enabled || cfg.Telemetry.InstanceID != "am-1" || cfg.Telemetry.ExportTimeout != 250*time.Millisecond {
		t.Fatalf("telemetry overrides: %+v, %v", cfg.Telemetry, err)
	}
}
