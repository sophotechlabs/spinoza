package listerr

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	maxDetails     = 3
	maxFailureSize = 256
)

type Collector struct {
	mu        sync.Mutex
	attempted map[string]struct{}
	failures  map[string]string
}

func New() *Collector {
	return &Collector{attempted: map[string]struct{}{}, failures: map[string]string{}}
}

func (c *Collector) Record(resource string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attempted == nil {
		c.attempted = map[string]struct{}{}
	}
	c.attempted[resource] = struct{}{}
	if err == nil {
		return
	}
	if c.failures == nil {
		c.failures = map[string]string{}
	}
	c.failures[resource] = shorten(err.Error())
}

func (c *Collector) RecordPanic(resource, what string, caught any) {
	if caught == nil {
		return
	}
	safe.Log(what, caught)
	c.Record(resource, errors.New("spinoza could not finish reading this resource"))
}

func shorten(message string) string {
	if len(message) <= maxFailureSize {
		return message
	}
	end := maxFailureSize - len("...")
	for end > 0 && !utf8.RuneStart(message[end]) {
		end--
	}
	return message[:end] + "..."
}

func (c *Collector) Message() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.failures) == 0 {
		return ""
	}
	names := make([]string, 0, len(c.failures))
	for name := range c.failures {
		names = append(names, name)
	}
	slices.Sort(names)
	head := fmt.Sprintf("%d of %d resource types could not be listed", len(names), len(c.attempted))
	shown := names
	if len(shown) > maxDetails {
		shown = shown[:maxDetails]
	}
	details := make([]string, 0, len(shown))
	for _, name := range shown {
		details = append(details, fmt.Sprintf("%s (%s)", name, c.failures[name]))
	}
	message := head + ": " + strings.Join(details, "; ")
	remaining := len(names) - len(shown)
	if remaining > 0 {
		message += fmt.Sprintf("; and %d more", remaining)
	}
	return message
}
