package charts

import "testing"

func TestRepositoryURLsRejectReservedAndLinkLocalAddressFamilies(t *testing.T) {
	urls := []string{
		"https://198.51.100.4/charts",
		"https://203.0.113.9/charts",
		"https://[2001:db8::1]/charts",
		"https://[fe80::1]/charts",
		"https://[ff02::1]/charts",
	}

	for _, raw := range urls {
		t.Run(raw, func(t *testing.T) {
			if err := CheckRepoURL(raw); err == nil {
				t.Fatalf("reserved repository address %q was accepted", raw)
			}
		})
	}
}
