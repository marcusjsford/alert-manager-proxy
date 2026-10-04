// Package config loads the proxy's configuration from an optional YAML file,
// then applies environment-variable overrides, then validates the result.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the full proxy configuration.
type Config struct {
	// ListenAddress serves client traffic: translated /api/v1/* and pass-through
	// for everything else.
	ListenAddress string `yaml:"listen_address"`
	// AdminListenAddress serves the proxy's own health endpoints, kept off the
	// client-facing port so /-/healthy there still means "the upstream
	// Alertmanager is healthy" for existing health checks.
	AdminListenAddress string        `yaml:"admin_listen_address"`
	Upstream           Upstream      `yaml:"upstream"`
	ShutdownTimeout    time.Duration `yaml:"shutdown_timeout"`
}

// Upstream is the single Alertmanager this proxy fronts (ADR decision 2).
type Upstream struct {
	URL string `yaml:"url"`
	// DialTimeout bounds connection establishment, so an unreachable upstream
	// fails fast instead of waiting on the OS connect timeout.
	DialTimeout time.Duration `yaml:"dial_timeout"`
	// ResponseHeaderTimeout bounds the wait for the upstream's response headers.
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`
}

// Defaults returns the configuration used for any field not set by file or env.
func Defaults() Config {
	return Config{
		ListenAddress:      ":9095",
		AdminListenAddress: ":9096",
		Upstream: Upstream{
			DialTimeout:           5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
		ShutdownTimeout: 15 * time.Second,
	}
}

// Load reads path (if non-empty) over the defaults, applies environment
// overrides from getenv, and validates.
func Load(path string, getenv func(string) string) (Config, error) {
	cfg := Defaults()
	if path != "" {
		raw, err := os.ReadFile(path) //nolint:gosec // G304: the operator chooses the config path
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if err := applyEnv(&cfg, getenv); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnv(cfg *Config, getenv func(string) string) error {
	strs := map[string]*string{
		"AMP_LISTEN_ADDRESS":       &cfg.ListenAddress,
		"AMP_ADMIN_LISTEN_ADDRESS": &cfg.AdminListenAddress,
		"AMP_UPSTREAM_URL":         &cfg.Upstream.URL,
	}
	for key, dst := range strs {
		if v := getenv(key); v != "" {
			*dst = v
		}
	}
	durs := map[string]*time.Duration{
		"AMP_UPSTREAM_DIAL_TIMEOUT":            &cfg.Upstream.DialTimeout,
		"AMP_UPSTREAM_RESPONSE_HEADER_TIMEOUT": &cfg.Upstream.ResponseHeaderTimeout,
		"AMP_SHUTDOWN_TIMEOUT":                 &cfg.ShutdownTimeout,
	}
	for key, dst := range durs {
		if v := getenv(key); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			*dst = d
		}
	}
	return nil
}

// Validate reports every problem with the configuration at once.
func (c Config) Validate() error {
	var errs []error
	if c.ListenAddress == "" {
		errs = append(errs, errors.New("listen_address is required"))
	}
	if c.AdminListenAddress == "" {
		errs = append(errs, errors.New("admin_listen_address is required"))
	}
	if c.ListenAddress != "" && c.ListenAddress == c.AdminListenAddress {
		errs = append(errs, errors.New("listen_address and admin_listen_address must differ"))
	}
	if c.Upstream.URL == "" {
		errs = append(errs, errors.New("upstream.url is required"))
	} else if u, err := url.Parse(c.Upstream.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("upstream.url %q must be an absolute http(s) URL", c.Upstream.URL))
	}
	if c.Upstream.DialTimeout <= 0 {
		errs = append(errs, errors.New("upstream.dial_timeout must be positive"))
	}
	if c.Upstream.ResponseHeaderTimeout <= 0 {
		errs = append(errs, errors.New("upstream.response_header_timeout must be positive"))
	}
	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown_timeout must be positive"))
	}
	return errors.Join(errs...)
}

// UpstreamURL returns the parsed upstream URL. Only valid after Validate.
func (c Config) UpstreamURL() *url.URL {
	u, _ := url.Parse(c.Upstream.URL)
	return u
}
