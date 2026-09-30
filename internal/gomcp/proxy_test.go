package gomcp

import "testing"

func TestProxyMatchingOnlyCanonicalizesDefaultHTTPSPort(t *testing.T) {
	for _, tc := range []struct {
		host, configured string
		want             bool
	}{
		{"memory.example", "memory.example:443", true},
		{"memory.example:443", "memory.example:443", true},
		{"memory.example:8443", "memory.example:8443", true},
		{"memory.example", "memory.example:8443", false},
		{"memory.example:443", "memory.example:8443", false},
		{"memory.example:8443", "memory.example:443", false},
		{"wrong.example", "memory.example:443", false},
		{"memory.example.evil", "memory.example:443", false},
		{"memory.example", "", false},
	} {
		if got := proxyHostMatches(tc.host, tc.configured); got != tc.want {
			t.Errorf("host %q configured %q: %t want %t", tc.host, tc.configured, got, tc.want)
		}
	}
	for _, tc := range []struct {
		actual, expected string
		want             bool
	}{
		{"https://memory.example", "https://memory.example:443", true},
		{"https://memory.example:443", "https://memory.example", true},
		{"http://memory.example", "https://memory.example", false},
		{"https://memory.example:8443", "https://memory.example", false},
		{"https://wrong.example", "https://memory.example", false},
		{"https://memory.example/path:443", "https://memory.example/path", false},
		{"https://memory.example:443/", "https://memory.example", false},
		{"https://user@memory.example:443", "https://memory.example", false},
	} {
		if got := originsMatch(tc.actual, tc.expected); got != tc.want {
			t.Errorf("origin %q expected %q: %t want %t", tc.actual, tc.expected, got, tc.want)
		}
	}
}
