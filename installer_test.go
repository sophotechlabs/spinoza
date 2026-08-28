//go:build !desktop

package main

import "testing"

func TestACommandLineBuildHasAnInstaller(t *testing.T) {
	if updateInstaller() == nil {
		t.Fatal("no installer was built for a command-line build")
	}
}
