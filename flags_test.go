package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sophotechlabs/spinoza/internal/debugcontainer"
)

func TestDefaultsMatchTheDocumentedOnes(t *testing.T) {
	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if opts.addr != "127.0.0.1:34115" {
		t.Fatalf("addr = %q", opts.addr)
	}
	if opts.cluster.DebugImage != debugcontainer.DefaultImage {
		t.Fatalf("debug image = %q", opts.cluster.DebugImage)
	}
	if opts.cluster.KubectlBinary != debugcontainer.DefaultBinary {
		t.Fatalf("kubectl = %q", opts.cluster.KubectlBinary)
	}
	if opts.cluster.PromSpec != "" {
		t.Fatalf("prometheus = %q, want discovery", opts.cluster.PromSpec)
	}
	if opts.tokenFile != "" {
		t.Fatalf("token file = %q, want none unless a script asked for one", opts.tokenFile)
	}
}

func TestEveryFlagIsCarriedThrough(t *testing.T) {
	opts, err := parseFlags([]string{
		"-addr", "127.0.0.1:9999",
		"-open",
		"-debug-image", "ghcr.io/acme/toolbox:2.1",
		"-kubectl", "/usr/local/bin/kubectl",
		"-prometheus", "monitoring/prom:9090",
		"-token-file", "/tmp/spinoza.token",
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if opts.addr != "127.0.0.1:9999" {
		t.Fatalf("addr = %q", opts.addr)
	}
	if !opts.openBrowser {
		t.Fatal("open was dropped")
	}
	if opts.tokenFile != "/tmp/spinoza.token" {
		t.Fatalf("token file = %q", opts.tokenFile)
	}
	if opts.cluster.DebugImage != "ghcr.io/acme/toolbox:2.1" {
		t.Fatalf("debug image = %q", opts.cluster.DebugImage)
	}
	if opts.cluster.KubectlBinary != "/usr/local/bin/kubectl" {
		t.Fatalf("kubectl = %q", opts.cluster.KubectlBinary)
	}
	if opts.cluster.PromSpec != "monitoring/prom:9090" {
		t.Fatalf("prometheus = %q", opts.cluster.PromSpec)
	}
}

func TestTheTokenFileIsWrittenForScriptsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")

	err := writeTokenFile(path, "s3cret")
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read: %v", readErr)
	}
	if string(data) != "s3cret\n" {
		t.Fatalf("contents = %q", data)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat: %v", statErr)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want the token readable by its owner only", info.Mode().Perm())
	}
}

func TestNoTokenFileMeansNoFile(t *testing.T) {
	err := writeTokenFile("", "s3cret")
	if err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestAnUnwritableTokenFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "token")

	err := writeTokenFile(path, "s3cret")

	if err == nil {
		t.Fatal("a token file that cannot be written was reported as fine")
	}
}

func TestAnUnknownFlagIsAnError(t *testing.T) {
	_, err := parseFlags([]string{"-nope"})

	if err == nil {
		t.Fatal("an unknown flag was accepted")
	}
}

func TestHelpIsNotAFailure(t *testing.T) {
	_, err := parseFlags([]string{"-h"})

	if !errors.Is(err, errHelp) {
		t.Fatalf("err = %v, want the help sentinel so the process can exit 0", err)
	}
}

func TestABadFlagIsStillAFailure(t *testing.T) {
	_, err := parseFlags([]string{"-nonsense"})

	if err == nil {
		t.Fatal("an unknown flag was accepted")
	}
	if errors.Is(err, errHelp) {
		t.Fatal("an unknown flag was treated as a help request")
	}
}
