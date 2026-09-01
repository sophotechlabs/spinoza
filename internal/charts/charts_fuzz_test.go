package charts

import (
	"bytes"
	"net/url"
	"testing"
)

func FuzzChartIndex(f *testing.F) {
	f.Add([]byte(indexBody))
	f.Add([]byte("entries: {}\n"))
	f.Add([]byte("entries:\n  app:\n    - version: not-semver\n"))
	f.Add([]byte("not: [valid"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		found, err := parseIndex(bytes.NewReader(raw))
		if err != nil {
			return
		}
		for at, chart := range found.charts {
			versions := found.versions[chart.Name]
			if len(versions) == 0 {
				t.Fatalf("chart %q has no versions", chart.Name)
			}
			if chart.Version != versions[0] {
				t.Fatalf("chart %q advertises %q, want %q", chart.Name, chart.Version, versions[0])
			}
			if at > 0 && found.charts[at-1].Name >= chart.Name {
				t.Fatalf("charts are not strictly sorted: %q then %q", found.charts[at-1].Name, chart.Name)
			}
			for _, version := range versions {
				if !ValidVersion(version) {
					t.Fatalf("chart %q kept invalid version %q", chart.Name, version)
				}
			}
		}
	})
}

func FuzzFetchableRepositoryURL(f *testing.F) {
	for _, seed := range []string{
		"https://charts.example.com/stable",
		"http://127.0.0.1/index.yaml",
		"oci://registry.example.com/team/chart",
		"file:///tmp/index.yaml",
		"https://user@example.com",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		if CheckFetchable(raw) != nil {
			return
		}
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("accepted URL does not parse: %v", err)
		}
		if !fetchableScheme(parsed.Scheme) {
			t.Fatalf("accepted scheme %q", parsed.Scheme)
		}
		if parsed.Host == "" {
			t.Fatal("accepted URL without a host")
		}
		if parsed.User != nil {
			t.Fatal("accepted URL with user information")
		}
	})
}
