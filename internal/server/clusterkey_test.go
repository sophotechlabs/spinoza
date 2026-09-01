package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClusterKeyUsesTheAskedClusterOrTheCurrentOne(t *testing.T) {
	srv := New(fixed(nil), testAssets(), testToken)
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "current", path: "/api/checks", want: "https://p-mk2:6443"},
		{name: "asked", path: "/api/checks?cluster=https%3A%2F%2Fp-mk1%3A6443", want: "https://p-mk1:6443"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, http.NoBody)
			if got := srv.clusterKey(req); got != tc.want {
				t.Fatalf("clusterKey() = %q, want %q", got, tc.want)
			}
		})
	}
}
