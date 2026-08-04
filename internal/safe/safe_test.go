package safe

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type journal struct {
	mu    sync.Mutex
	lines bytes.Buffer
}

func (j *journal) Write(p []byte) (int, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.lines.Write(p)
}

func (j *journal) String() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.lines.String()
}

func captureLog(t *testing.T) *journal {
	t.Helper()
	logged := &journal{}
	original := slog.Default()
	t.Cleanup(func() { slog.SetDefault(original) })
	slog.SetDefault(slog.New(slog.NewTextHandler(logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return logged
}

func TestRunKeepsTheCallerAliveThroughAPanic(t *testing.T) {
	logged := captureLog(t)

	Run("a unit of work", func() {
		panic("cells for a strange object")
	})

	if !strings.Contains(logged.String(), "a unit of work") {
		t.Fatalf("log = %q, want the work named", logged.String())
	}
	if !strings.Contains(logged.String(), "cells for a strange object") {
		t.Fatalf("log = %q, want the panic value", logged.String())
	}
}

func TestRunLeavesAnOrdinaryCallAlone(t *testing.T) {
	logged := captureLog(t)
	ran := false

	Run("a unit of work", func() { ran = true })

	if !ran {
		t.Fatal("the work never ran")
	}
	if logged.String() != "" {
		t.Fatalf("log = %q, want silence when nothing panicked", logged.String())
	}
}

func TestGoKeepsTheProcessAliveThroughAPanic(t *testing.T) {
	logged := captureLog(t)

	Go("a spawned unit of work", func() {
		panic("boom")
	})

	waitFor(t, logged, "a spawned unit of work")
}

func TestLogIgnoresACleanReturn(t *testing.T) {
	logged := captureLog(t)

	Log("a unit of work", nil)

	if logged.String() != "" {
		t.Fatalf("log = %q, want nothing recorded for a clean return", logged.String())
	}
}

func waitFor(t *testing.T, logged *journal, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logged.String(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log never mentioned %q", want)
}
