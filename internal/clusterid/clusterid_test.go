package clusterid

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		name   string
		server string
		want   string
	}{
		{name: "nothing", server: "", want: ""},
		{name: "blank", server: "   ", want: ""},
		{name: "already canonical", server: "https://10.0.0.5:6443", want: "https://10.0.0.5:6443"},
		{name: "surrounding space", server: "  https://10.0.0.5:6443  ", want: "https://10.0.0.5:6443"},
		{name: "trailing slash", server: "https://10.0.0.5:6443/", want: "https://10.0.0.5:6443"},
		{name: "several trailing slashes", server: "https://10.0.0.5:6443///", want: "https://10.0.0.5:6443"},
		{name: "upper case host", server: "https://Prod.Example.COM:6443", want: "https://prod.example.com:6443"},
		{name: "upper case scheme", server: "HTTPS://prod.example.com:6443", want: "https://prod.example.com:6443"},
		{name: "default https port", server: "https://prod.example.com:443", want: "https://prod.example.com"},
		{name: "default http port", server: "http://prod.example.com:80", want: "http://prod.example.com"},
		{name: "no port", server: "https://prod.example.com", want: "https://prod.example.com"},
		{name: "ipv6", server: "https://[::1]:6443", want: "https://[::1]:6443"},
		{name: "ipv6 upper case", server: "https://[FE80::1]:6443", want: "https://[fe80::1]:6443"},
		{name: "query dropped", server: "https://prod.example.com:6443?stale=true", want: "https://prod.example.com:6443"},
		{name: "fragment dropped", server: "https://prod.example.com:6443#one", want: "https://prod.example.com:6443"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.server)
			if got != tc.want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.server, got, tc.want)
			}
		})
	}
}

func TestAPathIsPartOfTheIdentity(t *testing.T) {
	one := Normalize("https://rancher.example.com/k8s/clusters/c-m-aaaaaaaa")
	two := Normalize("https://rancher.example.com/k8s/clusters/c-m-bbbbbbbb")

	if one == two {
		t.Fatalf("two Rancher clusters normalised to the same id: %q", one)
	}
	if one != "https://rancher.example.com/k8s/clusters/c-m-aaaaaaaa" {
		t.Fatalf("Normalize kept %q, want the path untouched", one)
	}
}

func TestAPathKeepsItsCase(t *testing.T) {
	got := Normalize("https://Rancher.Example.com/k8s/clusters/c-M-AbCdEfGh")

	if got != "https://rancher.example.com/k8s/clusters/c-M-AbCdEfGh" {
		t.Fatalf("Normalize = %q, want the host lowered and the path left alone", got)
	}
}

func TestAPathLosesOnlyItsTrailingSlash(t *testing.T) {
	got := Normalize("https://rancher.example.com/k8s/clusters/c-m-aaaaaaaa/")

	if got != "https://rancher.example.com/k8s/clusters/c-m-aaaaaaaa" {
		t.Fatalf("Normalize = %q, want the trailing slash gone and the rest kept", got)
	}
}

func TestSomethingThatIsNotAURLComesBackUnchanged(t *testing.T) {
	for _, server := range []string{"10.0.0.5:6443", "not a url", "://nope"} {
		got := Normalize(server)
		if got != server {
			t.Fatalf("Normalize(%q) = %q, want it handed back untouched", server, got)
		}
	}
}

func TestAnUnfamiliarSchemeKeepsItsPort(t *testing.T) {
	got := Normalize("ftp://example.com:21")

	if got != "ftp://example.com:21" {
		t.Fatalf("Normalize = %q, want the port kept; only http and https have a default to drop", got)
	}
}
