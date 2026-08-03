package listerr

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

const maxDetails = 3

type Collector struct {
	mu       sync.Mutex
	attempts int
	failures map[string]string
}

func New() *Collector {
	return &Collector{failures: map[string]string{}}
}

func (c *Collector) Record(resource string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.attempts++
	if err == nil {
		return
	}
	c.failures[resource] = err.Error()
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
	head := fmt.Sprintf("%d of %d resource types could not be listed", len(names), c.attempts)
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
