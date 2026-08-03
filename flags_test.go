package main

import (
	"errors"
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
}

func TestEveryFlagIsCarriedThrough(t *testing.T) {
	opts, err := parseFlags([]string{
		"-addr", "127.0.0.1:9999",
		"-open",
		"-debug-image", "ghcr.io/acme/toolbox:2.1",
		"-kubectl", "/usr/local/bin/kubectl",
		"-prometheus", "monitoring/prom:9090",
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
