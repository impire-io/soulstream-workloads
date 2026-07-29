// Package natsurl rewrites loopback NATS server URLs to a backend-specific
// host alias. Extracted from backend/msb (M2.1) so every isolating backend
// that moves a workload off the node's loopback — microVM guest, Kubernetes
// pod — shares one implementation. Only loopback is rewritten: from inside
// the guest or pod, 127.0.0.1 is the workload itself, and the alias is the
// backend's name for the node. Other hosts pass through untouched.
package natsurl

import (
	"net"
	"net/url"
	"strings"
)

// Rewrite maps loopback hosts in the server URLs to alias, leaving every
// other host untouched.
func Rewrite(servers []string, alias string) []string {
	out := make([]string, len(servers))
	for i, s := range servers {
		out[i] = RewriteOne(s, alias)
	}
	return out
}

// RewriteOne rewrites a single server address, accepting full URLs
// (nats://host:port), bare host:port, and bare host forms.
func RewriteOne(server, alias string) string {
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		if !isLoopback(u.Hostname()) {
			return server
		}
		if p := u.Port(); p != "" {
			u.Host = alias + ":" + p
		} else {
			u.Host = alias
		}
		return u.String()
	}
	// Bare host[:port] forms.
	if host, port, err := net.SplitHostPort(server); err == nil {
		if isLoopback(host) {
			return net.JoinHostPort(alias, port)
		}
		return server
	}
	if isLoopback(server) {
		return alias
	}
	return server
}

// HasLoopback reports whether any server address points at loopback — the
// check a backend uses to fail loud when it has no alias to rewrite to.
func HasLoopback(servers []string) bool {
	for _, s := range servers {
		if RewriteOne(s, "\x00probe") != s {
			return true
		}
	}
	return false
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
