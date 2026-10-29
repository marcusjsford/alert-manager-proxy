package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// Telemetry configures optional, asynchronous OTLP export of all three signals.
type Telemetry struct {
	Enabled        bool          `yaml:"enabled"`
	ServiceName    string        `yaml:"service_name"`
	InstanceID     string        `yaml:"instance_id"`
	Environment    string        `yaml:"environment"`
	Protocol       string        `yaml:"protocol"`
	Endpoint       string        `yaml:"endpoint"`
	ExportTimeout  time.Duration `yaml:"export_timeout"`
	BatchInterval  time.Duration `yaml:"batch_interval"`
	MetricInterval time.Duration `yaml:"metric_interval"`
}

// Clients maps request characteristics to operator-configured, bounded names.
type Clients struct {
	TrustedProxies []string     `yaml:"trusted_proxies"`
	Rules          []ClientRule `yaml:"rules"`
}

// ClientRule matches all configured conditions; the first matching rule wins.
type ClientRule struct {
	Name          string   `yaml:"name"`
	CIDRs         []string `yaml:"cidrs"`
	UserAgent     string   `yaml:"user_agent"`
	BasicAuthUser string   `yaml:"basic_auth_user"`
	Header        string   `yaml:"header"`
	HeaderValue   string   `yaml:"header_value"`
}

// Redaction never permits Authorization capture; silence text is optional.
type Redaction struct {
	Headers          []string `yaml:"headers"`
	SilenceComment   bool     `yaml:"silence_comment"`
	SilenceCreatedBy bool     `yaml:"silence_created_by"`
}

func parseTelemetryEnabled(cfg *Config, value string) error {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("AMP_TELEMETRY_ENABLED: %w", err)
	}
	cfg.Telemetry.Enabled = b
	return nil
}

// Validate rejects invalid exporters without contacting a collector.
func (t Telemetry) Validate() error {
	if t.Protocol != "grpc" && t.Protocol != "http/protobuf" {
		return errors.New("telemetry.protocol must be grpc or http/protobuf")
	}
	u, err := url.Parse(t.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("telemetry.endpoint must be an absolute http(s) URL without credentials, query or fragment")
	}
	if t.ServiceName == "" || t.Environment == "" {
		return errors.New("telemetry.service_name and environment are required")
	}
	if t.Enabled {
		_, ipErr := netip.ParseAddr(t.InstanceID)
		if !clientName.MatchString(t.InstanceID) || ipErr == nil {
			return errors.New("telemetry.instance_id must be a bounded non-IP Alertmanager identifier when enabled")
		}
	}
	if t.Protocol == "grpc" && u.Path != "" && u.Path != "/" {
		return errors.New("telemetry.endpoint must not include a path for grpc")
	}
	if t.ExportTimeout <= 0 || t.BatchInterval <= 0 || t.MetricInterval <= 0 {
		return errors.New("telemetry export_timeout, batch_interval and metric_interval must be positive")
	}
	return nil
}

var clientName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Validate checks every rule before any requests are served.
func (c Clients) Validate() error {
	for _, cidr := range c.TrustedProxies {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return fmt.Errorf("clients.trusted_proxies: invalid CIDR %q", cidr)
		}
	}
	for i, rule := range c.Rules {
		_, ipErr := netip.ParseAddr(rule.Name)
		if !clientName.MatchString(rule.Name) || ipErr == nil {
			return fmt.Errorf("clients.rules[%d]: name must be a 1–64 character bounded non-IP identifier", i)
		}
		if len(rule.CIDRs) == 0 && rule.UserAgent == "" && rule.BasicAuthUser == "" && rule.Header == "" {
			return fmt.Errorf("clients.rules[%d]: at least one matching condition is required", i)
		}
		for _, cidr := range rule.CIDRs {
			if _, err := netip.ParsePrefix(cidr); err != nil {
				return fmt.Errorf("clients.rules[%d]: invalid CIDR %q", i, cidr)
			}
		}
		if _, err := regexp.Compile(rule.UserAgent); err != nil {
			return fmt.Errorf("clients.rules[%d]: invalid user_agent regex", i)
		}
		if (rule.Header == "") != (rule.HeaderValue == "") {
			return fmt.Errorf("clients.rules[%d]: header and header_value must be set together", i)
		}
	}
	return nil
}
