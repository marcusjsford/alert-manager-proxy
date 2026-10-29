package clientid

import (
	"net/http/httptest"
	"testing"

	"alertmanager-proxy/internal/config"
)

func TestTrustedForwardingChain(t *testing.T) {
	resolver, err := New(config.Clients{TrustedProxies: []string{"10.0.0.0/8"}, Rules: []config.ClientRule{
		{Name: "legacy", CIDRs: []string{"192.0.2.0/24"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, peer, xff, address, client string }{
		{"untrusted spoof", "203.0.113.7:1234", "192.0.2.1", "203.0.113.7", "unknown"},
		{"trusted immediate peer", "10.0.0.1:1234", "192.0.2.1", "192.0.2.1", "legacy"},
		{"trusted chain", "10.0.0.1:1234", "192.0.2.1, 10.0.0.2", "192.0.2.1", "legacy"},
		{"stop at untrusted hop", "10.0.0.1:1234", "192.0.2.1, 203.0.113.1", "203.0.113.1", "unknown"},
		{"malformed chain", "10.0.0.1:1234", "192.0.2.1, invalid", "10.0.0.1", "unknown"},
		{"mapped IPv4", "[::ffff:10.0.0.1]:1234", "192.0.2.1", "192.0.2.1", "legacy"},
		{"no forwarding", "[2001:db8::1]:1234", "", "2001:db8::1", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v2/status", nil)
			req.RemoteAddr = tc.peer
			req.Header.Set("X-Forwarded-For", tc.xff)
			identity := resolver.Resolve(req)
			if identity.Address != tc.address || identity.Name != tc.client {
				t.Fatalf("identity = %+v, want %s/%s", identity, tc.client, tc.address)
			}
		})
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header["X-Forwarded-For"] = []string{"192.0.2.1", "203.0.113.1"}
	if got := resolver.Resolve(req).Address; got != "203.0.113.1" {
		t.Fatalf("repeated forwarding headers were not treated as one chain: %s", got)
	}
}

func TestRuleConjunctionAndOrder(t *testing.T) {
	resolver, err := New(config.Clients{Rules: []config.ClientRule{
		{Name: "specific", UserAgent: `^Legacy/`, Header: "X-Client", HeaderValue: "approved"},
		{Name: "generic", UserAgent: `^Legacy/`},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("User-Agent", "Legacy/1")
	if got := resolver.Resolve(req).Name; got != "generic" {
		t.Fatal(got)
	}
	req.Header.Set("X-Client", "approved")
	if got := resolver.Resolve(req).Name; got != "specific" {
		t.Fatal(got)
	}
}
