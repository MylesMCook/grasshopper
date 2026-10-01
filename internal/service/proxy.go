package service

import (
	"net"
	"strings"
)

// proxyHostMatches accepts the configured HTTPS authority and the browser's
// equivalent omitted default port. Other hosts and ports remain distinct.
func proxyHostMatches(host, configured string) bool {
	if configured == "" {
		return false
	}
	if host == configured {
		return true
	}
	name, port, err := net.SplitHostPort(configured)
	if err != nil || port != "443" {
		return false
	}
	if strings.Contains(name, ":") {
		name = "[" + name + "]"
	}
	return host == name
}

// canonicalHTTPSOrigin removes only the explicit HTTPS default port. It leaves
// HTTP, nondefault ports, malformed authorities and paths unchanged.
func canonicalHTTPSOrigin(origin string) string {
	if !strings.HasPrefix(origin, "https://") {
		return origin
	}
	authority := strings.TrimPrefix(origin, "https://")
	if strings.ContainsAny(authority, "/@?#\\") {
		return origin
	}
	_, port, err := net.SplitHostPort(authority)
	if err == nil && port == "443" {
		return strings.TrimSuffix(origin, ":443")
	}
	return origin
}

func originsMatch(actual, expected string) bool {
	return canonicalHTTPSOrigin(actual) == canonicalHTTPSOrigin(expected)
}
