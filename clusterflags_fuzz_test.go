package main

import (
	"net/url"
	"strings"
	"testing"
)

func FuzzServingCheckPublicURL(f *testing.F) {
	for _, seed := range []string{
		"https://spinoza.example.com",
		"http://127.0.0.1:8080/",
		"https://user@example.com",
		"https://example.com/path",
		"https://example.com?debug=true",
		"https://example.com#fragment",
		"not a url",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		err := (serving{on: true, publicURL: raw}).check()
		if err != nil {
			return
		}
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			t.Fatalf("check accepted a URL that does not parse: %v", parseErr)
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			t.Fatalf("check accepted scheme %q", parsed.Scheme)
		}
		if parsed.Host == "" {
			t.Fatal("check accepted a URL without a host")
		}
		if parsed.User != nil {
			t.Fatal("check accepted credentials")
		}
		if path := parsed.EscapedPath(); path != "" && path != "/" {
			t.Fatalf("check accepted path %q", path)
		}
		if parsed.RawQuery != "" || parsed.ForceQuery {
			t.Fatal("check accepted a query")
		}
		if strings.Contains(raw, "#") {
			t.Fatal("check accepted a fragment")
		}
	})
}
