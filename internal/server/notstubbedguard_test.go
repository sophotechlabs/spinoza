package server

import (
	"context"
	"strings"
	"testing"
)

func TestAnUnimplementedStubMethodNamesItself(t *testing.T) {
	var caught any
	func() {
		defer func() { caught = recover() }()
		_ = notStubbed{}.Counts(context.Background())
	}()

	if caught == nil {
		t.Fatal("calling a method the stub never implemented did nothing at all")
	}
	text, ok := caught.(string)
	if !ok {
		t.Fatalf("panicked with %T, want a message a reader can act on", caught)
	}
	if !strings.Contains(text, "Backend.Counts") {
		t.Fatalf("message = %q, want it to name the method that was missing", text)
	}
}

func TestAStubCarryingATestReportsRatherThanPanics(t *testing.T) {
	spy := &testing.T{}
	stub := notStubbed{t: spy}

	stub.missing("Something")

	if !spy.Failed() {
		t.Fatal("the stub reached an unimplemented method and the test was not failed")
	}
}

func TestEveryStubThatCannotDelegateUsesTheSharedOne(t *testing.T) {
	var _ Backend = notStubbed{}
	var _ Backend = &stubCatalog{}
	var _ Backend = &pinger{}
}
