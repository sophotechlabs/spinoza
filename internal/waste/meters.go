package waste

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/sophotechlabs/spinoza/internal/api"
	"github.com/sophotechlabs/spinoza/internal/safe"
)

const (
	liveSource    = "metrics-server"
	liveWindow    = "right now"
	promSource    = "prometheus"
	sampledSource = "spinoza's own samples"

	windowPodBudget = 200
	windowReaders   = 8
	milliPerCore    = 1000
)

type Sampler interface {
	Metrics(ctx context.Context) api.Metrics
}

type Historian interface {
	MetricHistory(ctx context.Context, namespace, pod string, span time.Duration) (api.MetricHistory, error)
}

type live struct {
	from Sampler
}

func RightNow(from Sampler) Meter {
	return &live{from: from}
}

func (lv *live) Usage(ctx context.Context, pods []Ref) (Reading, error) {
	held := lv.from.Metrics(ctx)
	if held.Error != "" {
		return Reading{}, errors.New(held.Error)
	}
	out := make(map[string]api.ResourceUsage, len(pods))
	for _, ref := range pods {
		key := ref.Namespace + "/" + ref.Name
		use, ok := held.Pods[key]
		if !ok {
			continue
		}
		out[key] = use
	}
	if len(out) == 0 {
		return Reading{}, errors.New("metrics-server named none of the pods this asked about")
	}
	return Reading{Pods: out, Source: liveSource, Window: liveWindow}, nil
}

type window struct {
	from   Historian
	span   time.Duration
	budget int
}

func OverAWindow(from Historian, span time.Duration) Meter {
	return &window{from: from, span: span, budget: windowPodBudget}
}

type sampled struct {
	key    string
	source string
	use    api.ResourceUsage
	ok     bool
	err    error
}

func (wd *window) Usage(ctx context.Context, pods []Ref) (Reading, error) {
	if len(pods) > wd.budget {
		return Reading{}, fmt.Errorf(
			"a window is read one pod at a time, and this asks about %d pods against the %d it will read; "+
				"ask about one namespace",
			len(pods), wd.budget,
		)
	}
	out := Reading{Pods: make(map[string]api.ResourceUsage, len(pods)), Window: windowWords(wd.span)}
	var last error
	for _, one := range wd.readAll(ctx, pods) {
		if !one.ok {
			last = one.err
			continue
		}
		if out.Source == "" {
			out.Source = one.source
		}
		out.Pods[one.key] = one.use
	}
	if len(out.Pods) > 0 {
		return out, nil
	}
	if last == nil {
		last = errors.New("no pod had a history to read")
	}
	return Reading{}, last
}

func (wd *window) readAll(ctx context.Context, pods []Ref) []sampled {
	out := make([]sampled, len(pods))
	gate := make(chan struct{}, windowReaders)
	var group sync.WaitGroup
	group.Add(len(pods))
	for at, ref := range pods {
		gate <- struct{}{}
		safe.Go("reading a pod's usage over a window", func() {
			defer group.Done()
			defer func() { <-gate }()
			out[at] = wd.one(ctx, ref)
		})
	}
	group.Wait()
	return out
}

func (wd *window) one(ctx context.Context, ref Ref) sampled {
	key := ref.Namespace + "/" + ref.Name
	history, err := wd.from.MetricHistory(ctx, ref.Namespace, ref.Name, wd.span)
	if err != nil {
		return sampled{err: err}
	}
	if len(history.CPU) == 0 && len(history.Memory) == 0 {
		return sampled{err: errors.New(key + " has no measured history")}
	}
	return sampled{key: key, source: sourceOf(history), use: meanOf(history), ok: true}
}

func sourceOf(history api.MetricHistory) string {
	if history.Sampled {
		return sampledSource
	}
	if history.Source != "" {
		return history.Source
	}
	return promSource
}

func meanOf(history api.MetricHistory) api.ResourceUsage {
	return api.ResourceUsage{
		CPUMilli: int64(mean(history.CPU) * milliPerCore),
		MemoryMi: int64(mean(history.Memory)) / bytesPerMi,
	}
}

func mean(points []api.MetricPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	var total float64
	for _, point := range points {
		total += point.Value
	}
	return total / float64(len(points))
}

func windowWords(span time.Duration) string {
	if span <= 0 {
		return liveWindow
	}
	seconds := int64(span / time.Second)
	if seconds < 60 {
		return "the last " + counted(seconds, "second")
	}
	minutes := seconds / 60
	if minutes < 60 {
		return "the last " + counted(minutes, "minute")
	}
	hours := minutes / 60
	if hours < 24 {
		return "the last " + counted(hours, "hour")
	}
	return "the last " + counted(hours/24, "day")
}

func counted(count int64, noun string) string {
	if count == 1 {
		return noun
	}
	return strconv.FormatInt(count, 10) + " " + noun + "s"
}
