package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

func hostMiddleware(publicURL string) (func(http.Handler) http.Handler, error) {
	u, err := url.Parse(publicURL)
	if err != nil {
		return nil, err
	}
	expected := normalizedAuthority(u.Scheme, u.Host)
	loopback := isLoopbackHostname(u.Hostname())
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actual := normalizedAuthority(u.Scheme, r.Host)
			valid := actual == expected
			if !valid && loopback {
				expectedPort := authorityPort(u.Scheme, u.Host)
				host, port, err := net.SplitHostPort(actual)
				valid = err == nil && port == expectedPort && isLoopbackHostname(host)
			}
			if !valid {
				http.Error(w, "host validation failed", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func normalizedAuthority(scheme, authority string) string {
	host := strings.ToLower(authority)
	hostname := host
	port := ""
	if parsedHost, parsedPort, err := net.SplitHostPort(host); err == nil {
		hostname, port = parsedHost, parsedPort
	}
	if port == "" {
		port = authorityPort(scheme, authority)
	}
	return net.JoinHostPort(strings.Trim(hostname, "[]"), port)
}

func authorityPort(scheme, authority string) string {
	if _, port, err := net.SplitHostPort(authority); err == nil {
		return port
	}
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func isLoopbackHostname(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
}
