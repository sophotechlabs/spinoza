package kubeconfig

import (
	"errors"
	"strings"
	"testing"
)

func TestResolveReportsAnAbsolutePathFailure(t *testing.T) {
	cause := errors.New("working directory unavailable")
	makeAbsolute := func(path string) (string, error) {
		if path != "config" {
			t.Fatalf("absolute path input = %q, want config", path)
		}
		return "", cause
	}

	_, err := resolveWith("config", makeAbsolute)

	if err == nil {
		t.Fatal("an absolute path failure was ignored")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("resolve error = %v, want the absolute path failure", err)
	}
	if !strings.Contains(err.Error(), "kubeconfig config") {
		t.Fatalf("resolve error = %q, want the path named", err.Error())
	}
}
