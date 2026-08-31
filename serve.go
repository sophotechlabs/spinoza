//go:build !desktop

package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/sophotechlabs/spinoza/internal/auth"
	"github.com/sophotechlabs/spinoza/internal/kube"
	"github.com/sophotechlabs/spinoza/internal/server"
)

const providerWait = 30 * time.Second

func toolKubeconfig(opts settings) string {
	return toolKubeconfigFrom(opts, kube.InCluster, os.UserConfigDir, kube.WriteInClusterKubeconfig)
}

func toolKubeconfigFrom(
	opts settings,
	inCluster func() bool,
	configDir func() (string, error),
	write func(string) (string, error),
) string {
	if !opts.serve.on || opts.cluster.Kubeconfig != "" {
		return ""
	}
	if !inCluster() {
		return ""
	}
	dir, err := configDir()
	if err != nil {
		slog.Warn("helm and kubectl will act as spinoza itself; there is nowhere to write them a kubeconfig", "error", err)
		return ""
	}
	path, writeErr := write(dir)
	if writeErr != nil {
		slog.Warn("helm and kubectl will act as spinoza itself", "error", writeErr)
		return ""
	}
	return path
}

func serveTeam(ctx context.Context, srv *server.Server, opts settings) error {
	building, cancel := context.WithTimeout(ctx, providerWait)
	defer cancel()
	authn, err := auth.New(building, opts.serve.auth)
	if err != nil {
		return err
	}
	if !authn.Enabled() {
		slog.Warn("cluster mode is on with no way to sign in, so anybody who reaches this address is an admin here; set --auth-mode")
	}
	if !opts.serve.impersonate {
		slog.Warn("impersonation is off, so every action runs as spinoza's own service account rather than the person who asked for it")
	}
	srv.UseClusterAuth(server.ClusterAuth{Authenticator: authn, PublicURL: opts.serve.publicURL})
	return nil
}

func checkListen(opts settings) error {
	if opts.serve.on {
		return nil
	}
	return server.CheckLoopback(opts.addr)
}

func runToken(opts settings) (string, error) {
	if opts.serve.on {
		return "", nil
	}
	token := server.NewToken()
	err := writeTokenFile(opts.tokenFile, token)
	if err != nil {
		return "", err
	}
	return token, nil
}
