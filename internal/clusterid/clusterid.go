package clusterid

import (
	"net/url"
	"strings"
)

func Normalize(server string) string {
	trimmed := strings.TrimSpace(server)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	if parsed.Host == "" {
		return trimmed
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme + "://" + authority(scheme, parsed) + prefix(parsed.Path)
}

func authority(scheme string, parsed *url.URL) string {
	host := strings.ToLower(parsed.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := parsed.Port()
	if port == "" {
		return host
	}
	if port == defaultPort(scheme) {
		return host
	}
	return host + ":" + port
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	if scheme == "http" {
		return "80"
	}
	return ""
}

func prefix(path string) string {
	return strings.TrimRight(path, "/")
}
