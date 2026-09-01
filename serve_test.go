//go:build !desktop

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/server"
)

func servedOpts(change func(*settings)) settings {
	opts := settings{}
	opts.serve = serving{
		on:          true,
		publicURL:   "https://spinoza.example.com",
		impersonate: true,
		auth:        auth.Config{Mode: auth.ModeProxy, PublicURL: "https://spinoza.example.com"},
	}
	if change != nil {
		change(&opts)
	}
	return opts
}

func TestServingTurnsTheServerIntoOneThatAsksWhoYouAre(t *testing.T) {
	srv := server.New(nil, nil, "")

	err := serveTeam(t.Context(), srv, servedOpts(nil))
	if err != nil {
		t.Fatalf("setting up cluster mode: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", http.NoBody)
	req.Host = "spinoza.example.com"
	recorded := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorded, req)

	var found api.Session
	if decodeErr := json.Unmarshal(recorded.Body.Bytes(), &found); decodeErr != nil {
		t.Fatalf("decoding the session: %v", decodeErr)
	}
	if !found.Cluster || found.Mode != auth.ModeProxy {
		t.Fatalf("session = %+v, want it serving a cluster behind a proxy", found)
	}
	if found.Authenticated {
		t.Fatal("a request with no identity on it read as signed in")
	}
}

func TestServingWithNoWayToSignInStillStarts(t *testing.T) {
	opts := servedOpts(func(opts *settings) {
		opts.serve.auth.Mode = auth.ModeNone
		opts.serve.impersonate = false
	})

	if err := serveTeam(t.Context(), server.New(nil, nil, ""), opts); err != nil {
		t.Fatalf("setting up cluster mode: %v", err)
	}
}

func TestAnAuthConfigThatCannotWorkStopsTheServerStarting(t *testing.T) {
	opts := servedOpts(func(opts *settings) {
		opts.serve.auth.Mode = "ldap"
	})

	err := serveTeam(t.Context(), server.New(nil, nil, ""), opts)
	if err == nil {
		t.Fatal("cluster mode started with an auth mode nobody implements")
	}
	if !strings.Contains(err.Error(), "ldap") {
		t.Fatalf("error = %q, want it to name the mode", err.Error())
	}
}

func TestOnlyALocalRunHasToBindLoopback(t *testing.T) {
	local := settings{addr: "0.0.0.0:8080"}

	if err := checkListen(local); err == nil {
		t.Fatal("a local run was allowed to bind every interface")
	}
	if err := checkListen(servedOpts(func(opts *settings) { opts.addr = "0.0.0.0:8080" })); err != nil {
		t.Fatalf("a served run was refused its own address: %v", err)
	}
}

func TestOnlyALocalRunMintsAToken(t *testing.T) {
	served, err := runToken(servedOpts(nil))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if served != "" {
		t.Fatal("a served spinoza minted a run token nobody would ever be given")
	}

	path := filepath.Join(t.TempDir(), "token")
	token, localErr := runToken(settings{tokenFile: path})
	if localErr != nil {
		t.Fatalf("minting: %v", localErr)
	}
	if token == "" {
		t.Fatal("a local run minted no token")
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading the token file: %v", readErr)
	}
	if strings.TrimSpace(string(body)) != token {
		t.Fatalf("token file holds %q, want %q", strings.TrimSpace(string(body)), token)
	}
}

func TestATokenFileThatCannotBeWrittenStopsTheRun(t *testing.T) {
	_, err := runToken(settings{tokenFile: filepath.Join(t.TempDir(), "no", "such", "dir", "token")})

	if err == nil {
		t.Fatal("a token file spinoza could not write was ignored")
	}
}

func TestWhoHelmAndKubectlActAs(t *testing.T) {
	inCluster := func() bool { return true }
	elsewhere := func() bool { return false }
	dir := func() (string, error) { return "/state", nil }
	noDir := func() (string, error) { return "", errors.New("no config directory") }
	wrote := func(string) (string, error) { return "/state/spinoza/in-cluster.kubeconfig", nil }
	broke := func(string) (string, error) { return "", errors.New("read-only") }

	cases := []struct {
		name string
		opts settings
		in   func() bool
		dir  func() (string, error)
		with func(string) (string, error)
		want string
	}{
		{
			name: "a local run needs none",
			opts: settings{},
			in:   inCluster,
			dir:  dir,
			with: wrote,
			want: "",
		},
		{
			name: "a served run outside a cluster reads the kubeconfig it was given",
			opts: servedOpts(nil),
			in:   elsewhere,
			dir:  dir,
			with: wrote,
			want: "",
		},
		{
			name: "a kubeconfig of your own wins",
			opts: servedOpts(func(opts *settings) { opts.cluster.Kubeconfig = "/etc/kube.yaml" }),
			in:   inCluster,
			dir:  dir,
			with: wrote,
			want: "",
		},
		{
			name: "nowhere to write one",
			opts: servedOpts(nil),
			in:   inCluster,
			dir:  noDir,
			with: wrote,
			want: "",
		},
		{
			name: "writing one failed",
			opts: servedOpts(nil),
			in:   inCluster,
			dir:  dir,
			with: broke,
			want: "",
		},
		{
			name: "in a pod, with somewhere to write it",
			opts: servedOpts(nil),
			in:   inCluster,
			dir:  dir,
			with: wrote,
			want: "/state/spinoza/in-cluster.kubeconfig",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := toolKubeconfigFrom(tc.opts, tc.in, tc.dir, tc.with); got != tc.want {
				t.Fatalf("kubeconfig = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAServerNotServingAClusterWritesNoKubeconfig(t *testing.T) {
	if got := toolKubeconfig(settings{}); got != "" {
		t.Fatalf("kubeconfig = %q, want none when spinoza runs as your own window", got)
	}
}
