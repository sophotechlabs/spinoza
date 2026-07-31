package prom

import (
	"context"
	"fmt"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
)

const (
	cpuQuery    = `sum(rate(container_cpu_usage_seconds_total{namespace=%q,pod=%q,container!=""}[5m]))`
	memoryQuery = `sum(container_memory_working_set_bytes{namespace=%q,pod=%q,container!=""})`

	maxPoints   = 240
	minStep     = 15 * time.Second
	DefaultSpan = time.Hour
	MaxSpan     = 24 * time.Hour
)

func ParseSpan(spec string) (time.Duration, error) {
	if spec == "" {
		return DefaultSpan, nil
	}
	span, err := time.ParseDuration(spec)
	if err != nil {
		return 0, fmt.Errorf("range must be a duration such as 1h: %w", err)
	}
	if span <= 0 {
		return 0, fmt.Errorf("range must be positive, got %s", span)
	}
	if span > MaxSpan {
		return 0, fmt.Errorf("range must be %s or less, got %s", MaxSpan, span)
	}
	return span, nil
}

func StepFor(span time.Duration) time.Duration {
	step := span / maxPoints
	if step < minStep {
		return minStep
	}
	return step.Round(time.Second)
}

func (c *Client) PodHistory(ctx context.Context, namespace, pod string, span time.Duration, now time.Time) (api.MetricHistory, error) {
	target, err := c.Target(ctx)
	if err != nil {
		return api.MetricHistory{}, err
	}
	end := now
	start := end.Add(-span)
	step := StepFor(span)

	cpu, err := c.Range(ctx, fmt.Sprintf(cpuQuery, namespace, pod), start, end, step)
	if err != nil {
		return api.MetricHistory{}, err
	}
	memory, err := c.Range(ctx, fmt.Sprintf(memoryQuery, namespace, pod), start, end, step)
	if err != nil {
		return api.MetricHistory{}, err
	}
	return api.MetricHistory{
		Namespace: namespace,
		Pod:       pod,
		Source:    target.String(),
		CPU:       cpu,
		Memory:    memory,
	}, nil
}
