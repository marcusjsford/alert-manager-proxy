// Package clientid resolves trusted source addresses and bounded client names.
package clientid

import (
	"net"
	"net/http"
	"net/netip"
	"regexp"
	"strings"

	"alertmanager-proxy/internal/config"
)

// Identity retains raw address and user agent only for spans/logs, never metrics.
type Identity struct {
	Name, Address, UserAgent string
}

type rule struct {
	config.ClientRule
	prefixes []netip.Prefix
	ua       *regexp.Regexp
}

// Resolver is immutable after construction and safe for concurrent requests.
type Resolver struct {
	trusted []netip.Prefix
	rules   []rule
}

// New compiles validated identity configuration.
func New(cfg config.Clients) (*Resolver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	r := &Resolver{}
	for _, cidr := range cfg.TrustedProxies {
		r.trusted = append(r.trusted, netip.MustParsePrefix(cidr))
	}
	for _, raw := range cfg.Rules {
		compiled := rule{ClientRule: raw}
		for _, cidr := range raw.CIDRs {
			compiled.prefixes = append(compiled.prefixes, netip.MustParsePrefix(cidr))
		}
		if raw.UserAgent != "" {
			compiled.ua = regexp.MustCompile(raw.UserAgent)
		}
		r.rules = append(r.rules, compiled)
	}
	return r, nil
}

func contains(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func (r *Resolver) address(req *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		host = req.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	peer = peer.Unmap()
	if !contains(r.trusted, peer) {
		return peer
	}
	xff := strings.Join(req.Header.Values("X-Forwarded-For"), ",")
	if xff == "" || len(xff) > 8192 {
		return peer
	}
	parts := strings.Split(xff, ",")
	if len(parts) > 32 {
		return peer
	}
	chain := make([]netip.Addr, len(parts))
	for i, part := range parts {
		ip, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return peer // malformed forwarding information is never trusted
		}
		chain[i] = ip.Unmap()
	}
	client := peer
	for i := len(chain) - 1; i >= 0 && contains(r.trusted, client); i-- {
		client = chain[i]
	}
	return client
}

// Resolve checks rules in declaration order, using only a trusted forwarding chain.
func (r *Resolver) Resolve(req *http.Request) Identity {
	address := r.address(req)
	identity := Identity{Name: "unknown", Address: address.String(), UserAgent: req.UserAgent()}
	if !address.IsValid() {
		identity.Address = req.RemoteAddr
	}
	username, _, _ := req.BasicAuth()
	for _, rule := range r.rules {
		if len(rule.prefixes) > 0 && !contains(rule.prefixes, address) {
			continue
		}
		if rule.ua != nil && !rule.ua.MatchString(identity.UserAgent) {
			continue
		}
		if rule.BasicAuthUser != "" && username != rule.BasicAuthUser {
			continue
		}
		if rule.Header != "" && req.Header.Get(rule.Header) != rule.HeaderValue {
			continue
		}
		identity.Name = rule.Name
		break
	}
	return identity
}
